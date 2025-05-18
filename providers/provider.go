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
	"sync"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/utils"
	"golang.org/x/exp/constraints"
)

var (
	// ErrUnknownProviderName is returned when provider name is not recognized.
	ErrUnknownProviderName = errors.New("unknown provider name")
	// ErrCreateClient is returned when provider client initialization fails.
	ErrCreateClient = errors.New("failed to create client")
	// ErrInvalidModelParams is returned when model parameters are invalid.
	ErrInvalidModelParams = errors.New("invalid model parameters for run")
	// ErrCompileSchema is returned when response schema compilation fails.
	ErrCompileSchema = errors.New("failed to compile response schema")
	// ErrGenerateResponse is returned when response generation fails.
	ErrGenerateResponse = errors.New("failed to generate response")
	// ErrCreatePromptRequest is returned when request generation fails.
	ErrCreatePromptRequest = errors.New("failed to create prompt request")
	// ErrFeatureNotSupported is returned when a requested feature is not supported by the provider.
	ErrFeatureNotSupported = errors.New("feature not supported by provider")
	// ErrFileNotSupported is returned when a task context file is not supported by the provider.
	ErrFileNotSupported = fmt.Errorf("%w: file type", ErrFeatureNotSupported)
)

var supportedImageMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
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

// Details returns a formatted string containing the stop reason and raw message
// from the ErrUnmarshalResponse error. This provides additional context for
// debugging and understanding the error.
func (e *ErrUnmarshalResponse) Details() string {
	return fmt.Sprintf("Raw response (stop-reason='%s'):\n\n%s", e.StopReason, e.RawMessage)
}

// NewErrUnmarshalResponse creates a new ErrUnmarshalResponse instance.
func NewErrUnmarshalResponse(cause error, rawMessage []byte, stopReason []byte) *ErrUnmarshalResponse {
	return &ErrUnmarshalResponse{
		Cause:      cause,
		RawMessage: rawMessage,
		StopReason: stopReason,
	}
}

// ResultJSONSchema is a lazily initialized JSON schema for the Result type.
var ResultJSONSchema = sync.OnceValue(func() *jsonschema.Schema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	return reflector.Reflect(Result{})
})

// DefaultResponseFormatInstruction generates default response formatting instruction to be passed to AI models that require it.
var DefaultResponseFormatInstruction = sync.OnceValue(func() string {
	schema, err := json.Marshal(ResultJSONSchema())
	if err != nil {
		panic(fmt.Errorf("%w: %v", ErrCompileSchema, err))
	}
	return fmt.Sprintf("Structure the response according to this JSON schema: %s", schema)
})

// DefaultAnswerFormatInstruction generates default answer formatting instruction for a given task to be passed to the AI model.
func DefaultAnswerFormatInstruction(task config.Task) string {
	return fmt.Sprintf("Provide the final answer in exactly this format: %s", task.ResponseResultFormat)
}

// DefaultTaskFileNameInstruction generates default task file name instruction to be passed to AI models that require it.
func DefaultTaskFileNameInstruction(file config.TaskFile) string {
	return fmt.Sprintf("[file: %s]", file.Name)
}

// Usage represents the token usage statistics for a response.
type Usage struct {
	InputTokens  *int64 `json:"-"` // Tokens used by the input if available.
	OutputTokens *int64 `json:"-"` // Tokens used by the output if available.
}

// Result represents the structured response received from an AI model.
type Result struct {
	// Title is a brief summary of the response.
	Title string `json:"title" validate:"required"`
	// Explanation is a detailed explanation of the answer.
	Explanation string `json:"explanation" validate:"required"`
	// FinalAnswer is the final answer to the task's query.
	FinalAnswer string        `json:"final_answer" validate:"required"`
	duration    time.Duration `json:"-"` // Time to generate the response.
	prompts     []string      `json:"-"` // Prompts used to generate the response.
	usage       Usage         `json:"-"` // Token usage statistics.
}

// Explain returns a formatted explanation of the result as generated by the AI model.
func (r Result) Explain() string {
	return fmt.Sprintf("%s\n\n%s", r.Title, r.Explanation)
}

// GetDuration returns the time duration it took to generate this result.
func (r Result) GetDuration() time.Duration {
	return r.duration
}

// GetPrompts returns the prompts used to generate this result.
func (r Result) GetPrompts() []string {
	return r.prompts
}

// GetUsage returns the token usage statistics for this result.
func (r Result) GetUsage() Usage {
	return r.usage
}

func timed[T any](f func() (T, error), out *time.Duration) (response T, err error) {
	start := time.Now()
	response, err = f()
	*out = time.Since(start)
	return
}

func (r *Result) recordPrompt(prompt string) string {
	r.prompts = append(r.prompts, prompt)
	return prompt
}

func recordUsage[T constraints.Signed](inputTokens *T, outputTokens *T, out *Usage) {
	addIfNotNil(&out.InputTokens, inputTokens)
	addIfNotNil(&out.OutputTokens, outputTokens)
}

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

// Validator verifies AI model responses.
type Validator interface {
	// IsCorrect checks if result matches expected value.
	IsCorrect(ctx context.Context, actual Result) bool
	// ToCanonical normalizes string for validation.
	ToCanonical(value string) string
}

type defaultValidator struct {
	acceptedAnswers utils.StringSet
}

// NewDefaultValidator returns a new Validator to check results against the provided expected value(s).
func NewDefaultValidator(expected utils.StringSet) Validator {
	return defaultValidator{
		acceptedAnswers: expected,
	}
}

func (v defaultValidator) IsCorrect(ctx context.Context, actual Result) bool {
	return v.acceptedAnswers.Any(func(expected string) bool {
		return v.ToCanonical(expected) == v.ToCanonical(actual.FinalAnswer)
	})
}

func (v defaultValidator) ToCanonical(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// Provider interacts with AI model services.
type Provider interface {
	// Name returns the provider's unique identifier.
	Name() string
	// Validator creates a validator for checking response correctness.
	Validator(expected utils.StringSet) Validator
	// Run executes a task using specified configuration and returns the result.
	Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error)
	// Close releases resources when the provider is no longer needed.
	Close(ctx context.Context) error
}
