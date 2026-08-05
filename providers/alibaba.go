// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/json"
	"fmt"

	openai "github.com/openai/openai-go/v3"
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
	openaiProvider := newOpenAICompletionsProvider(availableTools, openAIV3Opts...)
	openaiProvider.NewCompletionHandler = func(args any) CompletionHandler {
		var base CompletionHandler = &defaultCompletionHandler{}
		if hArgs, ok := args.(alibabaCompletionHandlerArgs); ok && hArgs.PreserveThinking {
			base = &moonshotAICompletionHandler{}
		}
		return &alibabaCompletionHandler{CompletionHandler: base}
	}

	return &Alibaba{openaiProvider: openaiProvider}
}

// alibabaCompletionHandler wraps the base handler selected for reasoning content
// handling (defaultCompletionHandler, or moonshotAICompletionHandler when
// preserve-thinking is enabled) and overrides InputCacheTokens to additionally
// read Alibaba's cache token counts.
type alibabaCompletionHandler struct {
	CompletionHandler
}

// InputCacheTokens reads both of Alibaba's cache counters from prompt_tokens_details.
// The read count is deliberately not delegated to the wrapped handler: that handler
// varies with preserve-thinking, and Moonshot's reads a top-level field Alibaba never
// reports. The write count uses a non-standard field name.
// See: https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-chat-completions
func (h *alibabaCompletionHandler) InputCacheTokens(usage openai.CompletionUsage) (writeTokens *int64, readTokens *int64) {
	var openAICompatible defaultCompletionHandler
	_, readTokens = openAICompatible.InputCacheTokens(usage)
	if raw, ok := extractExtraFieldRaw(usage.PromptTokensDetails.JSON.ExtraFields, "cache_creation_input_tokens"); ok {
		var value int64
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			writeTokens = &value
		}
	}
	return writeTokens, readTokens
}

// alibabaCompletionHandlerArgs is Alibaba-specific input to the fixed
// NewCompletionHandler factory set in NewAlibaba.
type alibabaCompletionHandlerArgs struct {
	// PreserveThinking requests that reasoning content be retained across
	// conversation turns.
	PreserveThinking bool
}

// Alibaba implements the Provider interface for Alibaba models.
// The Qwen models from Alibaba Cloud support OpenAI-compatible interfaces
// allowing them to be used with the existing OpenAI provider implementation.
type Alibaba struct {
	openaiProvider *openAICompletionsProvider
}

func (a Alibaba) Name() string {
	return config.ALIBABA
}

func (a *Alibaba) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	openAIV3Params := openAIV3ModelParams{ExtraFields: map[string]any{}}
	var preserveThinking bool

	// Alibaba Qwen models prefer legacy-json-schema instructions by default
	// unless structured output is explicitly disabled.
	if !cfg.DisableStructuredOutput {
		openAIV3Params.ResponseFormat = ResponseFormatLegacySchema.Ptr()
	}

	if cfg.ModelParams != nil {
		if alibabaParams, ok := cfg.ModelParams.(config.AlibabaModelParams); ok {
			a.copyToOpenAIV3Params(alibabaParams, &openAIV3Params)
			preserveThinking = alibabaParams.PreserveThinking != nil && *alibabaParams.PreserveThinking
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}
	cfg.ModelParams = openAIV3Params

	return a.openaiProvider.run(ctx, logger, cfg, task, alibabaCompletionHandlerArgs{PreserveThinking: preserveThinking})
}

func (a *Alibaba) Close(ctx context.Context) error {
	return a.openaiProvider.Close(ctx) // delegate to the OpenAI provider
}

// copyToOpenAIV3Params copies relevant fields from AlibabaModelParams to openAIV3ModelParams.
func (a *Alibaba) copyToOpenAIV3Params(alibabaParams config.AlibabaModelParams, openAIV3Params *openAIV3ModelParams) {
	if alibabaParams.ResponseFormat != nil {
		switch *alibabaParams.ResponseFormat {
		case config.ModelResponseFormatJSONSchema:
			openAIV3Params.ResponseFormat = ResponseFormatJSONSchema.Ptr()
		case config.ModelResponseFormatJSONObject:
			openAIV3Params.ResponseFormat = ResponseFormatJSONObject.Ptr()
		case config.ModelResponseFormatText:
			openAIV3Params.ResponseFormat = ResponseFormatText.Ptr()
		}
	} else {
		// Deprecated compatibility path.
		if alibabaParams.DisableLegacyJsonMode != nil && *alibabaParams.DisableLegacyJsonMode {
			openAIV3Params.ResponseFormat = nil // disable legacy mode; use the provider's strict schema default instead
		}
		if alibabaParams.TextResponseFormat != nil && *alibabaParams.TextResponseFormat {
			openAIV3Params.ResponseFormat = ResponseFormatText.Ptr()
		}
	}
	if alibabaParams.Stream {
		openAIV3Params.Stream = utils.Ptr(true)
	}
	if alibabaParams.EnableThinking != nil {
		openAIV3Params.ExtraFields["enable_thinking"] = *alibabaParams.EnableThinking
	}
	if alibabaParams.PreserveThinking != nil {
		openAIV3Params.ExtraFields["preserve_thinking"] = *alibabaParams.PreserveThinking
	}
	if alibabaParams.ThinkingBudget != nil {
		openAIV3Params.ExtraFields["thinking_budget"] = *alibabaParams.ThinkingBudget
	}
	openAIV3Params.ReasoningEffort = alibabaParams.ReasoningEffort
	if alibabaParams.Temperature != nil {
		openAIV3Params.Temperature = utils.Ptr(float64(*alibabaParams.Temperature))
	}
	if alibabaParams.TopP != nil {
		openAIV3Params.TopP = utils.Ptr(float64(*alibabaParams.TopP))
	}
	if alibabaParams.MaxTokens != nil {
		openAIV3Params.MaxTokens = utils.Ptr(int64(*alibabaParams.MaxTokens))
	}
	if alibabaParams.MaxCompletionTokens != nil {
		openAIV3Params.MaxCompletionTokens = utils.Ptr(int64(*alibabaParams.MaxCompletionTokens))
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
