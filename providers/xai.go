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
)

// NewXAI creates a new xAI provider instance with the given configuration.
func NewXAI(cfg config.XAIClientConfig) (*XAI, error) {
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
	return &XAI{client: client}, nil
}

// XAI implements the Provider interface for xAI.
type XAI struct {
	client *xai.APIClient
}

func (o XAI) Name() string {
	return config.XAI
}

func (o *XAI) Run(ctx context.Context, _ logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	// Prepare a completion request.
	req := xai.NewChatRequestWithDefaults()
	req.SetModel(cfg.Model)
	req.SetN(1)

	// Clear default penalty parameters to avoid model compatibility issues.
	// Some xAI models don't support these parameters, which would cause request failures.
	// These can be explicitly set later via cfg.ModelParams if needed.
	req.SetPresencePenaltyNil()
	req.SetFrequencyPenaltyNil()

	// Configure response format schema.
	responseSchema, err := ResultJSONSchemaRaw(task.ResponseResultFormat)
	if err != nil {
		return result, err
	}
	schema := map[string]interface{}{
		"schema": responseSchema,
	}
	req.SetResponseFormat(xai.ResponseFormatOneOf2AsResponseFormat(xai.NewResponseFormatOneOf2(schema, "json_schema")))

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
	if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
		sysContent := xai.StringAsContent(xai.PtrString(result.recordPrompt(answerFormatInstruction)))
		req.Messages = append(req.Messages, xai.MessageOneOfAsMessage(xai.NewMessageOneOf(sysContent, "system")))
	}

	// Add structured user messages.
	parts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
	}

	userContent := xai.ArrayOfContentPartAsContent(&parts)
	req.Messages = append(req.Messages, xai.MessageOneOf1AsMessage(xai.NewMessageOneOf1(userContent, "user")))

	// Execute the completion request.
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
	if err != nil {
		return result, WrapErrGenerateResponse(err)
	}

	// Parse the completion response.
	if resp != nil {
		if resp.Usage.IsSet() {
			if u := resp.Usage.Get(); u != nil {
				promptTokens := int64(u.PromptTokens)
				completionTokens := int64(u.CompletionTokens)
				recordUsage(&promptTokens, &completionTokens, &result.usage)
			}
		}
		for _, candidate := range resp.Choices {
			if contentPtr, ok := candidate.Message.GetContentOk(); ok && contentPtr != nil {
				content := *contentPtr
				if err := json.Unmarshal([]byte(content), &result); err != nil {
					// Stop reason may be present.
					var stopReason []byte
					if candidate.FinishReason.IsSet() {
						if fr := candidate.FinishReason.Get(); fr != nil {
							stopReason = []byte(*fr)
						}
					}
					return result, NewErrUnmarshalResponse(err, []byte(content), stopReason)
				}
			}
		}
	}

	return result, nil
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
