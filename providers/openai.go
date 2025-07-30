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
	"github.com/petmal/mindtrial/pkg/utils"
	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// NewOpenAI creates a new OpenAI provider instance with the given configuration.
func NewOpenAI(cfg config.OpenAIClientConfig) *OpenAI {
	return &OpenAI{
		client: openai.NewClient(cfg.APIKey),
	}
}

// OpenAI implements the Provider interface for OpenAI generative models.
type OpenAI struct {
	client *openai.Client
}

func (o OpenAI) Name() string {
	return config.OPENAI
}

func (o *OpenAI) Run(ctx context.Context, _ logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	schema, err := jsonschema.GenerateSchemaForType(&result)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCompileSchema, err)
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
			if modelParams.TextResponseFormat {
				request.ResponseFormat = &openai.ChatCompletionResponseFormat{
					Type: openai.ChatCompletionResponseFormatTypeText,
				}
				request.Messages = append(request.Messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser, // NOTE: system role not supported by all models
					Content: result.recordPrompt(DefaultResponseFormatInstruction()),
				})
			}
			if modelParams.Temperature != nil {
				request.Temperature = *modelParams.Temperature
			}
			if modelParams.TopP != nil {
				request.TopP = *modelParams.TopP
			}
			if modelParams.PresencePenalty != nil {
				request.PresencePenalty = *modelParams.PresencePenalty
			}
			if modelParams.FrequencyPenalty != nil {
				request.FrequencyPenalty = *modelParams.FrequencyPenalty
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

	request.Messages = append(request.Messages,
		openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser, // NOTE: system role not supported by all models
			Content: result.recordPrompt(DefaultAnswerFormatInstruction(task)),
		})

	resp, err := timed(func() (openai.ChatCompletionResponse, error) {
		return o.client.CreateChatCompletion(ctx, request)
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	recordUsage(&resp.Usage.PromptTokens, &resp.Usage.CompletionTokens, &result.usage)
	for _, candidate := range resp.Choices {
		content := candidate.Message.Content
		if request.ResponseFormat.Type == openai.ChatCompletionResponseFormatTypeText {
			content, err = utils.RepairTextJSON(content)
			if err != nil {
				return result, NewErrUnmarshalResponse(err, []byte(candidate.Message.Content), []byte(candidate.FinishReason))
			}
		}
		if err = schema.Unmarshal(content, &result); err != nil {
			return result, NewErrUnmarshalResponse(err, []byte(candidate.Message.Content), []byte(candidate.FinishReason))
		}
	}

	return result, nil
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

func (o *OpenAI) Close(ctx context.Context) error {
	return nil
}
