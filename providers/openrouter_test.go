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
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/require"
)

func TestOpenRouterCompletionHandlerPreservesReasoning(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NewTestLogger(t)

	t.Run("non-streaming reasoning details", func(t *testing.T) {
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":"working","reasoning_details":[{"type":"reasoning.encrypted","index":0,"id":"detail-1","format":"x","data":"cipher","signature":"sig"}],"tool_calls":[{"id":"call-1","type":"function","function":{"name":"tool","arguments":"{}"}}]}`), &message))

		encoded, err := json.Marshal((&openRouterCompletionHandler{}).ToParam(ctx, logger, message))
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(encoded, &payload))
		details := payload["reasoning_details"].([]any)
		require.Len(t, details, 1)
		require.Equal(t, "cipher", details[0].(map[string]any)["data"])
		require.Len(t, payload["tool_calls"].([]any), 1)
	})

	t.Run("streaming details merge consecutive same-type deltas", func(t *testing.T) {
		details := streamReasoningDetails(t, ctx, logger,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","index":0,"id":"detail-1","text":"first "}]}}]}`,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","index":0,"text":"second"}]}}]}`,
		)
		require.Len(t, details, 1)
		require.Equal(t, "first second", details[0].(map[string]any)["text"])
		require.Equal(t, "detail-1", details[0].(map[string]any)["id"])
	})

	t.Run("streaming details keep encrypted blocks discrete", func(t *testing.T) {
		// All three share index 0; only the surrounding text blocks are streamable.
		details := streamReasoningDetails(t, ctx, logger,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","index":0,"text":"A"}]}}]}`,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.encrypted","index":0,"id":"enc-1","data":"blob"}]}}]}`,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","index":0,"text":"B"}]}}]}`,
		)
		require.Len(t, details, 3)
		require.Equal(t, "A", details[0].(map[string]any)["text"])
		require.Equal(t, "blob", details[1].(map[string]any)["data"])
		require.Equal(t, "B", details[2].(map[string]any)["text"])
	})

	t.Run("streaming details never merge across types", func(t *testing.T) {
		details := streamReasoningDetails(t, ctx, logger,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.summary","index":0,"summary":"S"},{"type":"reasoning.encrypted","index":0,"data":"blob"}]}}]}`,
		)
		require.Len(t, details, 2)
		require.Equal(t, "S", details[0].(map[string]any)["summary"])
		require.NotContains(t, details[0].(map[string]any), "data")
	})

	t.Run("streaming details merge consecutive summary deltas", func(t *testing.T) {
		details := streamReasoningDetails(t, ctx, logger,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.summary","index":0,"summary":"one "}]}}]}`,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.summary","index":0,"summary":"two"}]}}]}`,
		)
		require.Len(t, details, 1)
		require.Equal(t, "one two", details[0].(map[string]any)["summary"])
	})

	t.Run("streaming details absorb a late signature-only delta", func(t *testing.T) {
		details := streamReasoningDetails(t, ctx, logger,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","text":"thought"}]}}]}`,
			`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","signature":"sig"}]}}]}`,
		)
		require.Len(t, details, 1)
		require.Equal(t, "thought", details[0].(map[string]any)["text"])
		require.Equal(t, "sig", details[0].(map[string]any)["signature"])
	})

	t.Run("streaming preserves an explicitly empty details array", func(t *testing.T) {
		details := streamReasoningDetails(t, ctx, logger,
			`{"choices":[{"delta":{"reasoning_details":[]}}]}`,
		)
		require.NotNil(t, details)
		require.Empty(t, details)
	})

	t.Run("reasoning content alias", func(t *testing.T) {
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":"result","reasoning_content":"thought"}`), &message))
		encoded, err := json.Marshal((&openRouterCompletionHandler{}).ToParam(ctx, logger, message))
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"reasoning_content":"thought"`)
	})

	t.Run("malformed details are omitted", func(t *testing.T) {
		var message openai.ChatCompletionMessage
		require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":"result","reasoning_details":"bad"}`), &message))
		encoded, err := json.Marshal((&openRouterCompletionHandler{}).ToParam(ctx, logger, message))
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "reasoning_details")
	})
}

// streamReasoningDetails feeds raw streaming chunks through a fresh handler and
// returns the reasoning_details it would replay on the next conversation turn.
func streamReasoningDetails(t *testing.T, ctx context.Context, logger logging.Logger, rawChunks ...string) []any {
	t.Helper()
	handler := &openRouterCompletionHandler{}
	for _, raw := range rawChunks {
		var chunk openai.ChatCompletionChunk
		require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
		require.True(t, handler.AddChunk(ctx, logger, chunk))
	}

	var message openai.ChatCompletionMessage
	require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":"result"}`), &message))
	encoded, err := json.Marshal(handler.ToParam(ctx, logger, message))
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	details, ok := payload["reasoning_details"].([]any)
	require.True(t, ok, "reasoning_details missing from replayed message: %s", encoded)
	return details
}

func TestOpenRouterCompletionHandlerIsTerminalStopReason(t *testing.T) {
	tests := []struct {
		name         string
		stopReason   string
		hasToolCalls bool
		terminal     bool
	}{
		{name: "tool_calls with tool calls is non-terminal", stopReason: "tool_calls", hasToolCalls: true, terminal: false},
		{name: "stop with tool calls is non-terminal", stopReason: "stop", hasToolCalls: true, terminal: false},
		{name: "empty reason with tool calls is non-terminal", stopReason: "", hasToolCalls: true, terminal: false},
		{name: "unknown reason with tool calls is terminal by exclusion", stopReason: "unknown", hasToolCalls: true, terminal: true},
		{name: "length with tool calls is still terminal", stopReason: "length", hasToolCalls: true, terminal: true},
		{name: "content_filter with tool calls is still terminal", stopReason: "content_filter", hasToolCalls: true, terminal: true},
		{name: "stop without tool calls is terminal", stopReason: "stop", hasToolCalls: false, terminal: true},
		{name: "empty reason without tool calls is non-terminal", stopReason: "", hasToolCalls: false, terminal: false},
	}

	handler := &openRouterCompletionHandler{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.terminal, handler.IsTerminalStopReason(mockChatCompletionChoice(t, test.stopReason, test.hasToolCalls)))
		})
	}
}

func TestOpenRouterSDKExtraFieldsOverrideTypedFields(t *testing.T) {
	request := openai.ChatCompletionNewParams{MaxTokens: param.NewOpt(int64(100))}
	request.SetExtraFields(map[string]any{"max_tokens": 500})
	data, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, `{"max_tokens":500}`, string(data))
}

func TestOpenRouter_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &OpenRouter{} // nil client is sufficient to exercise parameter mapping and validation

	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "openrouter-test",
		DisableStructuredOutput: true,
		ModelParams: config.OpenRouterModelParams{
			ResponseFormat: testutils.Ptr(config.ModelResponseFormatJSONObject), // JSONObject is incompatible with DisableStructuredOutput
		},
	}
	task := config.Task{Name: "t"}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestOpenRouter_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &OpenRouter{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "openrouter-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestOpenRouterCopyToOpenAIV3Params(t *testing.T) {
	buildParams := func(t *testing.T, cfg config.RunConfig) openAIV3ModelParams {
		params := openAIV3ModelParams{
			ResponseFormat: nil,
			ExtraFields:    map[string]any{},
		}
		if cfg.ModelParams == nil {
			return params
		}
		openRouterParams, ok := cfg.ModelParams.(config.OpenRouterModelParams)
		require.True(t, ok)

		provider := &OpenRouter{}
		provider.copyToOpenAIV3Params(openRouterParams, &params)
		return params
	}

	t.Run("user extra fields are copied to ExtraFields", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenRouterModelParams{
				Extra: map[string]any{
					"custom_field": "custom_value",
					"provider": map[string]any{
						"order": []any{"some-provider"},
					},
				},
			},
		}
		params := buildParams(t, cfg)
		require.Equal(t, "custom_value", params.ExtraFields["custom_field"])
		provider, ok := params.ExtraFields["provider"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, []any{"some-provider"}, provider["order"])
	})

	t.Run("OpenRouter-specific parameters to extra fields", func(t *testing.T) {
		topK := int32(40)
		minP := float32(0.05)
		topA := float32(0.8)
		repPenalty := float32(1.1)
		parallelToolCalls := true

		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenRouterModelParams{
				TopK:              &topK,
				MinP:              &minP,
				TopA:              &topA,
				RepetitionPenalty: &repPenalty,
				ParallelToolCalls: &parallelToolCalls,
			},
		}

		params := buildParams(t, cfg)
		require.Equal(t, int32(40), params.ExtraFields["top_k"])
		require.InDelta(t, float32(0.05), params.ExtraFields["min_p"], 0.0001)
		require.InDelta(t, float32(0.8), params.ExtraFields["top_a"], 0.0001)
		require.InDelta(t, float32(1.1), params.ExtraFields["repetition_penalty"], 0.0001)
		require.Equal(t, true, params.ExtraFields["parallel_tool_calls"])
	})

	t.Run("reasoning and completion limit parameters", func(t *testing.T) {
		params := buildParams(t, config.RunConfig{ModelParams: config.OpenRouterModelParams{
			ReasoningEffort:     testutils.Ptr("xhigh"),
			MaxCompletionTokens: testutils.Ptr(int32(65536)),
		}})
		require.Equal(t, "xhigh", *params.ReasoningEffort)
		require.Equal(t, int64(65536), *params.MaxCompletionTokens)
	})

	t.Run("typed reasoning effort does not clobber raw reasoning object", func(t *testing.T) {
		params := buildParams(t, config.RunConfig{ModelParams: config.OpenRouterModelParams{
			ReasoningEffort: testutils.Ptr("high"),
			Extra:           map[string]any{"reasoning": map[string]any{"exclude": true}},
		}})
		require.Equal(t, "high", *params.ReasoningEffort)
		require.Equal(t, map[string]any{"exclude": true}, params.ExtraFields["reasoning"])
	})

	t.Run("numeric parameters with type conversion", func(t *testing.T) {
		temp := float32(0.7)
		topP := float32(0.9)
		presencePenalty := float32(0.5)
		frequencyPenalty := float32(0.3)
		maxTokens := int32(1000)

		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenRouterModelParams{
				Temperature:      &temp,
				TopP:             &topP,
				PresencePenalty:  &presencePenalty,
				FrequencyPenalty: &frequencyPenalty,
				MaxTokens:        &maxTokens,
			},
		}

		params := buildParams(t, cfg)

		// Assert float32 -> float64 conversion with type check
		require.IsType(t, (*float64)(nil), params.Temperature)
		require.IsType(t, (*float64)(nil), params.TopP)
		require.IsType(t, (*float64)(nil), params.PresencePenalty)
		require.IsType(t, (*float64)(nil), params.FrequencyPenalty)
		require.InDelta(t, 0.7, *params.Temperature, 0.0001)
		require.InDelta(t, 0.9, *params.TopP, 0.0001)
		require.InDelta(t, 0.5, *params.PresencePenalty, 0.0001)
		require.InDelta(t, 0.3, *params.FrequencyPenalty, 0.0001)

		// Assert int32 -> int64 conversion with type check
		require.IsType(t, (*int64)(nil), params.MaxTokens)
		require.Equal(t, int64(1000), *params.MaxTokens)
	})

	t.Run("parameters copied without type conversion", func(t *testing.T) {
		seed := int64(42)
		verbosity := "verbose"

		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenRouterModelParams{
				Seed:      &seed,
				Verbosity: &verbosity,
			},
		}

		params := buildParams(t, cfg)
		require.Equal(t, int64(42), *params.Seed)
		require.Equal(t, "verbose", *params.Verbosity)
	})

	t.Run("ResponseFormat mapping", func(t *testing.T) {
		tests := []struct {
			name   string
			format config.ModelResponseFormat
			want   ResponseFormat
		}{
			{"Text", config.ModelResponseFormatText, ResponseFormatText},
			{"JSONObject", config.ModelResponseFormatJSONObject, ResponseFormatJSONObject},
			{"JSONSchema", config.ModelResponseFormatJSONSchema, ResponseFormatJSONSchema},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := config.RunConfig{
					Name: "run",
					ModelParams: config.OpenRouterModelParams{
						ResponseFormat: &tt.format,
					},
				}

				params := buildParams(t, cfg)
				require.NotNil(t, params.ResponseFormat)
				require.Equal(t, tt.want, *params.ResponseFormat)
			})
		}
	})

	t.Run("nil parameters remain nil", func(t *testing.T) {
		cfg := config.RunConfig{
			Name:        "run",
			ModelParams: config.OpenRouterModelParams{},
		}

		params := buildParams(t, cfg)
		require.Nil(t, params.Temperature)
		require.Nil(t, params.TopP)
		require.Nil(t, params.PresencePenalty)
		require.Nil(t, params.FrequencyPenalty)
		require.Nil(t, params.MaxTokens)
		require.Nil(t, params.Seed)
		require.Nil(t, params.Verbosity)
		require.Nil(t, params.ReasoningEffort)
		require.Nil(t, params.ResponseFormat)
		require.Empty(t, params.ServerTools)
	})

	t.Run("server tools translated with parameters", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenRouterModelParams{
				ServerTools: []config.ServerToolConfig{
					{
						Type: "openrouter:fusion",
						Parameters: map[string]any{
							"analysis_models": []any{
								"~anthropic/claude-opus-latest",
								"~openai/gpt-latest",
								"~google/gemini-pro-latest",
							},
							"model":                 "~anthropic/claude-opus-latest",
							"reasoning":             map[string]any{"effort": "xhigh"},
							"max_completion_tokens": 65536,
							"max_tool_calls":        16,
							"temperature":           0,
						},
					},
				},
			},
		}

		params := buildParams(t, cfg)
		require.Len(t, params.ServerTools, 1)
		require.Equal(t, "openrouter:fusion", params.ServerTools[0].Type)
		p := params.ServerTools[0].Parameters
		require.NotNil(t, p)
		require.Equal(t, []any{"~anthropic/claude-opus-latest", "~openai/gpt-latest", "~google/gemini-pro-latest"}, p["analysis_models"])
		require.Equal(t, "~anthropic/claude-opus-latest", p["model"])
		require.Equal(t, map[string]any{"effort": "xhigh"}, p["reasoning"])
		require.Equal(t, 65536, p["max_completion_tokens"])
		require.Equal(t, 16, p["max_tool_calls"])
		require.Equal(t, 0, p["temperature"])
	})

	t.Run("server tool without parameters omits parameters key", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenRouterModelParams{
				ServerTools: []config.ServerToolConfig{
					{Type: "openrouter:fusion"},
				},
			},
		}

		params := buildParams(t, cfg)
		require.Len(t, params.ServerTools, 1)
		require.Equal(t, "openrouter:fusion", params.ServerTools[0].Type)
		require.Empty(t, params.ServerTools[0].Parameters)
	})
}
