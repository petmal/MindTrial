// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package runners provides interfaces and implementations for executing MindTrial tasks and collecting their results.
package runners

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/utils"
)

// Success indicates that task finished successfully with correct result.
// Failure indicates that task finished successfully but with incorrect result.
// Error indicates that task failed to produce a result.
// NotSupported indicates that task could not finish because the provider does not support the required features.
const (
	Success ResultKind = iota
	Failure
	Error
	NotSupported
)

const runResultIDPrefix = "run"

var validIDCharMatcher = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

var (
	// ErrToolNotFound is returned when a required tool is not found in the available tools.
	ErrToolNotFound = errors.New("required tool not found")
)

// ResultKind represents the task execution result status.
type ResultKind int

// Runner executes tasks on configured AI providers.
type Runner interface {
	// Run executes all given tasks against all run configurations and returns when done.
	Run(ctx context.Context, tasks []config.Task) (ResultSet, error)
	// Start executes all given tasks against all run configurations asynchronously.
	// It returns immediately and the execution continues in the background,
	// offering progress updates and messages through the returned result set.
	Start(ctx context.Context, tasks []config.Task) (AsyncResultSet, error)
	// Close releases resources when the runner is no longer needed.
	Close(ctx context.Context)
}

// ResultSet represents the outcome of executing a set of tasks.
type ResultSet interface {
	// GetResults returns the task results for each provider.
	GetResults() Results
}

// AsyncResultSet extends the basic ResultSet interface to provide asynchronous operation capabilities.
// It offers channels for monitoring progress and receiving messages during execution,
// as well as the ability to cancel the ongoing run.
type AsyncResultSet interface {
	// GetResults returns the task results for each provider.
	// The call will block until the run is finished.
	GetResults() Results
	// ProgressEvents returns a channel that emits run progress as a value between 0 and 1.
	// The channel will be closed when the run is finished.
	ProgressEvents() <-chan float32
	// MessageEvents returns a channel that emits run log messages.
	// The channel will be closed when the run is finished.
	MessageEvents() <-chan string
	// Cancel stops the ongoing run execution.
	Cancel()
}

// Results stores task results for each provider.
type Results map[string][]RunResult

// ProviderResultsByRunAndKind organizes results by run configuration and result kind.
func (r Results) ProviderResultsByRunAndKind(provider string) map[string]map[ResultKind][]RunResult {
	resultsByRunAndKind := make(map[string]map[ResultKind][]RunResult)
	for _, result := range r[provider] {
		current, ok := resultsByRunAndKind[result.Run]
		if !ok {
			current = make(map[ResultKind][]RunResult)
		}
		current[result.Kind] = append(current[result.Kind], result)
		resultsByRunAndKind[result.Run] = current
	}
	return resultsByRunAndKind
}

// RunResult represents the outcome of executing a single task.
type RunResult struct {
	// TraceID is a globally unique identifier for this specific task result, used for tracing and correlation.
	TraceID string
	// Kind indicates the result status.
	Kind ResultKind
	// Task is the name of the executed task.
	Task string
	// Provider is the name of the AI provider that executed the task.
	Provider string
	// Run is the name of the provider's run configuration used.
	Run string
	// Got is the actual answer received from the AI model.
	// For plain text response format, this should be a string that follows the format instruction precisely.
	// For structured schema-based response format, this will be any object that conforms to the schema.
	Got interface{}
	// Want are the accepted valid answer(s) for the task.
	// For plain text response format: contains string values that should follow the format instruction precisely.
	// For structured schema-based response format: contains object values that conform to the schema.
	Want utils.ValueSet
	// Details contains comprehensive information about the generated response and validation assessment.
	Details Details
	// Duration represents the time taken to generate the response.
	Duration time.Duration
}

// GetID generates a unique, sanitized identifier for the RunResult.
// The ID must be non-empty, must not contain whitespace, must begin with a letter,
// and must only include letters, digits, dashes (-), and underscores (_).
func (r RunResult) GetID() (sanitizedTaskID string) {
	uniqueTaskID := fmt.Sprintf("%s-%s-%s-%s", runResultIDPrefix, r.Provider, r.Run, r.Task)
	sanitizedTaskID = strings.ReplaceAll(uniqueTaskID, " ", "-")
	sanitizedTaskID = validIDCharMatcher.ReplaceAllString(sanitizedTaskID, "_")
	return sanitizedTaskID
}

// Details encapsulates comprehensive information about task execution and validation.
type Details struct {
	// Answer contains details about the AI model's response and reasoning process.
	Answer AnswerDetails
	// Validation contains details about the answer verification and assessment.
	Validation ValidationDetails
	// Error contains details about any errors that occurred during task execution.
	Error ErrorDetails
}

// AnswerDetails defines structured information about the AI model's response to a task.
type AnswerDetails struct {
	// Title is a descriptive header for the response produced by the target AI model.
	Title string
	// Explanation of the answer produced by the target AI model.
	Explanation []string
	// ActualAnswer is the raw answer from the target AI model split into lines.
	ActualAnswer []string
	// ExpectedAnswer is a set of all acceptable correct answers, each being an array of lines.
	ExpectedAnswer [][]string
	// Usage contains token usage statistics for generating the answer.
	Usage TokenUsage
	// ToolUsage contains aggregated statistics for any tools invoked while producing the answer.
	ToolUsage map[string]ToolUsage `json:"ToolUsage,omitempty"`
}

// ValidationDetails defines structured information about answer verification and correctness assessment.
type ValidationDetails struct {
	// Title identifies the type of validation assessment performed.
	Title string
	// Explanation contains detailed analysis of why the validation succeeded or failed.
	Explanation []string
	// Usage contains token usage statistics for the response validation step.
	// This is typically populated when using an LLM judge validator.
	Usage TokenUsage
	// ToolUsage contains aggregated statistics for any tools invoked during validation.
	ToolUsage map[string]ToolUsage `json:"ToolUsage,omitempty"`
}

// ErrorDetails defines structured information about errors that occurred during execution.
type ErrorDetails struct {
	// Title provides a summary description of the error.
	Title string
	// Message contains the primary error message.
	Message string
	// Details contains any additional error information in a generic structure.
	Details map[string][]string
	// Usage contains token usage statistics if available even in error scenarios.
	// This is typically populated if the error occurs when parsing the generated response.
	Usage TokenUsage
	// ToolUsage contains aggregated statistics for any tools invoked prior to the error.
	ToolUsage map[string]ToolUsage `json:"ToolUsage,omitempty"`
}

// TokenUsage represents token usage consumed by an LLM request.
// Values are optional and may be nil if not available.
type TokenUsage struct {
	// InputTokens is the number of tokens consumed by the prompt/input.
	InputTokens *int64 `json:"InputTokens,omitempty"`
	// OutputTokens is the number of tokens generated in the completion/output.
	OutputTokens *int64 `json:"OutputTokens,omitempty"`
}

// ToolUsage represents aggregated tool invocation statistics captured during execution.
type ToolUsage struct {
	// CallCount is the number of times the tool was invoked.
	CallCount *int64 `json:"CallCount,omitempty"`
	// TotalDuration is the cumulative execution time for the tool.
	TotalDuration *time.Duration `json:"TotalDuration,omitempty"`
}

// toLines converts an ExpectedResultSet to [][]string where each value is converted to string and split into lines.
func toLines(expectedResult utils.ValueSet) [][]string {
	expectedValues := expectedResult.Values()
	result := make([][]string, 0, len(expectedValues))
	for _, value := range expectedValues {
		result = append(result, utils.ToLines(value))
	}
	return result
}
