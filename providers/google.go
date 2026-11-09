// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers/tools"
	"google.golang.org/genai"
	"google.golang.org/genai/interactions/models/apierrors"
	"google.golang.org/genai/interactions/models/components"
	"google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"
	"google.golang.org/genai/interactions/retry"
)

// NewGoogleAI creates a new GoogleAI provider instance with the given configuration.
// It returns an error if client initialization fails.
func NewGoogleAI(ctx context.Context, cfg config.GoogleAIClientConfig, availableTools []config.ToolConfig) (*GoogleAI, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateClient, err)
	}
	return &GoogleAI{
		client:         client,
		availableTools: availableTools,
	}, nil
}

// GoogleAI implements the Provider interface for Google AI generative models.
type GoogleAI struct {
	client         *genai.Client
	availableTools []config.ToolConfig
}

func (o GoogleAI) Name() string {
	return config.GOOGLE
}

func (o *GoogleAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task, environment ExecutionEnvironment) (result Result, err error) {
	// Store is required to chain conversation turns via previous_interaction_id.
	request := interactions.CreateModelInteraction{
		Model:            interactions.Model(cfg.Model),
		Store:            genai.Ptr(true),
		GenerationConfig: &interactions.GenerationConfig{},
	}

	forceTextResponseFormat := cfg.DisableStructuredOutput

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
			function := interactions.Function{
				Name: genai.Ptr(toolCfg.Name),
			}
			if toolCfg.Description != "" {
				function.Description = genai.Ptr(toolCfg.Description)
			}
			if toolCfg.Parameters != nil {
				function.Parameters = toolCfg.Parameters
			}
			request.Tools = append(request.Tools, interactions.NewTool(function))
		}
		// If user tools are present, allow auto tool choice.
		request.GenerationConfig.ToolChoice = genai.Ptr(interactions.NewToolChoice(interactions.ToolChoiceTypeAuto))
	}

	// Handle model parameters.
	var defaultImageResolution *interactions.MediaResolution
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.GoogleAIModelParams); ok {
			// Apply TextResponseFormat (all tasks) or TextResponseFormatWithTools (only tasks with tools).
			// Preserve disableStructuredOutput - it always forces text format.
			forceTextResponseFormat = cfg.DisableStructuredOutput ||
				modelParams.TextResponseFormat ||
				(len(request.Tools) > 0 && modelParams.TextResponseFormatWithTools)

			if modelParams.ThinkingLevel != nil {
				request.GenerationConfig.ThinkingLevel = o.mapThinkingLevel(*modelParams.ThinkingLevel)
			}
			if modelParams.ThinkingSummaries != nil {
				request.GenerationConfig.ThinkingSummaries = o.mapThinkingSummaries(*modelParams.ThinkingSummaries)
			}
			if modelParams.MediaResolution != nil {
				defaultImageResolution = o.mapMediaResolution(*modelParams.MediaResolution)
			}
			if modelParams.MaxOutputTokens != nil {
				request.GenerationConfig.MaxOutputTokens = genai.Ptr(int(*modelParams.MaxOutputTokens))
			}
			if modelParams.Temperature != nil {
				logger.Message(ctx, logging.LevelWarn, "temperature parameter is deprecated by the Interactions API and may be ignored by the model")
				request.GenerationConfig.Temperature = modelParams.Temperature
			}
			if modelParams.TopP != nil {
				logger.Message(ctx, logging.LevelWarn, "top-p parameter is deprecated by the Interactions API and may be ignored by the model")
				request.GenerationConfig.TopP = modelParams.TopP
			}
			if modelParams.TopK != nil {
				logger.Message(ctx, logging.LevelWarn, "top-k parameter is not supported by the Interactions API and will be ignored")
			}
			if modelParams.PresencePenalty != nil {
				logger.Message(ctx, logging.LevelWarn, "presence-penalty parameter is not supported by the Interactions API and will be ignored")
			}
			if modelParams.FrequencyPenalty != nil {
				logger.Message(ctx, logging.LevelWarn, "frequency-penalty parameter is not supported by the Interactions API and will be ignored")
			}
			if modelParams.Seed != nil {
				request.GenerationConfig.Seed = genai.Ptr(int(*modelParams.Seed))
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	// Configure response format.
	var systemParts []string
	if forceTextResponseFormat {
		request.ResponseFormat = o.newResponseFormat(interactions.TextResponseFormatMimeTypeTextPlain, nil)
		// Add response format instruction only when not in unstructured mode.
		if !cfg.DisableStructuredOutput {
			responseFormatInstruction, err := DefaultResponseFormatInstruction(task.ResponseResultFormat)
			if err != nil {
				return result, err
			}
			systemParts = append(systemParts, result.recordPrompt(responseFormatInstruction))
		}
	} else {
		responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		request.ResponseFormat = o.newResponseFormat(interactions.TextResponseFormatMimeTypeApplicationJSON, responseSchema)
	}

	if cfg.DisableStructuredOutput {
		systemParts = append(systemParts, result.recordPrompt(DefaultUnstructuredResponseInstruction()))
	}

	// Add answer format instruction to system instructions.
	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		systemParts = append(systemParts, result.recordPrompt(answerFormatInstruction))
	}

	// The Interactions API accepts a single system instruction string.
	if len(systemParts) > 0 {
		request.SystemInstruction = genai.Ptr(strings.Join(systemParts, "\n\n"))
	}

	// Create prompt content.
	promptContent, err := o.createPromptContent(ctx, logger, task.Prompt, task.Files, defaultImageResolution, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}
	request.Input = genai.Ptr(interactions.NewInteractionsInput(promptContent))

	// Disable SDK retries: they resend the identical request without an idempotency key,
	// which can create duplicate billed interactions. MindTrial's retry policy restarts the
	// whole task instead, so no interaction ID is ever reused across attempts.
	requestOptions := []operations.Option{
		operations.WithRetries(retry.Config{Strategy: "none"}),
	}

	// Conversation loop to handle tool calls.
	var turn int
	for {
		turn++
		if err := AssertTurnsAvailable(ctx, logger, task, turn); err != nil {
			return result, err
		}

		// Execute the interaction request. Interactions that require continuation are
		// resumed automatically by the SDK and returned merged as a single interaction.
		resp, err := timed(func() (*interactions.Interaction, error) {
			response, err := o.client.Interactions.Create(ctx, operations.CreateInteractionRequest{
				Body: operations.NewCreateInteractionRequestBody(request),
			}, requestOptions...)
			var interaction *interactions.Interaction
			if response != nil {
				interaction = response.Interaction
			}
			if err != nil && o.isTransientResponse(err) {
				return interaction, WrapErrRetryable(err)
			}
			return interaction, err
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		result.recordToolCalls(executor.GetCallSummaries())
		if err != nil {
			// Tokens of continuation hops that completed before the failure were billed.
			var interruptedErr *apierrors.ContinuationInterruptedError
			if errors.As(err, &interruptedErr) {
				logger.Message(ctx, logging.LevelDebug, "continuation interrupted after %d hop(s) (interaction ID: %q, continuation token: %q)", len(interruptedErr.ContinuationIDs), interruptedErr.InteractionID, interruptedErr.ContinuationToken)
				if interruptedErr.PartialInteraction != nil {
					o.recordInteractionUsage(interruptedErr.PartialInteraction.Usage, &result.usage)
				}
			}
			return result, WrapErrGenerateResponse(err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		// Parse the interaction response.
		o.recordInteractionUsage(resp.Usage, &result.usage)
		o.logThoughtSummaries(ctx, logger, resp.Steps)

		outputSteps := o.outputSteps(resp.Steps)
		stopReason := o.stopReason(resp)
		isTerminal := !o.hasPendingFunctionCalls(resp.Status, outputSteps)
		logFinishReason(ctx, logger, stopReason, isTerminal)

		// Handle explicit failure, cancellation and unfinished statuses.
		switch resp.Status {
		case interactions.InteractionStatusFailed:
			err := fmt.Errorf("%w: interaction failed: %s", ErrGenerateResponse, o.errorDetails(resp))
			if o.isRetryableInteractionFailure(resp) {
				return result, WrapErrRetryable(err)
			}
			return result, err
		case interactions.InteractionStatusCancelled:
			return result, ErrNoResponseCandidates
		case interactions.InteractionStatusInProgress, interactions.InteractionStatusQueued, interactions.InteractionStatusRequiresContinuation:
			// Synchronous interactions should never return unfinished. The SDK resumes continuations
			// automatically and only returns one when it cannot make progress; the task can be retried.
			return result, WrapErrRetryable(fmt.Errorf("%w: unexpected interaction status %q", ErrGenerateResponse, resp.Status))
		case interactions.InteractionStatusIncomplete, interactions.InteractionStatusBudgetExceeded:
			logger.Message(ctx, logging.LevelDebug, "interaction ended early with status %q: %s", resp.Status, o.errorDetails(resp))
		}

		if !isTerminal {
			if resp.ID == nil || *resp.ID == "" {
				return result, fmt.Errorf("%w: interaction ID is missing, cannot continue the conversation", ErrGenerateResponse)
			}
			// Maintain conversation context for the next turn; managed by the server.
			request.PreviousInteractionID = resp.ID

			logSkippedPreambleText(ctx, logger, stopReason, o.outputText(outputSteps))

			// Execute function calls and collect their results, in order, for the next turn.
			var functionResultSteps []interactions.Step
			for _, step := range outputSteps {
				if step.Type != interactions.StepTypeFunctionCall || step.FunctionCallStep == nil {
					continue
				}
				functionCall := step.FunctionCallStep
				argsBytes, err := json.Marshal(functionCall.Arguments)
				if err != nil {
					return result, fmt.Errorf("%w: failed to marshal function args: %v", ErrToolUse, err)
				}
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				response := map[string]interface{}{}
				// Model should not call tools that already reached max calls limit, but in case it does,
				// allow one final call of the tool (should short-circuit) and
				// remove it from the available tools to prevent further errors.
				wasExhausted := executor.IsToolExhausted(functionCall.Name)
				toolResult, err := executor.ExecuteTool(ctx, logger, functionCall.Name, json.RawMessage(argsBytes), data, &tools.ToolCallContext{CallID: functionCall.ID, ConversationTurn: turn})
				if runtimeErr := taskRuntimeToolError(err); runtimeErr != nil {
					return result, runtimeErr
				}
				isError := err != nil
				if isError {
					response["error"] = formatToolExecutionError(err)
				} else {
					response["result"] = string(toolResult)
				}
				if wasExhausted {
					logger.Message(ctx, logging.LevelWarn, "tool %q was called again after max-calls error; removing it from available tools for next turn", functionCall.Name)
					o.removeFunctionTool(&request, functionCall.Name)
				}
				resultStep, err := o.newFunctionResultStep(functionCall, response, isError)
				if err != nil {
					return result, fmt.Errorf("%w: failed to marshal function result: %v", ErrToolUse, err)
				}
				functionResultSteps = append(functionResultSteps, resultStep)
			}

			// With PreviousInteractionID, only the function results are needed as new input.
			request.Input = genai.Ptr(interactions.NewInteractionsInput(functionResultSteps))
		} else {
			if textContent := o.outputText(outputSteps); textContent != "" {
				if cfg.DisableStructuredOutput {
					err = UnmarshalUnstructuredResponse(ctx, logger, []byte(textContent), &result)
				} else {
					content := []byte(textContent)
					if forceTextResponseFormat {
						repaired, err := utils.RepairTextJSON(textContent)
						if err != nil {
							return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(stopReason))
						}
						content = []byte(repaired)
					}
					err = json.Unmarshal(content, &result)
				}
				if err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(stopReason))
				}
				return result, nil
			}

			return result, NewErrNoActionableContent([]byte(stopReason), nil)
		}
	} // move to the next conversation turn
}

// mapThinkingLevel maps a configured thinking level to the Interactions API value.
// Unrecognized values leave the thinking level unset (model default).
func (o *GoogleAI) mapThinkingLevel(level string) *interactions.ThinkingLevel {
	switch level {
	case "minimal":
		return genai.Ptr(interactions.ThinkingLevelMinimal)
	case "low":
		return genai.Ptr(interactions.ThinkingLevelLow)
	case "medium":
		return genai.Ptr(interactions.ThinkingLevelMedium)
	case "high":
		return genai.Ptr(interactions.ThinkingLevelHigh)
	default:
		return nil
	}
}

// mapThinkingSummaries maps a configured thinking summaries mode to the Interactions API value.
// Unrecognized values leave the mode unset (model default).
func (o *GoogleAI) mapThinkingSummaries(mode string) *interactions.ThinkingSummaries {
	switch mode {
	case "auto":
		return genai.Ptr(interactions.ThinkingSummariesAuto)
	case "none":
		return genai.Ptr(interactions.ThinkingSummariesNone)
	default:
		return nil
	}
}

// mapMediaResolution maps a configured media resolution to the Interactions API value.
// Unrecognized values leave the resolution unset (model default).
func (o *GoogleAI) mapMediaResolution(resolution string) *interactions.MediaResolution {
	switch resolution {
	case "low":
		return genai.Ptr(interactions.MediaResolutionLow)
	case "medium":
		return genai.Ptr(interactions.MediaResolutionMedium)
	case "high":
		return genai.Ptr(interactions.MediaResolutionHigh)
	case "ultra_high":
		return genai.Ptr(interactions.MediaResolutionUltraHigh)
	default:
		return nil
	}
}

func (o *GoogleAI) newResponseFormat(mimeType interactions.TextResponseFormatMimeType, schema map[string]interface{}) *interactions.CreateModelInteractionResponseFormat {
	return genai.Ptr(interactions.NewCreateModelInteractionResponseFormat(
		interactions.NewResponseFormat(interactions.TextResponseFormat{
			MimeType: &mimeType,
			Schema:   schema,
		}),
	))
}

// retryableGoogleStatusCodes lists the HTTP status codes that the retry policy should attempt again.
var retryableGoogleStatusCodes = []int{
	http.StatusTooManyRequests,
	http.StatusInternalServerError,
	http.StatusServiceUnavailable,
}

// isTransientResponse checks whether the error represents a transient condition
// that the retry policy should attempt again.
func (o *GoogleAI) isTransientResponse(err error) bool {
	statusCode, ok := o.errorStatusCode(err)
	return ok && slices.Contains(retryableGoogleStatusCodes, statusCode)
}

// errorStatusCode extracts the HTTP status code from an Interactions API error.
func (o *GoogleAI) errorStatusCode(err error) (int, bool) {
	var clientErr *apierrors.CreateInteractionClientError
	var serverErr *apierrors.CreateInteractionServerError
	var apiErr *apierrors.APIError
	var validationErr *apierrors.ResponseValidationError
	switch {
	case errors.As(err, &clientErr):
		return o.httpStatusCode(clientErr.HTTPMeta)
	case errors.As(err, &serverErr):
		return o.httpStatusCode(serverErr.HTTPMeta)
	case errors.As(err, &apiErr):
		return apiErr.StatusCode, true
	case errors.As(err, &validationErr):
		return validationErr.StatusCode, true
	}
	return 0, false
}

func (o *GoogleAI) httpStatusCode(meta components.HTTPMetadata) (int, bool) {
	if meta.Response == nil {
		return 0, false
	}
	return meta.Response.StatusCode, true
}

// recordInteractionUsage records the token usage of an interaction, if available.
func (o *GoogleAI) recordInteractionUsage(usage *interactions.Usage, out *Usage) {
	if usage != nil {
		recordUsage(InputTokenAccountingCacheTokensIncluded, OutputTokenAccountingReasoningTokensSeparate, usage.TotalInputTokens, usage.TotalOutputTokens, usage.TotalThoughtTokens, nil, usage.TotalCachedTokens, out)
	}
}

// logThoughtSummaries logs the text thought summaries included in the interaction steps.
// Summaries are only returned when thinking summaries are enabled. Each summary is
// prefixed with the index of its step in the interaction.
func (o *GoogleAI) logThoughtSummaries(ctx context.Context, logger logging.Logger, steps []interactions.Step) {
	thoughtLogger := logger.WithContext("thought summary: ")
	for index, step := range steps {
		if step.Type != interactions.StepTypeThought || step.ThoughtStep == nil {
			continue
		}
		for _, summary := range step.ThoughtStep.Summary {
			if summary.TextContent != nil && summary.TextContent.Text != "" {
				thoughtLogger.Message(ctx, logging.LevelInfo, "step %d: %s", index, summary.TextContent.Text)
			}
		}
	}
}

// outputSteps returns the steps produced by the model in the current turn,
// i.e. the steps following the last input (user input or function result) step.
func (o *GoogleAI) outputSteps(steps []interactions.Step) []interactions.Step {
	for index, step := range slices.Backward(steps) {
		if step.Type == interactions.StepTypeUserInput || step.Type == interactions.StepTypeFunctionResult {
			return steps[index+1:]
		}
	}
	return steps
}

// stopReason describes why the interaction stopped: its status, followed by
// the first reported error code, if any.
func (o *GoogleAI) stopReason(interaction *interactions.Interaction) string {
	stopReason := string(interaction.Status)
	for _, interactionErr := range interaction.Errors {
		if interactionErr.Code != nil && *interactionErr.Code != "" {
			return fmt.Sprintf("%s: %s", stopReason, *interactionErr.Code)
		}
	}
	return stopReason
}

// hasPendingFunctionCalls checks whether the model requested function calls that
// should be executed before the conversation can continue. Per the Interactions API, a tool
// request is signalled by the requires_action status; function calls returned with any other
// status (e.g. incomplete) are not executed and end the conversation.
func (o *GoogleAI) hasPendingFunctionCalls(status interactions.InteractionStatus, outputSteps []interactions.Step) bool {
	if status != interactions.InteractionStatusRequiresAction {
		return false
	}
	return slices.ContainsFunc(outputSteps, func(step interactions.Step) bool {
		return step.Type == interactions.StepTypeFunctionCall && step.FunctionCallStep != nil
	})
}

// errorDetails describes the errors reported for an interaction.
func (o *GoogleAI) errorDetails(interaction *interactions.Interaction) string {
	details := make([]string, 0, len(interaction.Errors))
	for _, interactionErr := range interaction.Errors {
		details = append(details, o.formatErrorDetail(interactionErr.Code, interactionErr.Message))
	}
	if len(details) == 0 {
		return "no error details provided"
	}
	return strings.Join(details, "; ")
}

func (o *GoogleAI) formatErrorDetail(code *string, message *string) string {
	var detail strings.Builder
	if code != nil && *code != "" {
		detail.WriteString(*code)
	}
	if message != nil && *message != "" {
		if detail.Len() > 0 {
			detail.WriteString(": ")
		}
		detail.WriteString(*message)
	}
	if detail.Len() == 0 {
		return "unknown error"
	}
	return detail.String()
}

// retryableGoogleErrorCodes lists the Interactions API error codes that the retry policy should
// attempt again: the documented snake_case codes delivered with the HTTP statuses of
// retryableGoogleStatusCodes (429, 500 and 503), including the snake_case status name fallback.
// See: https://aistudio.google.com/docs/api-errors
var retryableGoogleErrorCodes = []string{
	"rate_limit_exceeded", "quota_exceeded", "too_many_requests",
	"api_error", "internal_server_error",
	"service_unavailable",
}

// isRetryableInteractionFailure checks whether a failed interaction reported a transient error.
func (o *GoogleAI) isRetryableInteractionFailure(interaction *interactions.Interaction) bool {
	return slices.ContainsFunc(interaction.Errors, func(interactionErr interactions.Error) bool {
		return interactionErr.Code != nil && slices.Contains(retryableGoogleErrorCodes, strings.ToLower(*interactionErr.Code))
	})
}

// outputText concatenates the text content of all model output steps.
// Thought steps are excluded.
func (o *GoogleAI) outputText(outputSteps []interactions.Step) string {
	var textBuilder strings.Builder
	for _, step := range outputSteps {
		if step.Type != interactions.StepTypeModelOutput || step.ModelOutputStep == nil {
			continue
		}
		for _, content := range step.ModelOutputStep.Content {
			if content.TextContent != nil {
				textBuilder.WriteString(content.TextContent.Text)
			}
		}
	}
	return textBuilder.String()
}

// removeFunctionTool removes the named function tool from the request.
// The tool choice is cleared once no tools remain.
func (o *GoogleAI) removeFunctionTool(request *interactions.CreateModelInteraction, toolName string) {
	request.Tools = slices.DeleteFunc(request.Tools, func(tool interactions.Tool) bool {
		return tool.Function != nil && tool.Function.Name != nil && *tool.Function.Name == toolName
	})
	if len(request.Tools) == 0 {
		request.Tools = nil
		if request.GenerationConfig != nil {
			request.GenerationConfig.ToolChoice = nil
		}
	}
}

// newFunctionResultStep creates a function result step for the given function call.
// The result object is sent as raw JSON because the SDK's typed result object has no fields.
func (o *GoogleAI) newFunctionResultStep(functionCall *interactions.FunctionCallStep, response map[string]interface{}, isError bool) (interactions.Step, error) {
	rawResult, err := json.Marshal(response)
	if err != nil {
		return interactions.Step{}, err
	}
	resultStep := interactions.FunctionResultStep{
		CallID: functionCall.ID,
		Name:   genai.Ptr(functionCall.Name),
		Result: interactions.NewFunctionResultStepResultUnionUnknown(rawResult),
	}
	if isError {
		resultStep.IsError = genai.Ptr(true)
	}
	return interactions.NewStep(resultStep), nil
}

// googleNativeDocumentMimeTypes lists the document types the Interactions API natively accepts.
// See: https://ai.google.dev/static/api/interactions-v1.md.txt (DocumentContent.mime_type).
var googleNativeDocumentMimeTypes = map[string]bool{
	"application/pdf": true,
	"text/csv":        true,
}

// googleBestEffortDocumentMimeTypes lists document types that were accepted by the legacy
// generateContent API, but are not documented by the Interactions API. They are still sent
// natively on a best-effort basis. Plain text is sent verbatim as a text content block instead.
var googleBestEffortDocumentMimeTypes = map[string]bool{
	"application/json": true,
	"text/html":        true,
	"text/css":         true,
	"text/xml":         true,
	"text/rtf":         true,
	"text/javascript":  true,
}

func (o *GoogleAI) createPromptContent(ctx context.Context, logger logging.Logger, promptText string, files []config.TaskFile, defaultImageResolution *interactions.MediaResolution, result *Result) (contents []interactions.Content, err error) {
	for _, file := range files {
		contents = append(contents, interactions.NewContent(interactions.TextContent{Text: result.recordPrompt(DefaultTaskFileNameInstruction(file))}))
		if !file.HasAccess(config.FileAccessNative) {
			continue
		}
		fileType, err := file.TypeValue(ctx)
		if err != nil {
			return contents, err
		}
		mimeType := config.NormalizeMIMEType(fileType)
		isImage := isSupportedImageType(mimeType)
		isPlainText := isPlainTextType(mimeType)
		if !isImage && !isPlainText && !googleNativeDocumentMimeTypes[mimeType] && !googleBestEffortDocumentMimeTypes[mimeType] {
			return contents, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
		}
		content, err := file.Content(ctx)
		if err != nil {
			return contents, err
		}
		if isPlainText {
			contents = append(contents, interactions.NewContent(interactions.TextContent{Text: string(content)}))
			continue
		}
		data := base64.StdEncoding.EncodeToString(content)
		if isImage {
			contents = append(contents, interactions.NewContent(interactions.ImageContent{
				Data:       &data,
				MimeType:   genai.Ptr(interactions.ImageContentMimeType(canonicalImageMimeType(mimeType))),
				Resolution: o.resolveImageResolution(ctx, logger, file.GetResolvedFileOptions().ImageDetail, defaultImageResolution),
			}))
			continue
		}
		if googleBestEffortDocumentMimeTypes[mimeType] {
			logger.Message(ctx, logging.LevelWarn, "file %q type %q is not documented as natively supported by the Interactions API; sending it on a best-effort basis", file.Name, mimeType)
		}
		contents = append(contents, interactions.NewContent(interactions.DocumentContent{
			Data:     &data,
			MimeType: genai.Ptr(interactions.DocumentContentMimeType(mimeType)),
		}))
	}

	contents = append(contents, interactions.NewContent(interactions.TextContent{Text: result.recordPrompt(promptText)})) // append the prompt text after the file data for improved context integrity

	return contents, nil
}

// resolveImageResolution determines the resolution of an input image. An explicit per-file
// image detail always takes precedence over the run-level default: "auto" leaves the resolution
// unset so the model decides, while nil or unrecognized values fall back to the default.
// The generic "original" level maps to "ultra_high".
func (o *GoogleAI) resolveImageResolution(ctx context.Context, logger logging.Logger, detail *config.ImageDetail, defaultResolution *interactions.MediaResolution) *interactions.MediaResolution {
	if detail != nil {
		switch *detail {
		case config.ImageDetailAuto:
			return nil
		case config.ImageDetailLow:
			return genai.Ptr(interactions.MediaResolutionLow)
		case config.ImageDetailMedium:
			return genai.Ptr(interactions.MediaResolutionMedium)
		case config.ImageDetailHigh:
			return genai.Ptr(interactions.MediaResolutionHigh)
		case config.ImageDetailOriginal:
			return genai.Ptr(interactions.MediaResolutionUltraHigh)
		default:
			logger.Message(ctx, logging.LevelWarn, "unsupported image detail level %q, reverting to default behavior", *detail)
		}
	}
	return defaultResolution
}

func (o *GoogleAI) Close(ctx context.Context) error {
	return nil
}
