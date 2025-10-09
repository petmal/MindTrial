// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package providers implements various AI model service provider connectors supported by MindTrial.
package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/providers/tools"
	"golang.org/x/exp/constraints"
)

var (
	// ErrUnknownProviderName is returned when provider name is not recognized.
	ErrUnknownProviderName = errors.New("unknown provider name")
	// ErrCreateClient is returned when provider client initialization fails.
	ErrCreateClient = errors.New("failed to create client")
	// ErrInvalidModelParams is returned when model parameters are invalid.
	ErrInvalidModelParams = errors.New("invalid model parameters for run")
	// ErrCompileResponseSchema is returned when response schema compilation fails.
	ErrCompileResponseSchema = errors.New("failed to compile response schema")
	// ErrMalformedSchema is returned when raw schema data cannot be converted to a valid schema object.
	ErrMalformedSchema = errors.New("malformed schema")
	// ErrGenerateResponse is returned when response generation fails.
	ErrGenerateResponse = errors.New("failed to generate response")
	// ErrCreatePromptRequest is returned when request generation fails.
	ErrCreatePromptRequest = errors.New("failed to create prompt request")
	// ErrFeatureNotSupported is returned when a requested feature is not supported by the provider.
	ErrFeatureNotSupported = errors.New("feature not supported by provider")
	// ErrFileNotSupported is returned when a task context file is not supported by the provider.
	ErrFileNotSupported = fmt.Errorf("%w: file type", ErrFeatureNotSupported)
	// ErrFileUploadNotSupported is returned when file upload is not supported by the provider.
	ErrFileUploadNotSupported = fmt.Errorf("%w: file upload", ErrFeatureNotSupported)
	// ErrToolUse is returned when tool use fails.
	ErrToolUse = errors.New("tool use failed")
	// ErrToolSetup is returned when tool setup/configuration fails.
	ErrToolSetup = errors.New("tool setup failed")
	// ErrToolNotFound is returned when a requested tool is not found in available tools.
	ErrToolNotFound = errors.New("tool not found in available tools")
	// ErrRetryable is returned when an operation can be retried.
	ErrRetryable = errors.New("retryable error")
)

var supportedImageMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// Provider interacts with AI model services.
type Provider interface {
	// Name returns the provider's unique identifier.
	Name() string
	// Run executes a task using specified configuration and returns the result.
	Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error)
	// Close releases resources when the provider is no longer needed.
	Close(ctx context.Context) error
}

// ErrUnmarshalResponse is returned when response unmarshaling fails.
type ErrUnmarshalResponse struct {
	// Cause is the underlying error that caused the unmarshaling to fail.
	Cause error
	// RawMessage is the raw message that failed to be unmarshaled.
	RawMessage []byte
	// StopReason contains the reason why the AI model stopped generating the response.
	StopReason []byte
}

func (e *ErrUnmarshalResponse) Error() string {
	return fmt.Sprintf("failed to unmarshal the response: %v", e.Cause)
}

func (e *ErrUnmarshalResponse) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewErrUnmarshalResponse creates a new ErrUnmarshalResponse instance.
func NewErrUnmarshalResponse(cause error, rawMessage []byte, stopReason []byte) *ErrUnmarshalResponse {
	return &ErrUnmarshalResponse{
		Cause:      cause,
		RawMessage: rawMessage,
		StopReason: stopReason,
	}
}

// ErrAPIResponse holds additional information about an API error returned
// by a provider, including the raw HTTP response body when available.
type ErrAPIResponse struct {
	// Cause is the underlying error that caused the API call to fail.
	Cause error
	// Body contains the raw HTTP response body returned by the provider API when available.
	Body []byte
}

func (e *ErrAPIResponse) Error() string {
	return e.Cause.Error()
}

func (e *ErrAPIResponse) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewErrAPIResponse creates a new ErrAPIResponse instance.
func NewErrAPIResponse(cause error, body []byte) *ErrAPIResponse {
	return &ErrAPIResponse{Cause: cause, Body: body}
}

// WrapErrRetryable wraps an error as retryable, preserving the original error chain.
func WrapErrRetryable(err error) error {
	return fmt.Errorf("%w: %w", ErrRetryable, err)
}

// WrapErrGenerateResponse wraps an error as a generate response error, preserving the original error chain.
func WrapErrGenerateResponse(err error) error {
	return fmt.Errorf("%w: %w", ErrGenerateResponse, err)
}

// ResultJSONSchema generates a JSON schema for the Result type with the given response format
// injected into the final_answer field. If responseFormat is a schema, it will be used
// for the final_answer.content field. If responseFormat is a string, the entire final_answer
// field will be replaced with a string type constraint.
func ResultJSONSchema(responseFormat config.ResponseFormat) (*jsonschema.Schema, error) {
	// Get the raw schema map with injected response format.
	schemaMap, err := ResultJSONSchemaRaw(responseFormat)
	if err != nil {
		return nil, err
	}

	schema, err := MapToJSONSchema(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCompileResponseSchema, err)
	}
	return schema, nil
}

// ResultJSONSchemaRaw generates a JSON schema for the Result type as a map with the given
// response format injected into the final_answer field.
func ResultJSONSchemaRaw(responseFormat config.ResponseFormat) (map[string]interface{}, error) {
	// Get the base schema without any response format injection.
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	baseSchema := reflector.Reflect(Result{})

	// Convert to map for easier manipulation.
	schemaBytes, err := json.Marshal(baseSchema)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCompileResponseSchema, err)
	}

	var schemaMap map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schemaMap); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCompileResponseSchema, err)
	}

	// Inject the response format into the final_answer field.
	if properties, ok := schemaMap["properties"].(map[string]interface{}); ok {
		if finalAnswerProp, ok := properties["final_answer"].(map[string]interface{}); ok {
			if schemaObj, isSchema := responseFormat.AsSchema(); isSchema {
				if finalAnswerProps, ok := finalAnswerProp["properties"].(map[string]interface{}); ok {
					// Inject the response format schema directly into final_answer.content.
					finalAnswerProps["content"] = schemaObj
				}
				// Set description on final_answer field.
				finalAnswerProp["description"] = "The container holding the definitive answer to the task or question. The answer content must directly address what was asked, strictly follow any formatting instructions provided, and conform to the specified schema."
			} else if _, isString := responseFormat.AsString(); isString {
				// For string case, overwrite the entire final_answer schema with a new string schema.
				properties["final_answer"] = map[string]interface{}{
					"type":        "string",
					"title":       finalAnswerProp["title"], // copy the original title from final_answer
					"description": "The definitive answer to the task or question, provided as plain text. This should directly address what was asked and strictly follow any formatting instructions provided.",
				}
			}
		}
	}

	return schemaMap, nil
}

// MapToJSONSchema converts a JSON schema represented as a map to a jsonschema.Schema object.
func MapToJSONSchema(schemaMap map[string]interface{}) (*jsonschema.Schema, error) {
	schemaBytes, err := json.Marshal(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedSchema, err)
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedSchema, err)
	}

	return &schema, nil
}

// DefaultResponseFormatInstruction generates default response formatting instruction
// for the given response format to be passed to AI models that require it.
func DefaultResponseFormatInstruction(responseFormat config.ResponseFormat) (string, error) {
	schema, err := ResultJSONSchema(responseFormat)
	if err != nil {
		return "", err
	}
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCompileResponseSchema, err)
	}
	return fmt.Sprintf("Structure the response according to this JSON schema: %s", schemaBytes), nil
}

// DefaultAnswerFormatInstruction generates default answer formatting instruction for a given task to be passed to the AI model.
func DefaultAnswerFormatInstruction(task config.Task) string {
	if resolvedTemplate, ok := task.GetResolvedSystemPrompt(); ok {
		return resolvedTemplate
	}
	return ""
}

// DefaultTaskFileNameInstruction generates default task file name instruction to be passed to AI models that require it.
func DefaultTaskFileNameInstruction(file config.TaskFile) string {
	return fmt.Sprintf("[file: %s]", file.Name)
}

// Usage represents aggregated usage statistics for a response, including both token
// consumption and tool execution metrics when available.
type Usage struct {
	// InputTokens used by the input if available.
	InputTokens *int64 `json:"-"`
	// OutputTokens used by the output if available.
	OutputTokens *int64 `json:"-"`
	// ToolUsage contains per-tool execution metrics collected during the run if available.
	ToolUsage map[string]tools.ToolUsage `json:"-"`
}

// Answer wraps the final answer content to separate it from response metadata.
type Answer struct {
	// Content contains the actual answer content that follows the user-defined response format.
	// For plain text response format, this will be a string.
	// For structured schema-based response format, this will be an object that conforms to the schema.
	Content interface{} `json:"content" validate:"required"`
}

// UnmarshalJSON implements json.Unmarshaler for Answer.
// It supports unmarshaling from either a string (for plain text answers)
// or a structured object with a "content" field (for structured answers).
func (a *Answer) UnmarshalJSON(data []byte) error {
	// Handle null case.
	if string(data) == "null" {
		a.Content = nil
		return nil
	}

	// Try to unmarshal as string first.
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		a.Content = str
		return nil
	}

	// Try to unmarshal as structured object with "content" field.
	// Define an alias to the Answer structure to avoid recursive unmarshaling.
	type answerAlias Answer
	aliasValue := answerAlias{}

	// Create a decoder that disallows unknown fields to ensure strict schema compliance.
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&aliasValue); err != nil {
		return err
	}
	a.Content = aliasValue.Content
	return nil
}

// MarshalJSON implements json.Marshaler for Answer.
func (a Answer) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.Content)
}

// Result represents the structured response received from an AI model.
type Result struct {
	// Title is a brief summary of the response.
	Title string `json:"title" jsonschema:"title=Response Title" jsonschema_description:"A concise, descriptive title that summarizes what this response is about. Should be brief (typically 3-8 words) and capture the essence of the task or question being answered." validate:"required"`
	// Explanation is a detailed explanation of the answer.
	Explanation string `json:"explanation" jsonschema:"title=Response Explanation" jsonschema_description:"A comprehensive explanation of the reasoning process, methodology, and context behind the final answer. This should provide clear rationale for how the answer was derived, including any relevant analysis, steps taken, or considerations made." validate:"required"`
	// FinalAnswer contains the final answer to the task's query.
	FinalAnswer Answer        `json:"final_answer" jsonschema:"title=Final Answer" validate:"required"`
	duration    time.Duration `json:"-"` // Time to generate the response.
	prompts     []string      `json:"-"` // Prompts used to generate the response.
	usage       Usage         `json:"-"` // Usage statistics.
}

// GetDuration returns the time duration it took to generate this result.
func (r Result) GetDuration() time.Duration {
	return r.duration
}

// GetPrompts returns the prompts used to generate this result.
func (r Result) GetPrompts() []string {
	return r.prompts
}

// GetUsage returns the aggregated usage statistics for this result.
func (r Result) GetUsage() Usage {
	return r.usage
}

// GetFinalAnswerContent returns the actual final answer content wrapped in the `FinalAnswer` field.
// This is a convenience method to access `Result.FinalAnswer.Content` directly.
func (r Result) GetFinalAnswerContent() interface{} {
	return r.FinalAnswer.Content
}

// timed measures the duration of a function execution
// and adds it to the provided time.Duration pointer.
// Multiple calls with the same out pointer will accumulate the durations.
func timed[T any](f func() (T, error), out *time.Duration) (response T, err error) {
	start := time.Now()
	response, err = f()
	*out += time.Since(start)
	return
}

func (r *Result) recordPrompt(prompt string) string {
	r.prompts = append(r.prompts, prompt)
	return prompt
}

func (r *Result) recordToolUsage(usage map[string]tools.ToolUsage) {
	r.usage.ToolUsage = usage
}

func recordUsage[T constraints.Signed](inputTokens *T, outputTokens *T, out *Usage) {
	addIfNotNil(&out.InputTokens, inputTokens)
	addIfNotNil(&out.OutputTokens, outputTokens)
}

// addIfNotNil adds the values from src to dst if src is not nil.
func addIfNotNil[D ~int64, S constraints.Signed](dst **D, src *S) {
	if src != nil {
		if *dst == nil {
			*dst = new(D)
		}
		**dst += D(*src)
	}
}

func isSupportedImageType(mimeType string) bool {
	return supportedImageMimeTypes[strings.ToLower(mimeType)]
}

// findToolByName searches for a tool configuration by name in the provided available tools slice.
// Returns the tool configuration and true if found, nil and false otherwise.
func findToolByName(availableTools []config.ToolConfig, name string) (*config.ToolConfig, bool) {
	for i, tool := range availableTools {
		if tool.Name == name {
			return &availableTools[i], true
		}
	}
	return nil, false
}

// formatToolExecutionError formats a tool execution error message for consistent
// error reporting across all providers.
func formatToolExecutionError(err error) string {
	return fmt.Sprintf("Tool execution failed: %v", err)
}

// taskFilesToDataMap converts a slice of TaskFile to a map of filename to binary content data.
func taskFilesToDataMap(ctx context.Context, files []config.TaskFile) (map[string][]byte, error) {
	dataMap := make(map[string][]byte, len(files))
	for _, file := range files {
		content, err := file.Content(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read content for file %q: %w", file.Name, err)
		}
		dataMap[file.Name] = content
	}

	return dataMap, nil
}
