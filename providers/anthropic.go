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
	client anthropic.Client
}

func (o Anthropic) Name() string {
	return config.ANTHROPIC
}

func (o Anthropic) Validator(expected string) Validator {
	return NewDefaultValidator(expected)
}

func (o *Anthropic) Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error) {
	request := anthropic.MessageNewParams{
		MaxTokens: 2048,
		Model:     cfg.Model,
		System: []anthropic.TextBlockParam{
			{
				Text: result.recordPrompt(DefaultAnswerFormatInstruction(task)),
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(result.recordPrompt(task.Prompt))),
		},
		Tools: []anthropic.ToolUnionParam{
			{
				OfTool: &anthropic.ToolParam{
					Name:        responseFormatterToolName,
					Description: anthropic.String("Record the response using well-structured JSON."),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: ResultJSONSchema().Properties,
					},
				},
			},
		},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfToolChoiceTool: &anthropic.ToolChoiceToolParam{
				Name: responseFormatterToolName,
			},
		},
	}
	resp, err := timed(func() (*anthropic.Message, error) {
		return o.client.Messages.New(ctx, request)
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	if resp != nil {
		recordUsage(&resp.Usage.InputTokens, &resp.Usage.OutputTokens, &result.usage)
		for _, block := range resp.Content {
			switch block := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				if block.Name == responseFormatterToolName {
					if err = json.Unmarshal(block.Input, &result); err != nil {
						return result, NewErrUnmarshalResponse(err, block.Input, []byte(resp.StopReason))
					}
					break
				}
			}
		}
	}

	return result, nil
}

func (o *Anthropic) Close(ctx context.Context) error {
	return nil
}
