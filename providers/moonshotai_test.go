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
