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
	"log"
	"sync"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/providers"
)

// NewDefaultRunner creates a new Runner that executes tasks on all configured providers
// in parallel. The individual runs on a single provider are executed sequentially.
// It returns an error if any provider initialization fails.
func NewDefaultRunner(ctx context.Context, cfg []config.ProviderConfig, logger *log.Logger) (Runner, error) {
	targets := make(map[providers.Provider][]config.RunConfig, len(cfg))
	for _, providerConfig := range cfg {
		client, err := providers.NewProvider(ctx, providerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize task runner: %w", err)
		}
		targets[client] = providerConfig.Runs
	}
	return &defaultRunner{
		results: make(map[string][]RunResult),
		targets: targets,
		logger:  logger,
	}, nil
}

type defaultRunner struct {
	targets     map[providers.Provider][]config.RunConfig // All tasks will be executed against all run configurations of each target provider.
	resultsLock sync.RWMutex
	results     map[string][]RunResult
	logger      *log.Logger
}

func (r *defaultRunner) Run(ctx context.Context, tasks []config.Task) error {
	r.logger.Printf("starting %d task%s on %d provider%s...\n", pluralize(countable(len(tasks)), countable(len(r.targets)))...)
	start := time.Now()
	var wg sync.WaitGroup
	for provider, runs := range r.targets {
		wg.Add(1)
		// pass provider and its runs to avoid closure variable capture
		go func(p providers.Provider, rcs []config.RunConfig) {
			defer wg.Done()
			r.runTasks(ctx, p, rcs, tasks)
		}(provider, runs)
	}
	wg.Wait()
	r.logger.Printf("all tasks in all configurations have finished on all providers in %s.\n", time.Since(start))
	return nil
}

func (r *defaultRunner) runTasks(ctx context.Context, provider providers.Provider, runs []config.RunConfig, tasks []config.Task) {
	r.logger.Printf("%s: starting %d task%s on this provider in %d configuration%s...\n", pluralize(provider.Name(), countable(len(tasks)), countable(len(runs)))...)
	providerStart := time.Now()
	for _, run := range runs {
		var rateLimiter <-chan time.Time
		if run.MaxRequestsPerMinute > 0 {
			r.logger.Printf("%s: %s: request rate limited to %d requests/min.\n", provider.Name(), run.Name, run.MaxRequestsPerMinute)
			rateLimiter = time.Tick(time.Duration(int(time.Minute/time.Microsecond)/run.MaxRequestsPerMinute) * time.Microsecond)
		}
		lastTaskIndex := len(tasks) - 1
		for i, task := range tasks {
			runResult := RunResult{}
			r.logger.Printf("%s: %s: %s: starting task...\n", provider.Name(), run.Name, task.Name)
			runStart := time.Now()
			r.runTask(ctx, provider, run, task, provider.Validator(task.ExpectedResult), &runResult)
			r.logger.Printf("%s: %s: %s: task has finished in %s.\n", provider.Name(), run.Name, task.Name, time.Since(runStart))
			r.appendResult(runResult)
			if rateLimiter != nil && i < lastTaskIndex {
				<-rateLimiter
			}
		}
	}
	r.logger.Printf("%s: all tasks in all configurations have finished on this provider in %s.\n", provider.Name(), time.Since(providerStart))
}

func (r *defaultRunner) runTask(ctx context.Context, provider providers.Provider, run config.RunConfig, task config.Task, validator providers.Validator, runResult *RunResult) {
	runResult.Task = task.Name
	runResult.Provider = provider.Name()
	runResult.Run = run.Name
	runResult.Want = validator.ToCanonical(task.ExpectedResult)
	defer func() {
		if p := recover(); p != nil {
			runResult.Kind = Error
			runResult.Got = fmt.Sprintf("%v", p)
		}
	}()
	result, err := provider.Run(ctx, run, task)
	if err != nil { //nolint:gocritic
		runResult.Kind = Error
		runResult.Got = err.Error()
		var unmarshalErr *providers.ErrUnmarshalResponse
		if errors.As(err, &unmarshalErr) {
			runResult.Details = unmarshalErr.Details()

		}
	} else if !validator.IsCorrect(ctx, result) {
		runResult.Kind = Failure
		runResult.Got = validator.ToCanonical(result.FinalAnswer)
		runResult.Details = result.Explain()
	} else {
		runResult.Kind = Success
		runResult.Got = validator.ToCanonical(result.FinalAnswer)
		runResult.Details = result.Explain()
	}
	runResult.Duration = result.GetDuration()
}

func (r *defaultRunner) appendResult(result RunResult) {
	r.resultsLock.Lock()
	defer r.resultsLock.Unlock()
	r.results[result.Provider] = append(r.results[result.Provider], result)
}

func (r *defaultRunner) GetResults() Results {
	r.resultsLock.RLock()
	defer r.resultsLock.RUnlock()
	return r.results
}

func (r *defaultRunner) Close(ctx context.Context) {
	for provider := range r.targets {
		if err := provider.Close(ctx); err != nil {
			r.logger.Printf("%s: failed to close provider: %v\n", provider.Name(), err)
		}
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
