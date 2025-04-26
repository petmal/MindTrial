// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"fmt"

	deepseek "github.com/cohesion-org/deepseek-go"
	"github.com/petmal/mindtrial/config"
	"golang.org/x/exp/constraints"
)

// NewDeepseek creates a new Deepseek provider instance with the given configuration.
func NewDeepseek(cfg config.DeepseekClientConfig) (*Deepseek, error) {
	opts := make([]deepseek.Option, 0)
	if cfg.RequestTimeout != nil {
		opts = append(opts, deepseek.WithTimeout(*cfg.RequestTimeout))
	}
	client, err := deepseek.NewClientWithOptions(cfg.APIKey, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateClient, err)
	}
	return &Deepseek{
		client: client,
	}, nil
}

// Deepseek implements the Provider interface for Deepseek generative models.
type Deepseek struct {
	client *deepseek.Client
}

func (o Deepseek) Name() string {
	return config.DEEPSEEK
}

func (o Deepseek) Validator(expected string) Validator {
	return NewDefaultValidator(expected)
}

func (o *Deepseek) Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error) {
	responseFormatInstruction := result.recordPrompt(DefaultResponseFormatInstruction()) // NOTE: required with JSONMode
	answerFormatInstruction := result.recordPrompt(DefaultAnswerFormatInstruction(task))
	prompt := result.recordPrompt(task.Prompt)

	var request any
	if len(task.Files) > 0 {
		if !o.isFileUploadSupported() {
			return result, fmt.Errorf("%w: %s", ErrFeatureNotSupported, "file upload")
		}

		promptParts, err := o.createPromptMessageParts(ctx, prompt, task.Files, &result)
		if err != nil {
			return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
		}

		request = &deepseek.ChatCompletionRequestWithImage{
			Model: cfg.Model,
			Messages: []deepseek.ChatCompletionMessageWithImage{
				{Role: deepseek.ChatMessageRoleSystem, Content: responseFormatInstruction},
				{Role: deepseek.ChatMessageRoleSystem, Content: answerFormatInstruction},
				{Role: deepseek.ChatMessageRoleUser, Content: promptParts},
			},
			JSONMode: true,
		}
	} else {
		request = &deepseek.ChatCompletionRequest{
			Model: cfg.Model,
			Messages: []deepseek.ChatCompletionMessage{
				{Role: deepseek.ChatMessageRoleSystem, Content: responseFormatInstruction},
				{Role: deepseek.ChatMessageRoleSystem, Content: answerFormatInstruction},
				{Role: deepseek.ChatMessageRoleUser, Content: prompt},
			},
			JSONMode: true,
		}
	}

	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.DeepseekModelParams); ok {
			o.applyModelParameters(request, modelParams)
		}
	}

	resp, err := timed(func() (*deepseek.ChatCompletionResponse, error) {
		return o.createChatCompletion(ctx, request)
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	if resp != nil {
		recordUsage(&resp.Usage.PromptTokens, &resp.Usage.CompletionTokens, &result.usage)
		if len(resp.Choices) > 0 {
			if err := deepseek.NewJSONExtractor(nil).ExtractJSON(resp, &result); err != nil {
				return result, NewErrUnmarshalResponse(err, []byte(resp.Choices[0].Message.Content), []byte(resp.Choices[0].FinishReason))
			}
		}
	}

	return result, nil
}

func (o *Deepseek) isFileUploadSupported() bool {
	return false // NOTE: Deepseek API does not support file upload in the current version.
}

func (o *Deepseek) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []deepseek.ContentItem, err error) {
	parts = append(parts, deepseek.ContentItem{
		Type: "text",
		Text: promptText,
	})

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
		parts = append(parts, deepseek.ContentItem{
			Type: "text",
			Text: result.recordPrompt(DefaultTaskFileNameInstruction(file)),
		})
		parts = append(parts, deepseek.ContentItem{
			Type: "image",
			Image: &deepseek.ImageContent{
				URL: dataURL,
			},
		})
	}

	return parts, nil
}

func (o *Deepseek) applyModelParameters(request any, modelParams config.DeepseekModelParams) {
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		setIfNotNil(&req.Temperature, modelParams.Temperature)
		setIfNotNil(&req.TopP, modelParams.TopP)
		setIfNotNil(&req.FrequencyPenalty, modelParams.FrequencyPenalty)
		setIfNotNil(&req.PresencePenalty, modelParams.PresencePenalty)
	case *deepseek.ChatCompletionRequestWithImage:
		setIfNotNil(&req.Temperature, modelParams.Temperature)
		setIfNotNil(&req.TopP, modelParams.TopP)
		setIfNotNil(&req.FrequencyPenalty, modelParams.FrequencyPenalty)
		setIfNotNil(&req.PresencePenalty, modelParams.PresencePenalty)
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
}

func setIfNotNil[T constraints.Float](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func (o *Deepseek) createChatCompletion(ctx context.Context, request any) (response *deepseek.ChatCompletionResponse, err error) {
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		response, err = o.client.CreateChatCompletion(ctx, req)
	case *deepseek.ChatCompletionRequestWithImage:
		response, err = o.client.CreateChatCompletionWithImage(ctx, req)
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
	return
}

func (o *Deepseek) Close(ctx context.Context) error {
	return nil
}
