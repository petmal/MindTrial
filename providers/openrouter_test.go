// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/stretchr/testify/require"
)

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
		require.Nil(t, params.ResponseFormat)
	})
}
