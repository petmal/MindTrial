// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package execution provides unified provider execution patterns for the MindTrial application.
// It handles common execution concerns such as retry logic, rate limiting, and error handling
// that are shared between different components like task runners and validators.
package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/sethvargo/go-retry"
	"golang.org/x/time/rate"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/providers"
)

// BackoffWithCallback wraps a retry.Backoff with a callback function that is called
// before each retry attempt. The callback receives the next retry attempt number
// and the delay duration.
func BackoffWithCallback(onBackoff func(nextRetryAttempt uint64, nextDelay time.Duration), next retry.Backoff) retry.Backoff {
	var retryCounter uint64 = 0
	return retry.BackoffFunc(func() (nextDelay time.Duration, stop bool) {
		nextDelay, stop = next.Next()
		if stop {
			return
		}

		nextRetry := atomic.AddUint64(&retryCounter, 1)
		onBackoff(nextRetry, nextDelay)

		return
	})
}

// Executor provides a unified way to execute provider tasks with retry logic and rate limiting.
type Executor struct {
	Provider      providers.Provider
	RunConfig     config.RunConfig
	sharedLimiter *rate.Limiter
	limiter       *rate.Limiter
}

// NewExecutor creates a new provider executor with the given provider and run configuration.
// An optional shared rate limiter can be provided to enforce an aggregate rate limit across
// multiple executors (e.g., all runs within a provider). When both a shared limiter and
// a per-run limiter are configured, the shared limiter is checked first.
func NewExecutor(provider providers.Provider, runConfig config.RunConfig, sharedLimiter *rate.Limiter) *Executor {
	var limiter *rate.Limiter
	if runConfig.MaxRequestsPerMinute > 0 {
		ratePerSecond := rate.Limit(runConfig.MaxRequestsPerMinute) / 60
		limiter = rate.NewLimiter(ratePerSecond, runConfig.MaxRequestsPerMinute) // allow a burst up to the per-minute limit
	}

	return &Executor{
		Provider:      provider,
		RunConfig:     runConfig,
		sharedLimiter: sharedLimiter,
		limiter:       limiter,
	}
}

// Attempt wraps one provider execution attempt with optional attempt-scoped infrastructure.
// The execute callback runs the provider with the supplied environment; nil selects standalone execution.
type Attempt func(ctx context.Context, execute func(providers.ExecutionEnvironment) (providers.Result, error)) (providers.Result, error)

// ScopedEnvironment is an attempt-scoped execution environment that must be closed by its owner.
type ScopedEnvironment interface {
	providers.ExecutionEnvironment
	// Close releases all resources of the environment.
	Close(ctx context.Context) error
}

// NewScopedAttempt returns an Attempt that runs every attempt in a fresh environment created by
// newEnvironment. The environment of a failed or panicking attempt is closed; the environment of
// the successful attempt is passed to keep, which becomes responsible for closing it.
func NewScopedAttempt(logger logging.Logger, newEnvironment func(context.Context) (ScopedEnvironment, error), keep func(ScopedEnvironment)) Attempt {
	return func(ctx context.Context, execute func(providers.ExecutionEnvironment) (providers.Result, error)) (providers.Result, error) {
		environment, err := newEnvironment(ctx)
		if err != nil {
			return providers.Result{}, err
		}
		kept := false
		defer func() {
			if !kept {
				if closeErr := environment.Close(ctx); closeErr != nil {
					logger.Error(ctx, logging.LevelWarn, closeErr, "failed to close attempt environment")
				}
			}
		}()

		result, err := execute(environment)
		if err != nil {
			return result, err
		}
		keep(environment)
		kept = true
		return result, nil
	}
}

// Execute runs the task using the configured provider, applying retry logic and rate limiting as configured.
// Each attempt waits for rate-limit capacity before the attempt creates any attempt-scoped infrastructure.
// attempt may be nil for ordinary standalone execution.
func (e *Executor) Execute(ctx context.Context, logger logging.Logger, task config.Task, attempt Attempt) (providers.Result, error) {
	if attempt == nil {
		attempt = runStandalone
	}

	return e.execute(ctx, logger, func(attemptCtx context.Context) (providers.Result, error) {
		if err := e.waitForCapacity(attemptCtx, logger); err != nil {
			return providers.Result{}, err
		}
		return attempt(attemptCtx, func(environment providers.ExecutionEnvironment) (providers.Result, error) {
			return e.runProvider(attemptCtx, logger, task, environment)
		})
	})
}

func runStandalone(_ context.Context, execute func(providers.ExecutionEnvironment) (providers.Result, error)) (providers.Result, error) {
	return execute(nil)
}

func (e *Executor) execute(ctx context.Context, logger logging.Logger, attempt func(context.Context) (providers.Result, error)) (result providers.Result, err error) {
	if e.RunConfig.RetryPolicy == nil || e.RunConfig.RetryPolicy.MaxRetryAttempts == 0 {
		return attempt(ctx)
	}

	backoff := retry.NewExponential(time.Duration(e.RunConfig.RetryPolicy.InitialDelaySeconds) * time.Second)
	backoff = retry.WithMaxRetries(uint64(e.RunConfig.RetryPolicy.MaxRetryAttempts), backoff)
	backoff = BackoffWithCallback(func(nextRetryAttempt uint64, nextDelay time.Duration) {
		logger.Message(ctx, logging.LevelInfo, "retrying task %d/%d in %v",
			nextRetryAttempt, e.RunConfig.RetryPolicy.MaxRetryAttempts, nextDelay)
	}, backoff)

	err = retry.Do(ctx, backoff, func(ctx context.Context) error {
		executionResult, executionError := attempt(ctx)
		result = executionResult // capture the last attempt's result
		return executionError
	})

	return result, err
}

func (e *Executor) waitForCapacity(ctx context.Context, logger logging.Logger) error {
	if err := ctx.Err(); err != nil {
		logger.Error(ctx, logging.LevelWarn, err, "aborting task")
		return err
	}

	if e.sharedLimiter != nil {
		if err := e.sharedLimiter.Wait(ctx); err != nil {
			logger.Error(ctx, logging.LevelWarn, err, "aborting task")
			return err
		}
	}

	if e.limiter != nil {
		if err := e.limiter.Wait(ctx); err != nil {
			logger.Error(ctx, logging.LevelWarn, err, "aborting task")
			return err
		}
	}
	return nil
}

func (e *Executor) runProvider(ctx context.Context, logger logging.Logger, task config.Task, environment providers.ExecutionEnvironment) (result providers.Result, err error) {
	result, err = e.Provider.Run(ctx, logger, e.RunConfig, task, environment)
	if errors.Is(err, providers.ErrRetryable) {
		logger.Error(ctx, logging.LevelWarn, err, "task encountered a transient error")
		err = retry.RetryableError(err)
	}
	return
}
