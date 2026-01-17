// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"fmt"
	"maps"

	"github.com/openai/openai-go/v3/option"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/version"
)

// NewOpenRouter creates a new OpenRouter provider instance with the given configuration.
// Injects OpenRouter attribution headers derived from MindTrial metadata into every request.
func NewOpenRouter(cfg config.OpenRouterClientConfig, availableTools []config.ToolConfig) *OpenRouter {
	source := version.GetSource()
	appTitle := version.Name

	openAIV3Opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.GetEndpoint()),
	}
	if source != "" {
		openAIV3Opts = append(openAIV3Opts, option.WithHeader("HTTP-Referer", fmt.Sprintf("https://%s", source)))
	}
	if appTitle != "" {
		openAIV3Opts = append(openAIV3Opts, option.WithHeader("X-Title", appTitle))
	}

	openaiProvider := newOpenAIV3Provider(availableTools, openAIV3Opts...)

	return &OpenRouter{openaiProvider: openaiProvider}
}

// OpenRouter implements the Provider interface for models reachable via OpenRouter.
type OpenRouter struct {
	openaiProvider *openAIV3Provider
}

func (o OpenRouter) Name() string {
	return config.OPENROUTER
}

func (o *OpenRouter) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	openAIV3Params := openAIV3ModelParams{
		ResponseFormat: ResponseFormatJSONSchema.Ptr(),
		ExtraFields:    map[string]any{},
	}

	if cfg.ModelParams != nil {
		if openRouterParams, ok := cfg.ModelParams.(config.OpenRouterModelParams); ok {
			o.copyToOpenAIV3Params(openRouterParams, &openAIV3Params)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	cfg.ModelParams = openAIV3Params
	return o.openaiProvider.Run(ctx, logger, cfg, task)
}

func (o *OpenRouter) Close(ctx context.Context) error {
	return o.openaiProvider.Close(ctx)
}

func (o *OpenRouter) copyToOpenAIV3Params(openRouterParams config.OpenRouterModelParams, openAIV3Params *openAIV3ModelParams) {
	// Copy user's extra fields. If user provides "provider", it replaces the defaults entirely.
	maps.Copy(openAIV3Params.ExtraFields, openRouterParams.Extra)

	if openRouterParams.TopK != nil {
		openAIV3Params.ExtraFields["top_k"] = *openRouterParams.TopK
	}
	if openRouterParams.MinP != nil {
		openAIV3Params.ExtraFields["min_p"] = *openRouterParams.MinP
	}
	if openRouterParams.TopA != nil {
		openAIV3Params.ExtraFields["top_a"] = *openRouterParams.TopA
	}
	if openRouterParams.RepetitionPenalty != nil {
		openAIV3Params.ExtraFields["repetition_penalty"] = *openRouterParams.RepetitionPenalty
	}
	if openRouterParams.ParallelToolCalls != nil {
		openAIV3Params.ExtraFields["parallel_tool_calls"] = *openRouterParams.ParallelToolCalls
	}

	// Map user-facing ResponseFormat to internal ResponseFormat.
	if openRouterParams.ResponseFormat != nil {
		switch *openRouterParams.ResponseFormat {
		case config.ModelResponseFormatText:
			openAIV3Params.ResponseFormat = ResponseFormatText.Ptr()
		case config.ModelResponseFormatJSONObject:
			openAIV3Params.ResponseFormat = ResponseFormatJSONObject.Ptr()
		case config.ModelResponseFormatJSONSchema:
			openAIV3Params.ResponseFormat = ResponseFormatJSONSchema.Ptr()
		}
	}

	openAIV3Params.Verbosity = openRouterParams.Verbosity
	if openRouterParams.Temperature != nil {
		openAIV3Params.Temperature = utils.Ptr(float64(*openRouterParams.Temperature))
	}
	if openRouterParams.TopP != nil {
		openAIV3Params.TopP = utils.Ptr(float64(*openRouterParams.TopP))
	}
	if openRouterParams.PresencePenalty != nil {
		openAIV3Params.PresencePenalty = utils.Ptr(float64(*openRouterParams.PresencePenalty))
	}
	if openRouterParams.FrequencyPenalty != nil {
		openAIV3Params.FrequencyPenalty = utils.Ptr(float64(*openRouterParams.FrequencyPenalty))
	}
	if openRouterParams.MaxTokens != nil {
		openAIV3Params.MaxTokens = utils.Ptr(int64(*openRouterParams.MaxTokens))
	}
	openAIV3Params.Seed = openRouterParams.Seed
}
