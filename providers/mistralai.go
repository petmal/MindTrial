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

	"github.com/petmal/mindtrial/config"
	mistralai "github.com/petmal/mindtrial/pkg/mistralai"
	"github.com/petmal/mindtrial/pkg/utils"
)

// NewMistralAI creates a new Mistral AI provider instance with the given configuration.
func NewMistralAI(cfg config.MistralAIClientConfig) (*MistralAI, error) {
	clientCfg := mistralai.NewConfiguration()
	clientCfg.AddDefaultHeader("Authorization", "Bearer "+cfg.APIKey)

	client := mistralai.NewAPIClient(clientCfg)
	return &MistralAI{
		client: client,
	}, nil
}

// MistralAI implements the Provider interface for Mistral AI generative models.
type MistralAI struct {
	client *mistralai.APIClient
}

func (o MistralAI) Name() string {
	return config.MISTRALAI
}

func (o MistralAI) Validator(expected utils.StringSet, validationRules config.ValidationRules) Validator {
	return NewDefaultValidator(expected, validationRules)
}

func (o *MistralAI) Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error) {
	if len(task.Files) > 0 {
		if !o.isFileUploadSupported() {
			return result, ErrFileUploadNotSupported
		}
	}

	request := mistralai.NewChatCompletionRequestWithDefaults()
	request.SetModel(cfg.Model)
	request.SetN(1)

	schema := mistralai.NewJsonSchema("response", ResultJSONSchemaRaw())
	schema.SetDescription(mistralai.Description{
		String: mistralai.PtrString("Record the response using well-structured JSON."),
	})

	responseFormat := mistralai.NewResponseFormatWithDefaults()
	responseFormat.SetType(mistralai.JSON_SCHEMA)
	responseFormat.SetJsonSchema(*schema)
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

	request.Messages = append(request.Messages, mistralai.SystemMessageAsChatCompletionRequestMessagesInner(
		mistralai.NewSystemMessage(mistralai.Content4{
			String: mistralai.PtrString(result.recordPrompt(DefaultAnswerFormatInstruction(task))),
		})))

	request.Messages = append(request.Messages, mistralai.UserMessageAsChatCompletionRequestMessagesInner(
		mistralai.NewUserMessage(*mistralai.NewNullableContent3(&mistralai.Content3{
			String: mistralai.PtrString(result.recordPrompt(task.Prompt)),
		}))))

	resp, err := timed(func() (*mistralai.ChatCompletionResponse, error) {
		response, _, err := o.client.ChatAPI.ChatCompletionV1ChatCompletionsPost(ctx).ChatCompletionRequest(*request).Execute()
		return response, err
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	if resp != nil {
		if resp.Usage != nil {
			recordUsage(&resp.Usage.PromptTokens, &resp.Usage.CompletionTokens, &result.usage)
		}

		for _, candidate := range resp.Choices {
			if message, ok := candidate.Message.GetContentOk(); ok {
				if message != nil && message.String != nil {
					content := *message.String
					if err := json.Unmarshal([]byte(content), &result); err != nil {
						return result, NewErrUnmarshalResponse(err, []byte(content), []byte(candidate.FinishReason))
					}
				}
			}
		}
	}

	return result, nil
}

func (o *MistralAI) isFileUploadSupported() bool {
	return false // NOTE: Mistral AI API does not support file upload in the basic chat completion endpoint.
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

func (o *MistralAI) Close(ctx context.Context) error {
	return nil
}
