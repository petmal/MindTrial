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
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/providers/tools"
)

const responseFormatterToolName = "record_summary"
const defaultMaxTokens = 2048

// NewAnthropic creates a new Anthropic provider instance with the given configuration.
func NewAnthropic(cfg config.AnthropicClientConfig, availableTools []config.ToolConfig) *Anthropic {
	opts := []anthropicoption.RequestOption{anthropicoption.WithAPIKey(cfg.APIKey)}
	if cfg.RequestTimeout != nil {
		opts = append(opts, anthropicoption.WithRequestTimeout(*cfg.RequestTimeout))
	}
	return &Anthropic{
		client:         anthropic.NewClient(opts...),
		availableTools: availableTools,
	}
}

// Anthropic implements the Provider interface for Anthropic generative models.
type Anthropic struct {
	client         anthropic.Client
	availableTools []config.ToolConfig
}

func (o Anthropic) Name() string {
	return config.ANTHROPIC
}

func (o *Anthropic) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	schema, err := ResultJSONSchema(task.ResponseResultFormat)
	if err != nil {
		return result, err
	}

	request := anthropic.MessageNewParams{
		MaxTokens: defaultMaxTokens,
		Model:     anthropic.Model(cfg.Model),
		Tools: []anthropic.ToolUnionParam{
			{
				OfTool: &anthropic.ToolParam{
					Name:        responseFormatterToolName,
					Description: anthropic.String("Record the response using well-structured JSON."),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: schema.Properties,
						Required:   schema.Required,
					},
				},
			},
		},
		ToolChoice: anthropic.ToolChoiceParamOfTool(responseFormatterToolName),
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
			toolInputSchema, err := MapToJSONSchema(toolCfg.Parameters)
			if err != nil {
				return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
			}
			request.Tools = append(request.Tools, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        toolCfg.Name,
					Description: anthropic.String(toolCfg.Description),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: toolInputSchema.Properties,
						Required:   toolInputSchema.Required,
					},
				},
			})
		}
		// If user tools are present, allow auto tool choice.
		request.ToolChoice = anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		request.System = []anthropic.TextBlockParam{
			{
				Text: result.recordPrompt(answerFormatInstruction),
			},
		}
	}
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.AnthropicModelParams); ok {
			if modelParams.MaxTokens != nil {
				request.MaxTokens = *modelParams.MaxTokens
			}
			if modelParams.ThinkingBudgetTokens != nil {
				request.Thinking = anthropic.ThinkingConfigParamOfEnabled(*modelParams.ThinkingBudgetTokens)
				// Thinking may not be enabled when tool_choice forces tool use.
				// Use Auto instead.
				request.ToolChoice = anthropic.ToolChoiceUnionParam{
					OfAuto: &anthropic.ToolChoiceAutoParam{},
				}
			}
			if modelParams.Temperature != nil {
				request.Temperature = anthropic.Float(*modelParams.Temperature)
			}
			if modelParams.TopP != nil {
				request.TopP = anthropic.Float(*modelParams.TopP)
			}
			if modelParams.TopK != nil {
				request.TopK = anthropic.Int(*modelParams.TopK)
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	if promptParts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result); err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	} else {
		request.Messages = []anthropic.MessageParam{
			anthropic.NewUserMessage(promptParts...),
		}
	}

	// Conversation loop to handle tool calls.
	for {
		resp, err := timed(func() (*anthropic.Message, error) {
			return o.client.Messages.New(ctx, request)
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		if err != nil {
			return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		recordUsage(&resp.Usage.InputTokens, &resp.Usage.OutputTokens, &result.usage)

		// Append assistant message to conversation history before handling tool calls.
		request.Messages = append(request.Messages, resp.ToParam())

		for _, block := range resp.Content {
			switch block := block.AsAny().(type) { //nolint:gocritic
			case anthropic.ToolUseBlock:
				// No tool calls, this is the final response.
				if block.Name == responseFormatterToolName {
					if err = json.Unmarshal(block.Input, &result); err != nil {
						return result, NewErrUnmarshalResponse(err, block.Input, []byte(resp.StopReason))
					}
					return result, nil
				}

				// Handle tool calls.
				toolResult, err := executor.ExecuteTool(ctx, logger, block.Name, json.RawMessage(block.Input))
				isError := err != nil
				content := string(toolResult)
				if isError {
					content = formatToolExecutionError(err)
				}
				// Tool results must be sent in a user message.
				request.Messages = append(request.Messages, anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(block.ID, content, isError),
				))
			}
		}
	} // move to the next conversation turn
}

func (o *Anthropic) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []anthropic.ContentBlockParamUnion, err error) {
	for _, file := range files {
		fileType, err := file.TypeValue(ctx)
		if err != nil {
			return nil, err
		} else if !isSupportedImageType(fileType) {
			return nil, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
		}

		base64Data, err := file.Base64(ctx)
		if err != nil {
			return nil, err
		}

		// Attach file name as a text block before the image.
		parts = append(parts, anthropic.NewTextBlock(result.recordPrompt(DefaultTaskFileNameInstruction(file))))
		parts = append(parts, anthropic.NewImageBlockBase64(fileType, base64Data))
	}

	parts = append(parts, anthropic.NewTextBlock(result.recordPrompt(promptText))) // append the prompt text after the file data for improved context integrity

	return parts, nil
}

func (o *Anthropic) Close(ctx context.Context) error {
	return nil
}
