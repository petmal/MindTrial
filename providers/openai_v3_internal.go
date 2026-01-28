// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers/tools"
)

// openAIV3Provider is an OpenAI-compatible provider implementation using
// OpenAI's official Go SDK v3.
type openAIV3Provider struct {
	client         openai.Client
	availableTools []config.ToolConfig
}

// openAIV3ModelParams is an internal model configuration used by openAIV3Provider.
// It is not user-facing; provider wrappers translate their user-facing model params
// into this struct.
type openAIV3ModelParams struct {
	ReasoningEffort *string
	Verbosity       *string

	// ResponseFormat controls the API-level response_format setting and prompt instruction behavior.
	// When nil, the provider uses strict JSON schema mode by default without adding response format instructions.
	ResponseFormat *ResponseFormat

	Temperature         *float64
	TopP                *float64
	PresencePenalty     *float64
	FrequencyPenalty    *float64
	MaxCompletionTokens *int64
	MaxTokens           *int64
	Seed                *int64

	// ExtraFields are applied to the JSON request body.
	ExtraFields map[string]any
}

// ResponseFormat specifies the response format mode for the internal OpenAI-compatible provider.
// This is an internal type; provider wrappers map user-facing formats to these internal values.
type ResponseFormat string

const (
	// ResponseFormatJSONSchema uses strict json_schema mode without adding response format instructions to the prompt.
	// This is the default behavior when ResponseFormat is nil.
	ResponseFormatJSONSchema ResponseFormat = "json-schema"

	// ResponseFormatLegacySchema uses strict json_schema mode but adds response format instructions to the prompt.
	// Use this for legacy providers that require explicit JSON formatting guidance (e.g., Alibaba Qwen).
	ResponseFormatLegacySchema ResponseFormat = "legacy-json-schema"

	// ResponseFormatJSONObject uses json_object mode and adds response format instructions to the prompt.
	// Use this for providers that only support basic JSON object responses (e.g., Moonshot Kimi).
	ResponseFormatJSONObject ResponseFormat = "json-object"

	// ResponseFormatText uses text mode and adds response format instructions to the prompt.
	// The provider attempts to repair the text response into valid JSON.
	ResponseFormatText ResponseFormat = "text"
)

// Ptr returns a pointer to the ResponseFormat value.
func (r ResponseFormat) Ptr() *ResponseFormat {
	return utils.Ptr(r)
}

func newOpenAIV3Provider(availableTools []config.ToolConfig, opts ...option.RequestOption) *openAIV3Provider {
	clientOpts := append([]option.RequestOption{
		option.WithMaxRetries(0), // disable SDK retries since MindTrial has its own retry policy
	}, opts...)

	return &openAIV3Provider{
		client:         openai.NewClient(clientOpts...),
		availableTools: availableTools,
	}
}

func (o *openAIV3Provider) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	request := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(cfg.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{},
		N:        param.NewOpt(int64(1)), // generate only one candidate response
	}

	// Set response format based on structured output mode.
	if cfg.DisableStructuredOutput {
		request.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfText: &shared.ResponseFormatTextParam{}}
	} else {
		schema, err := ResultJSONSchema(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		request.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
			JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:   "response",
				Schema: schema,
				Strict: param.NewOpt(true),
			},
		}}
	}

	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(openAIV3ModelParams); ok {
			if len(modelParams.ExtraFields) > 0 {
				request.SetExtraFields(modelParams.ExtraFields)
			}

			if modelParams.ReasoningEffort != nil {
				request.ReasoningEffort = shared.ReasoningEffort(*modelParams.ReasoningEffort)
			}
			if modelParams.Verbosity != nil {
				request.Verbosity = openai.ChatCompletionNewParamsVerbosity(*modelParams.Verbosity)
			}
			if modelParams.ResponseFormat != nil {
				// Validate that DisableStructuredOutput is not used with non-text response format.
				if cfg.DisableStructuredOutput && *modelParams.ResponseFormat != ResponseFormatText {
					return result, ErrIncompatibleResponseFormat
				}
				// Skip response format handling when structured output is disabled (already forced to text above).
				if !cfg.DisableStructuredOutput {
					// Add response format instruction to prompt for all formats except strict schema-only mode.
					if *modelParams.ResponseFormat != ResponseFormatJSONSchema {
						responseFormatInstruction, err := DefaultResponseFormatInstruction(task.ResponseResultFormat)
						if err != nil {
							return result, err
						}
						request.Messages = append(request.Messages, openai.UserMessage(result.recordPrompt(responseFormatInstruction)))
					}

					// Override response format type for non-schema modes.
					switch *modelParams.ResponseFormat {
					case ResponseFormatText:
						request.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfText: &shared.ResponseFormatTextParam{}}
					case ResponseFormatJSONObject:
						request.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONObject: &shared.ResponseFormatJSONObjectParam{}}
					}
					// ResponseFormatLegacySchema and ResponseFormatJSONSchema keep the default json_schema format.
				}
			}
			if modelParams.Temperature != nil {
				request.Temperature = param.NewOpt(*modelParams.Temperature)
			}
			if modelParams.TopP != nil {
				request.TopP = param.NewOpt(*modelParams.TopP)
			}
			if modelParams.MaxCompletionTokens != nil {
				request.MaxCompletionTokens = param.NewOpt(*modelParams.MaxCompletionTokens)
			}
			if modelParams.MaxTokens != nil {
				request.MaxTokens = param.NewOpt(*modelParams.MaxTokens)
			}
			if modelParams.PresencePenalty != nil {
				request.PresencePenalty = param.NewOpt(*modelParams.PresencePenalty)
			}
			if modelParams.FrequencyPenalty != nil {
				request.FrequencyPenalty = param.NewOpt(*modelParams.FrequencyPenalty)
			}
			if modelParams.Seed != nil {
				request.Seed = param.NewOpt(*modelParams.Seed)
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	if cfg.DisableStructuredOutput {
		request.Messages = append(request.Messages, openai.UserMessage(result.recordPrompt(DefaultUnstructuredResponseInstruction())))
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		request.Messages = append(request.Messages, openai.UserMessage(result.recordPrompt(answerFormatInstruction)))
	}

	promptMessage, err := o.createPromptMessage(ctx, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}
	request.Messages = append(request.Messages, promptMessage)

	// Setup tools if any.
	var executor *tools.DockerToolExecutor
	toolSelector := task.GetResolvedToolSelector()
	if enabledTools, hasTools := toolSelector.GetEnabledToolsByName(); hasTools {
		var err error
		executor, err = tools.NewDockerToolExecutor(ctx)
		if err != nil {
			return result, fmt.Errorf("%w: %w", ErrToolSetup, err)
		}
		defer executor.Close()
		for toolName, toolSelection := range enabledTools {
			toolCfg, found := findToolByName(o.availableTools, toolName)
			if !found {
				return result, fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
			}
			tool := tools.NewDockerTool(toolCfg, toolSelection.MaxCalls, toolSelection.Timeout, toolSelection.MaxMemoryMB, toolSelection.CpuPercent)
			executor.RegisterTool(tool)
			request.Tools = append(request.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        toolCfg.Name,
				Description: param.NewOpt(toolCfg.Description),
				Strict:      param.NewOpt(false),
				Parameters:  toolCfg.Parameters,
			}))
		}
		request.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoAuto))}
	}

	// Conversation loop to handle tool calls.
	for {
		resp, err := timed(func() (*openai.ChatCompletion, error) {
			response, err := o.client.Chat.Completions.New(ctx, request)
			if err != nil && o.isTransientResponse(err) {
				return response, WrapErrRetryable(err)
			}
			return response, err
		}, &result.duration)
		if err != nil {
			return result, WrapErrGenerateResponse(err)
		}

		recordUsage(&resp.Usage.PromptTokens, &resp.Usage.CompletionTokens, &result.usage)
		result.recordToolUsage(executor.GetUsageStats())

		for _, candidate := range resp.Choices {
			if len(candidate.Message.ToolCalls) == 0 {
				// No tool calls, this is the final response.
				content := candidate.Message.Content

				var err error
				if cfg.DisableStructuredOutput {
					err = UnmarshalUnstructuredResponse(ctx, logger, []byte(content), &result)
				} else {
					if request.ResponseFormat.OfText != nil {
						content, err = utils.RepairTextJSON(content)
						if err != nil {
							return result, NewErrUnmarshalResponse(err, []byte(candidate.Message.Content), []byte(candidate.FinishReason))
						}
					}
					err = json.Unmarshal([]byte(content), &result)
				}
				if err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(candidate.Message.Content), []byte(candidate.FinishReason))
				}
				return result, nil
			}

			// Append assistant message to conversation history before handling tool calls.
			request.Messages = append(request.Messages, candidate.Message.ToParam())

			for _, toolCall := range candidate.Message.ToolCalls {
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				toolResult, err := executor.ExecuteTool(ctx, logger, toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments), data)
				toolContent := string(toolResult)
				if err != nil {
					toolContent = formatToolExecutionError(err)
				}
				request.Messages = append(request.Messages, openai.ToolMessage(toolContent, toolCall.ID))
			}
		}
	} // move to the next conversation turn
}

func (o *openAIV3Provider) createPromptMessage(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (message openai.ChatCompletionMessageParamUnion, err error) {
	if len(files) > 0 {
		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, (len(files)*2)+1)
		for _, file := range files {
			if fileType, err := file.TypeValue(ctx); err != nil {
				return message, err
			} else if !isSupportedImageType(fileType) {
				return message, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
			}
			dataURL, err := file.GetDataURL(ctx)
			if err != nil {
				return message, err
			}
			parts = append(parts, openai.TextContentPart(result.recordPrompt(DefaultTaskFileNameInstruction(file))))
			parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL:    dataURL,
				Detail: "auto",
			}))
		}
		// Append the prompt text after the file data for improved context integrity.
		parts = append(parts, openai.TextContentPart(result.recordPrompt(promptText)))
		return openai.UserMessage(parts), nil
	} else {
		return openai.UserMessage(result.recordPrompt(promptText)), nil
	}
}

func (o *openAIV3Provider) isTransientResponse(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return slices.Contains([]int{
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusServiceUnavailable,
		}, apiErr.StatusCode)
	}
	return false
}

func (o *openAIV3Provider) Close(ctx context.Context) error {
	return nil
}
