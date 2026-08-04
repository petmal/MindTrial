// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestAlibaba_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &Alibaba{} // nil client is sufficient to exercise parameter mapping and validation

	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "qwen-test",
		DisableStructuredOutput: true,
		// Alibaba does not set ResponseFormat when DisableStructuredOutput is true, so no incompatibility
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

func TestAlibaba_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &Alibaba{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "qwen-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestAlibabaCopyToOpenAIV3Params(t *testing.T) {
	buildParams := func(t *testing.T, cfg config.RunConfig) openAIV3ModelParams {
		params := openAIV3ModelParams{ExtraFields: map[string]any{}}
		if cfg.ModelParams == nil {
			return params
		}
		alibabaParams, ok := cfg.ModelParams.(config.AlibabaModelParams)
		require.True(t, ok)
		provider := &Alibaba{}
		provider.copyToOpenAIV3Params(alibabaParams, &params)
		return params
	}

	t.Run("DisableLegacyJsonMode disables ResponseFormat", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.AlibabaModelParams{
				DisableLegacyJsonMode: utils.Ptr(true),
			},
		}
		params := buildParams(t, cfg)
		require.Nil(t, params.ResponseFormat)
	})

	t.Run("TextResponseFormat sets ResponseFormat to text", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.AlibabaModelParams{
				TextResponseFormat: true,
			},
		}
		params := buildParams(t, cfg)
		require.NotNil(t, params.ResponseFormat)
		require.Equal(t, ResponseFormatText, *params.ResponseFormat)
	})

	t.Run("numeric parameters with type conversion", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.AlibabaModelParams{
				Temperature:      utils.Ptr(float32(0.7)),
				TopP:             utils.Ptr(float32(0.9)),
				PresencePenalty:  utils.Ptr(float32(0.5)),
				FrequencyPenalty: utils.Ptr(float32(0.3)),
				MaxTokens:        utils.Ptr(int32(1000)),
				Seed:             utils.Ptr(uint32(42)),
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
		// Assert uint32 -> int64 conversion
		require.IsType(t, (*int64)(nil), params.Seed)
		require.Equal(t, int64(42), *params.Seed)
	})

	t.Run("TextResponseFormat takes precedence over DisableLegacyJsonMode", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.AlibabaModelParams{
				DisableLegacyJsonMode: utils.Ptr(true),
				TextResponseFormat:    true,
			},
		}
		params := buildParams(t, cfg)
		require.NotNil(t, params.ResponseFormat)
		require.Equal(t, ResponseFormatText, *params.ResponseFormat)
	})

	t.Run("Stream enables streaming mode", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.AlibabaModelParams{
				Stream: true,
			},
		}
		params := buildParams(t, cfg)
		require.NotNil(t, params.Stream)
		require.True(t, *params.Stream)
	})

	t.Run("Stream disabled by default", func(t *testing.T) {
		cfg := config.RunConfig{
			Name: "run",
			ModelParams: config.AlibabaModelParams{
				Stream: false,
			},
		}
		params := buildParams(t, cfg)
		require.Nil(t, params.Stream) // false bool value should not set the pointer
	})

	t.Run("thinking parameters copy through", func(t *testing.T) {
		modelParams := config.AlibabaModelParams{
			EnableThinking:   utils.Ptr(false),
			PreserveThinking: utils.Ptr(true),
			ThinkingBudget:   utils.Ptr(int32(8192)),
		}
		params := buildParams(t, config.RunConfig{ModelParams: modelParams})
		require.Equal(t, false, params.ExtraFields["enable_thinking"])
		require.Equal(t, true, params.ExtraFields["preserve_thinking"])
		require.Equal(t, int32(8192), params.ExtraFields["thinking_budget"])
	})

	t.Run("nil parameters remain nil", func(t *testing.T) {
		cfg := config.RunConfig{
			Name:        "run",
			ModelParams: config.AlibabaModelParams{},
		}
		params := buildParams(t, cfg)
		require.Nil(t, params.ResponseFormat)
		require.Nil(t, params.Temperature)
		require.Nil(t, params.TopP)
		require.Nil(t, params.PresencePenalty)
		require.Nil(t, params.FrequencyPenalty)
		require.Nil(t, params.MaxTokens)
		require.Nil(t, params.Seed)
		require.Nil(t, params.Stream)
	})
}

func TestAlibaba_CompletionHandlerSelection(t *testing.T) {
	provider := NewAlibaba(config.AlibabaClientConfig{APIKey: "test-key"}, nil)

	t.Run("PreserveThinking enabled selects the reasoning-preserving handler", func(t *testing.T) {
		handler := provider.openaiProvider.NewCompletionHandler(alibabaCompletionHandlerArgs{PreserveThinking: true})
		require.IsType(t, &alibabaCompletionHandler{}, handler)
		require.IsType(t, &moonshotAICompletionHandler{}, handler.(*alibabaCompletionHandler).CompletionHandler)
	})

	t.Run("PreserveThinking disabled keeps the default handler", func(t *testing.T) {
		handler := provider.openaiProvider.NewCompletionHandler(alibabaCompletionHandlerArgs{PreserveThinking: false})
		require.IsType(t, &alibabaCompletionHandler{}, handler)
		require.IsType(t, &defaultCompletionHandler{}, handler.(*alibabaCompletionHandler).CompletionHandler)
	})

	t.Run("unexpected args type keeps the default handler", func(t *testing.T) {
		handler := provider.openaiProvider.NewCompletionHandler(nil)
		require.IsType(t, &alibabaCompletionHandler{}, handler)
		require.IsType(t, &defaultCompletionHandler{}, handler.(*alibabaCompletionHandler).CompletionHandler)
	})
}

func TestAlibabaCompletionHandler_InputCacheTokens(t *testing.T) {
	tests := []struct {
		name      string
		usageJSON string
		write     *int64
		read      *int64
	}{
		{name: "omitted", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}`},
		{name: "read only, no explicit cache creation", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":7}}`, read: testutils.Ptr(int64(7))},
		{name: "read and write reported", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":4,"cache_creation_input_tokens":6,"cache_creation":{"ephemeral_5m_input_tokens":6}}}`, write: testutils.Ptr(int64(6)), read: testutils.Ptr(int64(4))},
		{name: "cache_creation object without the sibling field is absent", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":4,"cache_creation":{"ephemeral_5m_input_tokens":6}}}`, read: testutils.Ptr(int64(4))},
		{name: "top-level cached_tokens is ignored", usageJSON: `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cached_tokens":9,"prompt_tokens_details":{"cached_tokens":4}}`, read: testutils.Ptr(int64(4))},
	}

	// A top-level cached_tokens must be ignored: the wrapped Moonshot handler would
	// otherwise report it, making cache accounting depend on preserve-thinking.
	wrapped := map[string]CompletionHandler{
		"default handler":  &defaultCompletionHandler{},
		"moonshot handler": &moonshotAICompletionHandler{},
	}
	for wrappedName, base := range wrapped {
		handler := &alibabaCompletionHandler{CompletionHandler: base}
		for _, test := range tests {
			t.Run(fmt.Sprintf("%s/%s", wrappedName, test.name), func(t *testing.T) {
				var usage openai.CompletionUsage
				require.NoError(t, json.Unmarshal([]byte(test.usageJSON), &usage))
				write, read := handler.InputCacheTokens(usage)
				require.Equal(t, test.write, write)
				require.Equal(t, test.read, read)
			})
		}
	}
}
