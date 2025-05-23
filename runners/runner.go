// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package runners provides interfaces and implementations for executing MindTrial tasks and collecting their results.
package runners

import (
	"context"
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
	// Kind indicates the result status.
	Kind ResultKind
	// Task is the name of the executed task.
	Task string
	// Provider is the name of the AI provider that executed the task.
	Provider string
	// Run is the name of the provider's run configuration used.
	Run string
	// Got is the actual answer received from the AI model.
	Got string
	// Want are the accepted valid answer(s) for the task.
	Want utils.StringSet
	// Details contains additional information about the task result.
	Details string
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
