// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3/option"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
)

// NewAlibaba creates a new Alibaba provider instance with the given configuration.
func NewAlibaba(cfg config.AlibabaClientConfig, availableTools []config.ToolConfig) *Alibaba {
	openAIV3Opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.GetEndpoint()),
	}
	openaiProvider := newOpenAIV3Provider(availableTools, openAIV3Opts...)

	return &Alibaba{openaiProvider: openaiProvider}
}

// Alibaba implements the Provider interface for Alibaba models.
// The Qwen models from Alibaba Cloud support OpenAI-compatible interfaces
// allowing them to be used with the existing OpenAI provider implementation.
type Alibaba struct {
	openaiProvider *openAIV3Provider
}

func (a Alibaba) Name() string {
	return config.ALIBABA
}

func (a *Alibaba) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	openAIV3Params := openAIV3ModelParams{}

	// Alibaba Qwen models prefer legacy-json-schema instructions by default
	// unless structured output is explicitly disabled.
	if !cfg.DisableStructuredOutput {
		openAIV3Params.ResponseFormat = ResponseFormatLegacySchema.Ptr()
	}

	if cfg.ModelParams != nil {
		if alibabaParams, ok := cfg.ModelParams.(config.AlibabaModelParams); ok {
			a.copyToOpenAIV3Params(alibabaParams, &openAIV3Params)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}
	cfg.ModelParams = openAIV3Params

	return a.openaiProvider.Run(ctx, logger, cfg, task)
}

func (a *Alibaba) Close(ctx context.Context) error {
	return a.openaiProvider.Close(ctx) // delegate to the OpenAI provider
}

// copyToOpenAIV3Params copies relevant fields from AlibabaModelParams to openAIV3ModelParams.
func (a *Alibaba) copyToOpenAIV3Params(alibabaParams config.AlibabaModelParams, openAIV3Params *openAIV3ModelParams) {
	if alibabaParams.DisableLegacyJsonMode != nil && *alibabaParams.DisableLegacyJsonMode {
		openAIV3Params.ResponseFormat = nil // disable legacy mode; use Open AI default instead
	}
	if alibabaParams.TextResponseFormat {
		openAIV3Params.ResponseFormat = ResponseFormatText.Ptr()
	}
	if alibabaParams.Stream {
		openAIV3Params.Stream = utils.Ptr(true)
	}
	if alibabaParams.Temperature != nil {
		openAIV3Params.Temperature = utils.Ptr(float64(*alibabaParams.Temperature))
	}
	if alibabaParams.TopP != nil {
		openAIV3Params.TopP = utils.Ptr(float64(*alibabaParams.TopP))
	}
	if alibabaParams.MaxTokens != nil {
		openAIV3Params.MaxTokens = utils.Ptr(int64(*alibabaParams.MaxTokens))
	}
	if alibabaParams.PresencePenalty != nil {
		openAIV3Params.PresencePenalty = utils.Ptr(float64(*alibabaParams.PresencePenalty))
	}
	if alibabaParams.FrequencyPenalty != nil {
		openAIV3Params.FrequencyPenalty = utils.Ptr(float64(*alibabaParams.FrequencyPenalty))
	}
	if alibabaParams.Seed != nil {
		openAIV3Params.Seed = utils.ConvertIntPtr[uint32, int64](alibabaParams.Seed)
	}
}
