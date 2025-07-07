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
	"sync"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
)

var retryCountRegex = regexp.MustCompile(`^retry_(\d+)$`)

type MockProvider struct {
	name    string
	retries sync.Map
}

func (m MockProvider) Name() string {
	return m.name
}

func (m MockProvider) Validator(expected utils.StringSet, validationRules config.ValidationRules) Validator {
	return NewDefaultValidator(expected, validationRules)
}

func (m *MockProvider) Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error) {
	result = Result{
		Title: task.Name,
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

	expectedValidAnswers := task.ExpectedResult.Values()
	if cfg.Name == "pass" {
		result.Explanation = "mock pass"
		result.FinalAnswer = expectedValidAnswers[0]
	} else if cfg.Name == "mock" {
		switch task.Name {
		case "error":
			return result, fmt.Errorf("mock error")
		case "not_supported":
			return result, fmt.Errorf("%w: %s", ErrFeatureNotSupported, "mock not supported")
		case "failure":
			result.Explanation = "mock failure"
			result.FinalAnswer = "Facere aperiam recusandae totam magnam nulla corrupti."
		default:
			if cfg.RetryPolicy != nil && cfg.RetryPolicy.MaxRetryAttempts > 0 {

				// Parse expected retry count from task name if it contains "retry_N".
				expectedRetries := 0
				if matches := retryCountRegex.FindStringSubmatch(task.Name); len(matches) > 1 {
					parsed, parseErr := strconv.Atoi(matches[1])
					if parseErr != nil {
						panic(fmt.Sprintf("failed to parse retry count from task name '%s': %v", task.Name, parseErr))
					}
					expectedRetries = parsed
				}

				// Use task name with config name as unique key for retry counting.
				key := fmt.Sprintf("%s-%s", cfg.Name, task.Name)
				currentRetryCount := m.addRetryAttempt(key)

				// Return retryable error until we've seen enough retries.
				if currentRetryCount < expectedRetries {
					cause := fmt.Errorf("mock transient error (retry %d)", currentRetryCount)
					return result, WrapErrGenerateResponse(WrapErrRetryable(cause))
				}

				result.Explanation = fmt.Sprintf("mock success after %d attempts", currentRetryCount+1)
			} else {
				result.Explanation = "mock success"
			}
			result.FinalAnswer = expectedValidAnswers[0]
		}
	} else {
		result.FinalAnswer = task.Name
	}

	return result, nil
}

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
		// CompareAndSwap failed because the stored value has changed in the meantime, retry
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
