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
	"log/slog"
	"maps"
	"slices"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
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

	openaiProvider := newOpenAICompletionsProvider(availableTools, openAIV3Opts...)
	openaiProvider.NewCompletionHandler = func(any) CompletionHandler {
		return &openRouterCompletionHandler{}
	}

	return &OpenRouter{openaiProvider: openaiProvider}
}

// openRouterCompletionHandler preserves reasoning metadata when an assistant
// tool-call message is replayed on the next conversation turn. Non-streaming
// responses prefer the complete reasoning_details array, then reasoning and
// reasoning_content. Streaming responses rebuild that array in arrival order,
// merging only consecutive deltas of the same streamable type; encrypted and
// unknown blocks stay discrete.
//
// See https://openrouter.ai/docs/guides/best-practices/reasoning-tokens#preserving-reasoning
// for OpenRouter's requirement to replay reasoning_details without rearranging
// or modifying the sequence.
type openRouterCompletionHandler struct {
	defaultCompletionHandler
	// reasoningDetails is non-nil once the stream carried a reasoning_details
	// array, including an explicitly empty one, which some providers require to
	// be echoed back to maintain conversation state.
	reasoningDetails []map[string]any
	reasoning        strings.Builder
	reasoningContent strings.Builder
}

// IsTerminalStopReason additionally treats a response carrying tool calls as
// non-terminal when the finish reason is "", "stop", or "tool_calls", since
// some OpenRouter-routed models (e.g. Gemini) report "stop" or omit the finish
// reason alongside tool calls and encrypted reasoning. Any other reason (e.g.
// "length", "content_filter", or one not yet known to MindTrial) stays
// terminal by exclusion, consistent with GoogleAI.hasTerminalStopReason.
// See https://openrouter.ai/docs/guides/best-practices/reasoning-tokens#preserving-reasoning
func (h *openRouterCompletionHandler) IsTerminalStopReason(candidate openai.ChatCompletionChoice) bool {
	if len(candidate.Message.ToolCalls) > 0 {
		return !slices.Contains([]string{"", "stop", "tool_calls"}, candidate.FinishReason)
	}
	return h.defaultCompletionHandler.IsTerminalStopReason(candidate)
}

func (h *openRouterCompletionHandler) AddChunk(ctx context.Context, logger logging.Logger, chunk openai.ChatCompletionChunk) bool {
	for _, choice := range chunk.Choices {
		h.mergeReasoningDetails(ctx, logger, choice.Delta.JSON.ExtraFields)
		h.appendReasoningString(ctx, logger, choice.Delta.JSON.ExtraFields, "reasoning", &h.reasoning)
		h.appendReasoningString(ctx, logger, choice.Delta.JSON.ExtraFields, reasoningContentKey, &h.reasoningContent)
	}
	return h.defaultCompletionHandler.AddChunk(ctx, logger, chunk)
}

func (h *openRouterCompletionHandler) ToParam(ctx context.Context, logger logging.Logger, message openai.ChatCompletionMessage) openai.ChatCompletionMessageParamUnion {
	result := h.defaultCompletionHandler.ToParam(ctx, logger, message)

	if h.reasoningDetails != nil {
		result.OfAssistant.SetExtraFields(map[string]any{"reasoning_details": h.reasoningDetails})
		return result
	}
	if raw, ok := extractExtraFieldRaw(message.JSON.ExtraFields, "reasoning_details"); ok {
		var details []map[string]any
		if err := json.Unmarshal([]byte(raw), &details); err != nil {
			logger.Error(ctx, slog.LevelWarn, err, "failed to unmarshal OpenRouter reasoning_details")
		} else {
			result.OfAssistant.SetExtraFields(map[string]any{"reasoning_details": details})
			return result
		}
	}

	if h.reasoning.Len() > 0 {
		result.OfAssistant.SetExtraFields(map[string]any{"reasoning": h.reasoning.String()})
		return result
	}
	if h.reasoningContent.Len() > 0 {
		result.OfAssistant.SetExtraFields(map[string]any{reasoningContentKey: h.reasoningContent.String()})
		return result
	}
	for _, key := range []string{"reasoning", reasoningContentKey} {
		if raw, ok := extractExtraFieldRaw(message.JSON.ExtraFields, key); ok {
			var value string
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				logger.Error(ctx, slog.LevelWarn, err, "failed to unmarshal OpenRouter %s field", key)
				continue
			}
			result.OfAssistant.SetExtraFields(map[string]any{key: value})
			return result
		}
	}
	return result
}

func (h *openRouterCompletionHandler) mergeReasoningDetails(ctx context.Context, logger logging.Logger, extraFields map[string]respjson.Field) {
	raw, ok := extractExtraFieldRaw(extraFields, "reasoning_details")
	if !ok {
		return
	}
	var details []map[string]any
	if err := json.Unmarshal([]byte(raw), &details); err != nil {
		logger.Error(ctx, slog.LevelWarn, err, "failed to unmarshal OpenRouter reasoning_details chunk")
		return
	}
	if h.reasoningDetails == nil {
		h.reasoningDetails = []map[string]any{} // must serialize as [], not null
	}
	for _, detail := range details {
		h.mergeReasoningDetail(detail)
	}
}

// mergeReasoningDetail appends detail to the accumulated sequence, merging it
// into the immediately preceding entry only when both are the same streamable
// type. OpenRouter streams a logical reasoning block as many deltas that often
// repeat the same index, so index cannot drive reassembly; arrival order and
// type transitions do.
func (h *openRouterCompletionHandler) mergeReasoningDetail(detail map[string]any) {
	detailType, _ := detail["type"].(string)
	field, mergeable := incrementalReasoningDetailField(detailType)
	if !mergeable || len(h.reasoningDetails) == 0 {
		h.reasoningDetails = append(h.reasoningDetails, detail)
		return
	}

	last := h.reasoningDetails[len(h.reasoningDetails)-1]
	if lastType, _ := last["type"].(string); lastType != detailType {
		h.reasoningDetails = append(h.reasoningDetails, detail)
		return
	}

	for key, value := range detail {
		if key == field {
			delta, _ := value.(string)
			current, _ := last[key].(string)
			last[key] = current + delta
			continue
		}
		// Late metadata such as a trailing signature fills gaps without
		// overwriting values already received.
		if current, exists := last[key]; !exists || current == nil || current == "" {
			last[key] = value
		}
	}
}

// incrementalReasoningDetailField returns the field that carries streamed text
// for reasoning detail types delivered as deltas. Encrypted and unknown types
// are opaque blocks and must never be merged.
func incrementalReasoningDetailField(detailType string) (string, bool) {
	switch detailType {
	case "reasoning.text":
		return "text", true
	case "reasoning.summary":
		return "summary", true
	default:
		return "", false
	}
}

func (h *openRouterCompletionHandler) appendReasoningString(ctx context.Context, logger logging.Logger, extraFields map[string]respjson.Field, key string, dst *strings.Builder) {
	raw, ok := extractExtraFieldRaw(extraFields, key)
	if !ok {
		return
	}
	var value string
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		logger.Error(ctx, slog.LevelWarn, err, "failed to unmarshal OpenRouter %s chunk", key)
		return
	}
	dst.WriteString(value)
}

// OpenRouter implements the Provider interface for models reachable via OpenRouter.
type OpenRouter struct {
	openaiProvider *openAICompletionsProvider
}

func (o OpenRouter) Name() string {
	return config.OPENROUTER
}

func (o *OpenRouter) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	openAIV3Params := openAIV3ModelParams{
		ExtraFields: map[string]any{},
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

	// Copied last so a raw field wins over its typed equivalent, matching the
	// precedence the SDK applies to fields it models. These keys are absent from
	// the SDK request struct, so nothing else arbitrates between them.
	maps.Copy(openAIV3Params.ExtraFields, openRouterParams.Extra)

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
	openAIV3Params.ReasoningEffort = openRouterParams.ReasoningEffort
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
	if openRouterParams.MaxCompletionTokens != nil {
		openAIV3Params.MaxCompletionTokens = utils.Ptr(int64(*openRouterParams.MaxCompletionTokens))
	}
	openAIV3Params.Seed = openRouterParams.Seed

	for _, st := range openRouterParams.ServerTools {
		openAIV3Params.ServerTools = append(openAIV3Params.ServerTools, openAIServerTool{
			Type:       st.Type,
			Parameters: st.Parameters,
		})
	}
}
