// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/oklog/ulid/v2"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
)

// DockerToolExecutor executes tools within Docker containers.
type DockerToolExecutor struct {
	client        *client.Client
	tools         sync.Map         // map[string]*DockerTool
	usage         sync.Map         // map[string]*ToolUsage
	calls         callSummaryState // shared log of every invocation attempt across all tools, in completion order
	getSharedDir  func(context.Context, *DockerToolExecutor) (string, error)
	sharedDirPath atomic.Pointer[string] // stores the actual shared directory path if created
}

// ToolUsage tracks aggregate execution statistics for a tool: CallCount and
// TotalDurationNs only reflect invocations whose container actually started running
// (regardless of exit code) - i.e. actual compute time spent, not every invocation
// attempt. An invocation that fails before the container ever starts (e.g. invalid
// arguments, or an infrastructure error during setup) does not affect these aggregates.
// See ToolCallSummary/GetCallSummaries for a complete per-invocation log that does
// include such attempts.
type ToolUsage struct {
	CallCount       int64
	TotalDurationNs int64
	Exhausted       int32
}

// callSummaryState holds the shared log of per-call summaries across all tools. A single
// shared log preserves the true chronological order calls completed in, across all tools.
type callSummaryState struct {
	sync.Mutex
	calls []ToolCallSummary
}

// record appends a per-call summary, synchronized internally.
func (s *callSummaryState) record(summary ToolCallSummary) {
	s.Lock()
	defer s.Unlock()
	s.calls = append(s.calls, summary)
}

// snapshot returns a deep copy of the recorded per-call summaries, synchronized internally,
// so callers never share the backing array.
func (s *callSummaryState) snapshot() []ToolCallSummary {
	s.Lock()
	defer s.Unlock()
	return slices.Clone(s.calls)
}

// maxCallOutputPreviewBytes caps the size of the Stdout/Stderr preview captured per tool call.
// Chosen from an empirical distribution of tool call output sizes across exit-status categories:
// the combined Q3 + 1.5*IQR outlier fence was ~1835 bytes, rounded up to the next power of two
// to leave headroom while still keeping previews small enough for diagnostic/report display.
const maxCallOutputPreviewBytes = 2048

// Tool call summary status values. See ToolCallSummary.Status.
const (
	toolCallStatusSuccess             = "success"
	toolCallStatusNonZeroExit         = "nonzero_exit"
	toolCallStatusEmptyOutput         = "empty_output"
	toolCallStatusTimeout             = "timeout"
	toolCallStatusInvalidArguments    = "invalid_arguments"
	toolCallStatusInfrastructureError = "infrastructure_error"
)

// OutputCapture holds a size-limited preview of a tool call's output stream.
type OutputCapture struct {
	// Bytes is the total size of the output stream, regardless of Truncated.
	Bytes int64
	// Preview is a truncated prefix of the output stream, or nil if not captured
	// (e.g. stdout on a successful call) or empty.
	Preview *string
	// Truncated indicates whether Preview was cut short of the full output.
	Truncated bool
}

// ToolCallSummary records the outcome of a single tool invocation.
type ToolCallSummary struct {
	// Tool is the name of the tool this call invoked.
	Tool string
	// CallID is a unique identifier (ULID) for this call. It is also included in the
	// prefix of every log line emitted while this call was in progress, so a specific
	// invocation can be correlated between this summary and the corresponding log lines.
	CallID string
	// ConversationTurn is the 1-based conversation turn this call was made during, or 0 if
	// unknown/not provided by the caller.
	ConversationTurn int
	// StartedAt is when this call began (start of setup, before the container ever runs).
	StartedAt time.Time
	// CompletedAt is when this call finished, successfully or not.
	CompletedAt time.Time
	// DurationNs is the wall-clock duration of the container's runtime (start through exit),
	// the same measurement window as the aggregate ToolUsage.TotalDurationNs. It does not
	// include setup (argument parsing, mounts, container creation) or teardown overhead, and
	// stays nil when no container process ever ran (e.g. an infrastructure_error during setup).
	DurationNs *int64
	// WallTimeNs is the wall-clock duration of the entire call attempt, from setup
	// through output retrieval - i.e. DurationNs plus setup/teardown overhead. Unlike
	// DurationNs, this is always set, even for calls whose container never ran.
	WallTimeNs int64
	// ExitCode is the container's exit code, or nil if no exit code is known (e.g. setup never
	// reached container start, or the run was aborted/cancelled before it could be observed).
	ExitCode *int64
	// TimedOut indicates the call was aborted due to exceeding its configured timeout.
	TimedOut bool
	// Status is one of: "success", "nonzero_exit", "empty_output", "timeout",
	// "invalid_arguments", "infrastructure_error". Statuses are chosen to be meaningful for
	// result analysis: "nonzero_exit", "empty_output", and "invalid_arguments" reflect the
	// tool being used incorrectly (a plausible model/tool-usage issue), while
	// "infrastructure_error" covers environment/tooling failures (container/filesystem setup,
	// Docker runtime errors including cancellation, and log retrieval failures) that the
	// model has no influence over and are not informative about the model or tool under test.
	Status string
	// Stdout is a size-limited capture of the call's standard output, or nil if no output was
	// ever captured (e.g. an infrastructure_error before or during log retrieval).
	Stdout *OutputCapture
	// Stderr is a size-limited capture of the call's standard error, or nil if no output was
	// ever captured (e.g. an infrastructure_error before or during log retrieval).
	Stderr *OutputCapture
	// ErrorMessage is a short explanation of the failure when Status is not "success".
	ErrorMessage string
}

// newSharedDirFactory creates a factory function that lazily creates a shared temporary directory.
// The directory is created once on the first call and the same path is returned for all subsequent calls.
func newSharedDirFactory() func(context.Context, *DockerToolExecutor) (string, error) {
	return config.OnceWithContext(func(ctx context.Context, state *DockerToolExecutor) (sharedDir string, err error) {
		sharedDir, err = os.MkdirTemp("", "mindtrial-tool-shared-*")
		if err != nil {
			return "", fmt.Errorf("failed to create shared temporary directory: %w", err)
		}
		state.sharedDirPath.Store(&sharedDir)
		return
	})
}

// NewDockerToolExecutor creates a new Docker tool executor.
func NewDockerToolExecutor(ctx context.Context) (*DockerToolExecutor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerToolExecutor{
		client:       cli,
		tools:        sync.Map{},
		getSharedDir: newSharedDirFactory(),
	}, nil
}

// RegisterTool registers a tool with the executor.
func (d *DockerToolExecutor) RegisterTool(tool *DockerTool) {
	d.tools.Store(tool.name, tool)
}

// ValidateTool ensures the Docker image referenced by the tool configuration is available locally.
func (d *DockerToolExecutor) ValidateTool(ctx context.Context, cfg config.ToolConfig) error {
	if cfg.Image == "" {
		return fmt.Errorf("%w: docker image is not configured for tool %q", ErrToolInternal, cfg.Name)
	}

	if _, err := d.client.ImageInspect(ctx, cfg.Image); err != nil {
		switch {
		case errdefs.IsNotFound(err):
			return fmt.Errorf("%w: docker image %q is not available locally. Pull the image with `docker pull %s` and try again", ErrToolNotAvailable, cfg.Image, cfg.Image)
		default:
			return fmt.Errorf("%w: failed to inspect docker image %q: %v", ErrToolInternal, cfg.Image, err)
		}
	}

	return nil
}

// ToolCallContext carries optional caller-supplied metadata to attach to the
// ToolCallSummary recorded for a single ExecuteTool call. All fields are optional; a nil
// ToolCallContext (or a zero-value one) simply leaves the corresponding ToolCallSummary
// fields unset. New fields can be added here in the future without changing ExecuteTool's
// signature again.
type ToolCallContext struct {
	// CallID is the provider's own identifier for this tool call, if the provider's API
	// assigns one (e.g. OpenAI/Anthropic/DeepSeek/Mistral AI/xAI's tool_call/tool_use ID).
	// Reusing the provider's own ID - rather than minting an unrelated one - means this same
	// ID can also be found in any API error message that references the call. If empty,
	// ExecuteTool generates one internally (a ULID) instead, so ToolCallSummary.CallID is
	// never empty; note this means CallID's shape/format varies depending on whether - and
	// how - the calling provider assigns its own IDs.
	CallID string
	// ConversationTurn is the 1-based conversation turn this call is being made during, or 0
	// if unknown/not applicable.
	ConversationTurn int
}

// ExecuteTool executes a tool by name with the given arguments and auxiliary data files.
// callCtx carries optional caller-supplied metadata (see ToolCallContext) to attach to the
// resulting ToolCallSummary; pass nil if there is none to provide.
func (d *DockerToolExecutor) ExecuteTool(ctx context.Context, logger logging.Logger, toolName string, args json.RawMessage, data map[string][]byte, callCtx *ToolCallContext) (json.RawMessage, error) {
	toolValue, exists := d.tools.Load(toolName)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotAvailable, toolName)
	}

	tool, ok := toolValue.(*DockerTool)
	if !ok {
		return nil, fmt.Errorf("tool %q encountered an error: %w: %w: %T", toolName, ErrToolInternal, ErrUnsupportedToolType, toolValue)
	}

	// Check MaxCalls limit.
	// NOTE: The current execution model creates a new DockerToolExecutor per provider Run,
	// so ExecuteTool is not called concurrently on this executor instance today.
	// If this ever changes (shared executor across concurrent calls), reserve a call slot
	// atomically before execution to avoid check-then-act races.
	if tool.maxCalls != nil {
		usageValue, _ := d.usage.LoadOrStore(toolName, &ToolUsage{})
		usage := usageValue.(*ToolUsage)
		callCount := atomic.LoadInt64(&usage.CallCount)
		if callCount >= int64(*tool.maxCalls) {
			atomic.StoreInt32(&usage.Exhausted, 1)
			return nil, fmt.Errorf("%w: tool %q has exceeded its maximum call limit of %d for this session. Do not call this tool again during the current conversation", ErrToolMaxCallsExceeded, toolName, *tool.maxCalls)
		}
	}

	// Assign an ID to this call so it can be correlated between the log and the recorded
	// ToolCallSummary - preferring the caller-supplied ID (see ToolCallContext.CallID) so
	// the same ID can also be matched against the provider's own API error messages, and
	// falling back to a freshly generated ULID when the caller did not supply one. Create a
	// logger that includes it alongside the tool name.
	callID := ""
	if callCtx != nil {
		callID = callCtx.CallID
	}
	if callID == "" {
		callID = ulid.Make().String()
	}
	toolLogger := logger.WithContext(fmt.Sprintf("%s[%s]: ", toolName, callID))

	// Execute the tool.
	result, err := d.executeDockerTool(ctx, toolLogger, tool, args, data, callID, callCtx)
	if err != nil {
		return nil, fmt.Errorf("tool %q encountered an error: %w", toolName, err)
	}
	return result, nil
}

// Close closes the Docker client connection and cleans up shared directories.
func (d *DockerToolExecutor) Close() error {
	// Clean up shared directory if it was created.
	if sharedDirPtr := d.sharedDirPath.Load(); sharedDirPtr != nil {
		defer os.RemoveAll(*sharedDirPtr)
	}

	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

// readContainerLogs reads and demultiplexes a container's stdout and stderr logs.
func (d *DockerToolExecutor) readContainerLogs(ctx context.Context, containerID string) (stdout string, stderr string, err error) {
	logs, err := d.client.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", "", fmt.Errorf("failed to get tool container logs: %w", err)
	}
	defer logs.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, logs); err != nil {
		return "", "", fmt.Errorf("failed to read tool container output: %w", err)
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// newOutputCapture builds an OutputCapture for a tool call output stream, truncating the
// preview at maxCallOutputPreviewBytes. Truncation is a plain byte-length cut and may split
// a multi-byte UTF-8 sequence; this is an acceptable trade-off for a diagnostic preview.
// If includePreview is false, Preview stays nil (used for stdout on a successful call, to
// save space) while Bytes/Truncated are still reported. Callers should only invoke this when
// the stream was actually read; leave the corresponding ToolCallSummary field nil otherwise.
func newOutputCapture(content string, includePreview bool) *OutputCapture {
	capture := OutputCapture{Bytes: int64(len(content)), Truncated: len(content) > maxCallOutputPreviewBytes}
	if includePreview && content != "" {
		preview := content
		if capture.Truncated {
			preview = content[:maxCallOutputPreviewBytes]
		}
		capture.Preview = &preview
	}
	return &capture
}

// executeDockerTool executes a Docker tool with the given arguments and auxiliary data
// files. callID uniquely identifies this call (see ExecuteTool) and callCtx carries
// optional caller-supplied metadata (see ToolCallContext); callCtx may be nil.
func (d *DockerToolExecutor) executeDockerTool(ctx context.Context, logger logging.Logger, tool *DockerTool, args json.RawMessage, data map[string][]byte, callID string, callCtx *ToolCallContext) (json.RawMessage, error) {
	startTime := time.Now()
	summary := ToolCallSummary{CallID: callID, StartedAt: startTime}
	if callCtx != nil {
		summary.ConversationTurn = callCtx.ConversationTurn
	}
	// Guarantees exactly one summary is recorded, and its outcome logged, per call
	// regardless of the return path. WallTimeNs covers the entire call attempt; DurationNs
	// is set separately, only once the container actually runs, to cover just its runtime
	// (see field docs).
	defer func() {
		summary.CompletedAt = time.Now()
		summary.WallTimeNs = summary.CompletedAt.Sub(startTime).Nanoseconds()
		d.recordCallSummary(tool.name, summary)
		logCallOutcome(ctx, logger, summary)
	}()

	logger.Message(ctx, logging.LevelInfo, "starting setup")

	// Parse the arguments.
	var argMap map[string]interface{}
	if err := json.Unmarshal(args, &argMap); err != nil {
		logger.Error(ctx, logging.LevelError, err, "failed to parse input arguments: %s", string(args))
		wrapErr := fmt.Errorf("%w: failed to parse input arguments as JSON object (expected format: {\"argName\": \"value\", ...}): %v", ErrInvalidToolArguments, err)
		summary.Status, summary.ErrorMessage = toolCallStatusInvalidArguments, wrapErr.Error()
		return nil, wrapErr
	}
	logger.Message(ctx, logging.LevelTrace, "parsed input arguments: %v", argMap)

	// Create a temporary directory for file mappings.
	tempDir, err := os.MkdirTemp("", "mindtrial-tool-*")
	if err != nil {
		wrapErr := fmt.Errorf("%w: failed to create temporary workspace directory: %v", ErrToolInternal, err)
		summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
		return nil, wrapErr
	}
	defer os.RemoveAll(tempDir) // clean up temp directory after execution
	logger.Message(ctx, logging.LevelDebug, "created temporary workspace directory: %s", tempDir)

	// Write parameter files and create individual file mounts.
	var mounts []mount.Mount
	for argName, containerPath := range tool.parameterFiles {
		if argValue, exists := argMap[argName]; exists {
			// Convert argument value to string.
			var content string
			switch v := argValue.(type) {
			case string:
				content = v
			default:
				// For non-string values, marshal back to JSON.
				contentBytes, err := json.Marshal(v)
				if err != nil {
					logger.Error(ctx, logging.LevelError, err, "failed to marshal argument %q to JSON: %v", argName, argValue)
					wrapErr := fmt.Errorf("%w: failed to serialize argument %q to JSON (argument values must be JSON-serializable): %v", ErrInvalidToolArguments, argName, err)
					summary.Status, summary.ErrorMessage = toolCallStatusInvalidArguments, wrapErr.Error()
					return nil, wrapErr
				}
				content = string(contentBytes)
			}

			// Create a unique temporary file for this mapping.
			tempFilePath, err := writeTempFile(tempDir, argName, content)
			if err != nil {
				wrapErr := fmt.Errorf("%w: failed to write argument %q to temporary file: %v", ErrToolInternal, argName, err)
				summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
				return nil, wrapErr
			}

			// Create a bind mount for this file.
			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeBind,
				Source: tempFilePath,
				Target: containerPath,
			})

			logger.Message(ctx, logging.LevelDebug, "mounted temporary file %s to container path %s for argument %q", tempFilePath, containerPath, argName)
		}
	}

	// Mount data files to auxiliary directory if configured.
	// Each file is mounted using its unique name exactly as provided.
	if tool.auxiliaryDir != "" {
		for fileName, fileContent := range data {
			// Create temporary file for the data file.
			tempFilePath, err := writeTempFile(tempDir, fileName, fileContent)
			if err != nil {
				wrapErr := fmt.Errorf("%w: failed to create temporary file for auxiliary data file %q: %v", ErrToolInternal, fileName, err)
				summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
				return nil, wrapErr
			}

			// Create container path for the data file.
			containerPath := path.Join(filepath.ToSlash(tool.auxiliaryDir), fileName)

			// Create a bind mount for this data file.
			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeBind,
				Source: tempFilePath,
				Target: containerPath,
			})

			logger.Message(ctx, logging.LevelDebug, "mounted auxiliary data file %q from %s to container path %s", fileName, tempFilePath, containerPath)
		}
	}

	// Mount shared directory if configured.
	if tool.sharedDir != "" {
		sharedTempDir, err := d.getSharedDir(ctx, d)
		if err != nil {
			wrapErr := fmt.Errorf("%w: %v", ErrToolInternal, err)
			summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
			return nil, wrapErr
		}

		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: sharedTempDir,
			Target: tool.sharedDir,
		})

		logger.Message(ctx, logging.LevelDebug, "mounted shared directory from %s to container path %s", sharedTempDir, tool.sharedDir)
	}

	// Prepare environment variables.
	env := make([]string, 0, len(tool.env))
	// Add tool-specific environment
	for k, v := range tool.env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	logger.Message(ctx, logging.LevelTrace, "setting environment variables: %v", env)

	// Create container configuration.
	containerConfig := &container.Config{
		Image:        tool.image,
		Cmd:          tool.command,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}
	logger.Message(ctx, logging.LevelTrace, "setting command: %v", tool.command)

	// Create host configuration with mounts.
	hostConfig := &container.HostConfig{
		Mounts:        mounts,
		AutoRemove:    false, // manually remove container after retrieving logs
		NetworkMode:   network.NetworkNone,
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		LogConfig:     container.LogConfig{Type: "json-file"}, // default logging driver; JSON format
	}

	// Set resource limits.
	if tool.maxMemoryMB != nil {
		// Convert MB to bytes
		hostConfig.Memory = int64(*tool.maxMemoryMB) * 1024 * 1024
		logger.Message(ctx, logging.LevelTrace, "setting memory limit to %d MB (%d bytes)", *tool.maxMemoryMB, hostConfig.Memory)
	}
	if tool.cpuPercent != nil {
		// Convert CPU percentage to NanoCPU units.
		// NanoCPUs = (numCPUs * percent / 100) * 1e9
		numCPUs := runtime.NumCPU()
		nanoCPUs := int64(numCPUs) * int64(*tool.cpuPercent) * 10000000 // 1e9 / 100 = 1e7
		hostConfig.NanoCPUs = nanoCPUs
		logger.Message(ctx, logging.LevelTrace, "setting CPU limit to %d%% (%d NanoCPUs, %d CPUs total)", *tool.cpuPercent, nanoCPUs, numCPUs)
	}

	// Generate a unique container name.
	containerName := fmt.Sprintf("%s-tool-%s", tool.name, ulid.Make().String())

	// Create the container.
	createResp, err := d.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		wrapErr := fmt.Errorf("%w: failed to create tool container (image: %q): %v", ErrToolInternal, tool.image, err)
		summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
		return nil, wrapErr
	}
	logger.Message(ctx, logging.LevelDebug, "created tool container %q (ID: %s)", containerName, createResp.ID)

	// Ensure container is removed even if execution fails.
	defer func() {
		err := d.client.ContainerRemove(ctx, createResp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		switch {
		case err == nil, errdefs.IsConflict(err), errdefs.IsNotFound(err):
			// Container removed successfully or already removed. Ignore.
		default:
			logger.Error(ctx, logging.LevelWarn, err, "failed to remove tool container after execution")
		}
	}()

	// Apply timeout if specified.
	execCtx := ctx
	if tool.timeout != nil {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, *tool.timeout)
		defer cancel()
	}

	// Start the container and wait for completion.
	runStart := time.Now()
	logger.Message(ctx, logging.LevelInfo, "starting execution")
	status, err := d.runContainer(execCtx, createResp.ID)
	runDuration := time.Since(runStart)
	d.recordUsage(tool.name, runDuration)
	durationNs := runDuration.Nanoseconds()
	summary.DurationNs = &durationNs

	// Handle execution errors.
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		wrapErr := fmt.Errorf("%w: execution timed out after %s", ErrToolTimeout, tool.getTimeoutValue())
		summary.Status, summary.TimedOut, summary.ErrorMessage = toolCallStatusTimeout, true, wrapErr.Error()
		return nil, wrapErr
	case errors.Is(err, context.Canceled):
		wrapErr := fmt.Errorf("%w: execution was cancelled", ErrToolInternal)
		summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
		return nil, wrapErr
	case err != nil:
		wrapErr := fmt.Errorf("%w: %v", ErrToolInternal, err)
		summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
		return nil, wrapErr
	}

	logger.Message(ctx, logging.LevelDebug, "tool container %q exited with code %d in %v", createResp.ID, status.StatusCode, runDuration)
	summary.ExitCode = &status.StatusCode

	if status.StatusCode != 0 {
		summary.Status = toolCallStatusNonZeroExit

		// Get output to see what went wrong.
		if stdout, stderr, logErr := d.readContainerLogs(ctx, createResp.ID); logErr == nil {
			logger.Message(ctx, logging.LevelTrace, "tool container %q stdout:\n%s\nstderr:\n%s", createResp.ID, stdout, stderr)
			summary.Stdout = newOutputCapture(stdout, true)
			summary.Stderr = newOutputCapture(stderr, true)
			combinedOutput := strings.TrimSpace(stdout + stderr)
			wrapErr := fmt.Errorf("%w: tool container exited with code %d: %s", ErrToolExecutionFailed, status.StatusCode, combinedOutput)
			summary.ErrorMessage = wrapErr.Error()
			return nil, wrapErr
		} else {
			logger.Error(ctx, logging.LevelWarn, logErr, "failed to retrieve tool container logs")
		}
		wrapErr := fmt.Errorf("%w: tool container exited with code %d", ErrToolExecutionFailed, status.StatusCode)
		summary.ErrorMessage = wrapErr.Error()
		return nil, wrapErr
	}
	logger.Message(ctx, logging.LevelInfo, "tool container %q finished successfully", createResp.ID)

	// Get the container logs.
	stdout, stderr, err := d.readContainerLogs(ctx, createResp.ID)
	if err != nil {
		wrapErr := fmt.Errorf("%w: failed to retrieve tool output from tool container: %v", ErrToolInternal, err)
		summary.Status, summary.ErrorMessage = toolCallStatusInfrastructureError, wrapErr.Error()
		return nil, wrapErr
	}
	logger.Message(ctx, logging.LevelTrace, "tool container %q stdout:\n%s\nstderr:\n%s", createResp.ID, stdout, stderr)
	summary.Stdout = newOutputCapture(stdout, false) // omit preview on success to save space
	summary.Stderr = newOutputCapture(stderr, true)  // report stray stderr output even on success

	// Parse the JSON result.
	result := strings.TrimSpace(stdout)
	if result == "" {
		wrapErr := fmt.Errorf("%w: tool returned no output", ErrToolExecutionFailed)
		summary.Status, summary.ErrorMessage = toolCallStatusEmptyOutput, wrapErr.Error()
		return nil, wrapErr
	}

	summary.Status = toolCallStatusSuccess
	return json.RawMessage(result), nil
}

// logCallOutcome emits a single terminal log line summarizing a completed tool call's
// outcome, so every return path (success or failure) is guaranteed exactly one entry: at
// LevelError for infrastructure failures and timeouts (environment/tooling problems, not
// attributable to the model or tool), LevelWarn for statuses reflecting apparent tool
// misuse (nonzero_exit, empty_output, invalid_arguments), and LevelInfo for success.
func logCallOutcome(ctx context.Context, logger logging.Logger, summary ToolCallSummary) {
	level := logging.LevelInfo
	switch summary.Status {
	case toolCallStatusInfrastructureError, toolCallStatusTimeout:
		level = logging.LevelError
	case toolCallStatusNonZeroExit, toolCallStatusEmptyOutput, toolCallStatusInvalidArguments:
		level = logging.LevelWarn
	}
	if summary.ErrorMessage != "" {
		logger.Message(ctx, level, "call finished: status=%s wall_time=%s error=%q", summary.Status, time.Duration(summary.WallTimeNs), summary.ErrorMessage)
		return
	}
	logger.Message(ctx, level, "call finished: status=%s wall_time=%s", summary.Status, time.Duration(summary.WallTimeNs))
}

// TextOrData is a constraint for types that can be written to files.
type TextOrData interface {
	~string | ~[]byte
}

// writeTempFile creates a temporary file with the given content and returns its path.
func writeTempFile[T TextOrData](tempDir string, prefix string, content T) (string, error) {
	tempFile, err := os.CreateTemp(tempDir, prefix+"-*")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	switch v := any(content).(type) {
	case string:
		if _, err := tempFile.WriteString(v); err != nil {
			return "", err
		}
	case []byte:
		if _, err := tempFile.Write(v); err != nil {
			return "", err
		}
	}

	return tempFile.Name(), nil
}

// runContainer starts a container and waits for it to complete, returning the final status.
func (d *DockerToolExecutor) runContainer(ctx context.Context, containerID string) (status container.WaitResponse, err error) {
	// Start the container.
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return status, fmt.Errorf("failed to start tool container: %w", err)
	}

	// Wait for the container to finish.
	statusCh, errCh := d.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return status, fmt.Errorf("failed waiting for tool to finish execution: %w", err)
		}
	case status = <-statusCh:
		return status, nil
	case <-ctx.Done():
		return status, fmt.Errorf("tool execution interrupted: %w", ctx.Err())
	}

	return status, nil
}

// recordUsage records the aggregate execution statistics for a tool.
func (d *DockerToolExecutor) recordUsage(toolName string, duration time.Duration) {
	usageValue, _ := d.usage.LoadOrStore(toolName, &ToolUsage{})
	toolUsage := usageValue.(*ToolUsage)

	atomic.AddInt64(&toolUsage.CallCount, 1)
	atomic.AddInt64(&toolUsage.TotalDurationNs, duration.Nanoseconds())
}

// recordCallSummary appends a per-call summary to the shared call log, tagging it with
// the tool name.
func (d *DockerToolExecutor) recordCallSummary(toolName string, summary ToolCallSummary) {
	summary.Tool = toolName
	d.calls.record(summary)
}

// IsToolExhausted reports whether the named tool has exceeded its maximum call limit.
// Returns false if the executor is nil or the tool has not been used.
func (d *DockerToolExecutor) IsToolExhausted(toolName string) bool {
	if d == nil {
		return false
	}
	usageValue, ok := d.usage.Load(toolName)
	if !ok {
		return false
	}
	return atomic.LoadInt32(&usageValue.(*ToolUsage).Exhausted) != 0
}

// GetUsageStats returns aggregate execution statistics for all tools.
func (d *DockerToolExecutor) GetUsageStats() map[string]ToolUsage {
	if d == nil {
		return nil
	}
	stats := make(map[string]ToolUsage)
	d.usage.Range(func(key, value interface{}) bool {
		toolName := key.(string)
		usage := value.(*ToolUsage)
		stats[toolName] = ToolUsage{
			CallCount:       atomic.LoadInt64(&usage.CallCount),
			TotalDurationNs: atomic.LoadInt64(&usage.TotalDurationNs),
			Exhausted:       atomic.LoadInt32(&usage.Exhausted),
		}
		return true
	})
	return stats
}

// GetCallSummaries returns a log of every recorded invocation attempt across all tools, in
// the order calls completed, including attempts that never actually ran (e.g. due to
// invalid arguments or an infrastructure error during setup). This is tracked
// independently of GetUsageStats; a tool with no entry in GetUsageStats (because its
// container never once ran) can still appear here. Returns nil if the executor is nil.
func (d *DockerToolExecutor) GetCallSummaries() []ToolCallSummary {
	if d == nil {
		return nil
	}
	return d.calls.snapshot()
}

type DockerTool struct {
	name           string
	image          string
	description    string
	parameters     map[string]interface{}
	parameterFiles map[string]string
	auxiliaryDir   string
	sharedDir      string
	command        []string
	env            map[string]string
	maxCalls       *int
	timeout        *time.Duration
	maxMemoryMB    *int
	cpuPercent     *int
}

// NewDockerTool creates a new Docker tool.
func NewDockerTool(cfg *config.ToolConfig, maxCalls *int, timeout *time.Duration, maxMemoryMB *int, cpuPercent *int) *DockerTool {
	return &DockerTool{
		name:           cfg.Name,
		image:          cfg.Image,
		description:    cfg.Description,
		parameters:     cfg.Parameters,
		parameterFiles: cfg.ParameterFiles,
		auxiliaryDir:   cfg.AuxiliaryDir,
		sharedDir:      cfg.SharedDir,
		command:        cfg.Command,
		env:            cfg.Env,
		maxCalls:       maxCalls,
		timeout:        timeout,
		maxMemoryMB:    maxMemoryMB,
		cpuPercent:     cpuPercent,
	}
}

func (t *DockerTool) getTimeoutValue() string {
	if t.timeout != nil {
		return fmt.Sprintf("%v", *t.timeout)
	}
	return "<none>"
}
