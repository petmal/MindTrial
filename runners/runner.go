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
)

// Success indicates that task finished successfully with correct result.
// Failure indicates that task finished successfully but with incorrect result.
// Error indicates that task failed to produce a result.
const (
	Success ResultKind = iota
	Failure
	Error
)

const runResultIDPrefix = "run"

var validIDCharMatcher = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// ResultKind represents the task execution result status.
type ResultKind int

// Runner executes tasks on configured AI providers.
type Runner interface {
	// Run executes all given tasks against all run configurations and returns when done.
	Run(ctx context.Context, tasks []config.Task) error
	// GetResults returns the results from the last Run call.
	GetResults() Results
	// Close releases resources when the runner is no longer needed.
	Close(ctx context.Context)
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
	// Want is the expected answer for the task.
	Want string
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
