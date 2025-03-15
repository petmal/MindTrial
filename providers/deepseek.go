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
)

// NewDeepseek creates a new Deepseek provider instance with the given configuration.
func NewDeepseek(cfg config.DeepseekClientConfig) *Deepseek {
	return &Deepseek{
		client: deepseek.NewClient(cfg.APIKey),
	}
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
	request := &deepseek.ChatCompletionRequest{
		Model: cfg.Model,
		Messages: []deepseek.ChatCompletionMessage{
			{Role: deepseek.ChatMessageRoleSystem, Content: DefaultResponseFormatInstruction()}, // NOTE: required with JSONMode
			{Role: deepseek.ChatMessageRoleSystem, Content: DefaultAnswerFormatInstruction(task)},
			{Role: deepseek.ChatMessageRoleUser, Content: task.Prompt},
		},
		JSONMode: true,
	}

	resp, err := timed(func() (*deepseek.ChatCompletionResponse, error) {
		return o.client.CreateChatCompletion(ctx, request)
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	if resp != nil && len(resp.Choices) > 0 {
		if err := deepseek.NewJSONExtractor(nil).ExtractJSON(resp, &result); err != nil {
			return result, NewErrUnmarshalResponse(err, []byte(resp.Choices[0].Message.Content), []byte(resp.Choices[0].FinishReason))
		}
	}

	return result, nil
}

func (o *Deepseek) Close(ctx context.Context) error {
	return nil
}
