// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tools

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
)

const testAPIVersion = "1.44"

type dockerAPIMock struct {
	t *testing.T

	server     *httptest.Server
	apiVersion string

	onPing         func(http.ResponseWriter, *http.Request)
	onImageInspect func(http.ResponseWriter, *http.Request)
	onCreate       func(http.ResponseWriter, *http.Request)
	onStart        func(http.ResponseWriter, *http.Request)
	onWait         func(http.ResponseWriter, *http.Request)
	onLogs         func(http.ResponseWriter, *http.Request)
	onRemove       func(http.ResponseWriter, *http.Request)
}

func newDockerAPIMock(t *testing.T) *dockerAPIMock {
	mock := &dockerAPIMock{
		t:          t,
		apiVersion: testAPIVersion,
	}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.handle))
	t.Cleanup(mock.Close)
	return mock
}

func (m *dockerAPIMock) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

func (m *dockerAPIMock) basePath() string {
	return "/v" + m.apiVersion
}

func (m *dockerAPIMock) handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if r.Method == http.MethodGet && path == "/_ping" {
		if m.onPing != nil {
			m.onPing(w, r)
		} else {
			w.Header().Set("API-Version", m.apiVersion)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}
		return
	}

	if strings.HasPrefix(path, m.basePath()+"/images") {
		trimmed := strings.TrimPrefix(path, m.basePath()+"/images")
		if r.Method == http.MethodGet && strings.HasSuffix(trimmed, "/json") {
			if m.onImageInspect != nil {
				m.onImageInspect(w, r)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"Id":"sha256:test"}`))
			}
			return
		}
	}

	if strings.HasPrefix(path, m.basePath()+"/containers") {
		trimmed := strings.TrimPrefix(path, m.basePath()+"/containers")
		switch {
		case r.Method == http.MethodPost && trimmed == "/create":
			if m.onCreate == nil {
				m.t.Fatalf("unexpected ContainerCreate call without handler: %s", path)
			}
			m.onCreate(w, r)
			return
		case r.Method == http.MethodPost && strings.HasSuffix(trimmed, "/start"):
			if m.onStart == nil {
				m.t.Fatalf("unexpected ContainerStart call without handler: %s", path)
			}
			m.onStart(w, r)
			return
		case r.Method == http.MethodPost && strings.HasSuffix(trimmed, "/wait"):
			if m.onWait == nil {
				m.t.Fatalf("unexpected ContainerWait call without handler: %s", path)
			}
			m.onWait(w, r)
			return
		case r.Method == http.MethodGet && strings.HasSuffix(trimmed, "/logs"):
			if m.onLogs == nil {
				m.t.Fatalf("unexpected ContainerLogs call without handler: %s", path)
			}
			m.onLogs(w, r)
			return
		case r.Method == http.MethodDelete:
			if m.onRemove == nil {
				m.t.Fatalf("unexpected ContainerRemove call without handler: %s", path)
			}
			m.onRemove(w, r)
			return
		}
	}

	m.t.Fatalf("unexpected request: %s %s", r.Method, path)
}

func (m *dockerAPIMock) host() string {
	return "tcp://" + m.server.Listener.Addr().String()
}

func encodeDockerFrames(frames ...dockerLogFrame) []byte {
	var out []byte //nolint:prealloc
	for _, frame := range frames {
		payload := []byte(frame.Data)
		payloadLen := len(payload)
		header := make([]byte, 8)
		header[0] = frame.Stream
		binary.BigEndian.PutUint32(header[4:], uint32(payloadLen)) //nolint:gosec
		out = append(out, header...)
		out = append(out, payload...)
	}
	return out
}

type dockerLogFrame struct {
	Stream byte
	Data   string
}

type containerCreatePayload struct {
	Image      string   `json:"Image"`
	Cmd        []string `json:"Cmd"`
	Env        []string `json:"Env"`
	HostConfig struct {
		Mounts []struct {
			Type   string `json:"Type"`
			Source string `json:"Source"`
			Target string `json:"Target"`
		} `json:"Mounts"`
		AutoRemove  bool   `json:"AutoRemove"`
		NetworkMode string `json:"NetworkMode"`
		Memory      int64  `json:"Memory"`
		NanoCPUs    int64  `json:"NanoCpus"`
	} `json:"HostConfig"`
}

func newTestExecutor(t *testing.T, mock *dockerAPIMock) *DockerToolExecutor {
	cli, err := client.NewClientWithOpts(
		client.WithHost(mock.host()),
		client.WithVersion(testAPIVersion),
	)
	require.NoError(t, err)

	cli.NegotiateAPIVersion(context.Background())

	executor := &DockerToolExecutor{
		client:       cli,
		getSharedDir: newSharedDirFactory(),
	}
	t.Cleanup(func() {
		_ = executor.Close()
	})
	return executor
}

func newTestTool(name string) *DockerTool {
	return &DockerTool{
		name:           name,
		image:          "alpine:latest",
		command:        []string{"/bin/echo"},
		env:            map[string]string{"FOO": "BAR"},
		parameterFiles: map[string]string{"input": "/workspace/input.txt"},
	}
}

func TestDockerToolExecutorValidateTool_ImageAvailable(t *testing.T) {
	mock := newDockerAPIMock(t)
	mock.onImageInspect = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Id":"sha256:test"}`))
	}

	executor := newTestExecutor(t, mock)
	cfg := config.ToolConfig{Name: "echo", Image: "alpine:latest"}

	require.NoError(t, executor.ValidateTool(context.Background(), cfg))
}

func TestDockerToolExecutorValidateTool_ImageMissing(t *testing.T) {
	mock := newDockerAPIMock(t)
	mock.onImageInspect = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such image"}`))
	}

	executor := newTestExecutor(t, mock)
	cfg := config.ToolConfig{Name: "echo", Image: "missing:latest"}

	err := executor.ValidateTool(context.Background(), cfg)
	require.Error(t, err)
	assert.EqualError(t, err, "tool not available: docker image \"missing:latest\" is not available locally. Pull the image with `docker pull missing:latest` and try again")
}

func TestDockerToolExecutorValidateTool_ImageInspectError(t *testing.T) {
	mock := newDockerAPIMock(t)
	mock.onImageInspect = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Internal server error"}`))
	}

	executor := newTestExecutor(t, mock)
	cfg := config.ToolConfig{Name: "echo", Image: "test:latest"}

	err := executor.ValidateTool(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool internal error: failed to inspect docker image \"test:latest\"")
}

func newTestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// onlyCall returns the single recorded ToolCallSummary for toolName, failing the test if
// there isn't exactly one.
func onlyCall(t *testing.T, executor *DockerToolExecutor, toolName string) ToolCallSummary {
	t.Helper()
	toolCalls := callsFor(executor, toolName)
	require.Len(t, toolCalls, 1)
	return toolCalls[0]
}

// callsFor returns the recorded ToolCallSummary values for toolName, in completion order.
func callsFor(executor *DockerToolExecutor, toolName string) []ToolCallSummary {
	var toolCalls []ToolCallSummary
	for _, call := range executor.GetCallSummaries() {
		if call.Tool == toolName {
			toolCalls = append(toolCalls, call)
		}
	}
	return toolCalls
}

func configureSuccessfulExecution(t *testing.T, mock *dockerAPIMock, tool *DockerTool, expectedFileContent, logOutput string, expectedAuxiliaryFiles map[string][]byte) func() string {
	var mountedFile string

	mock.onCreate = func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req containerCreatePayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode container create payload: %v", err)
		}

		assert.Equal(t, tool.image, req.Image)
		assert.Equal(t, tool.command, req.Cmd)
		assert.ElementsMatch(t, []string{"FOO=BAR"}, req.Env)

		// Calculate expected mount count: parameter file + auxiliary files + shared dir.
		expectedMountCount := 1 + len(expectedAuxiliaryFiles)
		if tool.sharedDir != "" {
			expectedMountCount++
		}

		if len(req.HostConfig.Mounts) != expectedMountCount {
			t.Fatalf("expected %d mounts, got %d", expectedMountCount, len(req.HostConfig.Mounts))
		}

		// Find parameter file mount, auxiliary file mounts, and shared dir mount.
		var parameterMount, auxiliaryMounts []struct{ Source, Target string }
		var sharedDirMount *struct{ Source, Target string }
		for _, mount := range req.HostConfig.Mounts {
			switch {
			case mount.Target == "/workspace/input.txt":
				parameterMount = append(parameterMount, struct{ Source, Target string }{mount.Source, mount.Target})
				mountedFile = mount.Source
			case tool.sharedDir != "" && mount.Target == tool.sharedDir:
				sharedDirMount = &struct{ Source, Target string }{mount.Source, mount.Target}
			case expectedAuxiliaryFiles != nil && strings.HasPrefix(mount.Target, filepath.ToSlash(tool.auxiliaryDir)):
				auxiliaryMounts = append(auxiliaryMounts, struct{ Source, Target string }{mount.Source, mount.Target})
			}
		}

		// Verify parameter file mount.
		assert.Len(t, parameterMount, 1)
		data := testutils.ReadFile(t, parameterMount[0].Source)
		assert.Equal(t, expectedFileContent, string(data))

		// Verify shared dir mount if expected.
		if tool.sharedDir != "" {
			assert.NotNil(t, sharedDirMount, "shared directory mount not found")
			assert.Equal(t, tool.sharedDir, sharedDirMount.Target)
		}

		// Verify auxiliary file mounts if expected.
		assert.Len(t, auxiliaryMounts, len(expectedAuxiliaryFiles))
		for _, auxMount := range auxiliaryMounts {
			fileName := path.Base(auxMount.Target)
			expectedContent, exists := expectedAuxiliaryFiles[fileName]
			assert.True(t, exists, "unexpected auxiliary file: %s", fileName)

			actualContent := testutils.ReadFile(t, auxMount.Source)
			assert.Equal(t, expectedContent, actualContent, "auxiliary file %s content mismatch", fileName)
		}

		expectedMemory := int64(0)
		if tool.maxMemoryMB != nil {
			expectedMemory = int64(*tool.maxMemoryMB) * 1024 * 1024
		}
		assert.Equal(t, expectedMemory, req.HostConfig.Memory)

		expectedNanoCPUs := int64(0)
		if tool.cpuPercent != nil {
			numCPUs := runtime.NumCPU()
			expectedNanoCPUs = int64(numCPUs) * int64(*tool.cpuPercent) * 10000000
		}
		assert.Equal(t, expectedNanoCPUs, req.HostConfig.NanoCPUs)

		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"Id": "mock-container"}); err != nil {
			t.Fatalf("failed to encode container create response: %v", err)
		}
	}

	mock.onStart = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}

	mock.onWait = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"StatusCode":0}`)); err != nil {
			t.Fatalf("failed to write wait response: %v", err)
		}
	}

	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		payload := encodeDockerFrames(dockerLogFrame{Stream: 1, Data: logOutput})
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("failed to write log payload: %v", err)
		}
	}

	mock.onRemove = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}

	return func() string {
		return mountedFile
	}
}

func TestWriteTempFile(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		content  interface{}
		expected string
	}{
		{
			name:     "string content",
			content:  "hello world",
			expected: "hello world",
		},
		{
			name:     "byte slice content",
			content:  []byte("hello world"),
			expected: "hello world",
		},
		{
			name:     "binary content",
			content:  []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0xFF},
			expected: "Hello\x00\xFF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			var err error

			switch v := tt.content.(type) {
			case string:
				filePath, err = writeTempFile(tempDir, "test", v)
			case []byte:
				filePath, err = writeTempFile(tempDir, "test", v)
			}

			require.NoError(t, err)
			require.NotEmpty(t, filePath)
			defer os.Remove(filePath)

			content := testutils.ReadFile(t, filePath)
			require.Equal(t, tt.expected, string(content))
		})
	}
}

func TestDockerToolExecutorExecuteTool_Success(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("success-tool")
	executor.RegisterTool(tool)

	logger := testutils.NewTestLogger(t)

	payload := `{"input":"payload"}`
	mountedFileFn := configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`, nil)

	ctx, cancel := newTestContext()
	defer cancel()

	before := time.Now()
	result, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(payload), nil, nil)
	after := time.Now()
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"ok"}`, string(result))

	stats := executor.GetUsageStats()
	usage, ok := stats[tool.name]
	require.True(t, ok)
	assert.Equal(t, int64(1), usage.CallCount)
	assert.GreaterOrEqual(t, usage.TotalDurationNs, int64(0))

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusSuccess, call.Status)
	require.NotNil(t, call.ExitCode)
	assert.Equal(t, int64(0), *call.ExitCode)
	assert.False(t, call.TimedOut)
	require.NotNil(t, call.DurationNs)
	assert.GreaterOrEqual(t, *call.DurationNs, int64(0))
	assert.Empty(t, call.ErrorMessage)
	require.NotNil(t, call.Stdout)
	assert.Equal(t, int64(len(`{"status":"ok"}`)), call.Stdout.Bytes)
	assert.Nil(t, call.Stdout.Preview, "stdout preview is omitted on success to save space")
	assert.False(t, call.Stdout.Truncated)
	require.NotNil(t, call.Stderr)
	assert.Zero(t, call.Stderr.Bytes)
	assert.Nil(t, call.Stderr.Preview)
	_, ulidErr := ulid.Parse(call.CallID)
	require.NoError(t, ulidErr, "CallID must be a valid ULID")
	assert.Zero(t, call.ConversationTurn, "omitted when no ToolCallContext was provided")
	assert.False(t, call.StartedAt.Before(before), "StartedAt must be within the call's wall-clock window")
	assert.False(t, call.CompletedAt.After(after), "CompletedAt must be within the call's wall-clock window")
	assert.False(t, call.CompletedAt.Before(call.StartedAt), "CompletedAt must not precede StartedAt")

	mountedFile := mountedFileFn()
	require.NotEmpty(t, mountedFile)
	_, statErr := os.Stat(mountedFile)
	require.Error(t, statErr)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestDockerToolExecutorExecuteTool_CallContext(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("call-context-tool")
	executor.RegisterTool(tool)
	configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`, nil)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, &ToolCallContext{CallID: "provider-native-id-123", ConversationTurn: 3})
	require.NoError(t, err)

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, 3, call.ConversationTurn)
	assert.Equal(t, "provider-native-id-123", call.CallID, "the caller-supplied CallID must be used as-is instead of generating one")
}

func TestDockerToolExecutorExecuteTool_CallContext_CallIDFallback(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("call-context-fallback-tool")
	executor.RegisterTool(tool)
	configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`, nil)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	// A ToolCallContext with an empty CallID (e.g. the calling provider does not assign
	// its own tool-call IDs) must still fall back to a generated one.
	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, &ToolCallContext{ConversationTurn: 1})
	require.NoError(t, err)

	call := onlyCall(t, executor, tool.name)
	_, ulidErr := ulid.Parse(call.CallID)
	require.NoError(t, ulidErr, "CallID must fall back to a generated ULID when the context does not supply one")
}

func TestDockerToolExecutorExecuteTool_CallIDUniquePerCall(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("unique-call-id-tool")
	executor.RegisterTool(tool)
	configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`, nil)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.NoError(t, err)
	_, err = executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.NoError(t, err)

	calls := callsFor(executor, tool.name)
	require.Len(t, calls, 2)
	assert.NotEmpty(t, calls[0].CallID)
	assert.NotEmpty(t, calls[1].CallID)
	assert.NotEqual(t, calls[0].CallID, calls[1].CallID, "each call must get its own unique ID")
}

func TestDockerToolExecutorExecuteTool_StderrCapturedOnSuccess(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("stderr-on-success")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`, nil)
	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		payload := encodeDockerFrames(
			dockerLogFrame{Stream: 1, Data: `{"status":"ok"}`},
			dockerLogFrame{Stream: 2, Data: "a stray warning\n"},
		)
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("failed to write log payload: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	result, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"ok"}`, string(result))

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusSuccess, call.Status)
	require.NotNil(t, call.Stdout)
	assert.Nil(t, call.Stdout.Preview, "stdout preview is still omitted on success even alongside stderr output")
	require.NotNil(t, call.Stderr)
	require.NotNil(t, call.Stderr.Preview, "stray stderr output should be captured even on an otherwise successful call")
	assert.Equal(t, "a stray warning\n", *call.Stderr.Preview)
}

func TestDockerToolExecutorExecuteTool_OutputPreviewTruncated(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("large-output")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	mock.onWait = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"StatusCode":1}`)); err != nil {
			t.Fatalf("failed to write wait response: %v", err)
		}
	}

	largeOutput := strings.Repeat("e", maxCallOutputPreviewBytes+100)
	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		payload := encodeDockerFrames(dockerLogFrame{Stream: 2, Data: largeOutput})
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("failed to write log payload: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusNonZeroExit, call.Status)
	require.NotNil(t, call.Stderr)
	assert.Equal(t, int64(len(largeOutput)), call.Stderr.Bytes)
	assert.True(t, call.Stderr.Truncated)
	require.NotNil(t, call.Stderr.Preview)
	assert.Len(t, *call.Stderr.Preview, maxCallOutputPreviewBytes)
	assert.Equal(t, largeOutput[:maxCallOutputPreviewBytes], *call.Stderr.Preview)
}

func TestDockerToolExecutorExecuteTool_ResourceLimits(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	maxMemory := 256
	cpuPercent := 25
	tool := newTestTool("resource-limits")
	tool.maxMemoryMB = &maxMemory
	tool.cpuPercent = &cpuPercent
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`, nil)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.NoError(t, err)
}

func TestDockerToolExecutorExecuteTool_ToolNotRegistered(t *testing.T) {
	mock := newDockerAPIMock(t)
	_ = newTestExecutor(t, mock)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	executor := &DockerToolExecutor{client: nil}

	_, err := executor.ExecuteTool(ctx, logger, "missing", json.RawMessage(`{}`), nil, nil)
	require.Error(t, err)
	assert.Equal(t, "tool not available: missing", err.Error())
}

func TestDockerToolExecutorExecuteTool_UnsupportedToolType(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	executor := &DockerToolExecutor{}
	executor.tools.Store("bad", 123)

	_, err := executor.ExecuteTool(ctx, logger, "bad", json.RawMessage(`{}`), nil, nil)
	require.Error(t, err)
	assert.Equal(t, "tool \"bad\" encountered an error: tool internal error: unsupported tool type: int", err.Error())
}

func TestDockerToolExecutorExecuteTool_MaxCallsExceeded(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	maxCalls := 1
	tool := newTestTool("limited-tool")
	tool.maxCalls = &maxCalls
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", `{"ok":true}`, nil)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.NoError(t, err)
	assert.False(t, executor.IsToolExhausted(tool.name), "tool should not be exhausted after successful call within limit")

	_, err = executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool max calls exceeded: tool \"limited-tool\" has exceeded its maximum call limit of 1 for this session. Do not call this tool again during the current conversation"
	assert.Equal(t, expected, err.Error())
	assert.True(t, executor.IsToolExhausted(tool.name), "tool should be exhausted after exceeding max calls")

	stats := executor.GetUsageStats()
	require.Contains(t, stats, tool.name)
	assert.Equal(t, int32(1), stats[tool.name].Exhausted)
}

func TestDockerToolExecutorGetUsageStats_NilReceiver(t *testing.T) {
	var executor *DockerToolExecutor
	stats := executor.GetUsageStats()
	require.Nil(t, stats)
}

func TestDockerToolExecutorIsToolExhausted(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*DockerToolExecutor)
		toolName string
		want     bool
	}{
		{
			name:     "unknown tool returns false",
			setup:    func(_ *DockerToolExecutor) {},
			toolName: "nonexistent",
			want:     false,
		},
		{
			name: "tool with remaining calls returns false",
			setup: func(e *DockerToolExecutor) {
				e.usage.Store("limited-tool", &ToolUsage{CallCount: 1})
			},
			toolName: "limited-tool",
			want:     false,
		},
		{
			name: "exhausted tool returns true",
			setup: func(e *DockerToolExecutor) {
				e.usage.Store("limited-tool", &ToolUsage{CallCount: 5, Exhausted: 1})
			},
			toolName: "limited-tool",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &DockerToolExecutor{}
			tt.setup(executor)
			assert.Equal(t, tt.want, executor.IsToolExhausted(tt.toolName))
		})
	}
}

func TestDockerToolExecutorIsToolExhausted_NilReceiver(t *testing.T) {
	var executor *DockerToolExecutor
	assert.False(t, executor.IsToolExhausted("any-tool"))
}

func TestDockerToolExecutorExecuteTool_InvalidArguments(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	executor := &DockerToolExecutor{}
	tool := newTestTool("invalid-args")
	executor.RegisterTool(tool)

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`[]`), nil, nil)
	require.Error(t, err)
	expected := "tool \"invalid-args\" encountered an error: invalid tool arguments: failed to parse input arguments as JSON object (expected format: {\"argName\": \"value\", ...}): json: cannot unmarshal array into Go value of type map[string]interface {}"
	assert.Equal(t, expected, err.Error())
	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusInvalidArguments, call.Status)
	assert.Nil(t, call.ExitCode, "no process ever ran for an invalid-arguments error during setup")
	assert.Nil(t, call.DurationNs, "no container ever ran, so duration is never recorded")
	assert.GreaterOrEqual(t, call.WallTimeNs, int64(0), "the call attempt's wall time is still recorded")
	assert.True(t, strings.HasSuffix(err.Error(), call.ErrorMessage))

	stats := executor.GetUsageStats()
	_, ok := stats[tool.name]
	assert.False(t, ok, "no usage stats are recorded when the container never ran")

	calls := executor.GetCallSummaries()
	assert.Len(t, calls, 1, "the failed attempt is still recorded in the per-call log")
	assert.Equal(t, tool.name, calls[0].Tool)
}

func TestDockerToolExecutorExecuteTool_CreateContainerError(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("create-error")
	executor.RegisterTool(tool)

	mock.onCreate = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"message":"create error"}`)); err != nil {
			t.Fatalf("failed to write create error response: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"create-error\" encountered an error: tool internal error: failed to create tool container (image: \"alpine:latest\"): Error response from daemon: {\"message\":\"create error\"}"
	assert.Equal(t, expected, err.Error())

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusInfrastructureError, call.Status)
	assert.Nil(t, call.ExitCode, "no process ever ran when container creation itself failed")
	assert.Nil(t, call.DurationNs)
	assert.Nil(t, call.Stdout)
	assert.Nil(t, call.Stderr)
}

func TestDockerToolExecutorExecuteTool_NonZeroExit(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("exit-failure")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	mock.onWait = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"StatusCode":2}`)); err != nil {
			t.Fatalf("failed to write wait response: %v", err)
		}
	}

	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		payload := encodeDockerFrames(dockerLogFrame{Stream: 2, Data: "fatal error\n"})
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("failed to write log payload: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"exit-failure\" encountered an error: tool execution failed: tool container exited with code 2: fatal error"
	assert.Equal(t, expected, err.Error())

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusNonZeroExit, call.Status)
	require.NotNil(t, call.ExitCode)
	assert.Equal(t, int64(2), *call.ExitCode)
	require.NotNil(t, call.Stdout)
	assert.Zero(t, call.Stdout.Bytes)
	require.NotNil(t, call.Stderr)
	require.NotNil(t, call.Stderr.Preview)
	assert.Equal(t, "fatal error\n", *call.Stderr.Preview)
	assert.False(t, call.Stderr.Truncated)
}

func TestDockerToolExecutorExecuteTool_LogRetrievalError(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("log-error")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"message":"log failure"}`)); err != nil {
			t.Fatalf("failed to write log error response: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"log-error\" encountered an error: tool internal error: failed to retrieve tool output from tool container: failed to get tool container logs: Error response from daemon: {\"message\":\"log failure\"}"
	assert.Equal(t, expected, err.Error())

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusInfrastructureError, call.Status)
	require.NotNil(t, call.ExitCode, "the container did start and exit 0 before the log read failed")
	assert.Equal(t, int64(0), *call.ExitCode)
	assert.Nil(t, call.Stdout, "no output was captured since the log read itself failed")
	assert.Nil(t, call.Stderr)
}

func TestDockerToolExecutorExecuteTool_LogFetchFailureFallback(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("log-fallback")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	mock.onWait = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"StatusCode":3}`)); err != nil {
			t.Fatalf("failed to write wait response: %v", err)
		}
	}

	logCallCount := 0
	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		logCallCount++
		if logCallCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte(`{"message":"log unavailable"}`)); err != nil {
				t.Fatalf("failed to write log error response: %v", err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		payload := encodeDockerFrames(dockerLogFrame{Stream: 1, Data: "unexpected"})
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("failed to write log payload: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"log-fallback\" encountered an error: tool execution failed: tool container exited with code 3"
	assert.Equal(t, expected, err.Error())
	assert.Equal(t, 1, logCallCount)

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusNonZeroExit, call.Status)
	require.NotNil(t, call.ExitCode)
	assert.Equal(t, int64(3), *call.ExitCode)
	assert.Nil(t, call.Stdout, "no output was captured since the log fetch itself failed")
	assert.Nil(t, call.Stderr)
}

func TestDockerToolExecutorExecuteTool_EmptyOutput(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("empty-output")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "", nil)

	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		payload := encodeDockerFrames(dockerLogFrame{Stream: 1, Data: "   \n"})
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("failed to write log payload: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"empty-output\" encountered an error: tool execution failed: tool returned no output"
	assert.Equal(t, expected, err.Error())

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusEmptyOutput, call.Status)
	require.NotNil(t, call.ExitCode, "the container exited 0 despite producing no usable output")
	assert.Equal(t, int64(0), *call.ExitCode)
	require.NotNil(t, call.Stdout, "the empty output was still read successfully")
}

func TestDockerToolExecutorExecuteTool_Timeout(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	timeout := 50 * time.Millisecond
	tool := newTestTool("timeout")
	tool.timeout = &timeout
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	mock.onWait = func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"timeout\" encountered an error: tool execution timeout: execution timed out after 50ms"
	assert.Equal(t, expected, err.Error())

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusTimeout, call.Status)
	assert.True(t, call.TimedOut)
	assert.Nil(t, call.ExitCode)
	assert.Nil(t, call.Stdout)
	assert.Nil(t, call.Stderr)
}

func TestDockerToolExecutorExecuteTool_ContextCanceled(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("canceled")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	waitStarted := make(chan struct{})
	mock.onWait = func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-waitStarted:
		default:
			close(waitStarted)
		}
		<-r.Context().Done()
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-waitStarted
		cancel()
	}()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"canceled\" encountered an error: tool internal error: execution was cancelled"
	assert.Equal(t, expected, err.Error())

	call := onlyCall(t, executor, tool.name)
	assert.Equal(t, toolCallStatusInfrastructureError, call.Status)
	assert.Nil(t, call.ExitCode, "cancellation does not yield a confirmed exit code")
	assert.Nil(t, call.Stdout)
	assert.Nil(t, call.Stderr)
}

func TestDockerToolExecutorExecuteTool_ContainerStartError(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("start-error")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	mock.onStart = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"message":"start failed"}`)); err != nil {
			t.Fatalf("failed to write start error response: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"start-error\" encountered an error: tool internal error: failed to start tool container: Error response from daemon: {\"message\":\"start failed\"}"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_ContainerWaitError(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("wait-error")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored", nil)

	mock.onWait = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"message":"wait failed"}`)); err != nil {
			t.Fatalf("failed to write wait error response: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`), nil, nil)
	require.Error(t, err)
	expected := "tool \"wait-error\" encountered an error: tool internal error: failed waiting for tool to finish execution: Error response from daemon: {\"message\":\"wait failed\"}"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_FileMappingJSONValue(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("json-file")
	executor.RegisterTool(tool)

	expectedFileContent := `{"key":"value"}`
	configureSuccessfulExecution(t, mock, tool, expectedFileContent, `{"status":"ok"}`, nil)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":{"key":"value"}}`), nil, nil)
	require.NoError(t, err)
}

func TestDockerToolExecutorExecuteTool_WithAuxiliaryFiles(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("auxiliary-tool")
	tool.auxiliaryDir = "/app/data"
	executor.RegisterTool(tool)

	logger := testutils.NewTestLogger(t)

	payload := `{"input":"test payload"}`

	// Create auxiliary data files.
	auxiliaryFiles := map[string][]byte{
		"sample-input.txt": []byte("Hello, World!"),
		"config.json":      []byte(`{"key": "value", "number": 42}`),
		"image-data.bin":   {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG header
	}

	configureSuccessfulExecution(t, mock, tool, "test payload", `{"status":"processed"}`, auxiliaryFiles)

	ctx, cancel := newTestContext()
	defer cancel()

	result, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(payload), auxiliaryFiles, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"processed"}`, string(result))

	stats := executor.GetUsageStats()
	usage, ok := stats[tool.name]
	require.True(t, ok)
	assert.Equal(t, int64(1), usage.CallCount)
	assert.GreaterOrEqual(t, usage.TotalDurationNs, int64(0))
}

func TestDockerToolExecutorExecuteTool_WithSharedDirectory(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("shared-dir-tool")
	tool.sharedDir = "/app/shared"
	executor.RegisterTool(tool)

	logger := testutils.NewTestLogger(t)

	payload := `{"input":"first call"}`

	// Configure first execution.
	configureSuccessfulExecution(t, mock, tool, "first call", `{"status":"first"}`, nil)

	ctx, cancel := newTestContext()
	defer cancel()

	// First call - shared directory should be created.
	result, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(payload), nil, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"first"}`, string(result))

	// Configure second execution.
	payload2 := `{"input":"second call"}`
	configureSuccessfulExecution(t, mock, tool, "second call", `{"status":"second"}`, nil)

	// Second call - shared directory should be reused.
	result, err = executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(payload2), nil, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"second"}`, string(result))

	stats := executor.GetUsageStats()
	usage, ok := stats[tool.name]
	require.True(t, ok)
	assert.Equal(t, int64(2), usage.CallCount)

	// Verify cleanup on Close.
	err = executor.Close()
	require.NoError(t, err)
}
