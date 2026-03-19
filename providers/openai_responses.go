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
	"net/http"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/CircleCI-Research/evalbench/config"
	"github.com/CircleCI-Research/evalbench/pkg/logging"
	"github.com/CircleCI-Research/evalbench/pkg/utils"
	"github.com/CircleCI-Research/evalbench/providers/tools"
)

// requiresResponsesAPI returns true if the model should use the Responses API
// instead of the Chat Completions API.
func requiresResponsesAPI(model string) bool {
	m := strings.ToLower(model)
	for _, prefix := range []string{
		"gpt-5.3", "gpt-5.4", "gpt-5.5", "gpt-5.6", "gpt-5.7", "gpt-5.8", "gpt-5.9",
		"gpt-6", "gpt-7", "gpt-8", "gpt-9",
	} {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

// RunResponses executes a task using the OpenAI Responses API (for GPT-5.3+ models).
func (o *openAIV3Provider) RunResponses(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	request := responses.ResponseNewParams{
		Model: shared.ResponsesModel(cfg.Model),
	}

	// Build input items.
	inputItems := responses.ResponseInputParam{}

	// Configure structured output via text format.
	if cfg.DisableStructuredOutput {
		request.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfText: &shared.ResponseFormatTextParam{},
			},
		}
	} else {
		schema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		request.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:   "response",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		}
	}

	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(openAIV3ModelParams); ok {
			if modelParams.ReasoningEffort != nil {
				request.Reasoning = shared.ReasoningParam{
					Effort: shared.ReasoningEffort(*modelParams.ReasoningEffort),
				}
			}
			if modelParams.Verbosity != nil {
				request.Text.Verbosity = responses.ResponseTextConfigVerbosity(*modelParams.Verbosity)
			}
			if modelParams.ResponseFormat != nil {
				if cfg.DisableStructuredOutput && *modelParams.ResponseFormat != ResponseFormatText {
					return result, ErrIncompatibleResponseFormat
				}
				if !cfg.DisableStructuredOutput {
					if *modelParams.ResponseFormat != ResponseFormatJSONSchema {
						responseFormatInstruction, err := DefaultResponseFormatInstruction(task.ResponseResultFormat)
						if err != nil {
							return result, err
						}
						inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
							OfMessage: &responses.EasyInputMessageParam{
								Role:    "user",
								Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt(result.recordPrompt(responseFormatInstruction))},
							},
						})
					}
					switch *modelParams.ResponseFormat {
					case ResponseFormatText:
						request.Text = responses.ResponseTextConfigParam{
							Format: responses.ResponseFormatTextConfigUnionParam{
								OfText: &shared.ResponseFormatTextParam{},
							},
						}
					case ResponseFormatJSONObject:
						request.Text = responses.ResponseTextConfigParam{
							Format: responses.ResponseFormatTextConfigUnionParam{
								OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
							},
						}
					}
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
			if modelParams.MaxTokens != nil {
				request.MaxOutputTokens = param.NewOpt(*modelParams.MaxTokens)
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	if cfg.DisableStructuredOutput {
		inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role:    "user",
				Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt(result.recordPrompt(DefaultUnstructuredResponseInstruction()))},
			},
		})
	}

	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role:    "user",
				Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt(result.recordPrompt(answerFormatInstruction))},
			},
		})
	}

	// Build prompt message (text + optional images).
	promptItem, err := o.createResponsesPromptItem(ctx, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}
	inputItems = append(inputItems, promptItem)

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
		request.ParallelToolCalls = param.NewOpt(true)
	}

	request.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: inputItems,
	}

	// Conversation loop to handle tool calls.
	var turn int
	for {
		turn++
		if err := AssertTurnsAvailable(ctx, logger, task, turn); err != nil {
			return result, err
		}

		resp, err := timed(func() (*responses.Response, error) {
			r, err := o.client.Responses.New(ctx, request)
			if err != nil && o.isResponsesTransient(err) {
				return r, WrapErrRetryable(err)
			}
			return r, err
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		if err != nil {
			return result, WrapErrGenerateResponse(err)
		}

		recordUsage(&resp.Usage.InputTokens, &resp.Usage.OutputTokens, &result.usage)

		// Process output items: look for function calls and message text.
		var functionCalls []responses.ResponseOutputItemUnion
		var outputText string
		for _, item := range resp.Output {
			switch item.Type {
			case "function_call":
				functionCalls = append(functionCalls, item)
			case "message":
				for _, content := range item.Content {
					if content.Type == "output_text" {
						outputText = content.Text
					}
				}
			}
		}

		if len(functionCalls) > 0 {
			// Re-send the previous response's output items back as input.
			newInputItems := responses.ResponseInputParam{}
			for _, item := range resp.Output {
				switch item.Type {
				case "function_call":
					newInputItems = append(newInputItems, responses.ResponseInputItemUnionParam{
						OfFunctionCall: &responses.ResponseFunctionToolCallParam{
							CallID:    item.CallID,
							Name:      item.Name,
							Arguments: item.Arguments.OfString,
							ID:        param.NewOpt(item.ID),
						},
					})
				case "message":
					msgParam := &responses.ResponseOutputMessageParam{
						ID:     item.ID,
						Status: responses.ResponseOutputMessageStatus(item.Status),
					}
					for _, c := range item.Content {
						if c.Type == "output_text" {
							msgParam.Content = append(msgParam.Content, responses.ResponseOutputMessageContentUnionParam{
								OfOutputText: &responses.ResponseOutputTextParam{
									Text: c.Text,
								},
							})
						}
					}
					newInputItems = append(newInputItems, responses.ResponseInputItemUnionParam{
						OfOutputMessage: msgParam,
					})
				}
			}

			// Execute each function call and append results.
			for _, fc := range functionCalls {
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				toolResult, err := executor.ExecuteTool(ctx, logger, fc.Name, json.RawMessage(fc.Arguments.OfString), data)
				toolContent := string(toolResult)
				if err != nil {
					toolContent = formatToolExecutionError(err)
				}
				newInputItems = append(newInputItems, responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: fc.CallID,
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
							OfString: param.NewOpt(toolContent),
						},
					},
				})
			}

			request.Input = responses.ResponseNewParamsInputUnion{
				OfInputItemList: newInputItems,
			}
			continue
		}

		// No function calls — process the text response.
		if outputText != "" {
			if cfg.DisableStructuredOutput {
				err = UnmarshalUnstructuredResponse(ctx, logger, []byte(outputText), &result)
			} else {
				content := outputText
				if request.Text.Format.OfText != nil {
					content, err = utils.RepairTextJSON(content)
					if err != nil {
						return result, NewErrUnmarshalResponse(err, []byte(outputText), nil)
					}
				}
				err = json.Unmarshal([]byte(content), &result)
			}
			if err != nil {
				return result, NewErrUnmarshalResponse(err, []byte(outputText), nil)
			}
			return result, nil
		}

		return result, NewErrNoActionableContent(nil)
	}
}

// createResponsesPromptItem builds the user prompt as a Responses API input item.
func (o *openAIV3Provider) createResponsesPromptItem(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (responses.ResponseInputItemUnionParam, error) {
	if len(files) > 0 {
		parts := responses.ResponseInputMessageContentListParam{}
		for _, file := range files {
			if fileType, err := file.TypeValue(ctx); err != nil {
				return responses.ResponseInputItemUnionParam{}, err
			} else if !isSupportedImageType(fileType) {
				return responses.ResponseInputItemUnionParam{}, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
			}
			dataURL, err := file.GetDataURL(ctx)
			if err != nil {
				return responses.ResponseInputItemUnionParam{}, err
			}
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputText: &responses.ResponseInputTextParam{Text: result.recordPrompt(DefaultTaskFileNameInstruction(file))},
			})
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{
					ImageURL: param.NewOpt(dataURL),
					Detail:   "auto",
				},
			})
		}
		parts = append(parts, responses.ResponseInputContentUnionParam{
			OfInputText: &responses.ResponseInputTextParam{Text: result.recordPrompt(promptText)},
		})
		return responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role:    "user",
				Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: parts},
			},
		}, nil
	}
	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role:    "user",
			Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt(result.recordPrompt(promptText))},
		},
	}, nil
}

// isResponsesTransient returns true if the error is a transient API error.
func (o *openAIV3Provider) isResponsesTransient(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return slices.Contains([]int{
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusServiceUnavailable,
		}, apiErr.StatusCode)
	}
	return false
}
