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

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"google.golang.org/genai"
)

// NewGoogleAI creates a new GoogleAI provider instance with the given configuration.
// It returns an error if client initialization fails.
func NewGoogleAI(ctx context.Context, cfg config.GoogleAIClientConfig) (*GoogleAI, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateClient, err)
	}
	return &GoogleAI{
		client: client,
	}, nil
}

// GoogleAI implements the Provider interface for Google AI generative models.
type GoogleAI struct {
	client *genai.Client
}

func (o GoogleAI) Name() string {
	return config.GOOGLE
}

func (o *GoogleAI) Run(ctx context.Context, _ logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	// Prepare the JSON schema for structured response.
	responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
	if err != nil {
		return result, err
	}

	// Create the generation config.
	generateConfig := &genai.GenerateContentConfig{
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: responseSchema,
		CandidateCount:     1,
	}

	// Collect all system instructions.
	var systemParts []*genai.Part

	// Handle model parameters.
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.GoogleAIModelParams); ok {
			if modelParams.TextResponseFormat {
				generateConfig.ResponseMIMEType = "text/plain"
				generateConfig.ResponseJsonSchema = nil
				responseFormatInstruction, err := DefaultResponseFormatInstruction(task.ResponseResultFormat)
				if err != nil {
					return result, err
				}
				// Add response format instruction to system instructions.
				systemParts = append(systemParts, genai.NewPartFromText(result.recordPrompt(responseFormatInstruction)))
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
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}

	contents := []*genai.Content{{Parts: promptParts}}

	// Execute the completion request.
	resp, err := timed(func() (*genai.GenerateContentResponse, error) {
		return o.client.Models.GenerateContent(ctx, cfg.Model, contents, generateConfig)
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	// Parse the completion response.
	if resp != nil {
		if resp.UsageMetadata != nil {
			recordUsage(&resp.UsageMetadata.PromptTokenCount, &resp.UsageMetadata.CandidatesTokenCount, &result.usage)
		}
		for _, candidate := range resp.Candidates {
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						content := []byte(part.Text)
						if generateConfig.ResponseJsonSchema == nil {
							repaired, err := utils.RepairTextJSON(part.Text)
							if err != nil {
								return result, NewErrUnmarshalResponse(err, []byte(part.Text), []byte(string(candidate.FinishReason)))
							}
							content = []byte(repaired)
						}
						if err := json.Unmarshal(content, &result); err != nil {
							return result, NewErrUnmarshalResponse(err, []byte(part.Text), []byte(string(candidate.FinishReason)))
						}
					}
				}
			}
		}
	}

	return result, nil
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
