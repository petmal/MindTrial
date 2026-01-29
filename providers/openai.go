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

// NewOpenAI creates a new OpenAI provider instance with the given configuration.
func NewOpenAI(cfg config.OpenAIClientConfig, availableTools []config.ToolConfig) *OpenAI {
	openaiProvider := newOpenAIV3Provider(availableTools, option.WithAPIKey(cfg.APIKey))
	return &OpenAI{openaiProvider: openaiProvider}
}

// OpenAI implements the Provider interface for OpenAI generative models.
type OpenAI struct {
	openaiProvider *openAIV3Provider
}

func (o OpenAI) Name() string {
	return config.OPENAI
}

func (o *OpenAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	openAIV3Params := openAIV3ModelParams{}

	if cfg.ModelParams != nil {
		if openAIModelParams, ok := cfg.ModelParams.(config.OpenAIModelParams); ok {
			o.copyToOpenAIV3Params(openAIModelParams, &openAIV3Params)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	cfg.ModelParams = openAIV3Params
	return o.openaiProvider.Run(ctx, logger, cfg, task)
}

func (o *OpenAI) Close(ctx context.Context) error {
	return o.openaiProvider.Close(ctx)
}

// copyToOpenAIV3Params copies relevant fields from OpenAIModelParams to openAIV3ModelParams.
func (o *OpenAI) copyToOpenAIV3Params(openAIModelParams config.OpenAIModelParams, openAIV3Params *openAIV3ModelParams) {
	if openAIModelParams.TextResponseFormat {
		openAIV3Params.ResponseFormat = ResponseFormatText.Ptr()
	}

	openAIV3Params.ReasoningEffort = openAIModelParams.ReasoningEffort
	openAIV3Params.Verbosity = openAIModelParams.Verbosity
	if openAIModelParams.Temperature != nil {
		openAIV3Params.Temperature = utils.Ptr(float64(*openAIModelParams.Temperature))
	}
	if openAIModelParams.TopP != nil {
		openAIV3Params.TopP = utils.Ptr(float64(*openAIModelParams.TopP))
	}
	if openAIModelParams.MaxCompletionTokens != nil {
		openAIV3Params.MaxCompletionTokens = utils.Ptr(int64(*openAIModelParams.MaxCompletionTokens))
	}
	if openAIModelParams.MaxTokens != nil {
		openAIV3Params.MaxTokens = utils.Ptr(int64(*openAIModelParams.MaxTokens))
	}
	if openAIModelParams.PresencePenalty != nil {
		openAIV3Params.PresencePenalty = utils.Ptr(float64(*openAIModelParams.PresencePenalty))
	}
	if openAIModelParams.FrequencyPenalty != nil {
		openAIV3Params.FrequencyPenalty = utils.Ptr(float64(*openAIModelParams.FrequencyPenalty))
	}
	openAIV3Params.Seed = openAIModelParams.Seed
}
