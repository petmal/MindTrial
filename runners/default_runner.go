// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package runners

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sethvargo/go-retry"
	"golang.org/x/time/rate"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/providers"
	"github.com/petmal/mindtrial/validators"
	"github.com/rs/zerolog"
)

const unknownLogValue = "<unknown>"
const asyncEventBufferSize = 3

// explanationSeparator is used to separate answer explanation from validation assessment.
const explanationSeparator = "\n\n\n"

type eventEmitter interface {
	emitProgressEvent()
	emitMessageEvent(message string)
}

type resultCollector interface {
	eventEmitter
	appendResult(result RunResult)
}

type resultSet struct {
	sync.RWMutex
	results       Results
	resultCounter atomic.Uint32
}

func (r *resultSet) GetResults() Results {
	if r != nil {
		r.RLock()
		defer r.RUnlock()
		return r.results
	}
	return Results{}
}

func (r *resultSet) appendResult(result RunResult) {
	r.Lock()
	defer r.Unlock()
	r.results[result.Provider] = append(r.results[result.Provider], result)
	r.resultCounter.Add(1)
}

func (r *resultSet) emitProgressEvent()        {}
func (r *resultSet) emitMessageEvent(_ string) {}

type asyncResultSet struct {
	*resultSet
	done           *sync.WaitGroup
	totalTaskCount int
	progressEvents chan float32
	messageEvents  chan string
	cancel         context.CancelFunc
}

func (r *asyncResultSet) GetResults() Results {
	if r != nil {
		r.done.Wait()
		return r.resultSet.GetResults()
	}
	return Results{}
}

func (r *asyncResultSet) ProgressEvents() <-chan float32 {
	return r.progressEvents
}

func (r *asyncResultSet) MessageEvents() <-chan string {
	return r.messageEvents
}

func (r *asyncResultSet) Cancel() {
	r.cancel()
}

func (r *asyncResultSet) emitProgressEvent() {
	select {
	case r.progressEvents <- float32(r.resultCounter.Load()) / float32(r.totalTaskCount):
	default:
		// drop event if channel is not ready or full
	}
}

func (r *asyncResultSet) emitMessageEvent(message string) {
	select {
	case r.messageEvents <- message:
	default:
		// drop event if channel is not ready or full
	}
}

// NewDefaultRunner creates a new Runner that executes tasks on all configured providers
// in parallel. The individual runs on a single provider are executed sequentially.
// It returns an error if any provider initialization fails.
func NewDefaultRunner(ctx context.Context, cfg []config.ProviderConfig, globalValidationRules config.ValidationRules, judges []config.JudgeConfig, logger zerolog.Logger) (Runner, error) {
	targets := make(map[providers.Provider][]config.RunConfig, len(cfg))
	totalTargetCount := 0
	for _, providerConfig := range cfg {
		client, err := providers.NewProvider(ctx, providerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize task runner: %w", err)
		}
		targets[client] = providerConfig.Runs
		totalTargetCount += len(providerConfig.Runs)
	}

	validatorFactory := validators.NewFactory(judges)

	return &defaultRunner{
		targets:               targets,
		totalTargetCount:      totalTargetCount,
		globalValidationRules: globalValidationRules,
		validatorFactory:      validatorFactory,
		logger:                logger,
	}, nil
}

type defaultRunner struct {
	targets               map[providers.Provider][]config.RunConfig // All tasks will be executed against all run configurations of each target provider.
	totalTargetCount      int
	globalValidationRules config.ValidationRules
	validatorFactory      *validators.Factory
	logger                zerolog.Logger
}

func (r *defaultRunner) assertCanRun(tasks []config.Task) error {
	var taskErrors []error
	for _, task := range tasks {
		// Resolve validation rules for this task.
		resolvedValidationRules := r.globalValidationRules.MergeWith(task.ValidationRules)

		// Check that if judge is enabled the configuration exists.
		if resolvedValidationRules.UseJudge() {
			if err := r.validatorFactory.AssertExists(resolvedValidationRules.Judge); err != nil {
				taskErrors = append(taskErrors, fmt.Errorf("task '%s' requires judge '%s' with variant '%s' that does not exist or is disabled: %w", task.Name, resolvedValidationRules.Judge.GetName(), resolvedValidationRules.Judge.GetVariant(), err))
			}
		}
	}

	if len(taskErrors) > 0 {
		return fmt.Errorf("could not start because:\n%w", errors.Join(taskErrors...))
	}
	return nil
}

func (r *defaultRunner) Start(ctx context.Context, tasks []config.Task) (AsyncResultSet, error) {
	if err := r.assertCanRun(tasks); err != nil {
		return nil, err
	}

	progress := make(chan float32, asyncEventBufferSize)
	messages := make(chan string, asyncEventBufferSize)
	var wg sync.WaitGroup
	wg.Add(1)
	runCtx, cancel := context.WithCancel(ctx)

	result := &asyncResultSet{
		resultSet: &resultSet{
			results: make(Results),
		},
		totalTaskCount: len(tasks) * r.totalTargetCount,
		progressEvents: progress,
		messageEvents:  messages,
		cancel:         cancel,
		done:           &wg,
	}

	var err error
	go func() {
		defer wg.Done()
		defer close(progress)
		defer close(messages)
		err = r.run(runCtx, tasks, result)
	}()

	return result, err
}

func (r *defaultRunner) Run(ctx context.Context, tasks []config.Task) (ResultSet, error) {
	if err := r.assertCanRun(tasks); err != nil {
		return nil, err
	}

	result := &resultSet{
		results: make(Results),
	}

	return result, r.run(ctx, tasks, result)
}

func (r *defaultRunner) run(ctx context.Context, tasks []config.Task, rs resultCollector) (err error) {
	r.logMessage(rs, r.logger.Info(), "starting %d task%s on %d provider%s...", pluralize(countable(len(tasks)), countable(len(r.targets)))...)
	start := time.Now()
	var wg sync.WaitGroup
	for provider, runs := range r.targets {
		wg.Add(1)
		// pass provider and its runs to avoid closure variable capture
		go func(p providers.Provider, rcs []config.RunConfig) {
			defer wg.Done()
			r.runTasks(ctx, p, rcs, tasks, rs)
		}(provider, runs)
	}
	wg.Wait()
	r.logMessage(rs, r.logger.Info(), "all tasks in all configurations have finished on all providers in %s.", time.Since(start))
	return
}

func (r *defaultRunner) logMessage(emitter eventEmitter, msgContext *zerolog.Event, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	msgContext.Msg(msg)
	emitter.emitMessageEvent(msg)
}

func (r *defaultRunner) runTasks(ctx context.Context, provider providers.Provider, runs []config.RunConfig, tasks []config.Task, rs resultCollector) {
	r.logMessage(rs, r.logger.Info(), "%s: starting %d task%s on this provider in %d configuration%s...", pluralize(provider.Name(), countable(len(tasks)), countable(len(runs)))...)
	providerStart := time.Now()
	for _, run := range runs {
		var limiter *rate.Limiter
		if run.MaxRequestsPerMinute > 0 {
			r.logMessage(rs, r.logger.Info(), "%s: %s: request rate limited to %d requests/min.", provider.Name(), run.Name, run.MaxRequestsPerMinute)
			// Allow a burst up to the per-minute limit.
			limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(run.MaxRequestsPerMinute)), run.MaxRequestsPerMinute)
		}

		for _, task := range tasks {
			if err := ctx.Err(); err != nil { // canceled or timed out
				r.logMessage(rs, r.logger.Warn().Err(err), "%s: %s: aborting remaining tasks", provider.Name(), run.Name)
				return
			}
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					r.logMessage(rs, r.logger.Warn().Err(err), "%s: %s: aborting remaining tasks", provider.Name(), run.Name)
					return
				}
			}

			runResult := RunResult{}
			r.logMessage(rs, r.logger.Info(), "%s: %s: %s: starting task...", provider.Name(), run.Name, task.Name)
			runStart := time.Now()
			r.runTask(ctx, provider, run, task, &runResult, rs, limiter)
			r.logMessage(rs, r.logger.Info(), "%s: %s: %s: task has finished in %s.", provider.Name(), run.Name, task.Name, time.Since(runStart))
			rs.appendResult(runResult)
			rs.emitProgressEvent()
		}
	}
	r.logMessage(rs, r.logger.Info(), "%s: all tasks in all configurations have finished on this provider in %s.", provider.Name(), time.Since(providerStart))
}

func (r *defaultRunner) runTask(ctx context.Context, provider providers.Provider, run config.RunConfig, task config.Task, runResult *RunResult, emitter eventEmitter, limiter *rate.Limiter) {
	// Resolve validation rules for this task.
	resolvedValidationRules := r.globalValidationRules.MergeWith(task.ValidationRules)

	// Create validator selected for this this task.
	validator, err := r.validatorFactory.GetValidator(ctx, resolvedValidationRules.Judge)
	if err != nil {
		runResult.Kind = Error
		runResult.Got = err.Error()
		return
	}

	runResult.Task = task.Name
	runResult.Provider = provider.Name()
	runResult.Run = run.Name
	runResult.Want = task.ExpectedResult.Map(func(value string) string {
		return validator.ToCanonical(resolvedValidationRules, value)
	})
	defer func() {
		if p := recover(); p != nil {
			runResult.Kind = Error
			runResult.Got = fmt.Sprintf("%v", p)
		}
	}()

	result, err := r.executeProviderRun(ctx, provider, run, task, emitter, limiter)
	usage := result.GetUsage()
	r.logMessage(emitter, r.logger.Debug(), "%s: %s: %s: token usage: [in:%s, out:%s]", provider.Name(), run.Name, task.Name, formatLogInt64(usage.InputTokens), formatLogInt64(usage.OutputTokens))
	r.logMessage(emitter, r.logger.Trace(), "%s: %s: %s: prompts:\n%s", provider.Name(), run.Name, task.Name, formatLogText(result.GetPrompts()))
	if err != nil { //nolint:gocritic
		runResult.Kind = Error
		runResult.Got = err.Error()
		if errors.Is(err, providers.ErrFeatureNotSupported) {
			runResult.Kind = NotSupported
		} else {
			r.logMessage(emitter, r.logger.Error().Err(err), "%s: %s: %s: task finished with error", provider.Name(), run.Name, task.Name)
		}
		var unmarshalErr *providers.ErrUnmarshalResponse
		if errors.As(err, &unmarshalErr) {
			runResult.Details = unmarshalErr.Details()
		}
	} else {
		r.logMessage(emitter, r.logger.Debug(), "%s: %s: %s: using %s for response evaluation", provider.Name(), run.Name, task.Name, validator.GetName())
		validationResult, err := validator.IsCorrect(ctx, resolvedValidationRules, task.ExpectedResult, result, task.Prompt, task.ResponseResultFormat)
		if err != nil { //nolint:gocritic
			runResult.Kind = Error
			runResult.Got = result.FinalAnswer
			runResult.Details = err.Error()
		} else {
			if assessmentResult := validationResult.GetAssessmentResult(); assessmentResult != nil {
				assessmentUsage := assessmentResult.GetUsage()
				r.logMessage(emitter, r.logger.Debug(), "%s: %s: %s: response evaluation token usage: [in:%s, out:%s]", provider.Name(), run.Name, task.Name, formatLogInt64(assessmentUsage.InputTokens), formatLogInt64(assessmentUsage.OutputTokens))
				r.logMessage(emitter, r.logger.Trace(), "%s: %s: %s: response evaluation prompts:\n%s", provider.Name(), run.Name, task.Name, formatLogText(assessmentResult.GetPrompts()))
				r.logMessage(emitter, r.logger.Debug(), "%s: %s: %s: response evaluation duration: %s", provider.Name(), run.Name, task.Name, assessmentResult.GetDuration())
			}

			if !validationResult.IsCorrect {
				runResult.Kind = Failure
				runResult.Got = validator.ToCanonical(resolvedValidationRules, result.FinalAnswer)
				runResult.Details = result.Explain()
				if validationExplanation := validationResult.Explain(); validationExplanation != "" {
					runResult.Details += explanationSeparator + validationExplanation
				}
			} else {
				runResult.Kind = Success
				runResult.Got = validator.ToCanonical(resolvedValidationRules, result.FinalAnswer)
				runResult.Details = result.Explain()
				if validationExplanation := validationResult.Explain(); validationExplanation != "" {
					runResult.Details += explanationSeparator + validationExplanation
				}
			}
		}
	}
	runResult.Duration = result.GetDuration()
}

func formatLogInt64(value *int64) string {
	if value != nil {
		return strconv.FormatInt(*value, 10)
	}
	return unknownLogValue
}

func formatLogText(lines []string) string {
	if len(lines) > 0 {
		return "\t" + strings.Join(lines, "\n\n\t")
	}
	return "\t" + unknownLogValue
}

func (r *defaultRunner) executeProviderRun(ctx context.Context, provider providers.Provider, run config.RunConfig, task config.Task, emitter eventEmitter, limiter *rate.Limiter) (providers.Result, error) {
	if run.RetryPolicy != nil && run.RetryPolicy.MaxRetryAttempts > 0 { // check if retry is enabled
		backoff := retry.NewExponential(time.Duration(run.RetryPolicy.InitialDelaySeconds) * time.Second)
		backoff = retry.WithMaxRetries(uint64(run.RetryPolicy.MaxRetryAttempts), backoff)
		backoff = backoffWithCallback(func(nextRetryAttempt uint64, nextDelay time.Duration) {
			r.logMessage(emitter, r.logger.Info(), "%s: %s: %s: retrying task %d/%d in %v", provider.Name(), run.Name, task.Name, nextRetryAttempt, run.RetryPolicy.MaxRetryAttempts, nextDelay)
		}, backoff)

		return retry.DoValue(ctx, backoff, func(ctx context.Context) (result providers.Result, err error) {
			if err := ctx.Err(); err != nil { // canceled or timed out
				r.logMessage(emitter, r.logger.Warn().Err(err), "%s: %s: %s: aborting task", provider.Name(), run.Name, task.Name)
				return result, err
			}
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					r.logMessage(emitter, r.logger.Warn().Err(err), "%s: %s: %s: aborting task", provider.Name(), run.Name, task.Name)
					return result, err
				}
			}

			result, err = provider.Run(ctx, run, task)
			if errors.Is(err, providers.ErrRetryable) {
				r.logMessage(emitter, r.logger.Warn().Err(err), "%s: %s: %s: task encountered a transient error", provider.Name(), run.Name, task.Name)
				return result, retry.RetryableError(err)
			}
			return result, err
		})
	} else {
		return provider.Run(ctx, run, task) // no retries enabled, run only once
	}
}

func backoffWithCallback(onBackoff func(nextRetryAttempt uint64, nextDelay time.Duration), next retry.Backoff) retry.Backoff {
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

func (r *defaultRunner) Close(ctx context.Context) {
	for provider := range r.targets {
		if err := provider.Close(ctx); err != nil {
			r.logger.Warn().Err(err).Msgf("%s: failed to close provider", provider.Name())
		}
	}
	if err := r.validatorFactory.Close(ctx); err != nil {
		r.logger.Warn().Err(err).Msg("failed to close validator factory")
	}
}

type countable int

func pluralize(tokens ...any) []interface{} {
	pluralized := make([]interface{}, 0, 2*len(tokens))
	for _, token := range tokens {
		pluralized = append(pluralized, token)
		if v, ok := any(token).(countable); ok {
			switch v {
			case 1:
				pluralized = append(pluralized, "")
			default:
				pluralized = append(pluralized, "s")
			}
		}
	}

	return pluralized
}
