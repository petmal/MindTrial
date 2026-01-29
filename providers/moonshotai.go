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

// NewMoonshotAI creates a new Moonshot AI provider instance with the given configuration.
func NewMoonshotAI(cfg config.MoonshotAIClientConfig, availableTools []config.ToolConfig) *MoonshotAI {
	openAIV3Opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.GetEndpoint()),
	}
	openaiProvider := newOpenAIV3Provider(availableTools, openAIV3Opts...)

	return &MoonshotAI{openaiProvider: openaiProvider}
}

// MoonshotAI implements the Provider interface for Moonshot AI models.
// The Kimi models from Moonshot AI support OpenAI-compatible interfaces
// allowing them to be used with the existing OpenAI provider implementation.
type MoonshotAI struct {
	openaiProvider *openAIV3Provider
}

func (m MoonshotAI) Name() string {
	return config.MOONSHOTAI
}

func (m *MoonshotAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	openAIV3Params := openAIV3ModelParams{}

	// Kimi models from MoonshotAI prefer json-object response mode by default
	// unless structured output is disabled.
	if !cfg.DisableStructuredOutput {
		openAIV3Params.ResponseFormat = ResponseFormatJSONObject.Ptr()
	}

	if cfg.ModelParams != nil {
		if moonshotAIParams, ok := cfg.ModelParams.(config.MoonshotAIModelParams); ok {
			m.copyToOpenAIV3Params(moonshotAIParams, &openAIV3Params)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}
	cfg.ModelParams = openAIV3Params

	return m.openaiProvider.Run(ctx, logger, cfg, task)
}

func (m *MoonshotAI) Close(ctx context.Context) error {
	return m.openaiProvider.Close(ctx) // delegate to the OpenAI provider
}

// copyToOpenAIV3Params copies relevant fields from MoonshotAIModelParams to openAIV3ModelParams.
func (m *MoonshotAI) copyToOpenAIV3Params(moonshotAIParams config.MoonshotAIModelParams, openAIV3Params *openAIV3ModelParams) {
	if moonshotAIParams.Temperature != nil {
		openAIV3Params.Temperature = utils.Ptr(float64(*moonshotAIParams.Temperature))
	}
	if moonshotAIParams.TopP != nil {
		openAIV3Params.TopP = utils.Ptr(float64(*moonshotAIParams.TopP))
	}
	if moonshotAIParams.MaxTokens != nil {
		openAIV3Params.MaxTokens = utils.Ptr(int64(*moonshotAIParams.MaxTokens))
	}
	if moonshotAIParams.PresencePenalty != nil {
		openAIV3Params.PresencePenalty = utils.Ptr(float64(*moonshotAIParams.PresencePenalty))
	}
	if moonshotAIParams.FrequencyPenalty != nil {
		openAIV3Params.FrequencyPenalty = utils.Ptr(float64(*moonshotAIParams.FrequencyPenalty))
	}
}
