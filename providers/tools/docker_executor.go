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
	tools         sync.Map // map[string]*DockerTool
	usage         sync.Map // map[string]*ToolUsage
	getSharedDir  func(context.Context, *DockerToolExecutor) (string, error)
	sharedDirPath atomic.Pointer[string] // stores the actual shared directory path if created
}

// ToolUsage tracks usage statistics for a tool.
type ToolUsage struct {
	CallCount   int64
	TotalTimeNs int64
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

// ExecuteTool executes a tool by name with the given arguments and auxiliary data files.
func (d *DockerToolExecutor) ExecuteTool(ctx context.Context, logger logging.Logger, toolName string, args json.RawMessage, data map[string][]byte) (json.RawMessage, error) {
	toolValue, exists := d.tools.Load(toolName)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotAvailable, toolName)
	}

	tool, ok := toolValue.(*DockerTool)
	if !ok {
		return nil, fmt.Errorf("tool %q encountered an error: %w: %w: %T", toolName, ErrToolInternal, ErrUnsupportedToolType, toolValue)
	}

	// Check MaxCalls limit.
	if tool.maxCalls != nil {
		usageValue, _ := d.usage.LoadOrStore(toolName, &ToolUsage{})
		usage := usageValue.(*ToolUsage)
		callCount := atomic.LoadInt64(&usage.CallCount)
		if callCount >= int64(*tool.maxCalls) {
			return nil, fmt.Errorf("%w: tool %q has exceeded its maximum call limit of %d for this session. Do not call this tool again during the current conversation", ErrToolMaxCallsExceeded, toolName, *tool.maxCalls)
		}
	}

	// Create a logger with tool name prefix.
	toolLogger := logger.WithContext(fmt.Sprintf("%s: ", toolName))

	// Execute the tool.
	result, err := d.executeDockerTool(ctx, toolLogger, tool, args, data)
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

// readContainerLogs reads logs from a container with the given options.
func (d *DockerToolExecutor) readContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (string, error) {
	logs, err := d.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("failed to get tool container logs: %w", err)
	}
	defer logs.Close()

	var buffer bytes.Buffer
	_, err = stdcopy.StdCopy(&buffer, &buffer, logs) // combine stdout and stderr
	if err != nil {
		return "", fmt.Errorf("failed to read tool container output: %w", err)
	}

	return buffer.String(), nil
}

// executeDockerTool executes a Docker tool with the given arguments and auxiliary data files.
func (d *DockerToolExecutor) executeDockerTool(ctx context.Context, logger logging.Logger, tool *DockerTool, args json.RawMessage, data map[string][]byte) (json.RawMessage, error) {
	logger.Message(ctx, logging.LevelInfo, "starting setup")

	// Parse the arguments.
	var argMap map[string]interface{}
	if err := json.Unmarshal(args, &argMap); err != nil {
		logger.Error(ctx, logging.LevelError, err, "failed to parse input arguments: %s", string(args))
		return nil, fmt.Errorf("%w: failed to parse input arguments as JSON object (expected format: {\"argName\": \"value\", ...}): %v", ErrInvalidToolArguments, err)
	}
	logger.Message(ctx, logging.LevelTrace, "parsed input arguments: %v", argMap)

	// Create a temporary directory for file mappings.
	tempDir, err := os.MkdirTemp("", "mindtrial-tool-*")
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create temporary workspace directory: %v", ErrToolInternal, err)
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
					return nil, fmt.Errorf("%w: failed to serialize argument %q to JSON (argument values must be JSON-serializable): %v", ErrInvalidToolArguments, argName, err)
				}
				content = string(contentBytes)
			}

			// Create a unique temporary file for this mapping.
			tempFilePath, err := writeTempFile(tempDir, argName, content)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to write argument %q to temporary file: %v", ErrToolInternal, argName, err)
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
				return nil, fmt.Errorf("%w: failed to create temporary file for auxiliary data file %q: %v", ErrToolInternal, fileName, err)
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
			return nil, fmt.Errorf("%w: %v", ErrToolInternal, err)
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
		return nil, fmt.Errorf("%w: failed to create tool container (image: %q): %v", ErrToolInternal, tool.image, err)
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
	startTime := time.Now()
	logger.Message(ctx, logging.LevelInfo, "starting execution")
	status, err := d.runContainer(execCtx, createResp.ID)
	duration := time.Since(startTime)
	d.recordUsage(tool.name, duration)

	// Handle execution errors.
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return nil, fmt.Errorf("%w: execution timed out after %s", ErrToolTimeout, tool.getTimeoutValue())
	case errors.Is(err, context.Canceled):
		return nil, fmt.Errorf("%w: execution was cancelled", ErrToolInternal)
	case err != nil:
		return nil, fmt.Errorf("%w: %v", ErrToolInternal, err)
	}

	logger.Message(ctx, logging.LevelDebug, "tool container %q exited with code %d in %v", createResp.ID, status.StatusCode, duration)

	if status.StatusCode != 0 {
		// Get output to see what went wrong.
		if output, logErr := d.readContainerLogs(ctx, createResp.ID, container.LogsOptions{
			ShowStderr: true,
			ShowStdout: true,
		}); logErr == nil {
			logger.Message(ctx, logging.LevelTrace, "tool container %q logs:\n%s", createResp.ID, output)
			return nil, fmt.Errorf("%w: tool container exited with code %d: %s", ErrToolExecutionFailed, status.StatusCode, strings.TrimSpace(output))
		} else {
			logger.Error(ctx, logging.LevelWarn, logErr, "failed to retrieve tool container logs")
		}
		return nil, fmt.Errorf("%w: tool container exited with code %d", ErrToolExecutionFailed, status.StatusCode)
	}
	logger.Message(ctx, logging.LevelInfo, "tool container %q finished successfully", createResp.ID)

	// Get the container logs (stdout).
	stdout, err := d.readContainerLogs(ctx, createResp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: false,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to retrieve tool output from tool container: %v", ErrToolInternal, err)
	}
	logger.Message(ctx, logging.LevelTrace, "tool container %q stdout:\n%s", createResp.ID, stdout)

	// Parse the JSON result.
	result := strings.TrimSpace(stdout)
	if result == "" {
		return nil, fmt.Errorf("%w: tool returned no output", ErrToolExecutionFailed)
	}

	logger.Message(ctx, logging.LevelInfo, "successfully finished")
	return json.RawMessage(result), nil
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

// recordUsage records the usage statistics for a tool.
func (d *DockerToolExecutor) recordUsage(toolName string, duration time.Duration) {
	usageValue, _ := d.usage.LoadOrStore(toolName, &ToolUsage{})
	toolUsage := usageValue.(*ToolUsage)

	atomic.AddInt64(&toolUsage.CallCount, 1)
	atomic.AddInt64(&toolUsage.TotalTimeNs, duration.Nanoseconds())
}

// GetUsageStats returns usage statistics for all tools.
func (d *DockerToolExecutor) GetUsageStats() map[string]ToolUsage {
	if d == nil {
		return nil
	}
	stats := make(map[string]ToolUsage)
	d.usage.Range(func(key, value interface{}) bool {
		toolName := key.(string)
		usage := value.(*ToolUsage)
		stats[toolName] = ToolUsage{
			CallCount:   atomic.LoadInt64(&usage.CallCount),
			TotalTimeNs: atomic.LoadInt64(&usage.TotalTimeNs),
		}
		return true
	})
	return stats
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
