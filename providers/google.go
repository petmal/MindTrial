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
	"slices"
	"strings"

	"github.com/CircleCI-Research/evalbench/config"
	"github.com/CircleCI-Research/evalbench/pkg/logging"
	"github.com/CircleCI-Research/evalbench/pkg/utils"
	"github.com/CircleCI-Research/evalbench/providers/tools"
	"google.golang.org/genai"
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

func (o *GoogleAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	// Create the generation config.
	generateConfig := &genai.GenerateContentConfig{
		CandidateCount: 1,
	}

	forceTextResponseFormat := cfg.DisableStructuredOutput

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
			generateConfig.Tools = append(generateConfig.Tools, &genai.Tool{
				FunctionDeclarations: []*genai.FunctionDeclaration{{
					Name:                 toolCfg.Name,
					Description:          toolCfg.Description,
					ParametersJsonSchema: toolCfg.Parameters,
				}},
			})
		}
		// If user tools are present, allow auto tool choice.
		generateConfig.ToolConfig = &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAuto,
			},
		}
	}

	// Handle model parameters.
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.GoogleAIModelParams); ok {
			// Apply TextResponseFormat (all tasks) or TextResponseFormatWithTools (only tasks with tools).
			// Preserve disableStructuredOutput - it always forces text format.
			forceTextResponseFormat = cfg.DisableStructuredOutput ||
				modelParams.TextResponseFormat ||
				(len(generateConfig.Tools) > 0 && modelParams.TextResponseFormatWithTools)

			// Apply ThinkingLevel parameter.
			if modelParams.ThinkingLevel != nil {
				var thinkingLevel genai.ThinkingLevel
				switch *modelParams.ThinkingLevel {
				case "minimal":
					thinkingLevel = genai.ThinkingLevelMinimal
				case "low":
					thinkingLevel = genai.ThinkingLevelLow
				case "medium":
					thinkingLevel = genai.ThinkingLevelMedium
				case "high":
					thinkingLevel = genai.ThinkingLevelHigh
				default:
					thinkingLevel = genai.ThinkingLevelUnspecified
				}
				generateConfig.ThinkingConfig = &genai.ThinkingConfig{
					ThinkingLevel: thinkingLevel,
				}
			}

			// Apply MediaResolution parameter.
			if modelParams.MediaResolution != nil {
				var mediaResolution genai.MediaResolution
				switch *modelParams.MediaResolution {
				case "low":
					mediaResolution = genai.MediaResolutionLow
				case "medium":
					mediaResolution = genai.MediaResolutionMedium
				case "high":
					mediaResolution = genai.MediaResolutionHigh
				default:
					mediaResolution = genai.MediaResolutionUnspecified
				}
				generateConfig.MediaResolution = mediaResolution
			}

			if modelParams.Temperature != nil {
				generateConfig.Temperature = modelParams.Temperature
			}
			if modelParams.TopP != nil {
				generateConfig.TopP = modelParams.TopP
			}
			if modelParams.TopK != nil {
				// TopK should logically be an integer (number of tokens), but the Go genai library
				// expects float32.
				generateConfig.TopK = genai.Ptr(float32(*modelParams.TopK))
			}
			if modelParams.PresencePenalty != nil {
				generateConfig.PresencePenalty = modelParams.PresencePenalty
			}
			if modelParams.FrequencyPenalty != nil {
				generateConfig.FrequencyPenalty = modelParams.FrequencyPenalty
			}
			if modelParams.Seed != nil {
				generateConfig.Seed = modelParams.Seed
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	// Configure response format.
	var systemParts []*genai.Part
	if forceTextResponseFormat {
		generateConfig.ResponseMIMEType = "text/plain"
		generateConfig.ResponseJsonSchema = nil
		// Add response format instruction only when not in unstructured mode.
		if !cfg.DisableStructuredOutput {
			responseFormatInstruction, err := DefaultResponseFormatInstruction(task.ResponseResultFormat)
			if err != nil {
				return result, err
			}
			systemParts = append(systemParts, genai.NewPartFromText(result.recordPrompt(responseFormatInstruction)))
		}
	} else {
		responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
		if err != nil {
			return result, err
		}
		generateConfig.ResponseMIMEType = "application/json"
		generateConfig.ResponseJsonSchema = responseSchema
	}

	if cfg.DisableStructuredOutput {
		systemParts = append(systemParts, genai.NewPartFromText(result.recordPrompt(DefaultUnstructuredResponseInstruction())))
	}

	// Add answer format instruction to system instructions.
	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		systemParts = append(systemParts, genai.NewPartFromText(result.recordPrompt(answerFormatInstruction)))
	}

	// Set system instruction if we have any.
	if len(systemParts) > 0 {
		generateConfig.SystemInstruction = &genai.Content{Parts: systemParts}
	}

	// Create prompt content.
	promptParts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
	if errors.Is(err, ErrFeatureNotSupported) {
		return result, err
	} else if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}

	contents := []*genai.Content{{Parts: promptParts}}

	// Conversation loop to handle tool calls.
	var turn int
	for {
		turn++
		if err := AssertTurnsAvailable(ctx, logger, task, turn); err != nil {
			return result, err
		}

		// Execute the completion request.
		resp, err := timed(func() (*genai.GenerateContentResponse, error) {
			return o.client.Models.GenerateContent(ctx, cfg.Model, contents, generateConfig)
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		if err != nil {
			return result, WrapErrGenerateResponse(err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		// Parse the completion response.

		if resp.UsageMetadata != nil {
			recordUsage(&resp.UsageMetadata.PromptTokenCount, &resp.UsageMetadata.CandidatesTokenCount, &result.usage)
		}
		if len(resp.Candidates) == 0 {
			return result, ErrNoResponseCandidates
		}
		for _, candidate := range resp.Candidates {
			isTerminal := o.hasTerminalStopReason(candidate)
			logFinishReason(ctx, logger, string(candidate.FinishReason), isTerminal)

			if !isTerminal {
				if candidate.Content != nil {
					// Append assistant content for every non-terminal turn.
					contents = append(contents, candidate.Content)

					for _, part := range candidate.Content.Parts {
						logSkippedPreambleText(ctx, logger, string(candidate.FinishReason), part.Text)
						if part.FunctionCall != nil {
							argsBytes, err := json.Marshal(part.FunctionCall.Args)
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
							wasExhausted := executor.IsToolExhausted(part.FunctionCall.Name)
							if toolResult, err := executor.ExecuteTool(ctx, logger, part.FunctionCall.Name, json.RawMessage(argsBytes), data); err != nil {
								response["error"] = formatToolExecutionError(err)
							} else {
								response["result"] = string(toolResult)
							}
							if wasExhausted {
								logger.Message(ctx, logging.LevelWarn, "tool %q was called again after max-calls error; removing it from available tools for next turn", part.FunctionCall.Name)
								removeFunctionCallFromRequestConfig(generateConfig, part.FunctionCall.Name)
							}
							functionResponseContent := genai.NewContentFromFunctionResponse(
								part.FunctionCall.Name,
								response,
								genai.RoleUser,
							)
							functionResponseContent.Parts[0].FunctionResponse.ID = part.FunctionCall.ID
							contents = append(contents, functionResponseContent)
						}
					}
				}
			} else {
				if textContent, ok := o.getMessageText(candidate); ok {
					if cfg.DisableStructuredOutput {
						err = UnmarshalUnstructuredResponse(ctx, logger, []byte(textContent), &result)
					} else {
						content := []byte(textContent)
						if generateConfig.ResponseJsonSchema == nil {
							repaired, err := utils.RepairTextJSON(textContent)
							if err != nil {
								return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(string(candidate.FinishReason)))
							}
							content = []byte(repaired)
						}
						err = json.Unmarshal(content, &result)
					}
					if err != nil {
						return result, NewErrUnmarshalResponse(err, []byte(textContent), []byte(string(candidate.FinishReason)))
					}
					return result, nil
				}

				return result, NewErrNoActionableContent([]byte(string(candidate.FinishReason)))
			}
		}
	} // move to the next conversation turn
}

func removeFunctionCallFromRequestConfig(config *genai.GenerateContentConfig, toolName string) {
	if config != nil {
		for _, tool := range config.Tools {
			if tool != nil {
				tool.FunctionDeclarations = slices.DeleteFunc(tool.FunctionDeclarations, func(declaration *genai.FunctionDeclaration) bool {
					return declaration != nil && declaration.Name == toolName
				})
			}
		}
	}
}

func (o *GoogleAI) hasTerminalStopReason(candidate *genai.Candidate) bool {
	var undefined genai.FinishReason
	if candidate == nil {
		return false
	}

	if o.hasFunctionCalls(candidate) {
		return !slices.Contains(
			[]genai.FinishReason{genai.FinishReasonStop, genai.FinishReasonUnspecified, undefined},
			candidate.FinishReason,
		)
	}

	return !slices.Contains([]genai.FinishReason{undefined, genai.FinishReasonUnspecified}, candidate.FinishReason)
}

func (o *GoogleAI) hasFunctionCalls(candidate *genai.Candidate) bool {
	return candidate != nil &&
		candidate.Content != nil &&
		slices.ContainsFunc(candidate.Content.Parts, func(part *genai.Part) bool {
			return part != nil && part.FunctionCall != nil
		})
}

func (o *GoogleAI) getMessageText(candidate *genai.Candidate) (text string, ok bool) {
	if candidate != nil && candidate.Content != nil {
		var textBuilder strings.Builder
		for _, part := range candidate.Content.Parts {
			if part != nil {
				textBuilder.WriteString(part.Text)
			}
		}
		text = textBuilder.String()
		ok = text != ""
	}
	return
}

func (o *GoogleAI) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []*genai.Part, err error) {
	for _, file := range files {
		fileType, err := file.TypeValue(ctx)
		if err != nil {
			return parts, err
		} else if !isSupportedImageType(fileType) {
			return parts, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
		}

		content, err := file.Content(ctx)
		if err != nil {
			return parts, err
		}

		// Attach file name as a text part before the blob, for reference.
		parts = append(parts, genai.NewPartFromText(result.recordPrompt(DefaultTaskFileNameInstruction(file))))
		parts = append(parts, genai.NewPartFromBytes(content, fileType))
	}

	parts = append(parts, genai.NewPartFromText(result.recordPrompt(promptText))) // append the prompt text after the file data for improved context integrity

	return parts, nil
}

func (o *GoogleAI) Close(ctx context.Context) error {
	return nil
}
