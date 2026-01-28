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
	openai "github.com/sashabaranov/go-openai"
)

// NewMoonshotAI creates a new Moonshot AI provider instance with the given configuration.
func NewMoonshotAI(cfg config.MoonshotAIClientConfig, availableTools []config.ToolConfig) *MoonshotAI {
	// Create OpenAI client with Moonshot AI-specific configuration.
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.GetEndpoint()

	return &MoonshotAI{
		openaiProvider: &OpenAI{
			client:         openai.NewClientWithConfig(clientConfig),
			availableTools: availableTools,
		},
	}
}

// MoonshotAI implements the Provider interface for Moonshot AI models.
// The Kimi models from Moonshot AI support OpenAI-compatible interfaces
// allowing them to be used with the existing OpenAI provider implementation.
type MoonshotAI struct {
	openaiProvider *OpenAI
}

func (m MoonshotAI) Name() string {
	return config.MOONSHOTAI
}

func (m *MoonshotAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	// Initialize model parameters for OpenAI provider.
	// Kimi models only support json_object mode and require explicit JSON format guidance in the prompt.
	openAIParams := config.OpenAIModelParams{}

	// Set json_object mode by default, unless structured output is disabled
	if !cfg.DisableStructuredOutput {
		openAIParams.LegacyJsonMode = config.LegacyJsonObject.Ptr()
	}

	if cfg.ModelParams != nil {
		if moonshotAIParams, ok := cfg.ModelParams.(config.MoonshotAIModelParams); ok {
			m.copyToOpenAIParams(moonshotAIParams, &openAIParams)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}
	cfg.ModelParams = openAIParams

	return m.openaiProvider.Run(ctx, logger, cfg, task) // delegate to the OpenAI provider
}

func (m *MoonshotAI) Close(ctx context.Context) error {
	return m.openaiProvider.Close(ctx) // delegate to the OpenAI provider
}

// copyToOpenAIParams copies relevant fields from MoonshotAIModelParams to OpenAIModelParams.
func (m *MoonshotAI) copyToOpenAIParams(moonshotAIParams config.MoonshotAIModelParams, openAIParams *config.OpenAIModelParams) {
	openAIParams.Temperature = moonshotAIParams.Temperature
	openAIParams.TopP = moonshotAIParams.TopP
	openAIParams.MaxTokens = moonshotAIParams.MaxTokens
	openAIParams.PresencePenalty = moonshotAIParams.PresencePenalty
	openAIParams.FrequencyPenalty = moonshotAIParams.FrequencyPenalty
}
