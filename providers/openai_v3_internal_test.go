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
	"github.com/CircleCI-Research/evalbench/config"
	"github.com/CircleCI-Research/evalbench/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIV3_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAIV3Provider{}
	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "gpt-test",
		DisableStructuredOutput: true,
		ModelParams: openAIV3ModelParams{
			ResponseFormat: ResponseFormatJSONObject.Ptr(),
		},
	}
	_, err := p.Run(context.Background(), logger, runCfg, config.Task{Name: "t"})
	require.ErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestOpenAIV3_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAIV3Provider{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "gpt-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
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

		assert.True(t, handler.IsTerminalStopReason("stop"))
		assert.True(t, handler.IsTerminalStopReason("length"))
		assert.True(t, handler.IsTerminalStopReason("content_filter"))
		assert.False(t, handler.IsTerminalStopReason("tool_calls"))
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
