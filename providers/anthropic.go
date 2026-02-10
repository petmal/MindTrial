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

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/providers/tools"
)

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
	request := anthropic.MessageNewParams{
		MaxTokens: defaultMaxTokens,
		Model:     anthropic.Model(cfg.Model),
	}

	// Configure native structured output via output_config.format when not disabled.
	if !cfg.DisableStructuredOutput {
		responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		request.OutputConfig.Format = anthropic.JSONOutputFormatParam{
			Schema: responseSchema,
		}
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

	if cfg.DisableStructuredOutput {
		request.System = append(request.System, anthropic.TextBlockParam{
			Text: result.recordPrompt(DefaultUnstructuredResponseInstruction()),
		})
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		request.System = append(request.System, anthropic.TextBlockParam{
			Text: result.recordPrompt(answerFormatInstruction),
		})
	}
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.AnthropicModelParams); ok {
			if modelParams.MaxTokens != nil {
				request.MaxTokens = *modelParams.MaxTokens
			}
			if modelParams.Effort != nil || modelParams.ThinkingBudgetTokens != nil {
				if modelParams.Effort != nil {
					// Adaptive thinking: Claude dynamically allocates reasoning depth.
					adaptive := anthropic.NewThinkingConfigAdaptiveParam()
					request.Thinking = anthropic.ThinkingConfigParamUnion{
						OfAdaptive: &adaptive,
					}
					request.OutputConfig.Effort = anthropic.OutputConfigEffort(*modelParams.Effort)
				} else {
					// Fixed budget: allocates a set number of tokens for reasoning.
					request.Thinking = anthropic.ThinkingConfigParamOfEnabled(*modelParams.ThinkingBudgetTokens)
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

	promptParts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}
	request.Messages = []anthropic.MessageParam{
		anthropic.NewUserMessage(promptParts...),
	}

	// Conversation loop to handle tool calls.
	for {
		resp, err := timed(func() (*anthropic.Message, error) {
			return o.client.Messages.New(ctx, request)
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		if err != nil {
			return result, WrapErrGenerateResponse(err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		recordUsage(&resp.Usage.InputTokens, &resp.Usage.OutputTokens, &result.usage)

		// Append assistant message to conversation history before processing content blocks.
		request.Messages = append(request.Messages, resp.ToParam())

		// Collect tool results from this turn. When user tools are invoked, all tool results
		// are gathered and sent together in a single user message to maintain valid
		// conversation structure (assistant message followed by user message with tool results).
		var toolResults []anthropic.ContentBlockParamUnion

		for _, block := range resp.Content {
			switch block := block.AsAny().(type) { //nolint:gocritic
			case anthropic.TextBlock:
				if cfg.DisableStructuredOutput {
					if err = UnmarshalUnstructuredResponse(ctx, logger, []byte(block.Text), &result); err != nil {
						return result, NewErrUnmarshalResponse(err, []byte(block.Text), []byte(resp.StopReason))
					}
					return result, nil
				}
				// In structured output mode, the text block contains the JSON response
				// produced by output_config.format constrained decoding.
				if err = json.Unmarshal([]byte(block.Text), &result); err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(block.Text), []byte(resp.StopReason))
				}
				return result, nil

			case anthropic.ToolUseBlock:
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				toolResult, err := executor.ExecuteTool(ctx, logger, block.Name, json.RawMessage(block.Input), data)
				isError := err != nil
				content := string(toolResult)
				if isError {
					content = formatToolExecutionError(err)
				}
				toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, content, isError))
			}
		}

		// If tool results were collected, send them in a single user message.
		if len(toolResults) > 0 {
			request.Messages = append(request.Messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		// No actionable content was found: no parseable text block and no tool calls.
		// This can occur when the model exhausts its token budget on thinking without
		// producing a text response.
		return result, fmt.Errorf("%w: model response contained no actionable content (stop_reason: %s)",
			ErrGenerateResponse, resp.StopReason,
		)
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
