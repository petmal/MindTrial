// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/json"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoonshotAI_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MoonshotAI{} // nil client is sufficient to exercise parameter mapping and validation

	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "kimi-test",
		DisableStructuredOutput: true,
		// MoonshotAI does not set ResponseFormat when DisableStructuredOutput is true, so no incompatibility
	}
	task := config.Task{
		Name: "t",
		Files: []config.TaskFile{
			mockTaskFile(t, "test.txt", "file://test.txt", "text/plain"), // Unsupported file type to cause early error
		},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.Error(t, err) // Should error due to unsupported file type
	require.NotErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestMoonshotAI_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MoonshotAI{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "kimi-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestMoonshotAICopyToOpenAIV3Params(t *testing.T) {
	buildParams := func(t *testing.T, cfg config.RunConfig) openAIV3ModelParams {
		params := openAIV3ModelParams{}
		if cfg.ModelParams == nil {
			return params
		}
		moonshotAIParams, ok := cfg.ModelParams.(config.MoonshotAIModelParams)
		require.True(t, ok)
		provider := &MoonshotAI{}
		provider.copyToOpenAIV3Params(moonshotAIParams, &params)
		return params
	}

	t.Run("numeric parameters with type conversion", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.MoonshotAIModelParams{
				Temperature:      utils.Ptr(float32(0.7)),
				TopP:             utils.Ptr(float32(0.9)),
				PresencePenalty:  utils.Ptr(float32(0.5)),
				FrequencyPenalty: utils.Ptr(float32(0.3)),
				MaxTokens:        utils.Ptr(int32(1000)),
			},
		}
		params := buildParams(t, cfg)
		// Assert float32 -> float64 conversion
		require.IsType(t, (*float64)(nil), params.Temperature)
		require.IsType(t, (*float64)(nil), params.TopP)
		require.IsType(t, (*float64)(nil), params.PresencePenalty)
		require.IsType(t, (*float64)(nil), params.FrequencyPenalty)
		require.InDelta(t, 0.7, *params.Temperature, 0.0001)
		require.InDelta(t, 0.9, *params.TopP, 0.0001)
		require.InDelta(t, 0.5, *params.PresencePenalty, 0.0001)
		require.InDelta(t, 0.3, *params.FrequencyPenalty, 0.0001)
		// Assert int32 -> int64 conversion
		require.IsType(t, (*int64)(nil), params.MaxTokens)
		require.Equal(t, int64(1000), *params.MaxTokens)
	})

	t.Run("nil parameters remain nil", func(t *testing.T) {
		cfg := config.RunConfig{
			Name:        "run",
			ModelParams: config.MoonshotAIModelParams{},
		}
		params := buildParams(t, cfg)
		require.Nil(t, params.Temperature)
		require.Nil(t, params.TopP)
		require.Nil(t, params.PresencePenalty)
		require.Nil(t, params.FrequencyPenalty)
		require.Nil(t, params.MaxTokens)
	})
}

func TestMoonshotAICompletionHandler_ToParam(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NewTestLogger(t)

	t.Run("preserves reasoning_content from non-streaming response", func(t *testing.T) {
		handler := &moonshotAICompletionHandler{}

		// Simulate a Moonshot API response containing reasoning_content.
		responseJSON := `{
			"role": "assistant",
			"content": "The answer is 42.",
			"reasoning_content": "Let me think step by step...",
			"tool_calls": [
				{
					"id": "call_123",
					"type": "function",
					"function": {
						"name": "run_code",
						"arguments": "{\"code\": \"print(42)\"}"
					}
				}
			]
		}`
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(responseJSON), &message))

		result := handler.ToParam(ctx, logger, message)

		// Verify standard fields are preserved.
		require.NotNil(t, result.OfAssistant)
		assert.Equal(t, "The answer is 42.", result.OfAssistant.Content.OfString.Value)
		require.Len(t, result.OfAssistant.ToolCalls, 1)
		assert.Equal(t, "call_123", result.OfAssistant.ToolCalls[0].OfFunction.ID)
		assert.Equal(t, "run_code", result.OfAssistant.ToolCalls[0].OfFunction.Function.Name)

		// Verify reasoning_content is injected into the serialized JSON.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "Let me think step by step...", raw["reasoning_content"])
	})

	t.Run("handles missing reasoning_content", func(t *testing.T) {
		handler := &moonshotAICompletionHandler{}

		// Simulate a response without reasoning_content (e.g., thinking disabled).
		responseJSON := `{
			"role": "assistant",
			"content": "Hello!",
			"tool_calls": []
		}`
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(responseJSON), &message))

		result := handler.ToParam(ctx, logger, message)

		// Verify standard fields are preserved.
		require.NotNil(t, result.OfAssistant)
		assert.Equal(t, "Hello!", result.OfAssistant.Content.OfString.Value)

		// Verify no reasoning_content in serialized output.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.NotContains(t, raw, "reasoning_content")
	})

	t.Run("handles null reasoning_content", func(t *testing.T) {
		handler := &moonshotAICompletionHandler{}

		responseJSON := `{
			"role": "assistant",
			"content": "Result.",
			"reasoning_content": null
		}`
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(responseJSON), &message))

		result := handler.ToParam(ctx, logger, message)

		require.NotNil(t, result.OfAssistant)
		assert.Equal(t, "Result.", result.OfAssistant.Content.OfString.Value)

		// Null reasoning_content should not be injected.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.NotContains(t, raw, "reasoning_content")
	})

	t.Run("accumulates reasoning_content across streaming chunks", func(t *testing.T) {
		handler := &moonshotAICompletionHandler{}

		chunks := []string{
			`{"choices": [{"delta": {"reasoning_content": "Let me "}}]}`,
			`{"choices": [{"delta": {"reasoning_content": "think step "}}]}`,
			`{"choices": [{"delta": {"reasoning_content": "by step..."}}]}`,
		}
		for _, raw := range chunks {
			var chunk openai.ChatCompletionChunk
			require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
			assert.True(t, handler.AddChunk(ctx, logger, chunk))
		}

		// Streaming response: message metadata is unpopulated, reasoning comes from chunks.
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(`{"role": "assistant", "content": "The answer is 42."}`), &message))

		result := handler.ToParam(ctx, logger, message)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "Let me think step by step...", raw["reasoning_content"])
	})

	t.Run("handles streaming chunks without reasoning_content", func(t *testing.T) {
		handler := &moonshotAICompletionHandler{}

		// Regular content chunks without reasoning_content.
		chunks := []string{
			`{"choices": [{"delta": {"content": "Hello"}}]}`,
			`{"choices": [{"delta": {"content": " world"}}]}`,
		}
		for _, raw := range chunks {
			var chunk openai.ChatCompletionChunk
			require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
			assert.True(t, handler.AddChunk(ctx, logger, chunk))
		}

		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(`{"role": "assistant", "content": "Hello world"}`), &message))

		result := handler.ToParam(ctx, logger, message)

		// No reasoning_content should be injected.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.NotContains(t, raw, "reasoning_content")
	})

	t.Run("streaming reasoning takes precedence over message metadata", func(t *testing.T) {
		handler := &moonshotAICompletionHandler{}

		// Accumulate reasoning from streaming chunks.
		chunkJSON := `{"choices": [{"delta": {"reasoning_content": "Streaming thought."}}]}`
		var chunk openai.ChatCompletionChunk
		require.NoError(t, json.Unmarshal([]byte(chunkJSON), &chunk))
		assert.True(t, handler.AddChunk(ctx, logger, chunk))

		// Message also has reasoning_content in metadata (should be overridden by streaming).
		responseJSON := `{
			"role": "assistant",
			"content": "Result.",
			"reasoning_content": "Metadata thought."
		}`
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(responseJSON), &message))

		result := handler.ToParam(ctx, logger, message)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "Streaming thought.", raw["reasoning_content"])
	})

	t.Run("handles null reasoning_content in streaming chunks", func(t *testing.T) {
		handler := &moonshotAICompletionHandler{}

		chunkJSON := `{"choices": [{"delta": {"reasoning_content": null}}]}`
		var chunk openai.ChatCompletionChunk
		require.NoError(t, json.Unmarshal([]byte(chunkJSON), &chunk))
		assert.True(t, handler.AddChunk(ctx, logger, chunk))

		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(`{"role": "assistant", "content": "Result."}`), &message))

		result := handler.ToParam(ctx, logger, message)

		// Null reasoning_content should not be injected.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.NotContains(t, raw, "reasoning_content")
	})
}

func TestMoonshotAICompletionHandler_IsTerminalStopReason(t *testing.T) {
	var handler CompletionHandler = &moonshotAICompletionHandler{}

	require.True(t, handler.IsTerminalStopReason("stop"))
	require.True(t, handler.IsTerminalStopReason("length"))
	require.True(t, handler.IsTerminalStopReason("content_filter"))
	require.False(t, handler.IsTerminalStopReason("tool_calls"))
}
