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

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/petmal/mindtrial/config"
)

const responseFormatterToolName = "record_summary"

// NewAnthropic creates a new Anthropic provider instance with the given configuration.
func NewAnthropic(cfg config.AnthropicClientConfig) *Anthropic {
	return &Anthropic{
		client: anthropic.NewClient(anthropicoption.WithAPIKey(cfg.APIKey)),
	}
}

// Anthropic implements the Provider interface for Anthropic generative models.
type Anthropic struct {
	client *anthropic.Client
}

func (o Anthropic) Name() string {
	return config.ANTHROPIC
}

func (o Anthropic) Validator(expected string) Validator {
	return NewDefaultValidator(expected)
}

func (o *Anthropic) Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error) {
	request := anthropic.MessageNewParams{
		MaxTokens: anthropic.Int(2048),
		Model:     anthropic.F(cfg.Model),
		System: anthropic.F([]anthropic.TextBlockParam{
			anthropic.NewTextBlock(DefaultAnswerFormatInstruction(task)),
		}),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(task.Prompt)),
		}),
		Tools: anthropic.F([]anthropic.ToolUnionUnionParam{
			anthropic.ToolParam{
				Name:        anthropic.F(responseFormatterToolName),
				Description: anthropic.F("Record the response using well-structured JSON."),
				InputSchema: anthropic.F(ResultJSONSchema()),
			},
		}),
		ToolChoice: anthropic.F(anthropic.ToolChoiceUnionParam(anthropic.ToolChoiceToolParam{
			Name: anthropic.F(responseFormatterToolName),
			Type: anthropic.F(anthropic.ToolChoiceToolTypeTool),
		})),
	}
	resp, err := timed(func() (*anthropic.Message, error) {
		return o.client.Messages.New(ctx, request)
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	for _, block := range resp.Content {
		if block.Type == anthropic.ContentBlockTypeToolUse {
			if block.Name == responseFormatterToolName {
				if err = json.Unmarshal(block.Input, &result); err != nil {
					return result, NewErrUnmarshalResponse(err, block.Input, []byte(resp.StopReason))
				}
				break
			}
		}
	}

	return result, nil
}

func (o *Anthropic) Close(ctx context.Context) error {
	return nil
}
