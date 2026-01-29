// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestOpenAI_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &OpenAI{} // nil client is sufficient to exercise parameter mapping and validation

	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "gpt-test",
		DisableStructuredOutput: true,
		// When DisableStructuredOutput is true, OpenAI defaults ResponseFormat to text, so no incompatibility
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

func TestOpenAI_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &OpenAI{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "gpt-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestOpenAICopyToOpenAIV3Params(t *testing.T) {
	buildParams := func(t *testing.T, cfg config.RunConfig) openAIV3ModelParams {
		params := openAIV3ModelParams{}
		if cfg.ModelParams == nil {
			return params
		}
		openAIParams, ok := cfg.ModelParams.(config.OpenAIModelParams)
		require.True(t, ok)
		provider := &OpenAI{}
		provider.copyToOpenAIV3Params(openAIParams, &params)
		return params
	}

	t.Run("TextResponseFormat sets ResponseFormat to text", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenAIModelParams{
				TextResponseFormat: true,
			},
		}
		params := buildParams(t, cfg)
		require.NotNil(t, params.ResponseFormat)
		require.Equal(t, ResponseFormatText, *params.ResponseFormat)
	})

	t.Run("ReasoningEffort and Verbosity copied", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenAIModelParams{
				ReasoningEffort: utils.Ptr("high"),
				Verbosity:       utils.Ptr("medium"),
			},
		}
		params := buildParams(t, cfg)
		require.Equal(t, "high", *params.ReasoningEffort)
		require.Equal(t, "medium", *params.Verbosity)
	})

	t.Run("numeric parameters with type conversion", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenAIModelParams{
				Temperature:         utils.Ptr(float32(0.7)),
				TopP:                utils.Ptr(float32(0.9)),
				PresencePenalty:     utils.Ptr(float32(0.5)),
				FrequencyPenalty:    utils.Ptr(float32(0.3)),
				MaxCompletionTokens: utils.Ptr(int32(1000)),
				MaxTokens:           utils.Ptr(int32(2000)),
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
		require.IsType(t, (*int64)(nil), params.MaxCompletionTokens)
		require.IsType(t, (*int64)(nil), params.MaxTokens)
		require.Equal(t, int64(1000), *params.MaxCompletionTokens)
		require.Equal(t, int64(2000), *params.MaxTokens)
	})

	t.Run("Seed copied without type conversion", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.OpenAIModelParams{
				Seed: utils.Ptr(int64(42)),
			},
		}
		params := buildParams(t, cfg)
		require.Equal(t, int64(42), *params.Seed)
	})

	t.Run("nil parameters remain nil", func(t *testing.T) {
		cfg := config.RunConfig{
			Name:        "run",
			ModelParams: config.OpenAIModelParams{},
		}
		params := buildParams(t, cfg)
		require.Nil(t, params.ResponseFormat)
		require.Nil(t, params.ReasoningEffort)
		require.Nil(t, params.Verbosity)
		require.Nil(t, params.Temperature)
		require.Nil(t, params.TopP)
		require.Nil(t, params.PresencePenalty)
		require.Nil(t, params.FrequencyPenalty)
		require.Nil(t, params.MaxCompletionTokens)
		require.Nil(t, params.MaxTokens)
		require.Nil(t, params.Seed)
	})
}
