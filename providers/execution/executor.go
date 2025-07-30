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
	Provider  providers.Provider
	RunConfig config.RunConfig
	limiter   *rate.Limiter
}

// NewExecutor creates a new provider executor with the given provider and run configuration.
func NewExecutor(provider providers.Provider, runConfig config.RunConfig) *Executor {
	var limiter *rate.Limiter
	if runConfig.MaxRequestsPerMinute > 0 {
		ratePerSecond := rate.Limit(runConfig.MaxRequestsPerMinute) / 60
		limiter = rate.NewLimiter(ratePerSecond, runConfig.MaxRequestsPerMinute) // allow a burst up to the per-minute limit
	}

	return &Executor{
		Provider:  provider,
		RunConfig: runConfig,
		limiter:   limiter,
	}
}

// Execute runs the task using the configured provider, applying retry logic and rate limiting as configured.
func (e *Executor) Execute(ctx context.Context, logger logging.Logger, task config.Task) (providers.Result, error) {
	if e.RunConfig.RetryPolicy != nil && e.RunConfig.RetryPolicy.MaxRetryAttempts > 0 {
		return e.executeWithRetry(ctx, logger, task)
	}
	return e.executeOnce(ctx, logger, task)
}

func (e *Executor) executeWithRetry(ctx context.Context, logger logging.Logger, task config.Task) (providers.Result, error) {
	backoff := retry.NewExponential(time.Duration(e.RunConfig.RetryPolicy.InitialDelaySeconds) * time.Second)
	backoff = retry.WithMaxRetries(uint64(e.RunConfig.RetryPolicy.MaxRetryAttempts), backoff)
	backoff = BackoffWithCallback(func(nextRetryAttempt uint64, nextDelay time.Duration) {
		logger.Message(ctx, logging.LevelInfo, "retrying task %d/%d in %v",
			nextRetryAttempt, e.RunConfig.RetryPolicy.MaxRetryAttempts, nextDelay)
	}, backoff)

	return retry.DoValue(ctx, backoff, func(ctx context.Context) (providers.Result, error) {
		return e.executeOnce(ctx, logger, task)
	})
}

func (e *Executor) executeOnce(ctx context.Context, logger logging.Logger, task config.Task) (result providers.Result, err error) {
	if err = ctx.Err(); err != nil {
		logger.Error(ctx, logging.LevelWarn, err, "aborting task")
		return
	}

	if e.limiter != nil {
		if err = e.limiter.Wait(ctx); err != nil {
			logger.Error(ctx, logging.LevelWarn, err, "aborting task")
			return
		}
	}

	result, err = e.Provider.Run(ctx, logger, e.RunConfig, task)
	if errors.Is(err, providers.ErrRetryable) {
		logger.Error(ctx, logging.LevelWarn, err, "task encountered a transient error")
		err = retry.RetryableError(err)
	}
	return
}
