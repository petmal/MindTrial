// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	"github.com/petmal/mindtrial/providers/tools"
	"golang.org/x/time/rate"
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
		name              string
		runConfig         config.RunConfig
		sharedLimiter     *rate.Limiter
		wantLimiter       bool
		wantSharedLimiter bool
	}{
		{
			name: "without rate limiting",
			runConfig: config.RunConfig{
				Name:  "test-run",
				Model: "test-model",
			},
			wantLimiter:       false,
			wantSharedLimiter: false,
		},
		{
			name: "with rate limiting",
			runConfig: config.RunConfig{
				Name:                 "test-run",
				Model:                "test-model",
				MaxRequestsPerMinute: 60,
			},
			wantLimiter:       true,
			wantSharedLimiter: false,
		},
		{
			name: "with shared limiter",
			runConfig: config.RunConfig{
				Name:  "test-run",
				Model: "test-model",
			},
			sharedLimiter:     rate.NewLimiter(rate.Limit(2), 2),
			wantLimiter:       false,
			wantSharedLimiter: true,
		},
		{
			name: "with both rate limiting and shared limiter",
			runConfig: config.RunConfig{
				Name:                 "test-run",
				Model:                "test-model",
				MaxRequestsPerMinute: 60,
			},
			sharedLimiter:     rate.NewLimiter(rate.Limit(2), 2),
			wantLimiter:       true,
			wantSharedLimiter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewExecutor(provider, tt.runConfig, tt.sharedLimiter)

			assert.Equal(t, provider, executor.Provider)
			assert.Equal(t, tt.runConfig, executor.RunConfig)

			if tt.wantLimiter {
				assert.NotNil(t, executor.limiter)
			} else {
				assert.Nil(t, executor.limiter)
			}

			if tt.wantSharedLimiter {
				assert.NotNil(t, executor.sharedLimiter)
			} else {
				assert.Nil(t, executor.sharedLimiter)
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
	executor := NewExecutor(provider, runConfig, nil)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "success",
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	result, err := executor.Execute(context.Background(), logger, task, nil)

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

	executor := NewExecutor(provider, runConfig, nil)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "retry_1: success", // will fail once, then succeed
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	result, err := executor.Execute(context.Background(), logger, task, nil)

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

	executor := NewExecutor(provider, runConfig, nil)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "retry_3", // will fail 3 times, but only 1 retry allowed
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	_, err = executor.Execute(context.Background(), logger, task, nil)

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

	executor := NewExecutor(provider, runConfig, nil)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "error",
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	_, err = executor.Execute(context.Background(), logger, task, nil)

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
	executor := NewExecutor(provider, runConfig, nil)
	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "success",
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = executor.Execute(ctx, logger, task, nil)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestExecutor_Execute_RateLimited(t *testing.T) {
	// The per-run limiter follows the same Wait(ctx) code path;
	// its correct initialization is verified in TestNewExecutor.
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	logger := testutils.NewTestLogger(t)
	task := config.Task{
		Name:           "success",
		ExpectedResult: utils.NewValueSet("expected answer"),
	}

	// All sub-tests use 10 tokens/sec with burst 1.
	// First call consumes the burst token instantly.
	// Each subsequent call waits ~100ms for a new token.
	// 4 calls: 1 burst + 3 waits of ~100ms = ~300ms minimum.
	callCount := 4
	minExpectedDuration := 250 * time.Millisecond

	executeAndMeasure := func(t *testing.T, executor *Executor) time.Duration {
		t.Helper()
		start := time.Now()
		for range callCount {
			result, err := executor.Execute(context.Background(), logger, task, nil)
			require.NoError(t, err)
			assert.Equal(t, "expected answer", result.GetFinalAnswerContent())
		}
		return time.Since(start)
	}

	t.Run("shared limiter only", func(t *testing.T) {
		sharedLimiter := rate.NewLimiter(rate.Limit(10), 1)
		executor := NewExecutor(provider, config.RunConfig{
			Name:  "mock",
			Model: "test-model",
		}, sharedLimiter)

		elapsed := executeAndMeasure(t, executor)

		assert.GreaterOrEqual(t, elapsed, minExpectedDuration,
			"shared limiter should throttle execution to ~10 req/sec")
	})

	t.Run("per-run limiter only", func(t *testing.T) {
		// NewExecutor sets burst = MaxRequestsPerMinute, which would allow all 4
		// test calls through instantly. We inject the limiter directly with burst=1
		// so the test stays deterministic.
		perRunLimiter := rate.NewLimiter(rate.Limit(10), 1)
		executor := &Executor{
			Provider: provider,
			RunConfig: config.RunConfig{
				Name:  "mock",
				Model: "test-model",
			},
			limiter: perRunLimiter,
		}

		elapsed := executeAndMeasure(t, executor)

		assert.GreaterOrEqual(t, elapsed, minExpectedDuration,
			"per-run limiter should throttle execution to ~10 req/sec")
	})

	t.Run("both limiters with shared as bottleneck", func(t *testing.T) {
		// Shared: 10 req/sec, Per-run: 20 req/sec.
		// The slower shared limiter should be the bottleneck.
		executor := &Executor{
			Provider: provider,
			RunConfig: config.RunConfig{
				Name:  "mock",
				Model: "test-model",
			},
			sharedLimiter: rate.NewLimiter(rate.Limit(10), 1),
			limiter:       rate.NewLimiter(rate.Limit(20), 1),
		}

		elapsed := executeAndMeasure(t, executor)

		assert.GreaterOrEqual(t, elapsed, minExpectedDuration,
			"the slower shared limiter should be the bottleneck")
	})

	t.Run("both limiters with per-run as bottleneck", func(t *testing.T) {
		// Shared: 20 req/sec, Per-run: 10 req/sec.
		// The slower per-run limiter should be the bottleneck.
		executor := &Executor{
			Provider: provider,
			RunConfig: config.RunConfig{
				Name:  "mock",
				Model: "test-model",
			},
			sharedLimiter: rate.NewLimiter(rate.Limit(20), 1),
			limiter:       rate.NewLimiter(rate.Limit(10), 1),
		}

		elapsed := executeAndMeasure(t, executor)

		assert.GreaterOrEqual(t, elapsed, minExpectedDuration,
			"the slower per-run limiter should be the bottleneck")
	})
}

func TestExecutor_Execute_PreservesMetadataOnError(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	logger := testutils.NewTestLogger(t)

	t.Run("retriable error preserves result metadata", func(t *testing.T) {
		runConfig := config.RunConfig{
			Name:  "mock",
			Model: "test-model",
			RetryPolicy: &config.RetryPolicy{
				MaxRetryAttempts:    1,
				InitialDelaySeconds: 1,
			},
		}

		task := config.Task{
			Name:           "retry_3", // provider will return retryable errors before succeeding
			ExpectedResult: utils.NewValueSet("expected answer"),
		}

		// Direct provider call returns a populated Result even when it returns an error.
		directResult, directErr := provider.Run(context.Background(), logger, runConfig, task, nil)
		require.Error(t, directErr)
		assert.NotEmpty(t, directResult.GetPrompts(), "provider should populate prompts on attempt")
		assert.NotNil(t, directResult.GetUsage().InputTokens, "provider should populate usage on attempt")

		// Executor should preserve the last attempt's Result.
		executor := NewExecutor(provider, runConfig, nil)
		execResult, execErr := executor.Execute(context.Background(), logger, task, nil)
		require.Error(t, execErr)
		assert.NotEmpty(t, execResult.GetPrompts(), "executor should preserve prompts from last attempt")
		assert.NotNil(t, execResult.GetUsage().InputTokens, "executor should preserve usage from last attempt")
	})

	t.Run("hard error preserves result metadata", func(t *testing.T) {
		runConfig := config.RunConfig{
			Name:  "mock",
			Model: "test-model",
			// even without retry policy, executor should preserve attempt's result on error
		}

		task := config.Task{
			Name:           "error",
			ExpectedResult: utils.NewValueSet("expected answer"),
		}

		directResult, directErr := provider.Run(context.Background(), logger, runConfig, task, nil)
		require.Error(t, directErr)
		assert.NotEmpty(t, directResult.GetPrompts(), "provider should populate prompts on hard error")
		assert.NotNil(t, directResult.GetUsage().InputTokens, "provider should populate usage on hard error")

		executor := NewExecutor(provider, runConfig, nil)
		execResult, execErr := executor.Execute(context.Background(), logger, task, nil)
		require.Error(t, execErr)
		assert.NotEmpty(t, execResult.GetPrompts(), "executor should preserve prompts on hard error")
		assert.NotNil(t, execResult.GetUsage().InputTokens, "executor should preserve usage on hard error")
	})
}

func TestExecutor_Execute_PreservesMetadataOnSuccess(t *testing.T) {
	provider, err := createMockProvider("test-provider")
	require.NoError(t, err)

	logger := testutils.NewTestLogger(t)

	t.Run("without retry", func(t *testing.T) {
		runConfig := config.RunConfig{
			Name:  "mock",
			Model: "test-model",
		}
		executor := NewExecutor(provider, runConfig, nil)
		task := config.Task{
			Name:           "success",
			ExpectedResult: utils.NewValueSet("expected answer"),
		}

		directRes, directErr := provider.Run(context.Background(), logger, runConfig, task, nil)
		require.NoError(t, directErr)
		assert.NotEmpty(t, directRes.GetPrompts(), "provider should populate prompts on success")
		assert.NotNil(t, directRes.GetUsage().InputTokens, "provider should populate usage on success")

		res, err := executor.Execute(context.Background(), logger, task, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, res.GetPrompts(), "prompts must be populated on success")
		assert.NotNil(t, res.GetUsage().InputTokens, "usage must be populated on success")
	})

	t.Run("with retry", func(t *testing.T) {
		runConfig := config.RunConfig{
			Name:  "mock",
			Model: "test-model",
			RetryPolicy: &config.RetryPolicy{
				MaxRetryAttempts:    2,
				InitialDelaySeconds: 1,
			},
		}
		executor := NewExecutor(provider, runConfig, nil)
		task := config.Task{
			Name:           "retry_1: success",
			ExpectedResult: utils.NewValueSet("expected answer"),
		}

		res, err := executor.Execute(context.Background(), logger, task, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, res.GetPrompts(), "prompts must be populated on retry success")
		assert.NotNil(t, res.GetUsage().InputTokens, "usage must be populated on retry success")
	})
}

var errAttemptFailed = errors.New("attempt failed")

// environmentRecordingProvider records the environment of every run; the configured number of initial runs fail.
type environmentRecordingProvider struct {
	failures     int
	retryable    bool
	panics       bool
	environments []providers.ExecutionEnvironment
}

func (p *environmentRecordingProvider) Name() string {
	return "recording"
}

func (p *environmentRecordingProvider) Run(_ context.Context, _ logging.Logger, _ config.RunConfig, _ config.Task, environment providers.ExecutionEnvironment) (providers.Result, error) {
	p.environments = append(p.environments, environment)
	if p.panics {
		panic("provider panic")
	}
	if len(p.environments) <= p.failures {
		if p.retryable {
			return providers.Result{}, providers.WrapErrRetryable(errAttemptFailed)
		}
		return providers.Result{}, errAttemptFailed
	}
	return providers.Result{Title: "success"}, nil
}

func (p *environmentRecordingProvider) Close(context.Context) error {
	return nil
}

// recordingEnvironment is a ScopedEnvironment that records whether it was closed.
type recordingEnvironment struct {
	closed bool
}

func (e *recordingEnvironment) NewToolExecutor(ctx context.Context) (*tools.DockerToolExecutor, error) {
	return tools.NewDockerToolExecutor(ctx)
}

func (e *recordingEnvironment) Close(context.Context) error {
	e.closed = true
	return nil
}

func TestExecutor_Execute_ScopedAttempt(t *testing.T) {
	tests := []struct {
		name             string
		provider         *environmentRecordingProvider
		environmentErr   error
		wantErr          error
		wantPanic        bool
		wantEnvironments int
	}{
		{
			name:             "retry runs in a fresh environment and keeps the successful one",
			provider:         &environmentRecordingProvider{failures: 1, retryable: true},
			wantEnvironments: 2,
		},
		{
			name:             "failed attempt closes its environment",
			provider:         &environmentRecordingProvider{failures: 1},
			wantErr:          errAttemptFailed,
			wantEnvironments: 1,
		},
		{
			name:             "panicking attempt closes its environment",
			provider:         &environmentRecordingProvider{panics: true},
			wantPanic:        true,
			wantEnvironments: 1,
		},
		{
			name:           "environment setup failure skips the provider",
			provider:       &environmentRecordingProvider{},
			environmentErr: errors.ErrUnsupported,
			wantErr:        errors.ErrUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.NewTestLogger(t)
			executor := NewExecutor(tt.provider, config.RunConfig{
				Name:  "scoped",
				Model: "test-model",
				RetryPolicy: &config.RetryPolicy{
					MaxRetryAttempts:    1,
					InitialDelaySeconds: 1,
				},
			}, nil)

			var created []*recordingEnvironment
			var kept []ScopedEnvironment
			attempt := NewScopedAttempt(logger, func(context.Context) (ScopedEnvironment, error) {
				if tt.environmentErr != nil {
					return nil, tt.environmentErr
				}
				environment := &recordingEnvironment{}
				created = append(created, environment)
				return environment, nil
			}, func(environment ScopedEnvironment) {
				kept = append(kept, environment)
			})

			execute := func() error {
				_, err := executor.Execute(context.Background(), logger, config.Task{Name: "scoped"}, attempt)
				return err
			}
			if tt.wantPanic {
				assert.Panics(t, func() { _ = execute() })
			} else if err := execute(); tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			require.Len(t, created, tt.wantEnvironments)
			require.Len(t, tt.provider.environments, tt.wantEnvironments)
			for i, environment := range created {
				assert.Same(t, environment, tt.provider.environments[i], "attempt %d must run in its own environment", i+1)
			}
			if tt.wantErr != nil || tt.wantPanic {
				assert.Empty(t, kept)
				for _, environment := range created {
					assert.True(t, environment.closed, "failed attempts must close their environment")
				}
				return
			}
			last := created[len(created)-1]
			require.Len(t, kept, 1)
			assert.Same(t, last, kept[0])
			assert.False(t, last.closed, "the successful environment belongs to the caller")
			for _, environment := range created[:len(created)-1] {
				assert.True(t, environment.closed, "retried attempts must close their environment")
			}
		})
	}
}

func TestExecutor_Execute_WaitsForRateLimitBeforeCreatingEnvironment(t *testing.T) {
	sharedLimiter := rate.NewLimiter(rate.Every(time.Hour), 1)
	require.True(t, sharedLimiter.Allow(), "consume the only token")
	provider := &environmentRecordingProvider{}
	executor := NewExecutor(provider, config.RunConfig{Name: "limited", Model: "test-model"}, sharedLimiter)
	logger := testutils.NewTestLogger(t)

	environmentsCreated := 0
	attempt := NewScopedAttempt(logger, func(context.Context) (ScopedEnvironment, error) {
		environmentsCreated++
		return &recordingEnvironment{}, nil
	}, func(ScopedEnvironment) {})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := executor.Execute(ctx, logger, config.Task{Name: "limited"}, attempt)

	require.Error(t, err)
	assert.Zero(t, environmentsCreated, "no environment may be created while waiting for rate-limit capacity")
	assert.Empty(t, provider.environments)
}
