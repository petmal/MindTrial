// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"fmt"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	openai "github.com/sashabaranov/go-openai"
)

// NewAlibaba creates a new Alibaba provider instance with the given configuration.
func NewAlibaba(cfg config.AlibabaClientConfig, availableTools []config.ToolConfig) *Alibaba {
	// Create OpenAI client with Alibaba-specific configuration.
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.GetEndpoint()

	return &Alibaba{
		openaiProvider: &OpenAI{
			client:         openai.NewClientWithConfig(clientConfig),
			availableTools: availableTools,
		},
	}
}

// Alibaba implements the Provider interface for Alibaba models.
// The Qwen models from Alibaba Cloud support OpenAI-compatible interfaces
// allowing them to be used with the existing OpenAI provider implementation.
type Alibaba struct {
	openaiProvider *OpenAI
}

func (a Alibaba) Name() string {
	return config.ALIBABA
}

func (a *Alibaba) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	// Initialize model parameters for OpenAI provider.
	openAIParams := config.OpenAIModelParams{}

	// Set legacy schema mode by default for better interoperability, unless structured output is disabled
	if !cfg.DisableStructuredOutput {
		openAIParams.LegacyJsonMode = config.LegacyJsonSchema.Ptr()
	}

	if cfg.ModelParams != nil {
		if alibabaParams, ok := cfg.ModelParams.(config.AlibabaModelParams); ok {
			a.copyToOpenAIParams(alibabaParams, &openAIParams)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}
	cfg.ModelParams = openAIParams

	return a.openaiProvider.Run(ctx, logger, cfg, task) // delegate to the OpenAI provider
}

func (a *Alibaba) Close(ctx context.Context) error {
	return a.openaiProvider.Close(ctx) // delegate to the OpenAI provider
}

// copyToOpenAIParams copies relevant fields from AlibabaModelParams to OpenAIModelParams.
func (a *Alibaba) copyToOpenAIParams(alibabaParams config.AlibabaModelParams, openAIParams *config.OpenAIModelParams) {
	openAIParams.TextResponseFormat = alibabaParams.TextResponseFormat
	openAIParams.Temperature = alibabaParams.Temperature
	openAIParams.TopP = alibabaParams.TopP
	openAIParams.MaxTokens = alibabaParams.MaxTokens
	openAIParams.PresencePenalty = alibabaParams.PresencePenalty
	openAIParams.FrequencyPenalty = alibabaParams.FrequencyPenalty
	openAIParams.Seed = utils.ConvertIntPtr[uint32, int64](alibabaParams.Seed)

	// Many Qwen models currently require the prompt to mention JSON explicitly
	// when `response_format` is `json_object`.
	// The legacy mode forces inclusion of the default response format instruction.
	// Disable legacy mode only if explicitly set.
	if alibabaParams.DisableLegacyJsonMode != nil && *alibabaParams.DisableLegacyJsonMode {
		openAIParams.LegacyJsonMode = nil
	}
}
