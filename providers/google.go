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

	genai "github.com/google/generative-ai-go/genai"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	gapioption "google.golang.org/api/option"
)

// NewGoogleAI creates a new GoogleAI provider instance with the given configuration.
// It returns an error if client initialization fails.
func NewGoogleAI(ctx context.Context, cfg config.GoogleAIClientConfig) (*GoogleAI, error) {
	client, err := genai.NewClient(ctx, gapioption.WithAPIKey(cfg.APIKey))
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
	model := o.client.GenerativeModel(cfg.Model)
	model.ResponseMIMEType = "application/json"
	model.SetCandidateCount(1) // generate only one candidate response
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"title":        {Type: genai.TypeString},
			"explanation":  {Type: genai.TypeString},
			"final_answer": {Type: genai.TypeString},
		},
	}

	systemInstructions := make([]genai.Part, 0)
	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.GoogleAIModelParams); ok {
			if modelParams.TextResponseFormat {
				model.ResponseMIMEType = "text/plain"
				model.ResponseSchema = nil
				systemInstructions = append(systemInstructions, genai.Text(result.recordPrompt(DefaultResponseFormatInstruction())))
			}
			if modelParams.Temperature != nil {
				model.Temperature = modelParams.Temperature
			}
			if modelParams.TopP != nil {
				model.TopP = modelParams.TopP
			}
			if modelParams.TopK != nil {
				model.TopK = modelParams.TopK
			}
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	systemInstructions = append(systemInstructions, genai.Text(result.recordPrompt(DefaultAnswerFormatInstruction(task))))
	model.SystemInstruction = genai.NewUserContent(systemInstructions...)

	promptParts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}

	resp, err := timed(func() (*genai.GenerateContentResponse, error) {
		return model.GenerateContent(ctx, promptParts...)
	}, &result.duration)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrGenerateResponse, err)
	}

	if resp != nil {
		if resp.UsageMetadata != nil {
			recordUsage(&resp.UsageMetadata.PromptTokenCount, &resp.UsageMetadata.CandidatesTokenCount, &result.usage)
		}
		for _, candidate := range resp.Candidates {
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					if value, ok := part.(genai.Text); ok {
						content := []byte(value)
						if model.ResponseSchema == nil {
							repaired, err := utils.RepairTextJSON(string(content))
							if err != nil {
								return result, NewErrUnmarshalResponse(err, []byte(value), []byte(candidate.FinishReason.String()))
							}
							content = []byte(repaired)
						}
						if err := json.Unmarshal(content, &result); err != nil {
							return result, NewErrUnmarshalResponse(err, []byte(value), []byte(candidate.FinishReason.String()))
						}
					}
				}
			}
		}
	}

	return result, nil
}

func (o *GoogleAI) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []genai.Part, err error) {
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
		parts = append(parts, genai.Text(result.recordPrompt(DefaultTaskFileNameInstruction(file))))
		parts = append(parts, genai.Blob{
			MIMEType: fileType,
			Data:     content,
		})
	}

	parts = append(parts, genai.Text(result.recordPrompt(promptText))) // append the prompt text after the file data for improved context integrity

	return parts, nil
}

func (o *GoogleAI) Close(ctx context.Context) error {
	return utils.NoPanic(func() error {
		return o.client.Close()
	})
}
