// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/providers/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockChatCompletionChoice builds a minimal ChatCompletionChoice fixture for testing
// CompletionHandler.IsTerminalStopReason implementations.
func mockChatCompletionChoice(t *testing.T, finishReason string, hasToolCalls bool) openai.ChatCompletionChoice {
	toolCalls := "[]"
	if hasToolCalls {
		toolCalls = `[{"id":"call_1","type":"function","function":{"name":"test_tool","arguments":"{}"}}]`
	}
	responseJSON := fmt.Sprintf(`{"finish_reason":%q,"message":{"role":"assistant","tool_calls":%s}}`, finishReason, toolCalls)
	var choice openai.ChatCompletionChoice
	require.NoError(t, json.Unmarshal([]byte(responseJSON), &choice))
	return choice
}

func TestOpenAICompletions_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAICompletionsProvider{}
	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "gpt-test",
		DisableStructuredOutput: true,
		ModelParams: openAIV3ModelParams{
			ResponseFormat: ResponseFormatJSONObject.Ptr(),
		},
	}
	_, err := p.Run(context.Background(), logger, runCfg, config.Task{Name: "t"}, nil)
	require.ErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestOpenAICompletionsProvider_ExtensionPoints(t *testing.T) {
	provider := newOpenAICompletionsProvider(nil)
	require.Equal(t, InputTokenAccountingCacheTokensIncluded, provider.InputTokenAccounting)
	require.Equal(t, OutputTokenAccountingReasoningTokensIncluded, provider.OutputTokenAccounting)
	assert.True(t, provider.isTransientResponse(ErrStreamResponse))
	assert.False(t, provider.isTransientResponse(errors.ErrUnsupported))

	provider.OutputTokenAccounting = OutputTokenAccountingReasoningTokensSeparate
	assert.Equal(t, OutputTokenAccountingReasoningTokensSeparate, provider.OutputTokenAccounting)
	provider.InputTokenAccounting = InputTokenAccountingCacheTokensSeparate
	assert.Equal(t, InputTokenAccountingCacheTokensSeparate, provider.InputTokenAccounting)

	called := false
	provider.IsRetryableError = func(err error) bool {
		called = true
		return err != nil
	}
	assert.True(t, provider.isTransientResponse(errors.ErrUnsupported))
	assert.True(t, called)
}

func TestOpenAICompletions_Run_ServerTools_CapturedBeforeValidation(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAICompletionsProvider{}
	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "gpt-test",
		DisableStructuredOutput: true,
		ModelParams: openAIV3ModelParams{
			ResponseFormat: ResponseFormatJSONObject.Ptr(),
			ServerTools: []openAIServerTool{
				{Type: "openrouter:fusion"},
			},
		},
	}
	_, err := p.Run(context.Background(), logger, runCfg, config.Task{Name: "t"}, nil)
	require.ErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestOpenAICompletions_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAICompletionsProvider{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "gpt-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "application/octet-stream")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task, nil)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestOpenAICompletions_Run_ToolErrors(t *testing.T) {
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
			wantErr:      tools.ErrTaskRuntimeConfig,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var requestBodies []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				mu.Lock()
				requestBodies = append(requestBodies, string(body))
				firstRequest := len(requestBodies) == 1
				mu.Unlock()

				finishReason, message := "stop", `{"role":"assistant","content":"done"}`
				if firstRequest {
					finishReason = "tool_calls"
					message = fmt.Sprintf(`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":%q,"arguments":"{}"}}]}`, tt.calledTool)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"id":"chatcmpl-1","object":"chat.completion","created":0,"model":"gpt-test","choices":[{"index":0,"finish_reason":%q,"message":%s}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, finishReason, message)
			}))
			t.Cleanup(server.Close)

			provider := newOpenAICompletionsProvider([]config.ToolConfig{worldClient}, option.WithBaseURL(server.URL), option.WithAPIKey("test"))
			task := config.Task{
				Name:                 "inspect world",
				Prompt:               "Inspect the world.",
				ResponseResultFormat: config.NewResponseFormat("text"),
				ToolSelector:         &config.ToolSelector{Tools: []config.ToolSelection{{Name: worldClient.Name}}},
			}
			task.ResolveToolSelector(config.ToolSelector{})
			runCfg := config.RunConfig{Name: "test-run", Model: "gpt-test", DisableStructuredOutput: true}

			// No environment is active, so the service-backed tool fails with a task runtime error.
			result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)

			mu.Lock()
			defer mu.Unlock()
			require.Len(t, requestBodies, tt.wantRequests)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.ErrorIs(t, err, ErrToolUse)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "done", result.GetFinalAnswerContent())
			assert.Contains(t, requestBodies[1], "Tool execution failed")
		})
	}
}

func TestDefaultCompletionHandler_ToParam(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NewTestLogger(t)

	t.Run("converts message to param", func(t *testing.T) {
		handler := &defaultCompletionHandler{}

		responseJSON := `{
			"role": "assistant",
			"content": "Hello!",
			"tool_calls": [
				{
					"id": "call_1",
					"type": "function",
					"function": {
						"name": "test_tool",
						"arguments": "{}"
					}
				}
			]
		}`
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(responseJSON), &message))

		result := handler.ToParam(ctx, logger, message)

		require.NotNil(t, result.OfAssistant)
		assert.Equal(t, "Hello!", result.OfAssistant.Content.OfString.Value)
		require.Len(t, result.OfAssistant.ToolCalls, 1)
		assert.Equal(t, "call_1", result.OfAssistant.ToolCalls[0].OfFunction.ID)
		assert.Equal(t, "test_tool", result.OfAssistant.ToolCalls[0].OfFunction.Function.Name)
	})

	t.Run("does not preserve extra fields", func(t *testing.T) {
		handler := &defaultCompletionHandler{}

		// The default handler should NOT preserve non-standard fields like reasoning_content.
		responseJSON := `{
			"role": "assistant",
			"content": "Result.",
			"reasoning_content": "Some reasoning..."
		}`
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(responseJSON), &message))

		result := handler.ToParam(ctx, logger, message)

		require.NotNil(t, result.OfAssistant)
		assert.Equal(t, "Result.", result.OfAssistant.Content.OfString.Value)

		// Default handler drops extra fields — this is expected SDK behavior.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.NotContains(t, raw, "reasoning_content")
	})

	t.Run("terminal stop reasons", func(t *testing.T) {
		handler := &defaultCompletionHandler{}

		assert.True(t, handler.IsTerminalStopReason(mockChatCompletionChoice(t, "stop", false)))
		assert.True(t, handler.IsTerminalStopReason(mockChatCompletionChoice(t, "length", false)))
		assert.True(t, handler.IsTerminalStopReason(mockChatCompletionChoice(t, "content_filter", false)))
		assert.False(t, handler.IsTerminalStopReason(mockChatCompletionChoice(t, "tool_calls", true)))
	})

	t.Run("accumulates streaming chunks", func(t *testing.T) {
		handler := &defaultCompletionHandler{}

		chunks := []string{
			`{"choices": [{"index": 0, "delta": {"role": "assistant", "content": "Hello"}}]}`,
			`{"choices": [{"index": 0, "delta": {"content": " world"}}]}`,
			`{"choices": [{"index": 0, "delta": {"content": "!"}}]}`,
		}
		for _, raw := range chunks {
			var chunk openai.ChatCompletionChunk
			require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
			assert.True(t, handler.AddChunk(ctx, logger, chunk))
		}

		result := handler.Result()
		require.NotNil(t, result)
		require.Len(t, result.Choices, 1)
		assert.Equal(t, "Hello world!", result.Choices[0].Message.Content)
		assert.Equal(t, "assistant", string(result.Choices[0].Message.Role))
	})

	t.Run("does not accumulate extra fields from streaming chunks", func(t *testing.T) {
		handler := &defaultCompletionHandler{}

		// Chunks with reasoning_content (non-standard extra field).
		chunks := []string{
			`{"choices": [{"index": 0, "delta": {"role": "assistant", "content": "Result", "reasoning_content": "Thinking..."}}]}`,
			`{"choices": [{"index": 0, "delta": {"content": "."}}]}`,
		}
		for _, raw := range chunks {
			var chunk openai.ChatCompletionChunk
			require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
			assert.True(t, handler.AddChunk(ctx, logger, chunk))
		}

		result := handler.Result()
		require.NotNil(t, result)
		require.Len(t, result.Choices, 1)
		assert.Equal(t, "Result.", result.Choices[0].Message.Content)

		// The SDK's ChatCompletionAccumulator drops extra fields during streaming.
		assert.Empty(t, result.Choices[0].Message.JSON.ExtraFields)
	})
}

func TestDefaultCompletionHandler_InputCacheTokens(t *testing.T) {
	tests := []struct {
		name      string
		usageJSON string
		write     *int64
		read      *int64
	}{
		{name: "omitted", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}`},
		{name: "details without cache counters", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"audio_tokens":1}}`},
		{name: "reported", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":4,"cache_write_tokens":3}}`, write: testutils.Ptr(int64(3)), read: testutils.Ptr(int64(4))},
		{name: "reported zero is not absent", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}`, write: testutils.Ptr(int64(0)), read: testutils.Ptr(int64(0))},
		{name: "explicit null is absent", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":null,"cache_write_tokens":null}}`},
		{name: "read only", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":7}}`, read: testutils.Ptr(int64(7))},
	}

	handler := &defaultCompletionHandler{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var usage openai.CompletionUsage
			require.NoError(t, json.Unmarshal([]byte(test.usageJSON), &usage))
			write, read := handler.InputCacheTokens(usage)
			require.Equal(t, test.write, write)
			require.Equal(t, test.read, read)
		})
	}
}

func TestMapImageDetailToOpenAI(t *testing.T) {
	provider := &openAICompletionsProvider{}
	logger := testutils.NewTestLogger(t)

	tests := []struct {
		name     string
		detail   *config.ImageDetail
		expected string
	}{
		{name: "nil defaults to auto", detail: nil, expected: "auto"},
		{name: "auto", detail: testutils.Ptr(config.ImageDetailAuto), expected: "auto"},
		{name: "low", detail: testutils.Ptr(config.ImageDetailLow), expected: "low"},
		{name: "medium maps to high", detail: testutils.Ptr(config.ImageDetailMedium), expected: "high"},
		{name: "high", detail: testutils.Ptr(config.ImageDetailHigh), expected: "high"},
		{name: "original", detail: testutils.Ptr(config.ImageDetailOriginal), expected: "original"},
		{name: "unknown falls back to auto", detail: testutils.Ptr(config.ImageDetail("unknown")), expected: "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.mapImageDetailToOpenAI(context.Background(), logger, tt.detail)
			assert.Equal(t, tt.expected, result)
		})
	}
}
