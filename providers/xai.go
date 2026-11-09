// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	xai "github.com/petmal/mindtrial/pkg/xai"
	"github.com/petmal/mindtrial/providers/tools"
)

// xaiResponsesPath is the path of the Responses API endpoint relative to the server URL.
const xaiResponsesPath = "/v1/responses"

// xaiEncryptedReasoningInclude requests encrypted reasoning items in the response so they can be
// replayed to the model when the conversation history is managed by the client.
// See: https://docs.x.ai/developers/model-capabilities/text/reasoning
const xaiEncryptedReasoningInclude = "reasoning.encrypted_content"

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

	return &XAI{
		clientCfg:      clientCfg,
		availableTools: availableTools,
	}, nil
}

// XAI implements the Provider interface for xAI.
type XAI struct {
	clientCfg      *xai.Configuration
	availableTools []config.ToolConfig
}

func (o XAI) Name() string {
	return config.XAI
}

func (o *XAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task, environment ExecutionEnvironment) (result Result, err error) {
	// Prepare a Responses API request.
	request := xai.NewModelRequestWithDefaults()
	// xAI rejects the generated background default, even when it is false.
	request.UnsetBackground()
	request.SetModel(cfg.Model)

	// Automatically derive a stable prompt cache key from the run configuration.
	// This requires no user configuration and improves prompt cache hit-rate
	// consistency whenever request content happens to be cacheable; it is a
	// routing hint only and has no effect otherwise.
	request.SetPromptCacheKey(promptCacheKeyFor(cfg))

	// Configure default response format.
	responseConfiguration := xai.NewModelResponseConfiguration()
	if cfg.DisableStructuredOutput {
		responseConfiguration.SetFormat(xai.ModelResponseFormatOneOfAsModelResponseFormat(xai.NewModelResponseFormatOneOf("text")))
	} else {
		responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		schemaFormat := xai.NewModelResponseFormatOneOf2(responseSchema, "json_schema")
		schemaFormat.SetName("response")
		responseConfiguration.SetFormat(xai.ModelResponseFormatOneOf2AsModelResponseFormat(schemaFormat))
	}
	request.SetText(*responseConfiguration)

	// Apply model-specific parameters.
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.XAIModelParams); ok {
			if modelParams.Temperature != nil {
				request.SetTemperature(*modelParams.Temperature)
			}
			if modelParams.TopP != nil {
				request.SetTopP(*modelParams.TopP)
			}
			if modelParams.MaxCompletionTokens != nil {
				request.SetMaxOutputTokens(*modelParams.MaxCompletionTokens)
			}
			if modelParams.PresencePenalty != nil {
				logger.Message(ctx, logging.LevelWarn, "presence-penalty parameter is not supported by the Responses API and will be ignored")
			}
			if modelParams.FrequencyPenalty != nil {
				logger.Message(ctx, logging.LevelWarn, "frequency-penalty parameter is not supported by the Responses API and will be ignored")
			}
			if modelParams.ReasoningEffort != nil {
				reasoning := xai.NewReasoningConfiguration()
				reasoning.SetEffort(*modelParams.ReasoningEffort)
				request.SetReasoning(*reasoning)
			}
			if modelParams.Seed != nil {
				logger.Message(ctx, logging.LevelWarn, "seed parameter is not supported by the Responses API and will be ignored")
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	// Add system instructions if available.
	var input []xai.ModelInputPart
	if cfg.DisableStructuredOutput {
		input = append(input, o.newTextInputMessage("system", result.recordPrompt(DefaultUnstructuredResponseInstruction())))
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		input = append(input, o.newTextInputMessage("system", result.recordPrompt(answerFormatInstruction)))
	}

	// Add structured user message.
	parts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}
	input = append(input, xai.ModelInputPartOneOfAsModelInputPart(xai.NewModelInputPartOneOf(xai.ArrayOfModelInputContentItemAsModelInputContent(&parts), "user")))
	request.SetInput(xai.ArrayOfModelInputPartAsModelInput(&input))

	// Conversation turns are chained server-side via previous_response_id by default.
	// xAI advises against storing requests that contain images on the server, so the
	// conversation history is replayed by the client in that case, including the encrypted
	// reasoning items that preserve the model's reasoning across turns.
	// See: https://docs.x.ai/developers/model-capabilities/images/understanding
	manageHistoryLocally := task.RequiresNativeFileInput()
	request.SetStore(!manageHistoryLocally)
	if manageHistoryLocally {
		request.Include = []string{xaiEncryptedReasoningInclude}
	}

	// Setup tools if any.
	var executor *tools.DockerToolExecutor
	toolSelector := task.GetResolvedToolSelector()
	if enabledTools, hasTools := toolSelector.GetEnabledToolsByName(); hasTools {
		var err error
		executor, err = tools.NewToolExecutor(ctx, environment)
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
			function := xai.NewModelToolOneOf(toolCfg.Name, toolCfg.Parameters, "function")
			function.SetDescription(toolCfg.Description)
			function.SetStrict(false)
			request.Tools = append(request.Tools, xai.ModelToolOneOfAsModelTool(function))
		}
		// If user tools are present, allow auto tool choice.
		request.SetToolChoice(xai.StringAsModelToolChoice(xai.PtrString("auto")))
	}

	// Conversation loop to handle tool calls.
	var turn int
	for {
		turn++
		if err := AssertTurnsAvailable(ctx, logger, task, turn); err != nil {
			return result, err
		}

		resp, err := timed(func() (*xaiModelResponse, error) {
			response, httpResponse, err := o.createResponse(ctx, request)
			if err != nil && o.isTransientResponse(httpResponse) {
				return response, WrapErrRetryable(err)
			}
			return response, err
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		result.recordToolCalls(executor.GetCallSummaries())
		if err != nil {
			return result, WrapErrGenerateResponse(err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		// Parse the model response.
		o.recordResponseUsage(resp.Usage, &result.usage)

		stopReason := o.stopReason(resp)
		isTerminal := !o.hasPendingFunctionCalls(resp)
		logFinishReason(ctx, logger, stopReason, isTerminal)

		// Handle failures and unfinished responses.
		if o.hasError(resp) {
			return result, fmt.Errorf("%w: response failed: %s", ErrGenerateResponse, o.errorDetails(resp))
		}
		switch resp.Status {
		case xaiResponseStatusInProgress:
			// Synchronous responses should never return unfinished; the task can be retried.
			return result, WrapErrRetryable(fmt.Errorf("%w: unexpected response status %q", ErrGenerateResponse, resp.Status))
		case xaiResponseStatusIncomplete:
			logger.Message(ctx, logging.LevelDebug, "response ended early: %s", o.incompleteReason(resp))
		}

		if !isTerminal {
			if resp.ID == "" && !manageHistoryLocally {
				return result, fmt.Errorf("%w: response ID is missing, cannot continue the conversation", ErrGenerateResponse)
			}

			textContent, _ := o.messageText(resp.Output)
			logSkippedPreambleText(ctx, logger, stopReason, textContent)

			// Execute function calls and collect their outputs, in order, for the next turn.
			var functionCallOutputs []xai.ModelInputPart
			for _, item := range resp.Output {
				if item.FunctionCall == nil {
					continue
				}
				functionCall := item.FunctionCall
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				toolResult, err := executor.ExecuteTool(ctx, logger, functionCall.Name, json.RawMessage(functionCall.Arguments), data, &tools.ToolCallContext{CallID: functionCall.CallId, ConversationTurn: turn})
				if runtimeErr := taskRuntimeToolError(err); runtimeErr != nil {
					return result, runtimeErr
				}
				content := string(toolResult)
				if err != nil {
					content = formatToolExecutionError(err)
				}
				functionCallOutput := xai.NewFunctionToolCallOutput(functionCall.CallId, xai.StringAsModelInputContent(&content), "function_call_output")
				functionCallOutputs = append(functionCallOutputs, xai.FunctionToolCallOutputAsModelInputPart(functionCallOutput))
			}

			if manageHistoryLocally {
				// Replay the model output of this turn, followed by the function call outputs.
				for _, item := range resp.Output {
					if inputPart, ok := item.toInputPart(); ok {
						input = append(input, inputPart)
					}
				}
				input = append(input, functionCallOutputs...)
			} else {
				// Maintain conversation context for the next turn; managed by the server.
				// Only the function call outputs are needed as new input.
				request.SetPreviousResponseId(resp.ID)
				input = functionCallOutputs
			}
			request.SetInput(xai.ArrayOfModelInputPartAsModelInput(&input))
		} else {
			textContent, refusal := o.messageText(resp.Output)
			if textContent != "" {
				if cfg.DisableStructuredOutput {
					err = UnmarshalUnstructuredResponse(ctx, logger, []byte(textContent), &result)
				} else {
					err = json.Unmarshal([]byte(textContent), &result)
				}
				if err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(stopReason))
				}
				return result, nil
			}

			var stopDetails any
			if refusal != "" {
				stopDetails = map[string]string{"refusal": refusal}
			}
			return result, NewErrNoActionableContent([]byte(stopReason), stopDetails)
		}
	} // move to the next conversation turn
}

// newTextInputMessage creates an input message with the given role and text content.
func (o *XAI) newTextInputMessage(role string, text string) xai.ModelInputPart {
	return xai.ModelInputPartOneOfAsModelInputPart(xai.NewModelInputPartOneOf(xai.StringAsModelInputContent(&text), role))
}

// createResponse sends the request to the Responses API endpoint and decodes the response body.
// The generated client cannot execute this call: on real API responses, its untagged oneOf
// unions (output items, tools, text format and incomplete details) match more than one variant
// and fail to decode, so the body is decoded into the provider's own response view instead.
func (o *XAI) createResponse(ctx context.Context, request *xai.ModelRequest) (*xaiModelResponse, *http.Response, error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	serverURL, err := o.clientCfg.ServerURLWithContext(ctx, "V1APIService.HandleGenericModelRequest")
	if err != nil {
		return nil, nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+xaiResponsesPath, bytes.NewReader(requestBody))
	if err != nil {
		return nil, nil, err
	}
	for name, value := range o.clientCfg.DefaultHeader {
		httpRequest.Header.Set(name, value)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", o.clientCfg.UserAgent)

	httpClient := o.clientCfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		return nil, nil, err
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, httpResponse, err
	}
	if httpResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, httpResponse, NewErrAPIResponse(fmt.Errorf("%w: %s", ErrAPIRequest, httpResponse.Status), responseBody)
	}

	var response xaiModelResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, httpResponse, NewErrAPIResponse(err, responseBody)
	}
	return &response, httpResponse, nil
}

// xaiModelResponse is the subset of the Responses API response body consumed by the provider.
// See: https://docs.x.ai/developers/rest-api-reference/inference/responses
type xaiModelResponse struct {
	// ID is the unique ID of the response, used to continue the conversation.
	ID string `json:"id"`
	// Status is the status of the response: completed, in_progress or incomplete.
	Status string `json:"status"`
	// Error is the error object returned when the model fails to generate a response.
	Error json.RawMessage `json:"error,omitempty"`
	// IncompleteDetails describes why the response is incomplete, when it is.
	IncompleteDetails *xaiIncompleteDetails `json:"incomplete_details,omitempty"`
	// Output contains the items generated by the model.
	Output []xaiOutputItem `json:"output"`
	// Usage contains the token usage of the response.
	Usage *xaiUsage `json:"usage,omitempty"`
}

// Response statuses of the Responses API.
const (
	xaiResponseStatusCompleted  = "completed"
	xaiResponseStatusInProgress = "in_progress"
	xaiResponseStatusIncomplete = "incomplete"
)

// xaiResponseError is the structure of the error object reported for a failed response.
// The API reference does not document its fields; the OpenAI-compatible code and message
// pair is decoded on a best-effort basis.
type xaiResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// xaiIncompleteDetails describes why a response is incomplete.
type xaiIncompleteDetails struct {
	Reason string `json:"reason"`
}

// xaiUsage is the token usage reported for a response. Reasoning tokens are reported
// separately from the output tokens, which count the final text output only.
// See: https://docs.x.ai/developers/tools/tool-usage-details#understanding-token-usage
type xaiUsage struct {
	InputTokens        int64 `json:"input_tokens"`
	InputTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokens        int64 `json:"output_tokens"`
	OutputTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// Output item types handled by the provider.
const (
	xaiOutputItemTypeMessage      = "message"
	xaiOutputItemTypeFunctionCall = "function_call"
	xaiOutputItemTypeReasoning    = "reasoning"
)

// xaiOutputItem is a response output item decoded according to its type. Exactly one of the
// typed fields is set for the item types handled by the provider; items of any other type
// (e.g. server-side tool calls) carry only their type.
type xaiOutputItem struct {
	Type         string
	Message      *xai.OutputMessage
	FunctionCall *xai.FunctionToolCall
	Reasoning    *xai.Reasoning
}

func (item *xaiOutputItem) UnmarshalJSON(data []byte) error {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	*item = xaiOutputItem{Type: header.Type}
	switch header.Type {
	case xaiOutputItemTypeMessage:
		item.Message = &xai.OutputMessage{}
		return json.Unmarshal(data, item.Message)
	case xaiOutputItemTypeFunctionCall:
		item.FunctionCall = &xai.FunctionToolCall{}
		return json.Unmarshal(data, item.FunctionCall)
	case xaiOutputItemTypeReasoning:
		item.Reasoning = &xai.Reasoning{}
		return json.Unmarshal(data, item.Reasoning)
	default:
		return nil
	}
}

// toInputPart converts the item into an input part that replays it to the model.
// Items of types not handled by the provider cannot be replayed.
func (item xaiOutputItem) toInputPart() (xai.ModelInputPart, bool) {
	output := xai.ModelOutput{
		OutputMessage:    item.Message,
		FunctionToolCall: item.FunctionCall,
		Reasoning:        item.Reasoning,
	}
	if output.GetActualInstance() == nil {
		return xai.ModelInputPart{}, false
	}
	return xai.ModelOutputAsModelInputPart(&output), true
}

// recordResponseUsage adds the token usage of a response to the accumulated usage.
// Cached input tokens are included in the input tokens, while reasoning tokens are
// reported separately from the output tokens.
func (o *XAI) recordResponseUsage(usage *xaiUsage, out *Usage) {
	if usage == nil {
		return
	}
	recordUsage(InputTokenAccountingCacheTokensIncluded, OutputTokenAccountingReasoningTokensSeparate, &usage.InputTokens, &usage.OutputTokens, &usage.OutputTokensDetails.ReasoningTokens, nil, &usage.InputTokensDetails.CachedTokens, out)
}

// stopReason describes why the response stopped: its status, followed by the
// incomplete reason or the error code, if any.
func (o *XAI) stopReason(resp *xaiModelResponse) string {
	if resp.Status == xaiResponseStatusIncomplete && resp.IncompleteDetails != nil && resp.IncompleteDetails.Reason != "" {
		return fmt.Sprintf("%s: %s", resp.Status, resp.IncompleteDetails.Reason)
	}
	if responseErr, ok := o.responseError(resp); ok && responseErr.Code != "" {
		return fmt.Sprintf("%s: %s", resp.Status, responseErr.Code)
	}
	return resp.Status
}

// hasPendingFunctionCalls checks whether the model requested function calls that should be
// executed before the conversation can continue. Function calls returned with a status other
// than completed (e.g. incomplete) may be truncated, so they are not executed and end the
// conversation.
func (o *XAI) hasPendingFunctionCalls(resp *xaiModelResponse) bool {
	if resp.Status != xaiResponseStatusCompleted {
		return false
	}
	return slices.ContainsFunc(resp.Output, func(item xaiOutputItem) bool {
		return item.FunctionCall != nil
	})
}

// hasError checks whether the response reports an error object.
func (o *XAI) hasError(resp *xaiModelResponse) bool {
	return len(resp.Error) > 0 && string(resp.Error) != "null"
}

// responseError returns the structured error reported for a failed response, if any.
func (o *XAI) responseError(resp *xaiModelResponse) (xaiResponseError, bool) {
	var responseErr xaiResponseError
	if !o.hasError(resp) {
		return responseErr, false
	}
	if err := json.Unmarshal(resp.Error, &responseErr); err != nil {
		return responseErr, false
	}
	return responseErr, true
}

// errorDetails describes the error reported for a failed response.
func (o *XAI) errorDetails(resp *xaiModelResponse) string {
	if responseErr, ok := o.responseError(resp); ok {
		var detail strings.Builder
		detail.WriteString(responseErr.Code)
		if responseErr.Message != "" {
			if detail.Len() > 0 {
				detail.WriteString(": ")
			}
			detail.WriteString(responseErr.Message)
		}
		if detail.Len() > 0 {
			return detail.String()
		}
	}
	if o.hasError(resp) {
		return string(resp.Error) // unrecognized error structure
	}
	return "no error details provided"
}

// incompleteReason describes why a response is incomplete.
func (o *XAI) incompleteReason(resp *xaiModelResponse) string {
	if resp.IncompleteDetails != nil && resp.IncompleteDetails.Reason != "" {
		return resp.IncompleteDetails.Reason
	}
	return "no details provided"
}

// messageText concatenates the text content of all output messages and, separately,
// any refusal content. Reasoning items are excluded.
func (o *XAI) messageText(output []xaiOutputItem) (text string, refusal string) {
	var textBuilder, refusalBuilder strings.Builder
	for _, item := range output {
		if item.Message == nil {
			continue
		}
		for _, content := range item.Message.Content {
			if content.OutputMessageContentOneOf != nil {
				textBuilder.WriteString(content.OutputMessageContentOneOf.Text)
			}
			if content.OutputMessageContentOneOf1 != nil {
				refusalBuilder.WriteString(content.OutputMessageContentOneOf1.Refusal)
			}
		}
	}
	return textBuilder.String(), refusalBuilder.String()
}

// isTransientResponse checks whether the HTTP response represents a transient condition
// that the retry policy should attempt again.
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

func (o *XAI) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []xai.ModelInputContentItem, err error) {
	for _, file := range files {
		parts = append(parts, o.newTextContentItem(result.recordPrompt(DefaultTaskFileNameInstruction(file))))
		if !file.HasAccess(config.FileAccessNative) {
			continue
		}
		if fileType, err := file.TypeValue(ctx); err != nil {
			return parts, err
		} else if !o.isSupportedImageType(fileType) {
			return parts, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
		}

		dataURL, err := file.GetDataURL(ctx)
		if err != nil {
			return parts, err
		}

		// Add image data part.
		parts = append(parts, xai.ModelInputContentItemOneOf2AsModelInputContentItem(xai.NewModelInputContentItemOneOf2(dataURL, "input_image")))
	}

	// Append the prompt text after the file data for improved context integrity.
	parts = append(parts, o.newTextContentItem(result.recordPrompt(promptText)))

	return parts, nil
}

// newTextContentItem creates a text input content item.
func (o *XAI) newTextContentItem(text string) xai.ModelInputContentItem {
	return xai.ModelInputContentItemOneOfAsModelInputContentItem(xai.NewModelInputContentItemOneOf(text, "input_text"))
}

// isSupportedImageType checks if the provided MIME type is supported by the xAI image understanding API.
// For more information, see: https://docs.x.ai/developers/model-capabilities/images/understanding
func (o XAI) isSupportedImageType(mimeType string) bool {
	return slices.Contains([]string{
		"image/jpeg",
		"image/jpg",
		"image/png",
	}, config.NormalizeMIMEType(mimeType))
}
