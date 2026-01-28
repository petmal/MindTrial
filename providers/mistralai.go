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
	"strings"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	mistralai "github.com/petmal/mindtrial/pkg/mistralai"
	"github.com/petmal/mindtrial/providers/tools"
)

// NewMistralAI creates a new Mistral AI provider instance with the given configuration.
func NewMistralAI(cfg config.MistralAIClientConfig, availableTools []config.ToolConfig) (*MistralAI, error) {
	clientCfg := mistralai.NewConfiguration()
	clientCfg.AddDefaultHeader("Authorization", "Bearer "+cfg.APIKey)

	client := mistralai.NewAPIClient(clientCfg)
	return &MistralAI{
		client:         client,
		availableTools: availableTools,
	}, nil
}

// MistralAI implements the Provider interface for Mistral AI generative models.
type MistralAI struct {
	client         *mistralai.APIClient
	availableTools []config.ToolConfig
}

func (o MistralAI) Name() string {
	return config.MISTRALAI
}

func (o *MistralAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	if len(task.Files) > 0 {
		if !o.isFileUploadSupported(cfg.Model) {
			return result, ErrFileUploadNotSupported
		}
	}

	request := mistralai.NewChatCompletionRequestWithDefaults()
	request.SetModel(cfg.Model)
	request.SetN(1)

	// Configure response format.
	responseFormat := mistralai.NewResponseFormatWithDefaults()
	if cfg.DisableStructuredOutput {
		responseFormat.SetType(mistralai.TEXT)
	} else {
		responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		schema := mistralai.NewJsonSchema("response", responseSchema)
		schema.SetDescription(mistralai.Description{
			String: mistralai.PtrString("Record the response using well-structured JSON."),
		})
		responseFormat.SetType(mistralai.JSON_SCHEMA)
		responseFormat.SetJsonSchema(*schema)
	}
	request.SetResponseFormat(*responseFormat)

	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.MistralAIModelParams); ok {
			if err := o.applyModelParameters(request, modelParams); err != nil {
				return result, fmt.Errorf("%w: %s: %v", ErrInvalidModelParams, cfg.Name, err)
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	if cfg.DisableStructuredOutput {
		request.Messages = append(request.Messages, mistralai.SystemMessageAsChatCompletionRequestMessagesInner(
			mistralai.NewSystemMessage(mistralai.Content4{
				String: mistralai.PtrString(result.recordPrompt(DefaultUnstructuredResponseInstruction())),
			})))
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		request.Messages = append(request.Messages, mistralai.SystemMessageAsChatCompletionRequestMessagesInner(
			mistralai.NewSystemMessage(mistralai.Content4{
				String: mistralai.PtrString(result.recordPrompt(answerFormatInstruction)),
			})))
	}

	contentParts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}

	request.Messages = append(request.Messages, mistralai.UserMessageAsChatCompletionRequestMessagesInner(
		mistralai.NewUserMessage(*mistralai.NewNullableContent3(&mistralai.Content3{
			ArrayOfContentChunk: &contentParts,
		}))))

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
			function := mistralai.NewFunction(toolCfg.Name, toolCfg.Parameters)
			function.SetDescription(toolCfg.Description)
			function.SetStrict(false)
			toolDef := mistralai.NewTool(*function)
			request.Tools = append(request.Tools, *toolDef)
		}
		// If user tools are present, allow auto tool choice.
		request.ToolChoice = mistralai.ToolChoiceEnum("auto").Ptr()
	}

	// Conversation loop to handle tool calls.
	for {
		resp, err := timed(func() (*mistralai.ChatCompletionResponse, error) {
			response, httpResponse, err := o.client.ChatAPI.ChatCompletionV1ChatCompletionsPost(ctx).ChatCompletionRequest(*request).Execute()
			if err != nil {
				var apiErr *mistralai.GenericOpenAPIError
				switch {
				case o.isTransientResponse(httpResponse):
					return response, WrapErrRetryable(err)
				case errors.As(err, &apiErr):
					return response, NewErrAPIResponse(err, apiErr.Body())
				}
			}
			return response, err
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		if err != nil {
			return result, WrapErrGenerateResponse(err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		if resp.Usage != nil {
			recordUsage(&resp.Usage.PromptTokens, &resp.Usage.CompletionTokens, &result.usage)
		}
		for _, candidate := range resp.Choices {
			if len(candidate.Message.ToolCalls) == 0 {
				// No tool calls, this is the final response.
				if message, ok := candidate.Message.GetContentOk(); ok {
					if message != nil && message.String != nil {
						content := *message.String

						var err error
						if cfg.DisableStructuredOutput {
							err = UnmarshalUnstructuredResponse(ctx, logger, []byte(content), &result)
						} else {
							err = json.Unmarshal([]byte(content), &result)
						}
						if err != nil {
							return result, NewErrUnmarshalResponse(err, []byte(content), []byte(candidate.FinishReason))
						}
						return result, nil
					}
				}
			}

			// Append assistant message to conversation history before handling tool calls.
			request.Messages = append(request.Messages, mistralai.AssistantMessageAsChatCompletionRequestMessagesInner(&candidate.Message))

			// Handle tool calls.
			for _, toolCall := range candidate.Message.ToolCalls {
				args, err := marshalToolArguments(toolCall.Function.Arguments)
				if err != nil {
					return result, fmt.Errorf("%w: failed to extract tool arguments: %v", ErrToolUse, err)
				}
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				toolResult, err := executor.ExecuteTool(ctx, logger, toolCall.Function.Name, args, data)
				content := string(toolResult)
				if err != nil {
					content = formatToolExecutionError(err)
				}
				content3 := mistralai.Content3{
					String: &content,
				}
				nullableContent := mistralai.NewNullableContent3(&content3)
				toolMessage := mistralai.NewToolMessage(*nullableContent)
				toolMessage.ToolCallId.Set(toolCall.Id)
				request.Messages = append(request.Messages, mistralai.ToolMessageAsChatCompletionRequestMessagesInner(toolMessage))
			}
		}
	} // move to the next conversation turn
}

func (o *MistralAI) isFileUploadSupported(model string) bool {
	// Mistral AI models with vision capabilities.
	// See: https://docs.mistral.ai/capabilities/vision/
	// Supported: mistral-large, mistral-medium, mistral-small, ministral, pixtral, magistral
	// Not supported: mistral-embed, mistral-moderation, mistral-nemo, codestral, devstral, voxtral
	return slices.ContainsFunc([]string{
		"mistral-large-",
		"mistral-medium-",
		"mistral-small-",
		"ministral-",
		"pixtral-",
		"magistral-",
	}, func(prefix string) bool {
		return strings.HasPrefix(model, prefix)
	})
}

func (o *MistralAI) isTransientResponse(response *http.Response) bool {
	return response != nil && slices.Contains([]int{
		http.StatusTooManyRequests,
		http.StatusRequestTimeout,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}, response.StatusCode)
}

func (o *MistralAI) applyModelParameters(request *mistralai.ChatCompletionRequest, modelParams config.MistralAIModelParams) error {
	if modelParams.Temperature != nil {
		request.SetTemperature(*modelParams.Temperature)
	}

	if modelParams.TopP != nil {
		request.SetTopP(*modelParams.TopP)
	}

	if modelParams.MaxTokens != nil {
		request.SetMaxTokens(*modelParams.MaxTokens)
	}

	if modelParams.PresencePenalty != nil {
		request.SetPresencePenalty(*modelParams.PresencePenalty)
	}

	if modelParams.FrequencyPenalty != nil {
		request.SetFrequencyPenalty(*modelParams.FrequencyPenalty)
	}

	if modelParams.RandomSeed != nil {
		request.SetRandomSeed(*modelParams.RandomSeed)
	}

	if modelParams.PromptMode != nil {
		promptMode, err := mistralai.NewMistralPromptModeFromValue(*modelParams.PromptMode)
		if err != nil {
			return err
		}
		request.SetPromptMode(*promptMode)
	}

	if modelParams.SafePrompt != nil {
		request.SetSafePrompt(*modelParams.SafePrompt)
	}

	return nil
}

func (o *MistralAI) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []mistralai.ContentChunk, err error) {
	for _, file := range files {
		if fileType, err := file.TypeValue(ctx); err != nil {
			return parts, err
		} else if !isSupportedImageType(fileType) {
			return parts, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
		}

		dataURL, err := file.GetDataURL(ctx)
		if err != nil {
			return parts, err
		}

		// Attach file name as a separate text block before the image.
		parts = append(parts, mistralai.TextChunkAsContentChunk(
			mistralai.NewTextChunk(result.recordPrompt(DefaultTaskFileNameInstruction(file)))))

		// Create image URL struct and chunk.
		imageURLChunk := mistralai.NewImageURLChunk(mistralai.ImageUrl{
			ImageURLStruct: mistralai.NewImageURLStruct(dataURL),
		})
		parts = append(parts, mistralai.ImageURLChunkAsContentChunk(imageURLChunk))
	}

	// Append the prompt text after the file data for improved context integrity.
	parts = append(parts, mistralai.TextChunkAsContentChunk(
		mistralai.NewTextChunk(result.recordPrompt(promptText))))

	return parts, nil
}

func marshalToolArguments(args mistralai.Arguments) (argsData json.RawMessage, err error) {
	if args.MapmapOfStringAny != nil {
		argsData, err = json.Marshal(*args.MapmapOfStringAny)
	} else if args.String != nil {
		argsData, err = json.RawMessage(*args.String), nil
	}
	return
}

func (o *MistralAI) Close(ctx context.Context) error {
	return nil
}
