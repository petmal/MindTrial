// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/option"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
)

// NewOpenAI creates a new OpenAI provider instance with the given configuration.
func NewOpenAI(cfg config.OpenAIClientConfig, availableTools []config.ToolConfig) *OpenAI {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	return &OpenAI{
		completionProvider: newOpenAIV3Provider(availableTools, opts...),
		responsesProvider:  newOpenAIResponsesProvider(availableTools, opts...),
	}
}

// OpenAI implements the Provider interface for OpenAI generative models.
type OpenAI struct {
	completionProvider *openAIV3Provider
	responsesProvider  *openAIResponsesProvider
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
	if useChatCompletionsAPI(cfg.Model) {
		logger.Message(ctx, logging.LevelInfo, "using Chat Completions API")
		return o.completionProvider.Run(ctx, logger, cfg, task)
	}
	logger.Message(ctx, logging.LevelInfo, "using Responses API")
	return o.responsesProvider.Run(ctx, logger, cfg, task)
}

func (o *OpenAI) Close(ctx context.Context) error {
	return errors.Join(
		o.completionProvider.Close(ctx),
		o.responsesProvider.Close(ctx),
	)
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

// chatCompletionModelPrefixes lists model name prefixes that should be routed to
// the legacy Chat Completions API. All other models default to the Responses API.
var chatCompletionModelPrefixes = []string{
	"gpt-3", // gpt-3.5-turbo (sunset Sep 2026)
	"gpt-4", // gpt-4o, gpt-4o-mini, gpt-4.1, gpt-4.1-mini, gpt-4.1-nano
	"o1",    // o1 reasoning model
	"o3",    // o3 reasoning model
	"o4",    // o4-mini reasoning model
}

// useChatCompletionsAPI reports whether the given model should use the legacy
// Chat Completions API instead of the Responses API. Known pre-GPT-5 model
// families are routed to Chat Completions; all other models (including future
// ones) default to the Responses API.
func useChatCompletionsAPI(model string) bool {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	return slices.ContainsFunc(chatCompletionModelPrefixes, func(prefix string) bool {
		return strings.HasPrefix(normalizedModel, prefix)
	})
}
