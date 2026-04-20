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

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/providers/tools"
)

const defaultMaxTokens = 2048
const submitResponseToolName = "submit_response"

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
	var useStreaming bool
	var useLegacyStructuredOutput bool
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.AnthropicModelParams); ok {
			if modelParams.MaxTokens != nil {
				request.MaxTokens = *modelParams.MaxTokens
			}
			if modelParams.Effort != nil || modelParams.ThinkingBudgetTokens != nil {
				if modelParams.Effort != nil {
					// Adaptive thinking: Claude dynamically allocates reasoning depth.
					request.Thinking = anthropic.ThinkingConfigParamUnion{
						OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
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
			useStreaming = modelParams.Stream
			useLegacyStructuredOutput = modelParams.LegacyStructuredOutput && !cfg.DisableStructuredOutput
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	// Configure structured output mode.
	if !cfg.DisableStructuredOutput {
		if useLegacyStructuredOutput {
			// Validate that no user tool conflicts with the internal response tool name.
			if slices.ContainsFunc(request.Tools, func(t anthropic.ToolUnionParam) bool {
				return t.OfTool != nil && t.OfTool.Name == submitResponseToolName
			}) {
				return result, fmt.Errorf("%w: tool name %q is reserved for legacy structured output", ErrToolSetup, submitResponseToolName)
			}
			// Tool-based structured output: register a submit_response tool with the
			// result schema as its input schema. This works around models that have difficulty
			// producing valid responses with native JSON schema output when extended thinking
			// is enabled.
			responseSchema, err := ResultJSONSchema(task.ResponseResultFormat)
			if err != nil {
				return result, err
			}
			request.Tools = append(request.Tools, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        submitResponseToolName,
					Description: anthropic.String("Submit your final response to the task. Call this tool exactly once when you have determined your answer. Do not call it for any other purpose."),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: responseSchema.Properties,
						Required:   responseSchema.Required,
					},
				},
			})
			request.System = append(request.System, anthropic.TextBlockParam{
				Text: result.recordPrompt("Always use the " + submitResponseToolName + " tool to submit your final response. Do not write the response as plain text."),
			})
			// Ensure auto tool choice is set for the response tool.
			request.ToolChoice = anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			}
		} else {
			// Native structured output via output_config.format constrained decoding.
			responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
			if err != nil {
				return result, err
			}
			request.OutputConfig.Format = anthropic.JSONOutputFormatParam{
				Schema: responseSchema,
			}
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
	var turn int
	for {
		turn++
		if err := AssertTurnsAvailable(ctx, logger, task, turn); err != nil {
			return result, err
		}

		resp, err := timed(func() (*anthropic.Message, error) {
			response, err := o.handleRequest(ctx, request, useStreaming)
			if err != nil && o.isTransientResponse(err) {
				return response, WrapErrRetryable(err)
			}
			return response, err
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		if err != nil {
			return result, WrapErrGenerateResponse(err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		recordUsage(&resp.Usage.InputTokens, &resp.Usage.OutputTokens, &result.usage)
		isTerminal := o.isTerminalStopReason(resp.StopReason)
		logFinishReason(ctx, logger, string(resp.StopReason), isTerminal)

		// Append assistant message to conversation history before processing content blocks.
		request.Messages = append(request.Messages, sanitizeAssistantMessage(resp.ToParam()))

		// Collect tool results from this turn. When user tools are invoked, all tool results
		// are gathered and sent together in a single user message to maintain valid
		// conversation structure (assistant message followed by user message with tool results).
		var toolResults []anthropic.ContentBlockParamUnion
		var terminalTextBuilder strings.Builder

		for _, block := range resp.Content {
			switch block := block.AsAny().(type) {
			case anthropic.TextBlock:
				if isTerminal {
					terminalTextBuilder.WriteString(block.Text)
				} else {
					// For non-terminal turns, text blocks are preambles before additional
					// assistant output/tool requests in subsequent turns.
					logSkippedPreambleText(ctx, logger, string(resp.StopReason), block.Text)
				}

			case anthropic.ToolUseBlock:
				// Intercept the response tool in legacy structured output mode.
				if useLegacyStructuredOutput && block.Name == submitResponseToolName {
					if err := json.Unmarshal([]byte(block.Input), &result); err != nil {
						return result, NewErrUnmarshalResponse(err, []byte(block.Input), []byte(resp.StopReason))
					}
					return result, nil
				}
				if !isTerminal {
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

			default:
				logger.Message(ctx, logging.LevelTrace, "unhandled content block type: %T", block)
			}
		}

		if isTerminal {
			if textContent := terminalTextBuilder.String(); textContent != "" {
				if cfg.DisableStructuredOutput {
					err = UnmarshalUnstructuredResponse(ctx, logger, []byte(textContent), &result)
				} else {
					// In structured output mode, text blocks contain the JSON response
					// produced by output_config.format constrained decoding.
					err = json.Unmarshal([]byte(textContent), &result)
				}
				if err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(resp.StopReason))
				}
				return result, nil
			}

			// No actionable content was found: no parseable text block and no tool calls.
			// Return an error only when the model has clearly terminated. Otherwise,
			// continue the conversation loop and ask for forgiveness.
			return result, NewErrNoActionableContent([]byte(resp.StopReason))
		}

		// If tool results were collected, send them in a single user message.
		if len(toolResults) > 0 {
			request.Messages = append(request.Messages, anthropic.NewUserMessage(toolResults...))
		}
	} // move to the next conversation turn
}

// sanitizeAssistantMessage removes empty text blocks from a MessageParam.
// This works around a known SDK bug where resp.ToParam() preserves empty text
// blocks that the API rejects with 400 "text content blocks must be non-empty"
// on subsequent requests.
// See: https://github.com/anthropics/anthropic-sdk-go/issues/242
func sanitizeAssistantMessage(msg anthropic.MessageParam) anthropic.MessageParam {
	filtered := msg.Content[:0]
	for _, block := range msg.Content {
		if block.OfText != nil && block.OfText.Text == "" {
			continue
		}
		filtered = append(filtered, block)
	}
	msg.Content = filtered
	return msg
}

func (o *Anthropic) isTerminalStopReason(stopReason anthropic.StopReason) bool {
	var undefined anthropic.StopReason
	return !slices.Contains([]anthropic.StopReason{
		undefined,
		anthropic.StopReasonToolUse,
		anthropic.StopReasonPauseTurn,
	}, stopReason)
}

// handleRequest dispatches the request to the appropriate handler based on streaming mode.
func (o *Anthropic) handleRequest(ctx context.Context, request anthropic.MessageNewParams, stream bool) (*anthropic.Message, error) {
	if stream {
		return o.handleStreamingRequest(ctx, request)
	}
	return o.client.Messages.New(ctx, request)
}

// handleStreamingRequest executes a streaming message request, buffering all events
// into a single [anthropic.Message] via the SDK's Accumulate method.
// Streaming is recommended for requests with large MaxTokens values, especially
// when extended thinking is enabled, to prevent HTTP timeouts on long-running requests.
func (o *Anthropic) handleStreamingRequest(ctx context.Context, request anthropic.MessageNewParams) (*anthropic.Message, error) {
	stream := o.client.Messages.NewStreaming(ctx, request)
	defer stream.Close()

	message := anthropic.Message{}
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return nil, ErrStreamResponse
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return &message, nil
}

// isTransientResponse checks whether the error represents a transient condition
// that the retry policy should attempt again.
func (o *Anthropic) isTransientResponse(err error) bool {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return slices.Contains([]int{
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusServiceUnavailable,
		}, apiErr.StatusCode)
	} else if errors.Is(err, ErrStreamResponse) {
		return true
	}
	return false
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
