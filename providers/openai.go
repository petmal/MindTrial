// Copyright (C) 2025 Petr Malik
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

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers/tools"
	openai "github.com/sashabaranov/go-openai"
)

// NewOpenAI creates a new OpenAI provider instance with the given configuration.
func NewOpenAI(cfg config.OpenAIClientConfig, availableTools []config.ToolConfig) *OpenAI {
	return &OpenAI{
		client:         openai.NewClient(cfg.APIKey),
		availableTools: availableTools,
	}
}

// OpenAI implements the Provider interface for OpenAI generative models.
type OpenAI struct {
	client         *openai.Client
	availableTools []config.ToolConfig
}

func (o OpenAI) Name() string {
	return config.OPENAI
}

func (o *OpenAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	schema, err := ResultJSONSchema(task.ResponseResultFormat)
	if err != nil {
		return result, err
	}
	request := openai.ChatCompletionRequest{
		Model:    cfg.Model,
		Messages: []openai.ChatCompletionMessage{},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   "response",
				Schema: schema,
				Strict: true,
			},
		},
		N: 1, // generate only one candidate response
	}

	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.OpenAIModelParams); ok {
			if modelParams.ReasoningEffort != nil {
				request.ReasoningEffort = *modelParams.ReasoningEffort
			}
			if modelParams.TextResponseFormat || modelParams.EnableLegacyJsonMode {
				responseFormatInstruction, err := DefaultResponseFormatInstruction(task.ResponseResultFormat)
				if err != nil {
					return result, err
				}
				request.Messages = append(request.Messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser, // NOTE: system role not supported by all models
					Content: result.recordPrompt(responseFormatInstruction),
				})

				// For TextResponseFormat, change the response format to plain text; otherwise keep JSON schema.
				if modelParams.TextResponseFormat {
					request.ResponseFormat = &openai.ChatCompletionResponseFormat{
						Type: openai.ChatCompletionResponseFormatTypeText,
					}
				}
			}
			if modelParams.Temperature != nil {
				request.Temperature = *modelParams.Temperature
			}
			if modelParams.TopP != nil {
				request.TopP = *modelParams.TopP
			}
			if modelParams.MaxCompletionTokens != nil {
				request.MaxCompletionTokens = int(*modelParams.MaxCompletionTokens)
			}
			if modelParams.MaxTokens != nil {
				request.MaxTokens = int(*modelParams.MaxTokens)
			}
			if modelParams.PresencePenalty != nil {
				request.PresencePenalty = *modelParams.PresencePenalty
			}
			if modelParams.FrequencyPenalty != nil {
				request.FrequencyPenalty = *modelParams.FrequencyPenalty
			}
			if modelParams.Seed != nil {
				request.Seed = utils.ConvertIntPtr[int64, int](modelParams.Seed)
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	if promptMessage, err := o.createPromptMessage(ctx, task.Prompt, task.Files, &result); err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	} else {
		request.Messages = append(request.Messages, promptMessage)
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		request.Messages = append(request.Messages,
			openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser, // NOTE: system role not supported by all models
				Content: result.recordPrompt(answerFormatInstruction),
			})
	}

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
			// Find the tool config from available tools.
			toolCfg, found := findToolByName(o.availableTools, toolName)
			if !found {
				return result, fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
			}
			tool := tools.NewDockerTool(toolCfg, toolSelection.MaxCalls, toolSelection.Timeout, toolSelection.MaxMemoryMB, toolSelection.CpuPercent)
			executor.RegisterTool(tool)
			request.Tools = append(request.Tools, openai.Tool{
				Type: "function",
				Function: &openai.FunctionDefinition{
					Name:        toolCfg.Name,
					Description: toolCfg.Description,
					Strict:      false,
					Parameters:  toolCfg.Parameters,
				},
			})
		}
		// If user tools are present, allow auto tool choice.
		request.ToolChoice = "auto"
	}

	// Conversation loop to handle tool calls.
	for {
		resp, err := timed(func() (openai.ChatCompletionResponse, error) {
			response, err := o.client.CreateChatCompletion(ctx, request)
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
				if request.ResponseFormat.Type == openai.ChatCompletionResponseFormatTypeText {
					content, err = utils.RepairTextJSON(content)
					if err != nil {
						return result, NewErrUnmarshalResponse(err, []byte(candidate.Message.Content), []byte(candidate.FinishReason))
					}
				}
				if err = json.Unmarshal([]byte(content), &result); err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(candidate.Message.Content), []byte(candidate.FinishReason))
				}
				return result, nil
			}

			// Append assistant message to conversation history before handling tool calls.
			request.Messages = append(request.Messages, candidate.Message)

			// Handle tool calls.
			for _, toolCall := range candidate.Message.ToolCalls {
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				toolResult, err := executor.ExecuteTool(ctx, logger, toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments), data)
				content := string(toolResult)
				if err != nil {
					content = formatToolExecutionError(err)
				}
				request.Messages = append(request.Messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    content,
					ToolCallID: toolCall.ID,
				})
			}
		}
	} // move to the next conversation turn
}

func (o *OpenAI) createPromptMessage(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (message openai.ChatCompletionMessage, err error) {
	message.Role = openai.ChatMessageRoleUser

	if len(files) > 0 {
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
			// Attach file name as a separate text part before the image, for reference.
			message.MultiContent = append(message.MultiContent, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeText,
				Text: result.recordPrompt(DefaultTaskFileNameInstruction(file)),
			})
			message.MultiContent = append(message.MultiContent, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeImageURL,
				ImageURL: &openai.ChatMessageImageURL{
					URL:    dataURL,
					Detail: openai.ImageURLDetailAuto,
				},
			})
		}

		message.MultiContent = append(message.MultiContent, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeText,
			Text: result.recordPrompt(promptText),
		}) // append the prompt text after the file data for improved context integrity
	} else {
		message.Content = result.recordPrompt(promptText)
	}

	return message, nil
}

func (o *OpenAI) isTransientResponse(err error) bool {
	var apiError *openai.APIError
	if errors.As(err, &apiError) {
		return slices.Contains([]int{
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusServiceUnavailable,
		}, apiError.HTTPStatusCode)
	}
	return false
}

func (o *OpenAI) Close(ctx context.Context) error {
	return nil
}
