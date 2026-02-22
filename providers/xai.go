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
	xai "github.com/petmal/mindtrial/pkg/xai"
	"github.com/petmal/mindtrial/providers/tools"
)

// NewXAI creates a new xAI provider instance with the given configuration.
func NewXAI(cfg config.XAIClientConfig, availableTools []config.ToolConfig) (*XAI, error) {
	clientCfg := xai.NewConfiguration()
	clientCfg.AddDefaultHeader("Authorization", "Bearer "+cfg.APIKey)

	// Set xAI API base endpoint.
	clientCfg.Servers = xai.ServerConfigurations{
		{
			URL:         "https://api.x.ai",
			Description: "xAI production API server",
		},
	}

	client := xai.NewAPIClient(clientCfg)
	return &XAI{
		client:         client,
		availableTools: availableTools,
	}, nil
}

// XAI implements the Provider interface for xAI.
type XAI struct {
	client         *xai.APIClient
	availableTools []config.ToolConfig
}

func (o XAI) Name() string {
	return config.XAI
}

func (o *XAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	// Prepare a completion request.
	req := xai.NewChatRequestWithDefaults()
	req.SetModel(cfg.Model)
	req.SetN(1)

	// Clear default penalty parameters to avoid model compatibility issues.
	// Some xAI models don't support these parameters, which would cause request failures.
	// These can be explicitly set later via cfg.ModelParams if needed.
	req.SetPresencePenaltyNil()
	req.SetFrequencyPenaltyNil()

	// Configure default response format.
	if cfg.DisableStructuredOutput {
		req.SetResponseFormat(xai.ResponseFormatOneOfAsResponseFormat(xai.NewResponseFormatOneOf("text")))
	} else {
		responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		schema := map[string]interface{}{
			"schema": responseSchema,
		}
		req.SetResponseFormat(xai.ResponseFormatOneOf2AsResponseFormat(xai.NewResponseFormatOneOf2(schema, "json_schema")))
	}

	// Apply model-specific parameters.
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.XAIModelParams); ok {
			if modelParams.Temperature != nil {
				req.SetTemperature(*modelParams.Temperature)
			}
			if modelParams.TopP != nil {
				req.SetTopP(*modelParams.TopP)
			}
			if modelParams.MaxCompletionTokens != nil {
				req.SetMaxCompletionTokens(*modelParams.MaxCompletionTokens)
			}
			if modelParams.PresencePenalty != nil {
				req.SetPresencePenalty(*modelParams.PresencePenalty)
			}
			if modelParams.FrequencyPenalty != nil {
				req.SetFrequencyPenalty(*modelParams.FrequencyPenalty)
			}
			if modelParams.ReasoningEffort != nil {
				req.SetReasoningEffort(*modelParams.ReasoningEffort)
			}
			if modelParams.Seed != nil {
				req.SetSeed(*modelParams.Seed)
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	// Add system instruction if available.
	if cfg.DisableStructuredOutput {
		sysContent := xai.StringAsContent(xai.PtrString(result.recordPrompt(DefaultUnstructuredResponseInstruction())))
		req.Messages = append(req.Messages, xai.MessageOneOfAsMessage(xai.NewMessageOneOf(sysContent, "system")))
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		sysContent := xai.StringAsContent(xai.PtrString(result.recordPrompt(answerFormatInstruction)))
		req.Messages = append(req.Messages, xai.MessageOneOfAsMessage(xai.NewMessageOneOf(sysContent, "system")))
	}

	// Add structured user messages.
	parts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}

	userContent := xai.ArrayOfContentPartAsContent(&parts)
	req.Messages = append(req.Messages, xai.MessageOneOf1AsMessage(xai.NewMessageOneOf1(userContent, "user")))

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
			// Find the tool config from available tools
			toolCfg, found := findToolByName(o.availableTools, toolName)
			if !found {
				return result, fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
			}
			tool := tools.NewDockerTool(toolCfg, toolSelection.MaxCalls, toolSelection.Timeout, toolSelection.MaxMemoryMB, toolSelection.CpuPercent)
			executor.RegisterTool(tool)
			funcDef := xai.NewFunctionDefinition(toolCfg.Name, toolCfg.Parameters)
			funcDef.SetDescription(toolCfg.Description)
			funcDef.SetStrict(false)
			toolDef := xai.ToolOneOfAsTool(xai.NewToolOneOf(*funcDef, "function"))
			req.Tools = append(req.Tools, toolDef)
		}
		// If user tools are present, allow auto tool choice.
		req.SetToolChoice(xai.ToolChoiceOneOfAsToolChoice(xai.NewToolChoiceOneOf("auto")))
	}

	// Conversation loop to handle tool calls.
	for {
		resp, err := timed(func() (*xai.ChatResponse, error) {
			response, httpResp, err := o.client.V1API.HandleGenericCompletionRequest(ctx).ChatRequest(*req).Execute()
			if err != nil {
				var apiErr *xai.GenericOpenAPIError
				switch {
				case o.isTransientResponse(httpResp):
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

		// Parse the completion response.

		if resp.Usage.IsSet() {
			if u := resp.Usage.Get(); u != nil {
				promptTokens := int64(u.PromptTokens)
				completionTokens := int64(u.CompletionTokens)
				recordUsage(&promptTokens, &completionTokens, &result.usage)
			}
		}
		if len(resp.Choices) == 0 {
			return result, ErrNoResponseCandidates
		}
		for _, candidate := range resp.Choices {
			finishReason := o.getFinishReason(candidate)
			isTerminal := o.isTerminalStopReason(finishReason)

			if !isTerminal {
				// Append assistant message for every non-terminal turn.
				msg := o.newAssistantHistoryMessage(candidate.Message)
				req.Messages = append(req.Messages, xai.MessageOneOf2AsMessage(msg))

				textContent, _ := o.getMessageText(candidate.Message)
				logSkippedPreambleText(ctx, logger, finishReason, textContent)

				// Handle tool calls.
				for _, toolCall := range candidate.Message.ToolCalls {
					args := json.RawMessage(toolCall.Function.Arguments)
					data, err := taskFilesToDataMap(ctx, task.Files)
					if err != nil {
						return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
					}
					toolResult, err := executor.ExecuteTool(ctx, logger, toolCall.Function.Name, args, data)
					content := string(toolResult)
					if err != nil {
						content = formatToolExecutionError(err)
					}
					toolMessage := xai.NewMessageOneOf3(xai.StringAsContent(&content), "tool")
					toolMessage.SetToolCallId(toolCall.Id)
					req.Messages = append(req.Messages, xai.MessageOneOf3AsMessage(toolMessage))
				}
			} else {
				if textContent, ok := o.getMessageText(candidate.Message); ok {
					if cfg.DisableStructuredOutput {
						err = UnmarshalUnstructuredResponse(ctx, logger, []byte(textContent), &result)
					} else {
						err = json.Unmarshal([]byte(textContent), &result)
					}
					if err != nil {
						return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(finishReason))
					}
					return result, nil
				}

				return result, NewErrNoActionableContent([]byte(finishReason))
			}
		}
	} // move to the next conversation turn
}

func (o *XAI) isTerminalStopReason(stopReason string) bool {
	return !slices.Contains([]string{"", "tool_calls"}, stopReason)
}

func (o *XAI) getFinishReason(candidate xai.Choice) (finishReason string) {
	if candidate.FinishReason.IsSet() {
		if value := candidate.FinishReason.Get(); value != nil {
			finishReason = *value
		}
	}
	return
}

func (o *XAI) newAssistantHistoryMessage(message xai.ChoiceMessage) *xai.MessageOneOf2 {
	msg := xai.NewMessageOneOf2(message.Role)

	if contentPtr, hasContent := message.GetContentOk(); hasContent {
		if contentPtr == nil {
			msg.SetContentNil()
		} else {
			msg.SetContent(xai.StringAsContent(contentPtr))
		}
	}

	if reasoningContent, hasReasoningContent := message.GetReasoningContentOk(); hasReasoningContent {
		if reasoningContent == nil {
			msg.SetReasoningContentNil()
		} else {
			msg.SetReasoningContent(*reasoningContent)
		}
	}

	if toolCalls, hasToolCalls := message.GetToolCallsOk(); hasToolCalls {
		msg.SetToolCalls(toolCalls)
	}

	return msg
}

func (o *XAI) getMessageText(message xai.ChoiceMessage) (text string, ok bool) {
	if contentPtr, hasContent := message.GetContentOk(); hasContent && contentPtr != nil {
		text = *contentPtr
		ok = text != ""
	}
	return
}

func (o *XAI) isTransientResponse(response *http.Response) bool {
	return response != nil && slices.Contains([]int{
		http.StatusTooManyRequests,
		http.StatusRequestTimeout,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}, response.StatusCode)
}

func (o *XAI) Close(ctx context.Context) error {
	return nil
}

func (o *XAI) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []xai.ContentPart, err error) {
	for _, file := range files {
		if fileType, err := file.TypeValue(ctx); err != nil {
			return parts, err
		} else if !o.isSupportedImageType(fileType) {
			return parts, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
		}

		dataURL, err := file.GetDataURL(ctx)
		if err != nil {
			return parts, err
		}

		// Add filename as a text part before the image.
		cpText := xai.NewContentPart("text")
		cpText.SetText(result.recordPrompt(DefaultTaskFileNameInstruction(file)))
		parts = append(parts, *cpText)

		// Add image data part.
		imgCp := xai.NewContentPart("image_url")
		imgCp.SetImageUrl(*xai.NewImageUrl(dataURL))
		parts = append(parts, *imgCp)
	}

	// Append the prompt text after the file data for improved context integrity.
	cpFinal := xai.NewContentPart("text")
	cpFinal.SetText(result.recordPrompt(promptText))
	parts = append(parts, *cpFinal)

	return parts, nil
}

// isSupportedImageType checks if the provided MIME type is supported by the xAI image understanding API.
// For more information, see: https://docs.x.ai/docs/guides/image-understanding
func (o XAI) isSupportedImageType(mimeType string) bool {
	return slices.Contains([]string{
		"image/jpeg",
		"image/jpg",
		"image/png",
	}, strings.ToLower(mimeType))
}
