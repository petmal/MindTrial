// Copyright (C) 2026 Petr Malik
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
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers/tools"
)

// ResponseAccumulator handles the accumulation of streaming response events
// into a final Response object. It is used by handleStreamingRequest to
// delegate event processing.
type ResponseAccumulator interface {
	// AddEvent feeds a streaming event to the accumulator. Implementations may
	// return retryable or non-retryable errors based on event details.
	AddEvent(ctx context.Context, logger logging.Logger, event responses.ResponseStreamEventUnion) error

	// GetResponse returns the fully accumulated Response after streaming ends.
	GetResponse() *responses.Response
}

// ResponseHandler extends ResponseAccumulator.
//
// Delegating providers can supply a custom implementation to capture
// non-standard streaming fields. A fresh instance is created per API call
// via the NewResponseHandler factory.
type ResponseHandler interface {
	ResponseAccumulator
}

// defaultResponseHandler is the standard ResponseHandler that captures the full
// Response from terminal stream events.
type defaultResponseHandler struct {
	response *responses.Response
}

func (h *defaultResponseHandler) AddEvent(_ context.Context, _ logging.Logger, event responses.ResponseStreamEventUnion) error {
	switch event.Type {
	case "response.completed", "response.failed", "response.incomplete":
		h.response = &event.Response
		return nil
	case "error": // model returned an error event; retry only if the response error code indicates a transient condition
		errorEvent := event.AsError()
		err := fmt.Errorf("%w: code=%s, param=%s, message=%s", ErrGenerateResponse, errorEvent.Code, errorEvent.Param, errorEvent.Message)
		if isRetryableResponseErrorCode(responses.ResponseErrorCode(errorEvent.Code)) {
			return WrapErrRetryable(err)
		}
		return err
	default:
		return nil
	}
}

func (h *defaultResponseHandler) GetResponse() *responses.Response {
	return h.response
}

// openAIResponsesProvider is an OpenAI Responses API provider implementation
// using the official Go SDK v3.
type openAIResponsesProvider struct {
	client         openai.Client
	availableTools []config.ToolConfig

	// NewResponseHandler is a factory that creates a fresh ResponseHandler
	// for each API call (both streaming and non-streaming). When nil, the
	// defaultResponseHandler is used.
	NewResponseHandler func() ResponseHandler
}

func newOpenAIResponsesProvider(availableTools []config.ToolConfig, opts ...option.RequestOption) *openAIResponsesProvider {
	clientOpts := append([]option.RequestOption{
		option.WithMaxRetries(0), // disable SDK retries since MindTrial has its own retry policy
	}, opts...)

	return &openAIResponsesProvider{
		client:         openai.NewClient(clientOpts...),
		availableTools: availableTools,
	}
}

func (o *openAIResponsesProvider) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	request := responses.ResponseNewParams{
		Model: shared.ResponsesModel(cfg.Model),
		Store: param.NewOpt(true), // enables PreviousResponseID for multi-turn
	}

	useStreaming := false

	// Set response format based on structured output mode.
	if cfg.DisableStructuredOutput {
		request.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfText: &shared.ResponseFormatTextParam{},
			},
		}
	} else {
		schemaMap, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		request.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:   "response",
					Schema: schemaMap,
					Strict: param.NewOpt(true),
				},
			},
		}
	}

	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(openAIV3ModelParams); ok {
			if len(modelParams.ExtraFields) > 0 {
				request.SetExtraFields(modelParams.ExtraFields)
			}

			if modelParams.ReasoningEffort != nil {
				request.Reasoning = shared.ReasoningParam{
					Effort: shared.ReasoningEffort(*modelParams.ReasoningEffort),
				}
			}
			if modelParams.Verbosity != nil {
				request.Text.Verbosity = responses.ResponseTextConfigVerbosity(*modelParams.Verbosity)
			}
			if modelParams.ResponseFormat != nil {
				// Validate that DisableStructuredOutput is not used with non-text response format.
				if cfg.DisableStructuredOutput && *modelParams.ResponseFormat != ResponseFormatText {
					return result, ErrIncompatibleResponseFormat
				}
				// Skip response format handling when structured output is disabled (already forced to text above).
				if !cfg.DisableStructuredOutput {
					// Add response format instruction to input for all formats except strict schema-only mode.
					if *modelParams.ResponseFormat != ResponseFormatJSONSchema {
						responseFormatInstruction, err := DefaultResponseFormatInstruction(task.ResponseResultFormat)
						if err != nil {
							return result, err
						}
						request.Input = appendInputDeveloperMessage(request.Input, result.recordPrompt(responseFormatInstruction))
					}

					// Override response format type for non-schema modes.
					switch *modelParams.ResponseFormat {
					case ResponseFormatText:
						request.Text.Format = responses.ResponseFormatTextConfigUnionParam{
							OfText: &shared.ResponseFormatTextParam{},
						}

					case ResponseFormatJSONObject:
						request.Text.Format = responses.ResponseFormatTextConfigUnionParam{
							OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
						}
					}
					// ResponseFormatLegacySchema and ResponseFormatJSONSchema keep the default json_schema format.
				}
			}
			if modelParams.Temperature != nil {
				request.Temperature = param.NewOpt(*modelParams.Temperature)
			}
			if modelParams.TopP != nil {
				request.TopP = param.NewOpt(*modelParams.TopP)
			}
			if modelParams.MaxCompletionTokens != nil {
				request.MaxOutputTokens = param.NewOpt(*modelParams.MaxCompletionTokens)
			}
			if modelParams.MaxTokens != nil && modelParams.MaxCompletionTokens == nil {
				request.MaxOutputTokens = param.NewOpt(*modelParams.MaxTokens)
			}
			if modelParams.Seed != nil {
				logger.Message(ctx, logging.LevelWarn, "seed parameter is not supported by the Responses API and will be ignored")
			}
			if modelParams.PresencePenalty != nil {
				logger.Message(ctx, logging.LevelWarn, "presence_penalty parameter is not supported by the Responses API and will be ignored")
			}
			if modelParams.FrequencyPenalty != nil {
				logger.Message(ctx, logging.LevelWarn, "frequency_penalty parameter is not supported by the Responses API and will be ignored")
			}
			if modelParams.Stream != nil && *modelParams.Stream {
				useStreaming = true
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	if cfg.DisableStructuredOutput {
		request.Input = appendInputDeveloperMessage(request.Input, result.recordPrompt(DefaultUnstructuredResponseInstruction()))
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		request.Input = appendInputDeveloperMessage(request.Input, result.recordPrompt(answerFormatInstruction))
	}

	promptItems, err := o.createPromptInputItems(ctx, logger, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}
	request.Input.OfInputItemList = append(request.Input.OfInputItemList, promptItems...)

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
			toolCfg, found := findToolByName(o.availableTools, toolName)
			if !found {
				return result, fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
			}
			tool := tools.NewDockerTool(toolCfg, toolSelection.MaxCalls, toolSelection.Timeout, toolSelection.MaxMemoryMB, toolSelection.CpuPercent)
			executor.RegisterTool(tool)
			request.Tools = append(request.Tools, responses.ToolUnionParam{
				OfFunction: &responses.FunctionToolParam{
					Name:        toolCfg.Name,
					Description: param.NewOpt(toolCfg.Description),
					Strict:      param.NewOpt(false),
					Parameters:  toolCfg.Parameters,
				},
			})
		}
		request.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	}

	// Conversation loop to handle tool calls.
	var turn int
	for {
		turn++
		if err := AssertTurnsAvailable(ctx, logger, task, turn); err != nil {
			return result, err
		}

		// Create a fresh response handler for each API call.
		handler := o.newResponseHandler()

		resp, err := timed(func() (*responses.Response, error) {
			response, err := o.handleRequest(ctx, logger, request, useStreaming, handler)
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

		isTerminal := o.isTerminalResponseStatus(resp)
		logFinishReason(ctx, logger, string(resp.Status), isTerminal)

		// Handle explicit failure and cancellation statuses.
		switch resp.Status {
		case responses.ResponseStatusFailed:
			err := fmt.Errorf("%w: %s", ErrGenerateResponse, resp.Error.Message)
			if isRetryableResponseErrorCode(resp.Error.Code) {
				return result, WrapErrRetryable(err)
			}
			return result, err
		case responses.ResponseStatusCancelled:
			return result, ErrNoResponseCandidates
		case responses.ResponseStatusIncomplete:
			logger.Message(ctx, logging.LevelDebug, "response incomplete: %s", resp.IncompleteDetails.Reason)
		}

		if !isTerminal {
			// Maintain conversation context for the next turn; managed by the server.
			request.PreviousResponseID = param.NewOpt(resp.ID)

			logSkippedPreambleText(ctx, logger, string(resp.Status), resp.OutputText())

			// Execute function calls and build output items for the next turn.
			var functionCallOutputItems responses.ResponseInputParam
			for _, outputItem := range resp.Output {
				if outputItem.Type == "function_call" {
					fc := outputItem.AsFunctionCall()
					data, err := taskFilesToDataMap(ctx, task.Files)
					if err != nil {
						return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
					}
					toolResult, err := executor.ExecuteTool(ctx, logger, fc.Name, json.RawMessage(fc.Arguments), data)
					toolContent := string(toolResult)
					if err != nil {
						toolContent = formatToolExecutionError(err)
					}
					functionCallOutputItems = append(functionCallOutputItems, responses.ResponseInputItemUnionParam{
						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
							CallID: fc.CallID,
							Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
								OfString: param.NewOpt(toolContent),
							},
						},
					})
				}
			}

			if len(functionCallOutputItems) > 0 {
				// With PreviousResponseID, only tool output items are needed as new input.
				request.Input = responses.ResponseNewParamsInputUnion{
					OfInputItemList: functionCallOutputItems,
				}
			}
		} else {
			otherText, finalAnswer := o.responseOutputText(resp)
			textContent := finalAnswer
			if textContent == "" {
				textContent = otherText // fallback for models that don't emit phase
			} else if preamble := otherText; preamble != "" {
				logSkippedPreambleText(ctx, logger, string(resp.Status), preamble)
			}

			if textContent != "" {
				// Unmarshal response.
				if cfg.DisableStructuredOutput {
					err = UnmarshalUnstructuredResponse(ctx, logger, []byte(textContent), &result)
				} else {
					content := textContent
					if request.Text.Format.OfText != nil {
						content, err = utils.RepairTextJSON(content)
						if err != nil {
							return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(resp.Status))
						}
					}
					err = json.Unmarshal([]byte(content), &result)
				}
				if err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(resp.Status))
				}
				return result, nil
			}

			return result, NewErrNoActionableContent([]byte(resp.Status))
		}
	} // move to the next conversation turn
}

// createPromptInputItems builds response input items from the prompt text and optional files.
func (o *openAIResponsesProvider) createPromptInputItems(ctx context.Context, logger logging.Logger, promptText string, files []config.TaskFile, result *Result) ([]responses.ResponseInputItemUnionParam, error) {
	var content responses.EasyInputMessageContentUnionParam

	if len(files) > 0 {
		parts := make(responses.ResponseInputMessageContentListParam, 0, (len(files)*2)+1)
		for _, file := range files {
			if fileType, err := file.TypeValue(ctx); err != nil {
				return nil, err
			} else if !isSupportedImageType(fileType) {
				return nil, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
			}
			dataURL, err := file.GetDataURL(ctx)
			if err != nil {
				return nil, err
			}
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputText: &responses.ResponseInputTextParam{
					Text: result.recordPrompt(DefaultTaskFileNameInstruction(file)),
				},
			})
			detail := mapImageDetailToResponses(ctx, logger, file.GetResolvedFileOptions().ImageDetail)
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{
					ImageURL: param.NewOpt(dataURL),
					Detail:   detail,
				},
			})
		}
		// Append the prompt text after the file data for improved context integrity.
		parts = append(parts, responses.ResponseInputContentUnionParam{
			OfInputText: &responses.ResponseInputTextParam{
				Text: result.recordPrompt(promptText),
			},
		})
		content.OfInputItemContentList = parts
	} else {
		content.OfString = param.NewOpt(result.recordPrompt(promptText))
	}

	return []responses.ResponseInputItemUnionParam{
		{
			OfMessage: &responses.EasyInputMessageParam{
				Role:    responses.EasyInputMessageRoleUser,
				Content: content,
			},
		},
	}, nil
}

// mapImageDetailToResponses maps a provider-agnostic ImageDetail value to the Responses API
// image detail setting. Supports "auto", "low", "high", and "original". The generic "medium"
// value is mapped to "high" (nearest higher) to avoid artificially reducing image fidelity.
// A nil or unrecognised value maps to "auto" (default behavior); a warning is logged
// for unrecognised values.
func mapImageDetailToResponses(ctx context.Context, logger logging.Logger, detail *config.ImageDetail) responses.ResponseInputImageDetail {
	if detail != nil {
		switch *detail {
		case config.ImageDetailAuto:
			return responses.ResponseInputImageDetailAuto
		case config.ImageDetailLow:
			return responses.ResponseInputImageDetailLow
		case config.ImageDetailMedium, config.ImageDetailHigh:
			return responses.ResponseInputImageDetailHigh
		case config.ImageDetailOriginal:
			return responses.ResponseInputImageDetailOriginal
		default:
			logger.Message(ctx, logging.LevelWarn, "unsupported image detail level %q, reverting to default behavior", *detail)
		}
	}
	return responses.ResponseInputImageDetailAuto
}

// handleRequest dispatches the request to the appropriate handler based on streaming mode.
func (o *openAIResponsesProvider) handleRequest(ctx context.Context, logger logging.Logger, request responses.ResponseNewParams, streaming bool, acc ResponseAccumulator) (*responses.Response, error) {
	if streaming {
		return o.handleStreamingRequest(ctx, logger, request, acc)
	}
	return o.client.Responses.New(ctx, request)
}

// handleStreamingRequest executes a streaming Responses API request,
// delegating event accumulation to the provided ResponseAccumulator.
func (o *openAIResponsesProvider) handleStreamingRequest(ctx context.Context, logger logging.Logger, request responses.ResponseNewParams, acc ResponseAccumulator) (resp *responses.Response, err error) {
	stream := o.client.Responses.NewStreaming(ctx, request)
	defer stream.Close()

	for stream.Next() {
		if err = acc.AddEvent(ctx, logger, stream.Current()); err != nil {
			return nil, err
		}
	}
	if err = stream.Err(); err != nil {
		return nil, err
	}
	resp = acc.GetResponse()
	if resp == nil {
		return nil, ErrStreamResponse
	}
	return resp, nil
}

func (o *openAIResponsesProvider) isTransientResponse(err error) bool {
	return isOpenAITransientResponse(err)
}

// newResponseHandler returns a fresh ResponseHandler for the current API call.
// If a custom factory is set, it is used; otherwise, the defaultResponseHandler is returned.
func (o *openAIResponsesProvider) newResponseHandler() ResponseHandler {
	if o.NewResponseHandler != nil {
		return o.NewResponseHandler()
	}
	return &defaultResponseHandler{}
}

func (o *openAIResponsesProvider) Close(ctx context.Context) error {
	return nil
}

// appendInputDeveloperMessage appends a developer-role text message to the input union.
// Developer messages are used for internal instructions (response format, answer format, etc.)
// rather than user-provided content.
func appendInputDeveloperMessage(input responses.ResponseNewParamsInputUnion, text string) responses.ResponseNewParamsInputUnion {
	input.OfInputItemList = append(input.OfInputItemList, responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRoleDeveloper,
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.NewOpt(text),
			},
		},
	})
	return input
}

// isRetryableResponseErrorCode reports whether a Responses API error code represents
// a transient condition that may succeed on retry.
func isRetryableResponseErrorCode(code responses.ResponseErrorCode) bool {
	return slices.Contains([]responses.ResponseErrorCode{
		responses.ResponseErrorCodeServerError,
		responses.ResponseErrorCodeRateLimitExceeded,
		responses.ResponseErrorCodeVectorStoreTimeout,
	}, code)
}

// responseOutputText extracts text from message output items, returning two
// values: otherText contains concatenated text from all non-final_answer message
// items (commentary/preambles), and finalAnswer contains concatenated text from
// final_answer-phase message items only. Models that don't emit the phase field
// will have all text in otherText and an empty finalAnswer.
func (o *openAIResponsesProvider) responseOutputText(resp *responses.Response) (otherText, finalAnswer string) {
	var other, final strings.Builder
	for _, item := range resp.Output {
		if item.Type == "message" {
			for _, c := range item.Content {
				if c.Type == "output_text" {
					if item.Phase == responses.ResponseOutputMessagePhaseFinalAnswer {
						final.WriteString(c.Text)
					} else {
						other.WriteString(c.Text)
					}
				}
			}
		}
	}
	return other.String(), final.String()
}

// isTerminalResponseStatus reports whether the given response should terminate
// the conversation loop. When function calls are present, a completed status
// is non-terminal since the calls need processing. Incomplete responses are
// always terminal — the output is likely truncated (e.g. token limit) and any
// function calls may be incomplete. Statuses not known to be non-terminal are
// considered terminal, so new/unknown statuses safely break the loop.
func (o *openAIResponsesProvider) isTerminalResponseStatus(resp *responses.Response) bool {
	switch resp.Status {
	case "", responses.ResponseStatusQueued, responses.ResponseStatusInProgress:
		return false
	case responses.ResponseStatusCompleted:
		return !slices.ContainsFunc(resp.Output, func(item responses.ResponseOutputItemUnion) bool {
			return item.Type == "function_call"
		})
	default:
		return true
	}
}
