// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/testutils"
	xai "github.com/petmal/mindtrial/pkg/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXAI_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &XAI{} // nil client configuration is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "grok-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task, nil)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestXAI_InvalidModelParams(t *testing.T) {
	p := &XAI{}
	runCfg := config.RunConfig{Name: "test-run", Model: "grok-test", ModelParams: config.OpenAIModelParams{}}
	_, err := p.Run(context.Background(), testutils.NewTestLogger(t), runCfg, config.Task{Name: "t"}, nil)
	require.ErrorIs(t, err, ErrInvalidModelParams)
}

func TestXAI_IsTransientResponse(t *testing.T) {
	p := &XAI{}
	tests := []struct {
		name     string
		response *http.Response
		want     bool
	}{
		{name: "nil response", response: nil, want: false},
		{name: "rate limited", response: &http.Response{StatusCode: http.StatusTooManyRequests}, want: true},
		{name: "service unavailable", response: &http.Response{StatusCode: http.StatusServiceUnavailable}, want: true},
		{name: "bad request", response: &http.Response{StatusCode: http.StatusBadRequest}, want: false},
		{name: "success", response: &http.Response{StatusCode: http.StatusOK}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.isTransientResponse(tt.response))
		})
	}
}

func TestXAI_OutputItemDecoding(t *testing.T) {
	tests := []struct {
		name            string
		item            string
		wantType        string
		wantMessageText string
		wantFunction    string
		wantReasoning   bool
		wantReplayable  bool
	}{
		{
			name:            "message",
			item:            `{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"hello","annotations":[]}]}`,
			wantType:        "message",
			wantMessageText: "hello",
			wantReplayable:  true,
		},
		{
			name:           "function call",
			item:           `{"id":"fc_1","type":"function_call","status":"completed","call_id":"call_1","name":"tool","arguments":"{}"}`,
			wantType:       "function_call",
			wantFunction:   "tool",
			wantReplayable: true,
		},
		{
			name:           "reasoning",
			item:           `{"id":"rs_1","type":"reasoning","status":"completed","summary":[],"encrypted_content":"secret"}`,
			wantType:       "reasoning",
			wantReasoning:  true,
			wantReplayable: true,
		},
		{
			name:     "unknown item type is tolerated",
			item:     `{"id":"ws_1","type":"web_search_call","status":"completed","action":{"type":"search"}}`,
			wantType: "web_search_call",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var item xaiOutputItem
			require.NoError(t, json.Unmarshal([]byte(tt.item), &item))
			assert.Equal(t, tt.wantType, item.Type)
			if tt.wantMessageText != "" {
				require.NotNil(t, item.Message)
				require.Len(t, item.Message.Content, 1)
				require.NotNil(t, item.Message.Content[0].OutputMessageContentOneOf)
				assert.Equal(t, tt.wantMessageText, item.Message.Content[0].OutputMessageContentOneOf.Text)
			} else {
				assert.Nil(t, item.Message)
			}
			if tt.wantFunction != "" {
				require.NotNil(t, item.FunctionCall)
				assert.Equal(t, tt.wantFunction, item.FunctionCall.Name)
			} else {
				assert.Nil(t, item.FunctionCall)
			}
			assert.Equal(t, tt.wantReasoning, item.Reasoning != nil)

			inputPart, replayable := item.toInputPart()
			assert.Equal(t, tt.wantReplayable, replayable)
			if replayable {
				// Replayed items must round-trip unchanged.
				replayed, err := json.Marshal(inputPart)
				require.NoError(t, err)
				assert.JSONEq(t, tt.item, string(replayed))
			}
		})
	}
}

type xaiFakeResponse struct {
	statusCode int
	body       string
}

type xaiWarningLogger struct {
	logging.Logger
	warnings []string
}

func (l *xaiWarningLogger) Message(ctx context.Context, level slog.Level, msg string, args ...any) {
	l.Logger.Message(ctx, level, msg, args...)
	if level == logging.LevelWarn {
		l.warnings = append(l.warnings, fmt.Sprintf(msg, args...))
	}
}

// xaiFakeServer is a fake Responses API server returning canned responses in order.
type xaiFakeServer struct {
	mu        sync.Mutex
	responses []xaiFakeResponse
	requests  []map[string]any
	paths     []string
	headers   []http.Header
}

func newXAIFakeServer(t *testing.T, responses ...xaiFakeResponse) (*xaiFakeServer, *httptest.Server) {
	fake := &xaiFakeServer{responses: responses}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var request map[string]any
		assert.NoError(t, json.Unmarshal(body, &request))

		fake.mu.Lock()
		index := len(fake.requests)
		fake.requests = append(fake.requests, request)
		fake.paths = append(fake.paths, r.URL.Path)
		fake.headers = append(fake.headers, r.Header.Clone())
		fake.mu.Unlock()

		response := xaiFakeResponse{statusCode: http.StatusInternalServerError, body: `{"error":"unexpected request"}`}
		if index < len(fake.responses) {
			response = fake.responses[index]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.statusCode)
		_, _ = io.WriteString(w, response.body)
	}))
	t.Cleanup(server.Close)
	return fake, server
}

func (f *xaiFakeServer) recordedRequests() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func newXAITestProvider(t *testing.T, serverURL string, availableTools []config.ToolConfig) *XAI {
	provider, err := NewXAI(config.XAIClientConfig{APIKey: "test-key"}, availableTools)
	require.NoError(t, err)
	provider.clientCfg.Servers = xai.ServerConfigurations{{URL: serverURL}}
	return provider
}

func xaiResponseJSON(t *testing.T, id string, status string, output ...map[string]any) string {
	if output == nil {
		output = []map[string]any{}
	}
	response := map[string]any{
		"id":     id,
		"object": "response",
		"status": status,
		"model":  "grok-test",
		"output": output,
		"usage": map[string]any{
			"input_tokens":          10,
			"input_tokens_details":  map[string]any{"cached_tokens": 2},
			"output_tokens":         5,
			"output_tokens_details": map[string]any{"reasoning_tokens": 3},
			"total_tokens":          18,
		},
	}
	data, err := json.Marshal(response)
	require.NoError(t, err)
	return string(data)
}

func xaiMessageItem(id string, texts ...string) map[string]any {
	content := make([]any, 0, len(texts))
	for _, text := range texts {
		content = append(content, map[string]any{"type": "output_text", "text": text, "annotations": []any{}})
	}
	return map[string]any{"id": id, "type": "message", "status": "completed", "role": "assistant", "content": content}
}

func xaiReasoningItem(id string, encryptedContent string) map[string]any {
	return map[string]any{"id": id, "type": "reasoning", "status": "completed", "summary": []any{}, "encrypted_content": encryptedContent}
}

func xaiFunctionCallItem(id string, callID string, name string) map[string]any {
	return map[string]any{"id": id, "type": "function_call", "status": "completed", "call_id": callID, "name": name, "arguments": `{"q":"x"}`}
}

const xaiTestFinalAnswerJSON = `{"title":"t","explanation":"e","final_answer":{"content":"done"}}`

func TestXAI_Run_SingleTurn(t *testing.T) {
	fake, server := newXAIFakeServer(t, xaiFakeResponse{
		statusCode: http.StatusOK,
		body: xaiResponseJSON(t, "resp-1", "completed",
			xaiReasoningItem("rs-1", "secret"),
			xaiMessageItem("msg-1", `{"title":"t","explanation":"e",`, `"final_answer":{"content":"done"}}`),
		),
	})
	provider := newXAITestProvider(t, server.URL, nil)

	runCfg := config.RunConfig{
		Name:  "test-run",
		Model: "grok-test",
		ModelParams: config.XAIModelParams{
			Temperature:         testutils.Ptr(float32(0.5)),
			TopP:                testutils.Ptr(float32(0.25)),
			MaxCompletionTokens: testutils.Ptr(int32(1024)),
			PresencePenalty:     testutils.Ptr(float32(0.1)),
			FrequencyPenalty:    testutils.Ptr(float32(0.1)),
			ReasoningEffort:     testutils.Ptr("xhigh"),
			Seed:                testutils.Ptr(int32(7)),
		},
	}
	task := config.Task{Name: "t", Prompt: "the prompt", ResponseResultFormat: config.NewResponseFormat("text")}

	logger := &xaiWarningLogger{Logger: testutils.NewTestLogger(t)}
	result, err := provider.Run(t.Context(), logger, runCfg, task, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", result.GetFinalAnswerContent(), "text of all message parts must be concatenated")
	assert.Equal(t, []string{
		"presence-penalty parameter is not supported by the Responses API and will be ignored",
		"frequency-penalty parameter is not supported by the Responses API and will be ignored",
		"seed parameter is not supported by the Responses API and will be ignored",
	}, logger.warnings)

	usage := result.GetUsage()
	assert.Equal(t, testutils.Ptr(int64(10)), usage.InputTokens)
	assert.Equal(t, testutils.Ptr(int64(5)), usage.OutputTokens)
	assert.Equal(t, testutils.Ptr(int64(3)), usage.ReasoningTokens)
	assert.Equal(t, testutils.Ptr(int64(2)), usage.InputCacheReadTokens)
	assert.Equal(t, InputTokenAccountingCacheTokensIncluded, usage.InputTokenAccounting)
	assert.Equal(t, OutputTokenAccountingReasoningTokensSeparate, usage.OutputTokenAccounting, "xAI reports reasoning tokens separately from output tokens")

	requests := fake.recordedRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, "/v1/responses", fake.paths[0])
	assert.Equal(t, "Bearer test-key", fake.headers[0].Get("Authorization"))
	assert.Equal(t, "application/json", fake.headers[0].Get("Content-Type"))

	request := requests[0]
	assert.Equal(t, "grok-test", request["model"])
	assert.Equal(t, true, request["store"])
	assert.NotEmpty(t, request["prompt_cache_key"])
	assert.NotContains(t, request, "previous_response_id")
	assert.NotContains(t, request, "include")
	assert.NotContains(t, request, "tools")
	assert.NotContains(t, request, "tool_choice")
	assert.InDelta(t, 0.5, request["temperature"], 0.0001)
	assert.InDelta(t, 0.25, request["top_p"], 0.0001)
	assert.InDelta(t, 1024, request["max_output_tokens"], 0)
	assert.Equal(t, map[string]any{"effort": "xhigh"}, request["reasoning"])
	for _, unsupported := range []string{"background", "presence_penalty", "frequency_penalty", "seed", "reasoning_effort"} {
		assert.NotContains(t, request, unsupported, "unsupported parameters must not be sent")
	}

	textFormat := request["text"].(map[string]any)["format"].(map[string]any)
	assert.Equal(t, "json_schema", textFormat["type"])
	assert.Equal(t, "response", textFormat["name"])
	assert.NotEmpty(t, textFormat["schema"])

	input := request["input"].([]any)
	require.Len(t, input, 1, "no system message is needed for JSON schema mode without answer format")
	userMessage := input[0].(map[string]any)
	assert.Equal(t, "user", userMessage["role"])
	assert.Equal(t, []any{map[string]any{"type": "input_text", "text": "the prompt"}}, userMessage["content"])
}

func TestXAI_Run_DocumentedTokenUsage(t *testing.T) {
	_, server := newXAIFakeServer(t, xaiFakeResponse{
		statusCode: http.StatusOK,
		body: `{
			"id":"resp-1",
			"status":"completed",
			"output":[{"id":"msg-1","type":"message","status":"completed","role":"assistant",
				"content":[{"type":"output_text","text":"done","annotations":[]}]}],
			"usage":{
				"input_tokens":32,
				"input_tokens_details":{"cached_tokens":8},
				"output_tokens":9,
				"output_tokens_details":{"reasoning_tokens":110},
				"total_tokens":151
			}
		}`,
	})
	provider := newXAITestProvider(t, server.URL, nil)
	runCfg := config.RunConfig{Name: "test-run", Model: "grok-test", DisableStructuredOutput: true}
	task := config.Task{Name: "t", Prompt: "the prompt", ResponseResultFormat: config.NewResponseFormat("text")}

	result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", result.GetFinalAnswerContent())

	usage := result.GetUsage()
	require.NotNil(t, usage.InputTokens)
	require.NotNil(t, usage.OutputTokens)
	require.NotNil(t, usage.ReasoningTokens)
	assert.Equal(t, int64(32), *usage.InputTokens)
	assert.Equal(t, testutils.Ptr(int64(8)), usage.InputCacheReadTokens)
	assert.Equal(t, int64(9), *usage.OutputTokens)
	assert.Equal(t, int64(110), *usage.ReasoningTokens)
	assert.Equal(t, InputTokenAccountingCacheTokensIncluded, usage.InputTokenAccounting)
	assert.Equal(t, OutputTokenAccountingReasoningTokensSeparate, usage.OutputTokenAccounting)
	assert.Equal(t, int64(151), *usage.InputTokens+*usage.OutputTokens+*usage.ReasoningTokens)
}

func TestXAI_Run_UnstructuredOutput(t *testing.T) {
	fake, server := newXAIFakeServer(t, xaiFakeResponse{
		statusCode: http.StatusOK,
		body:       xaiResponseJSON(t, "resp-1", "completed", xaiMessageItem("msg-1", "the ", "answer")),
	})
	provider := newXAITestProvider(t, server.URL, nil)

	runCfg := config.RunConfig{Name: "test-run", Model: "grok-test", DisableStructuredOutput: true}
	task := config.Task{Name: "t", Prompt: "the prompt", ResponseResultFormat: config.NewResponseFormat("text")}

	result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)
	require.NoError(t, err)
	assert.Equal(t, "the answer", result.GetFinalAnswerContent(), "the response text is the answer in unstructured mode")

	requests := fake.recordedRequests()
	require.Len(t, requests, 1)
	textFormat := requests[0]["text"].(map[string]any)["format"].(map[string]any)
	assert.Equal(t, map[string]any{"type": "text"}, textFormat)

	input := requests[0]["input"].([]any)
	require.Len(t, input, 2)
	systemMessage := input[0].(map[string]any)
	assert.Equal(t, "system", systemMessage["role"])
	assert.Equal(t, DefaultUnstructuredResponseInstruction(), systemMessage["content"], "unstructured response instruction must be sent as system message")
	assert.Equal(t, "user", input[1].(map[string]any)["role"])
}

func TestXAI_Run_ToolConversation(t *testing.T) {
	worldClient := config.ToolConfig{
		Name:         "world-client",
		Image:        "world-client:latest",
		Description:  "Inspects the world.",
		Parameters:   map[string]interface{}{"type": "object"},
		Dependencies: []config.ServiceDependency{{Service: "world"}},
	}

	tests := []struct {
		name         string
		calledTool   string
		wantErr      error
		wantRequests int
	}{
		{
			name:         "ordinary tool error is returned to the model",
			calledTool:   "unknown-tool",
			wantRequests: 2,
		},
		{
			name:         "task runtime error ends the task",
			calledTool:   worldClient.Name,
			wantErr:      ErrToolUse,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, server := newXAIFakeServer(t,
				xaiFakeResponse{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "completed",
					xaiReasoningItem("rs-1", "secret"),
					xaiMessageItem("msg-1", "Let me check."),
					xaiFunctionCallItem("fc-1", "call_1", tt.calledTool),
				)},
				xaiFakeResponse{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-2", "completed",
					xaiMessageItem("msg-2", "done"),
				)},
			)
			provider := newXAITestProvider(t, server.URL, []config.ToolConfig{worldClient})
			task := config.Task{
				Name:                 "inspect world",
				Prompt:               "Inspect the world.",
				ResponseResultFormat: config.NewResponseFormat("text"),
				ToolSelector:         &config.ToolSelector{Tools: []config.ToolSelection{{Name: worldClient.Name}}},
			}
			task.ResolveToolSelector(config.ToolSelector{})
			runCfg := config.RunConfig{Name: "test-run", Model: "grok-test", DisableStructuredOutput: true}

			// No environment is active, so the service-backed tool fails with a task runtime error.
			result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)

			requests := fake.recordedRequests()
			require.Len(t, requests, tt.wantRequests)
			first := requests[0]
			assert.Equal(t, []any{map[string]any{
				"type":        "function",
				"name":        worldClient.Name,
				"description": worldClient.Description,
				"parameters":  map[string]any{"type": "object"},
				"strict":      false,
			}}, first["tools"])
			assert.Equal(t, "auto", first["tool_choice"])
			assert.Equal(t, true, first["store"])
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "done", result.GetFinalAnswerContent())

			second := requests[1]
			assert.Equal(t, "resp-1", second["previous_response_id"], "conversation must be chained server-side")
			assert.Equal(t, first["tools"], second["tools"], "tools must be sent on every turn")
			assert.Equal(t, first["tool_choice"], second["tool_choice"], "tool choice must be sent on every turn")
			assert.Equal(t, first["text"], second["text"], "response format must be sent on every turn")
			assert.NotContains(t, second, "include")

			input := second["input"].([]any)
			require.Len(t, input, 1, "only the function call output is needed with previous_response_id")
			functionCallOutput := input[0].(map[string]any)
			assert.Equal(t, "function_call_output", functionCallOutput["type"])
			assert.Equal(t, "call_1", functionCallOutput["call_id"])
			assert.Contains(t, functionCallOutput["output"], "Tool execution failed")

			usage := result.GetUsage()
			assert.Equal(t, testutils.Ptr(int64(20)), usage.InputTokens, "usage must be accumulated across turns")
			assert.Equal(t, testutils.Ptr(int64(4)), usage.InputCacheReadTokens)
			assert.Equal(t, testutils.Ptr(int64(10)), usage.OutputTokens)
			assert.Equal(t, testutils.Ptr(int64(6)), usage.ReasoningTokens)
			assert.Equal(t, InputTokenAccountingCacheTokensIncluded, usage.InputTokenAccounting)
			assert.Equal(t, OutputTokenAccountingReasoningTokensSeparate, usage.OutputTokenAccounting)
		})
	}
}

func TestXAI_Run_ToolConversationWithImage(t *testing.T) {
	tool := config.ToolConfig{
		Name:        "lookup",
		Image:       "lookup:latest",
		Description: "Looks things up.",
		Parameters:  map[string]interface{}{"type": "object"},
	}
	fake, server := newXAIFakeServer(t,
		xaiFakeResponse{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "completed",
			xaiReasoningItem("rs-1", "secret"),
			xaiFunctionCallItem("fc-1", "call_1", "unknown-tool"),
		)},
		xaiFakeResponse{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-2", "completed",
			xaiMessageItem("msg-2", xaiTestFinalAnswerJSON),
		)},
	)
	provider := newXAITestProvider(t, server.URL, []config.ToolConfig{tool})

	imageBytes := []byte("fake-png-bytes")
	imagePath := testutils.CreateMockFile(t, "img-*.png", imageBytes)
	task := config.Task{
		Name:                 "describe image",
		Prompt:               "Describe the image.",
		ResponseResultFormat: config.NewResponseFormat("text"),
		Files:                []config.TaskFile{mockTaskFileWithAccess(t, "img.png", imagePath, "image/png", []config.FileAccess{config.FileAccessNative})},
		ToolSelector:         &config.ToolSelector{Tools: []config.ToolSelection{{Name: tool.Name}}},
	}
	task.ResolveToolSelector(config.ToolSelector{})
	runCfg := config.RunConfig{Name: "test-run", Model: "grok-test"}

	result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", result.GetFinalAnswerContent())

	requests := fake.recordedRequests()
	require.Len(t, requests, 2)

	first := requests[0]
	assert.Equal(t, false, first["store"], "requests with images must not be stored on the server")
	assert.Equal(t, []any{"reasoning.encrypted_content"}, first["include"])
	firstInput := first["input"].([]any)
	require.Len(t, firstInput, 1)
	userMessage := firstInput[0].(map[string]any)
	assert.Equal(t, "user", userMessage["role"])
	assert.Equal(t, []any{
		map[string]any{"type": "input_text", "text": "[file: img.png]"},
		map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)},
		map[string]any{"type": "input_text", "text": "Describe the image."},
	}, userMessage["content"], "the prompt text must follow the file data")

	second := requests[1]
	assert.NotContains(t, second, "previous_response_id", "conversation history must be managed by the client")
	assert.Equal(t, false, second["store"])
	assert.Equal(t, first["tools"], second["tools"], "tools must be sent on every turn")
	secondInput := second["input"].([]any)
	require.Len(t, secondInput, 4, "input must replay the original message, the model output and the function call output")
	assert.Equal(t, userMessage, secondInput[0])
	assert.Equal(t, xaiReasoningItem("rs-1", "secret"), secondInput[1], "encrypted reasoning must be replayed unchanged")
	assert.Equal(t, xaiFunctionCallItem("fc-1", "call_1", "unknown-tool"), secondInput[2])
	functionCallOutput := secondInput[3].(map[string]any)
	assert.Equal(t, "function_call_output", functionCallOutput["type"])
	assert.Equal(t, "call_1", functionCallOutput["call_id"])
	assert.Contains(t, functionCallOutput["output"], "Tool execution failed")
}

func TestXAI_Run_ResponseHandling(t *testing.T) {
	tests := []struct {
		name          string
		responses     []xaiFakeResponse
		wantErr       error
		wantRetryable bool
		wantNoContent bool
		wantBody      string
		wantMessage   string
		wantRequests  int
	}{
		{
			name:          "transient HTTP error is retryable",
			responses:     []xaiFakeResponse{{statusCode: http.StatusServiceUnavailable, body: `{"error":"overloaded"}`}},
			wantErr:       ErrAPIRequest,
			wantRetryable: true,
			wantBody:      `{"error":"overloaded"}`,
			wantMessage:   "503 Service Unavailable",
			wantRequests:  1,
		},
		{
			name:         "client HTTP error is not retryable",
			responses:    []xaiFakeResponse{{statusCode: http.StatusBadRequest, body: `{"error":"bad request"}`}},
			wantErr:      ErrAPIRequest,
			wantBody:     `{"error":"bad request"}`,
			wantMessage:  "400 Bad Request",
			wantRequests: 1,
		},
		{
			name:         "malformed response body",
			responses:    []xaiFakeResponse{{statusCode: http.StatusOK, body: `not json`}},
			wantErr:      ErrGenerateResponse,
			wantBody:     `not json`,
			wantRequests: 1,
		},
		{
			name:         "response with error object is not retryable",
			responses:    []xaiFakeResponse{{statusCode: http.StatusOK, body: `{"id":"resp-1","status":"incomplete","error":{"code":"server_error","message":"boom"},"output":[]}`}},
			wantErr:      ErrGenerateResponse,
			wantMessage:  "server_error: boom",
			wantRequests: 1,
		},
		{
			name:         "response error code alone does not imply a rate limit",
			responses:    []xaiFakeResponse{{statusCode: http.StatusOK, body: `{"id":"resp-1","status":"incomplete","error":{"code":"rate_limit_exceeded","message":"boom"},"output":[]}`}},
			wantErr:      ErrGenerateResponse,
			wantMessage:  "rate_limit_exceeded: boom",
			wantRequests: 1,
		},
		{
			name:         "response with unstructured error object is not retryable",
			responses:    []xaiFakeResponse{{statusCode: http.StatusOK, body: `{"id":"resp-1","status":"incomplete","error":"something broke","output":[]}`}},
			wantErr:      ErrGenerateResponse,
			wantMessage:  "something broke",
			wantRequests: 1,
		},
		{
			name:          "unfinished response is retryable",
			responses:     []xaiFakeResponse{{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "in_progress")}},
			wantErr:       ErrGenerateResponse,
			wantRetryable: true,
			wantRequests:  1,
		},
		{
			name:          "incomplete response without text",
			responses:     []xaiFakeResponse{{statusCode: http.StatusOK, body: `{"id":"resp-1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`}},
			wantNoContent: true,
			wantRequests:  1,
		},
		{
			name: "incomplete response with function calls is terminal",
			responses: []xaiFakeResponse{{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "incomplete",
				xaiFunctionCallItem("fc-1", "call_1", "tool"),
			)}},
			wantNoContent: true,
			wantRequests:  1,
		},
		{
			name: "unknown status with function calls is terminal",
			responses: []xaiFakeResponse{{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "future_status",
				xaiFunctionCallItem("fc-1", "call_1", "tool"),
			)}},
			wantNoContent: true,
			wantRequests:  1,
		},
		{
			name: "refusal without text",
			responses: []xaiFakeResponse{{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "completed",
				map[string]any{"id": "msg-1", "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "refusal", "refusal": "no"}}},
			)}},
			wantNoContent: true,
			wantRequests:  1,
		},
		{
			name:         "incomplete response with text is parsed",
			responses:    []xaiFakeResponse{{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "incomplete", xaiMessageItem("msg-1", xaiTestFinalAnswerJSON))}},
			wantRequests: 1,
		},
		{
			name: "unknown output item types are ignored",
			responses: []xaiFakeResponse{{statusCode: http.StatusOK, body: xaiResponseJSON(t, "resp-1", "completed",
				map[string]any{"id": "ws-1", "type": "web_search_call", "status": "completed", "action": map[string]any{"type": "search"}},
				xaiMessageItem("msg-1", xaiTestFinalAnswerJSON),
			)}},
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, server := newXAIFakeServer(t, tt.responses...)
			provider := newXAITestProvider(t, server.URL, nil)
			runCfg := config.RunConfig{Name: "test-run", Model: "grok-test"}
			task := config.Task{Name: "t", Prompt: "the prompt", ResponseResultFormat: config.NewResponseFormat("text")}

			result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)

			assert.Len(t, fake.recordedRequests(), tt.wantRequests)
			switch {
			case tt.wantNoContent:
				var noContentErr *ErrNoActionableContent
				require.ErrorAs(t, err, &noContentErr)
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.wantRetryable, errors.Is(err, ErrRetryable))
				if tt.wantMessage != "" {
					assert.Contains(t, err.Error(), tt.wantMessage)
				}
				if tt.wantBody != "" {
					var apiErr *ErrAPIResponse
					require.ErrorAs(t, err, &apiErr)
					assert.Equal(t, tt.wantBody, string(apiErr.Body))
				}
			default:
				require.NoError(t, err)
				assert.Equal(t, "done", result.GetFinalAnswerContent())
			}
		})
	}
}
