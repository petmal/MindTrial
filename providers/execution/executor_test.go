// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package execution

import (
	"context"
	"testing"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
)

func TestBackoffWithCallback(t *testing.T) {
	var callbackCalls []struct {
		attempt uint64
		delay   time.Duration
	}

	callback := func(nextRetryAttempt uint64, nextDelay time.Duration) {
		callbackCalls = append(callbackCalls, struct {
			attempt uint64
			delay   time.Duration
		}{nextRetryAttempt, nextDelay})
	}

	// Create a simple backoff that returns 3 delays then stops.
	baseBackoff := retry.BackoffFunc(func() (time.Duration, bool) {
		callCount := len(callbackCalls)
		if callCount >= 3 {
			return 0, true // stop after 3 calls
		}
		return time.Duration(callCount+1) * time.Millisecond, false
	})

	backoff := BackoffWithCallback(callback, baseBackoff)

	// Test the backoff behavior
	for i := 0; i < 5; i++ {
		delay, stop := backoff.Next()
		if stop {
			break
		}
		if i < 3 {
			expectedDelay := time.Duration(i+1) * time.Millisecond
			assert.Equal(t, expectedDelay, delay)
		}
	}

	// Verify callback was called with correct parameters.
	assert.Len(t, callbackCalls, 3)
	for i, call := range callbackCalls {
		expectedAttempt := uint64(i + 1) //nolint:gosec
		expectedDelay := time.Duration(i+1) * time.Millisecond
		assert.Equal(t, expectedAttempt, call.attempt, "Call %d: expected attempt", i)
		assert.Equal(t, expectedDelay, call.delay, "Call %d: expected delay", i)
	}
}

func createMockProvider(name string) (providers.Provider, error) {
	return providers.NewProvider(context.Background(), config.ProviderConfig{
		Name: name,
	}, nil)
}

func TestNewExecutor(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	tests := []struct {
		name        string
		runConfig   config.RunConfig
		wantLimiter bool
	}{
		{
			name: "without rate limiting",
			runConfig: config.RunConfig{
				Name:  "test-run",
				Model: "test-model",
			},
			wantLimiter: false,
		},
		{
			name: "with rate limiting",
			runConfig: config.RunConfig{
				Name:                 "test-run",
				Model:                "test-model",
				MaxRequestsPerMinute: 60,
			},
			wantLimiter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewExecutor(provider, tt.runConfig)

			assert.Equal(t, provider, executor.Provider)
			assert.Equal(t, tt.runConfig, executor.RunConfig)

			if tt.wantLimiter {
				assert.NotNil(t, executor.limiter)
			} else {
				assert.Nil(t, executor.limiter)
			}
		})
	}
}

func TestExecutor_Execute_WithoutRetry(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	runConfig := config.RunConfig{
		Name:  "mock",
		Model: "test-model",
	}
	executor := NewExecutor(provider, runConfig)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "success",
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	result, err := executor.Execute(context.Background(), logger, task)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Title)
	assert.Equal(t, "expected answer", result.GetFinalAnswerContent())
}

func TestExecutor_Execute_WithRetry_Success(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	runConfig := config.RunConfig{
		Name:  "mock",
		Model: "test-model",
		RetryPolicy: &config.RetryPolicy{
			MaxRetryAttempts:    2,
			InitialDelaySeconds: 1,
		},
	}

	executor := NewExecutor(provider, runConfig)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "retry_1: success", // will fail once, then succeed
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	result, err := executor.Execute(context.Background(), logger, task)

	require.NoError(t, err)
	assert.Equal(t, "retry_1: success", result.Title)
	assert.Contains(t, result.Explanation, "mock success after 2 attempts")
	assert.Equal(t, "expected answer", result.GetFinalAnswerContent())
}

func TestExecutor_Execute_WithRetry_Failure(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	runConfig := config.RunConfig{
		Name:  "mock",
		Model: "test-model",
		RetryPolicy: &config.RetryPolicy{
			MaxRetryAttempts:    1,
			InitialDelaySeconds: 1,
		},
	}

	executor := NewExecutor(provider, runConfig)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "retry_3", // will fail 3 times, but only 1 retry allowed
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	_, err = executor.Execute(context.Background(), logger, task)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock transient error")
}

func TestExecutor_Execute_PermanentError(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	runConfig := config.RunConfig{
		Name:  "mock",
		Model: "test-model",
		RetryPolicy: &config.RetryPolicy{
			MaxRetryAttempts:    2,
			InitialDelaySeconds: 1,
		},
	}

	executor := NewExecutor(provider, runConfig)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "error",
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	_, err = executor.Execute(context.Background(), logger, task)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock error")
}

func TestExecutor_Execute_ContextCanceled(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	runConfig := config.RunConfig{
		Name:  "mock",
		Model: "test-model",
	}
	executor := NewExecutor(provider, runConfig)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "success",
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = executor.Execute(ctx, logger, task)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}
