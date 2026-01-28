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

	deepseek "github.com/cohesion-org/deepseek-go"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/providers/tools"
	"golang.org/x/exp/constraints"
)

// NewDeepseek creates a new DeepSeek provider instance with the given configuration.
func NewDeepseek(cfg config.DeepseekClientConfig, availableTools []config.ToolConfig) (*Deepseek, error) {
	opts := make([]deepseek.Option, 0)
	if cfg.RequestTimeout != nil {
		opts = append(opts, deepseek.WithTimeout(*cfg.RequestTimeout))
	}
	client, err := deepseek.NewClientWithOptions(cfg.APIKey, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateClient, err)
	}
	return &Deepseek{
		client:         client,
		availableTools: availableTools,
	}, nil
}

// Deepseek implements the Provider interface for DeepSeek generative models.
type Deepseek struct {
	client         *deepseek.Client
	availableTools []config.ToolConfig
}

func (o Deepseek) Name() string {
	return config.DEEPSEEK
}

func (o *Deepseek) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	// Initialize the response instruction depending on structured/unstructured mode.
	// In structured mode this will contain the JSON schema instruction; in unstructured
	// mode this becomes the unstructured response instruction.
	var responseFormatInstruction string
	if cfg.DisableStructuredOutput {
		responseFormatInstruction = DefaultUnstructuredResponseInstruction()
	} else {
		responseFormatInstruction, err = DefaultResponseFormatInstruction(task.ResponseResultFormat) // NOTE: required with JSONMode
		if err != nil {
			return result, err
		}
	}

	var request any
	if len(task.Files) > 0 {
		if !o.isFileUploadSupported() {
			return result, ErrFileUploadNotSupported
		}

		messages := []deepseek.ChatCompletionMessageWithImage{}
		if responseFormatInstruction != "" {
			messages = append(messages, deepseek.ChatCompletionMessageWithImage{
				Role:    deepseek.ChatMessageRoleSystem,
				Content: result.recordPrompt(responseFormatInstruction),
			})
		}

		if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
			messages = append(messages, deepseek.ChatCompletionMessageWithImage{
				Role:    deepseek.ChatMessageRoleSystem,
				Content: result.recordPrompt(answerFormatInstruction),
			})
		}

		promptParts, err := o.createPromptMessageParts(ctx, task.Prompt, task.Files, &result)
		if errors.Is(err, ErrFeatureNotSupported) {
			return result, err
		} else if err != nil {
			return result, fmt.Errorf("%w: %v", ErrCreatePromptRequest, err)
		}
		messages = append(messages, deepseek.ChatCompletionMessageWithImage{
			Role:    deepseek.ChatMessageRoleUser,
			Content: promptParts,
		})

		request = &deepseek.ChatCompletionRequestWithImage{
			Model:    cfg.Model,
			Messages: messages,
			JSONMode: !cfg.DisableStructuredOutput,
		}
	} else {
		messages := []deepseek.ChatCompletionMessage{}
		if responseFormatInstruction != "" {
			messages = append(messages, deepseek.ChatCompletionMessage{
				Role:    deepseek.ChatMessageRoleSystem,
				Content: result.recordPrompt(responseFormatInstruction),
			})
		}
		if answerFormatInstruction := DefaultAnswerFormatInstruction(task); answerFormatInstruction != "" {
			messages = append(messages, deepseek.ChatCompletionMessage{
				Role:    deepseek.ChatMessageRoleSystem,
				Content: result.recordPrompt(answerFormatInstruction),
			})
		}
		messages = append(messages, deepseek.ChatCompletionMessage{
			Role:    deepseek.ChatMessageRoleUser,
			Content: result.recordPrompt(task.Prompt),
		})

		request = &deepseek.ChatCompletionRequest{
			Model:    cfg.Model,
			Messages: messages,
			JSONMode: !cfg.DisableStructuredOutput,
		}
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
			if err := o.addToolToRequest(request, *toolCfg); err != nil {
				return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
			}
		}
		// If user tools are present, allow auto tool choice.
		o.setToolChoice(request, "auto")
	}

	if cfg.ModelParams != nil {
		if modelParams, ok := cfg.ModelParams.(config.DeepseekModelParams); ok {
			o.applyModelParameters(request, modelParams)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	// Conversation loop to handle tool calls.
	for {
		resp, err := timed(func() (*deepseek.ChatCompletionResponse, error) {
			return o.createChatCompletion(ctx, request)
		}, &result.duration)
		result.recordToolUsage(executor.GetUsageStats())
		if err != nil {

			return result, WrapErrGenerateResponse(err)
		} else if resp == nil {
			return result, nil // return current result state
		}

		recordUsage(&resp.Usage.PromptTokens, &resp.Usage.CompletionTokens, &result.usage)
		for _, candidate := range resp.Choices {
			if len(candidate.Message.ToolCalls) == 0 {
				// No tool calls, this is the final response.
				var err error
				if cfg.DisableStructuredOutput {
					err = UnmarshalUnstructuredResponse(ctx, logger, []byte(candidate.Message.Content), &result)
				} else {
					err = deepseek.NewJSONExtractor(nil).ExtractJSON(resp, &result)
				}
				if err != nil {
					return result, NewErrUnmarshalResponse(err, []byte(candidate.Message.Content), []byte(candidate.FinishReason))
				}
				return result, nil
			}

			// Append assistant message to conversation history before handling tool calls.
			o.addAssistantMessageToRequest(request, candidate.Message)

			// Handle tool calls.
			for _, toolCall := range candidate.Message.ToolCalls {
				data, err := taskFilesToDataMap(ctx, task.Files)
				if err != nil {
					return result, fmt.Errorf("%w: %v", ErrToolSetup, err)
				}
				toolResult, err := executor.ExecuteTool(ctx, logger, toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments), data)
				content := string(toolResult)
				if err != nil {
					content = formatToolExecutionError(err)
				}
				o.addToolMessageToRequest(request, toolCall.ID, content)
			}
		}
	} // move to the next conversation turn
}

func (o *Deepseek) isFileUploadSupported() bool {
	return false // NOTE: DeepSeek API does not support file upload in the current version.
}

func (o *Deepseek) createPromptMessageParts(ctx context.Context, promptText string, files []config.TaskFile, result *Result) (parts []deepseek.ContentItem, err error) {
	for _, file := range files {
		if fileType, err := file.TypeValue(ctx); err != nil {
			return parts, err
		} else if !isSupportedImageType(fileType) {
			return parts, fmt.Errorf("%w: %s", ErrFileNotSupported, fileType)
		}

		dataURL, err := file.GetDataURL(ctx)
		if err != nil {
			return parts, err
		}

		// Attach file name as a separate text block before the image.
		parts = append(parts, deepseek.ContentItem{
			Type: "text",
			Text: result.recordPrompt(DefaultTaskFileNameInstruction(file)),
		})
		parts = append(parts, deepseek.ContentItem{
			Type: "image",
			Image: &deepseek.ImageContent{
				URL: dataURL,
			},
		})
	}

	parts = append(parts, deepseek.ContentItem{
		Type: "text",
		Text: result.recordPrompt(promptText),
	}) // append the prompt text after the file data for improved context integrity

	return parts, nil
}

func (o *Deepseek) applyModelParameters(request any, modelParams config.DeepseekModelParams) {
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		setIfNotNil(&req.Temperature, modelParams.Temperature)
		setIfNotNil(&req.TopP, modelParams.TopP)
		setIfNotNil(&req.FrequencyPenalty, modelParams.FrequencyPenalty)
		setIfNotNil(&req.PresencePenalty, modelParams.PresencePenalty)
	case *deepseek.ChatCompletionRequestWithImage:
		setIfNotNil(&req.Temperature, modelParams.Temperature)
		setIfNotNil(&req.TopP, modelParams.TopP)
		setIfNotNil(&req.FrequencyPenalty, modelParams.FrequencyPenalty)
		setIfNotNil(&req.PresencePenalty, modelParams.PresencePenalty)
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
}

func setIfNotNil[T constraints.Float](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func (o *Deepseek) createChatCompletion(ctx context.Context, request any) (response *deepseek.ChatCompletionResponse, err error) {
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		response, err = o.client.CreateChatCompletion(ctx, req)
	case *deepseek.ChatCompletionRequestWithImage:
		response, err = o.client.CreateChatCompletionWithImage(ctx, req)
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
	return
}

func (o *Deepseek) addToolToRequest(request any, toolCfg config.ToolConfig) error {
	parameters, err := o.mapToFunctionParameters(toolCfg.Parameters)
	if err != nil {
		return err
	}

	tool := deepseek.Tool{
		Type: "function",
		Function: deepseek.Function{
			Name:        toolCfg.Name,
			Description: toolCfg.Description,
			Parameters:  parameters,
		},
	}
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		req.Tools = append(req.Tools, tool)
	case *deepseek.ChatCompletionRequestWithImage:
		req.Tools = append(req.Tools, tool)
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
	return nil
}

// mapToFunctionParameters converts a map[string]interface{} schema to deepseek.FunctionParameters.
func (o *Deepseek) mapToFunctionParameters(schema map[string]interface{}) (*deepseek.FunctionParameters, error) {
	jsonSchema, err := MapToJSONSchema(schema)
	if err != nil {
		return nil, err
	}

	var propertiesMap map[string]interface{}
	if properties, ok := schema["properties"]; ok {
		if propMap, ok := properties.(map[string]interface{}); ok {
			propertiesMap = propMap
		}
	}

	return &deepseek.FunctionParameters{
		Type:       jsonSchema.Type,
		Properties: propertiesMap,
		Required:   jsonSchema.Required,
	}, nil
}

func (o *Deepseek) addAssistantMessageToRequest(request any, message deepseek.Message) {
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		req.Messages = append(req.Messages, deepseek.ChatCompletionMessage{
			Role:             message.Role,
			Content:          message.Content,
			ReasoningContent: message.ReasoningContent,
			ToolCalls:        message.ToolCalls,
		})
	case *deepseek.ChatCompletionRequestWithImage:
		req.Messages = append(req.Messages, deepseek.ChatCompletionMessageWithImage{
			Role:             message.Role,
			Content:          message.Content,
			ReasoningContent: message.ReasoningContent,
			ToolCalls:        message.ToolCalls,
		})
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
}

func (o *Deepseek) addToolMessageToRequest(request any, toolCallID, content string) {
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		req.Messages = append(req.Messages, deepseek.ChatCompletionMessage{
			Role:       deepseek.ChatMessageRoleTool,
			Content:    content,
			ToolCallID: toolCallID,
		})
	case *deepseek.ChatCompletionRequestWithImage:
		req.Messages = append(req.Messages, deepseek.ChatCompletionMessageWithImage{
			Role:       deepseek.ChatMessageRoleTool,
			Content:    content,
			ToolCallID: toolCallID,
		})
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
}

func (o *Deepseek) setToolChoice(request any, toolChoice interface{}) {
	switch req := request.(type) {
	case *deepseek.ChatCompletionRequest:
		req.ToolChoice = toolChoice
	case *deepseek.ChatCompletionRequestWithImage:
		req.ToolChoice = toolChoice
	default:
		panic(fmt.Sprintf("unsupported request type: %T", request))
	}
}

func (o *Deepseek) Close(ctx context.Context) error {
	return nil
}
