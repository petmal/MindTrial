//go:build test

// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/testutils"
)

var (
	retryPattern  = regexp.MustCompile(`^retry_(\d+)(?:: (.+))?$`)
	expectedRegex = regexp.MustCompile(`Expected answer\(s\).*?:\n((?:- .+\n?)+)`)
	answerRegex   = regexp.MustCompile(`(?m)^- (.+)$`)
	actualRegex   = regexp.MustCompile(`Candidate response:\n(.+?)\n\nValidation flags:`)
)

// MockProvider provides a test implementation of the Provider interface for testing purposes.
type MockProvider struct {
	name    string
	retries sync.Map
}

func (m MockProvider) Name() string {
	return m.name
}

// Run executes a mock task and returns a simulated result based on the configuration and task name.
//
// The method supports several modes based on cfg.Name:
//   - "pass": Always returns success with the first expected answer.
//   - "mock": Handles special task names (error, not_supported, failure and and retry_N patterns).
//   - "judge_evaluation": Parses judge prompts and evaluates responses.
//   - Other: Returns the task name as the final answer.
func (m *MockProvider) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	logger.Message(ctx, logging.LevelDebug, "executing mock run for task '%s' with config '%s'", task.Name, cfg.Name)
	result = m.createBaseResult(task.Name)

	switch cfg.Name {
	case "pass":
		expectedValidAnswers := task.ExpectedResult.Values()
		return m.handlePassMode(result, expectedValidAnswers[0]), nil
	case "mock":
		return m.handleMockMode(result, cfg, task)
	case "judge_evaluation":
		return m.handleJudgeEvaluation(result, cfg, task)
	default:
		result.FinalAnswer = Answer{Content: task.Name}
		return result, nil
	}
}

// createBaseResult creates a base Result with mock data.
func (m *MockProvider) createBaseResult(taskName string) Result {
	return Result{
		Title: taskName,
		prompts: []string{
			"Porro laudantium quam voluptas.",
			"Et magnam velit unde.",
			"Dolore odio esse et esse.",
		},
		usage: Usage{
			InputTokens:  testutils.Ptr(int64(8200209999917998)),
			OutputTokens: nil,
		},
		duration: 7211609999927884 * time.Nanosecond,
	}
}

func (m *MockProvider) handlePassMode(result Result, expectedAnswer interface{}) Result {
	result.Explanation = "mock pass"
	result.FinalAnswer = Answer{Content: expectedAnswer}
	return result
}

func (m *MockProvider) handleMockMode(result Result, cfg config.RunConfig, task config.Task) (Result, error) {
	parsedResponse, retryKey, err := m.parseResponseFromExpression(cfg, task.Name, task.Name)
	if err != nil {
		return result, err
	}

	if parsedResponse == "failure" {
		result.Explanation = "mock failure"
		result.FinalAnswer = Answer{Content: "Facere aperiam recusandae totam magnam nulla corrupti."}
	} else {
		result.Explanation = m.getExplanationForRetry(retryKey)
		expectedValidAnswers := task.ExpectedResult.Values()
		result.FinalAnswer = Answer{Content: expectedValidAnswers[0]}
	}

	return result, nil
}

func (m *MockProvider) handleJudgeEvaluation(result Result, cfg config.RunConfig, task config.Task) (Result, error) {
	expectedAnswers := m.extractExpectedAnswers(task.Prompt)
	actualResponse := m.extractActualResponse(task.Prompt)

	// Handle retry pattern and special cases.
	parsedResponse, retryKey, err := m.parseResponseFromExpression(cfg, task.Name, actualResponse)
	if err != nil {
		return result, err
	}

	result.Explanation = m.getExplanationForRetry(retryKey)
	result.FinalAnswer = Answer{Content: m.evaluateResponse(parsedResponse, expectedAnswers)}
	return result, nil
}

// extractExpectedAnswers extracts and parses expected answers from the judge prompt.
func (m *MockProvider) extractExpectedAnswers(prompt string) []string {
	expectedMatches := expectedRegex.FindStringSubmatch(prompt)
	if len(expectedMatches) < 2 {
		panic("could not find expected answers in judge prompt")
	}

	answerMatches := answerRegex.FindAllStringSubmatch(expectedMatches[1], -1)
	answers := make([]string, 0, len(answerMatches))
	for _, match := range answerMatches {
		if len(match) > 1 {
			answers = append(answers, strings.TrimSpace(match[1]))
		}
	}
	return answers
}

// extractActualResponse extracts the actual response from the judge prompt.
func (m *MockProvider) extractActualResponse(prompt string) string {
	actualMatches := actualRegex.FindStringSubmatch(prompt)
	if len(actualMatches) < 2 {
		panic("could not find actual response in judge prompt")
	}
	return strings.TrimSpace(actualMatches[1])
}

// evaluateResponse compares the actual response with expected answers and returns a JSON object with "correct" boolean field.
func (m *MockProvider) evaluateResponse(actualResponse string, expectedAnswers []string) map[string]interface{} {
	actualTrimmed := strings.TrimSpace(actualResponse)
	for _, expectedAnswer := range expectedAnswers {
		if actualTrimmed == strings.TrimSpace(expectedAnswer) {
			return map[string]interface{}{"correct": true}
		}
	}
	return map[string]interface{}{"correct": false}
}

// getExplanationForRetry returns an appropriate explanation based on whether retry logic was used.
func (m *MockProvider) getExplanationForRetry(retryKey string) string {
	if retryKey != "" {
		if retryAttempts, ok := m.retries.Load(retryKey); ok {
			return fmt.Sprintf("mock success after %d attempts", retryAttempts.(int))
		}
	}
	return "mock success"
}

// parseResponseFromExpression parses expression for special values and retry patterns.
//
// It handles:
//   - Special values: "error", "not_supported"
//   - Retry patterns: "retry_N" or "retry_N: response"
//
// Returns the parsed response value, retry key (if retry logic was used), and any error.
func (m *MockProvider) parseResponseFromExpression(cfg config.RunConfig, taskName string, expression string) (responseValue string, retryKey string, err error) {
	responseValue, retryKey, err = m.handleRetryPattern(cfg, taskName, expression)

	// Handle special values.
	switch responseValue {
	case "error":
		return responseValue, retryKey, fmt.Errorf("mock error")
	case "not_supported":
		return responseValue, retryKey, fmt.Errorf("%w: %s", ErrFeatureNotSupported, "mock not supported")
	}

	return
}

// handleRetryPattern processes retry patterns in the expression.
func (m *MockProvider) handleRetryPattern(cfg config.RunConfig, taskName string, expression string) (responseValue string, retryKey string, err error) {
	responseValue = expression
	matches := retryPattern.FindStringSubmatch(expression)
	if len(matches) <= 1 {
		return responseValue, "", nil
	}

	expectedRetries, parseErr := strconv.Atoi(matches[1])
	if parseErr != nil {
		panic(fmt.Sprintf("failed to parse retry count from '%s': %v", expression, parseErr))
	}

	// Extract actual response if provided.
	if len(matches) > 2 {
		responseValue = matches[2]
	}

	// Handle retry logic if retries are configured and expected.
	if expectedRetries > 0 && cfg.RetryPolicy != nil && cfg.RetryPolicy.MaxRetryAttempts > 0 {
		retryKey = fmt.Sprintf("%s-%s", cfg.Name, taskName)
		currentRetryCount := m.addRetryAttempt(retryKey)

		if currentRetryCount < expectedRetries {
			cause := fmt.Errorf("mock transient error (retry %d)", currentRetryCount)
			return responseValue, retryKey, WrapErrGenerateResponse(WrapErrRetryable(cause))
		}
	}

	return responseValue, retryKey, nil
}

// addRetryAttempt atomically increments the retry count for the given key and returns the previous count.
func (m *MockProvider) addRetryAttempt(key string) int {
	for {
		currentVal, loaded := m.retries.LoadOrStore(key, 1)
		if !loaded {
			return 0 // key didn't exist, we stored 1, so return 0 (this is the first attempt)
		}

		// Key exists, try to increment it.
		currentCount := currentVal.(int)
		newCount := currentCount + 1

		if m.retries.CompareAndSwap(key, currentCount, newCount) {
			return currentCount // successfully incremented, return the previous count
		}
		// CompareAndSwap failed because the stored value has changed in the meantime, retry.
	}
}

func (m *MockProvider) Close(ctx context.Context) error {
	return nil
}

func NewProvider(ctx context.Context, cfg config.ProviderConfig) (Provider, error) {
	return &MockProvider{
		name: cfg.Name,
	}, nil
}
