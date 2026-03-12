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
	"log/slog"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
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
	openaiProvider := newOpenAICompletionsProvider(availableTools, openAIV3Opts...)
	openaiProvider.NewCompletionHandler = func() CompletionHandler {
		return &moonshotAICompletionHandler{}
	}

	return &MoonshotAI{openaiProvider: openaiProvider}
}

// MoonshotAI implements the Provider interface for Moonshot AI models.
// The Kimi models from Moonshot AI support OpenAI-compatible interfaces
// allowing them to be used with the existing OpenAI provider implementation.
type MoonshotAI struct {
	openaiProvider *openAICompletionsProvider
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

// moonshotAICompletionHandler extends the default completion handler to preserve
// the non-standard reasoning_content field required by Moonshot AI's thinking models
// (e.g., kimi-k2.5, kimi-k2-thinking) during multi-turn tool-call conversations.
//
// See: https://platform.moonshot.ai/docs/guide/use-kimi-k2-thinking-model#accessing-the-reasoning-content
type moonshotAICompletionHandler struct {
	defaultCompletionHandler
	reasoning strings.Builder
}

// reasoningContentKey is the non-standard field name used by Moonshot AI's thinking models
// to convey step-by-step reasoning alongside the assistant's response.
const reasoningContentKey = "reasoning_content"

func (h *moonshotAICompletionHandler) AddChunk(ctx context.Context, logger logging.Logger, chunk openai.ChatCompletionChunk) bool {
	for _, choice := range chunk.Choices {
		if raw, ok := extractExtraFieldRaw(choice.Delta.JSON.ExtraFields, reasoningContentKey); ok {
			var delta string
			if err := json.Unmarshal([]byte(raw), &delta); err != nil {
				logger.Error(ctx, slog.LevelWarn, err, "failed to unmarshal reasoning_content from chunk delta")
			} else {
				h.reasoning.WriteString(delta)
			}
		}
	}
	return h.defaultCompletionHandler.AddChunk(ctx, logger, chunk)
}

func (h *moonshotAICompletionHandler) ToParam(ctx context.Context, logger logging.Logger, message openai.ChatCompletionMessage) openai.ChatCompletionMessageParamUnion {
	param := h.defaultCompletionHandler.ToParam(ctx, logger, message)

	// Prefer streaming-accumulated reasoning_content (non-empty builder means streaming was used).
	if h.reasoning.Len() > 0 {
		h.setReasoningContent(param.OfAssistant, h.reasoning.String())
		return param
	}

	// Fall back to non-streaming: extract from message JSON metadata.
	if raw, ok := extractExtraFieldRaw(message.JSON.ExtraFields, reasoningContentKey); ok {
		var reasoningContent string
		if err := json.Unmarshal([]byte(raw), &reasoningContent); err != nil {
			logger.Error(ctx, slog.LevelWarn, err, "failed to unmarshal reasoning_content from response metadata")
		} else {
			h.setReasoningContent(param.OfAssistant, reasoningContent)
		}
	}
	return param
}

// extractExtraFieldRaw returns the raw JSON string for a non-standard field if it is
// present and non-null. The SDK's respjson.Field.Valid() returns false for ExtraFields,
// so presence is checked via Raw() instead.
func extractExtraFieldRaw(extraFields map[string]respjson.Field, key string) (string, bool) {
	if field, ok := extraFields[key]; ok && field.Raw() != "" && field.Raw() != "null" {
		return field.Raw(), true
	}
	return "", false
}

// setReasoningContent injects reasoning_content into an assistant message parameter
// via SetExtraFields.
func (h *moonshotAICompletionHandler) setReasoningContent(param *openai.ChatCompletionAssistantMessageParam, value string) {
	param.SetExtraFields(map[string]any{
		reasoningContentKey: value,
	})
}
