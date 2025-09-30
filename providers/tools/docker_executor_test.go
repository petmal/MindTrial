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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/pkg/testutils"
)

const testAPIVersion = "1.44"

type dockerAPIMock struct {
	t *testing.T

	server     *httptest.Server
	apiVersion string

	onPing   func(http.ResponseWriter, *http.Request)
	onCreate func(http.ResponseWriter, *http.Request)
	onStart  func(http.ResponseWriter, *http.Request)
	onWait   func(http.ResponseWriter, *http.Request)
	onLogs   func(http.ResponseWriter, *http.Request)
	onRemove func(http.ResponseWriter, *http.Request)
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
	var out []byte
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

	executor := &DockerToolExecutor{client: cli}
	t.Cleanup(func() {
		_ = executor.Close()
	})
	return executor
}

func newTestTool(name string) *DockerTool {
	return &DockerTool{
		name:         name,
		image:        "alpine:latest",
		command:      []string{"/bin/echo"},
		env:          map[string]string{"FOO": "BAR"},
		fileMappings: map[string]string{"input": "/workspace/input.txt"},
	}
}

func newTestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func configureSuccessfulExecution(t *testing.T, mock *dockerAPIMock, tool *DockerTool, expectedFileContent, logOutput string) func() string {
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
		if len(req.HostConfig.Mounts) != 1 {
			t.Fatalf("expected exactly one mount, got %d", len(req.HostConfig.Mounts))
		}
		mount := req.HostConfig.Mounts[0]
		assert.Equal(t, "/workspace/input.txt", mount.Target)
		mountedFile = mount.Source
		data, err := os.ReadFile(mount.Source)
		if err != nil {
			t.Fatalf("failed to read mounted file: %v", err)
		}
		assert.Equal(t, expectedFileContent, string(data))

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

func TestDockerToolExecutorExecuteTool_Success(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("success-tool")
	executor.RegisterTool(tool)

	logger := testutils.NewTestLogger(t)

	payload := `{"input":"payload"}`
	mountedFileFn := configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`)

	ctx, cancel := newTestContext()
	defer cancel()

	result, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(payload))
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"ok"}`, string(result))

	stats := executor.GetUsageStats()
	usage, ok := stats[tool.name]
	require.True(t, ok)
	assert.Equal(t, int64(1), usage.CallCount)
	assert.Positive(t, usage.TotalTimeNs)

	mountedFile := mountedFileFn()
	require.NotEmpty(t, mountedFile)
	_, statErr := os.Stat(mountedFile)
	require.Error(t, statErr)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
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

	configureSuccessfulExecution(t, mock, tool, "payload", `{"status":"ok"}`)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.NoError(t, err)
}

func TestDockerToolExecutorExecuteTool_ToolNotRegistered(t *testing.T) {
	mock := newDockerAPIMock(t)
	_ = newTestExecutor(t, mock)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	executor := &DockerToolExecutor{client: nil}

	_, err := executor.ExecuteTool(ctx, logger, "missing", json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Equal(t, "tool not available: missing", err.Error())
}

func TestDockerToolExecutorExecuteTool_UnsupportedToolType(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	executor := &DockerToolExecutor{}
	executor.tools.Store("bad", 123)

	_, err := executor.ExecuteTool(ctx, logger, "bad", json.RawMessage(`{}`))
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

	configureSuccessfulExecution(t, mock, tool, "payload", `{"ok":true}`)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.NoError(t, err)

	_, err = executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool max calls exceeded: tool \"limited-tool\" has exceeded its maximum call limit of 1 for this session. Do not call this tool again during the current conversation"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorGetUsageStats_NilReceiver(t *testing.T) {
	var executor *DockerToolExecutor
	stats := executor.GetUsageStats()
	require.Nil(t, stats)
}

func TestDockerToolExecutorExecuteTool_InvalidArguments(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	executor := &DockerToolExecutor{}
	tool := newTestTool("invalid-args")
	executor.RegisterTool(tool)

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`[]`))
	require.Error(t, err)
	expected := "tool \"invalid-args\" encountered an error: invalid tool arguments: failed to parse input arguments as JSON object (expected format: {\"argName\": \"value\", ...}): json: cannot unmarshal array into Go value of type map[string]interface {}"
	assert.Equal(t, expected, err.Error())
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

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"create-error\" encountered an error: tool internal error: failed to create tool container (image: \"alpine:latest\"): Error response from daemon: {\"message\":\"create error\"}"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_NonZeroExit(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("exit-failure")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored")

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

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"exit-failure\" encountered an error: tool execution failed: tool container exited with code 2: fatal error"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_LogRetrievalError(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("log-error")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored")

	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"message":"log failure"}`)); err != nil {
			t.Fatalf("failed to write log error response: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"log-error\" encountered an error: tool internal error: failed to retrieve tool output from tool container: failed to get tool container logs: Error response from daemon: {\"message\":\"log failure\"}"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_LogFetchFailureFallback(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("log-fallback")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored")

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

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"log-fallback\" encountered an error: tool execution failed: tool container exited with code 3"
	assert.Equal(t, expected, err.Error())
	assert.Equal(t, 1, logCallCount)
}

func TestDockerToolExecutorExecuteTool_EmptyOutput(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("empty-output")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "")

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

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"empty-output\" encountered an error: tool execution failed: tool returned no output"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_Timeout(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	timeout := 50 * time.Millisecond
	tool := newTestTool("timeout")
	tool.timeout = &timeout
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored")

	mock.onWait = func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"timeout\" encountered an error: tool execution timeout: execution timed out after 50ms"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_ContextCanceled(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("canceled")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored")

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

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"canceled\" encountered an error: tool internal error: execution was cancelled"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_ContainerStartError(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("start-error")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored")

	mock.onStart = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"message":"start failed"}`)); err != nil {
			t.Fatalf("failed to write start error response: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
	require.Error(t, err)
	expected := "tool \"start-error\" encountered an error: tool internal error: failed to start tool container: Error response from daemon: {\"message\":\"start failed\"}"
	assert.Equal(t, expected, err.Error())
}

func TestDockerToolExecutorExecuteTool_ContainerWaitError(t *testing.T) {
	mock := newDockerAPIMock(t)
	executor := newTestExecutor(t, mock)

	tool := newTestTool("wait-error")
	executor.RegisterTool(tool)

	configureSuccessfulExecution(t, mock, tool, "payload", "ignored")

	mock.onWait = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"message":"wait failed"}`)); err != nil {
			t.Fatalf("failed to write wait error response: %v", err)
		}
	}

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":"payload"}`))
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
	configureSuccessfulExecution(t, mock, tool, expectedFileContent, `{"status":"ok"}`)

	logger := testutils.NewTestLogger(t)
	ctx, cancel := newTestContext()
	defer cancel()

	_, err := executor.ExecuteTool(ctx, logger, tool.name, json.RawMessage(`{"input":{"key":"value"}}`))
	require.NoError(t, err)
}
