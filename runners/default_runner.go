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
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	"github.com/petmal/mindtrial/providers/execution"
	providertools "github.com/petmal/mindtrial/providers/tools"
	"github.com/petmal/mindtrial/validators"
	"github.com/rs/zerolog"
)

const asyncEventBufferSize = 3

type toolValidator interface {
	ValidateTool(ctx context.Context, cfg config.ToolConfig) error
	Close() error
}

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
func NewDefaultRunner(ctx context.Context, cfg []config.ProviderConfig, judges []config.JudgeConfig, tools []config.ToolConfig, logger zerolog.Logger) (Runner, error) {
	toolValidator, err := providertools.NewDockerToolExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tool validator: %w", err)
	}

	targets := make(map[providers.Provider][]config.RunConfig, len(cfg))
	totalTargetCount := 0
	for _, providerConfig := range cfg {
		client, err := providers.NewProvider(ctx, providerConfig, tools)
		if err != nil {
			if cleanupErr := toolValidator.Close(); cleanupErr != nil {
				logger.Warn().Err(cleanupErr).Msg("failed to close tool validator")
			}

			return nil, fmt.Errorf("failed to initialize task runner: %w", err)
		}
		targets[client] = providerConfig.Runs
		totalTargetCount += len(providerConfig.Runs)
	}

	validatorFactory := validators.NewFactory(judges)

	return &defaultRunner{
		targets:          targets,
		totalTargetCount: totalTargetCount,
		validatorFactory: validatorFactory,
		tools:            tools,
		logger:           logger,
		toolValidator:    toolValidator,
	}, nil
}

type defaultRunner struct {
	targets          map[providers.Provider][]config.RunConfig // All tasks will be executed against all run configurations of each target provider.
	totalTargetCount int
	validatorFactory *validators.Factory
	tools            []config.ToolConfig
	logger           zerolog.Logger
	toolValidator    toolValidator
}

func (r *defaultRunner) assertCanRun(ctx context.Context, tasks []config.Task) error {
	var taskErrors []error
	availableTools := make(map[string]config.ToolConfig, len(r.tools))
	for _, toolCfg := range r.tools {
		availableTools[toolCfg.Name] = toolCfg
	}

	validatedTools := make(map[string]bool)

	for _, task := range tasks {
		// Resolve validation rules for this task.
		resolvedValidationRules := task.GetResolvedValidationRules()

		// Check that if judge is enabled the configuration exists.
		if resolvedValidationRules.UseJudge() {
			if err := r.validatorFactory.AssertExists(resolvedValidationRules.Judge); err != nil {
				taskErrors = append(taskErrors, fmt.Errorf("task '%s' requires judge '%s' with variant '%s' that does not exist or is disabled: %w", task.Name, resolvedValidationRules.Judge.GetName(), resolvedValidationRules.Judge.GetVariant(), err))
			}
		}

		// Check that all tools referenced in the task's tool selector exist in tools.
		resolvedToolSelector := task.GetResolvedToolSelector()
		enabledTools, _ := resolvedToolSelector.GetEnabledToolsByName()
		for toolName := range enabledTools {
			toolCfg, exists := availableTools[toolName]
			if !exists {
				taskErrors = append(taskErrors, fmt.Errorf("%w: task '%s' requires tool '%s' that does not exist in tools", ErrToolNotFound, task.Name, toolName))
				continue
			}

			// Validate tool if not already validated.
			if _, alreadyValidated := validatedTools[toolName]; !alreadyValidated {
				if err := r.toolValidator.ValidateTool(ctx, toolCfg); err != nil {
					taskErrors = append(taskErrors, fmt.Errorf("tool '%s' cannot be used: %w", toolName, err))
				}
				validatedTools[toolName] = true
			}
		}
	}

	if len(taskErrors) > 0 {
		return fmt.Errorf("could not start because:\n%w", errors.Join(taskErrors...))
	}
	return nil
}

func (r *defaultRunner) Start(ctx context.Context, tasks []config.Task) (AsyncResultSet, error) {
	if err := r.assertCanRun(ctx, tasks); err != nil {
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
	if err := r.assertCanRun(ctx, tasks); err != nil {
		return nil, err
	}

	result := &resultSet{
		results: make(Results),
	}

	return result, r.run(ctx, tasks, result)
}

func (r *defaultRunner) run(ctx context.Context, tasks []config.Task, rs resultCollector) (err error) {
	logger := NewEmittingLogger(r.logger, rs)
	logger.Message(ctx, logging.LevelInfo, "starting %d task%s on %d provider%s...", pluralize(countable(len(tasks)), countable(len(r.targets)))...)
	start := time.Now()
	var wg sync.WaitGroup
	for provider, runs := range r.targets {
		wg.Add(1)
		// pass provider and its runs to avoid closure variable capture
		go func(p providers.Provider, rcs []config.RunConfig) {
			defer wg.Done()
			r.runTasks(ctx, logger, p, rcs, tasks, rs)
		}(provider, runs)
	}
	wg.Wait()
	logger.Message(ctx, logging.LevelInfo, "all tasks in all configurations have finished on all providers in %s.", time.Since(start))
	return
}

func (r *defaultRunner) runTasks(ctx context.Context, logger logging.Logger, provider providers.Provider, runs []config.RunConfig, tasks []config.Task, rs resultCollector) {
	logger.Message(ctx, logging.LevelInfo, "%s: starting %d task%s on this provider in %d configuration%s...", pluralize(provider.Name(), countable(len(tasks)), countable(len(runs)))...)
	providerStart := time.Now()
	for _, run := range runs {
		if run.MaxRequestsPerMinute > 0 {
			logger.Message(ctx, logging.LevelInfo, "%s: %s: request rate limited to %d requests/min.", provider.Name(), run.Name, run.MaxRequestsPerMinute)
		}
		executor := execution.NewExecutor(provider, run)

		for _, task := range tasks {
			runResult := RunResult{TraceID: ulid.Make().String()}

			// Create prefixed logger for this specific task.
			taskLogger := logger.WithContext(fmt.Sprintf("[%s] %s: %s: %s: ", runResult.TraceID, provider.Name(), run.Name, task.Name))

			taskLogger.Message(ctx, logging.LevelInfo, "starting task...")
			runStart := time.Now()
			r.runTask(ctx, taskLogger, executor, task, &runResult)
			taskLogger.Message(ctx, logging.LevelInfo, "task has finished in %s.", time.Since(runStart))
			rs.appendResult(runResult)
			rs.emitProgressEvent()
		}
	}
	logger.Message(ctx, logging.LevelInfo, "%s: all tasks in all configurations have finished on this provider in %s.", provider.Name(), time.Since(providerStart))
}

func (r *defaultRunner) runTask(ctx context.Context, logger logging.Logger, executor *execution.Executor, task config.Task, runResult *RunResult) {
	// Resolve validation rules for this task.
	resolvedValidationRules := task.GetResolvedValidationRules()

	// Create validator selected for this task.
	validator, err := r.validatorFactory.GetValidator(ctx, resolvedValidationRules.Judge)
	if err != nil {
		runResult.Kind = Error
		runResult.Got = err.Error()
		runResult.Details.Error = ErrorDetails{
			Title:   "Configuration Error",
			Message: err.Error(),
		}
		return
	}

	runResult.Task = task.Name
	runResult.Provider = executor.Provider.Name()
	runResult.Run = executor.RunConfig.Name
	runResult.Want = task.ExpectedResult.Map(func(value interface{}) interface{} {
		return validator.ToCanonical(resolvedValidationRules, value)
	})
	defer func() {
		if p := recover(); p != nil {
			msg := fmt.Sprintf("%v", p)
			runResult.Kind = Error
			runResult.Got = msg
			runResult.Details.Error = ErrorDetails{
				Title:   "Execution Error",
				Message: msg,
			}
		}
	}()

	result, err := executor.Execute(ctx, logger, task)
	usage := result.GetUsage()
	logger.Message(ctx, logging.LevelDebug, "token usage: [in:%s, out:%s]", logging.FormatLogInt64(usage.InputTokens), logging.FormatLogInt64(usage.OutputTokens))
	logger.Message(ctx, logging.LevelTrace, "prompts:\n%s", logging.FormatLogText(result.GetPrompts()))
	if err != nil { //nolint:gocritic
		runResult.Kind = Error
		runResult.Got = err.Error()

		switch {
		case errors.Is(err, providers.ErrFeatureNotSupported):
			runResult.Kind = NotSupported
			runResult.Details.Error = ErrorDetails{
				Title:     "Feature Not Supported",
				Message:   err.Error(),
				Usage:     toTokenUsage(usage),
				ToolUsage: toToolUsage(usage),
			}
		default:
			var unmarshalErr *providers.ErrUnmarshalResponse
			if errors.As(err, &unmarshalErr) {
				runResult.Details.Error = ErrorDetails{
					Title:     "Response Parsing Error",
					Message:   unmarshalErr.Cause.Error(),
					Usage:     toTokenUsage(usage),
					ToolUsage: toToolUsage(usage),
				}
			} else {
				runResult.Details.Error = ErrorDetails{
					Title:     "Execution Error",
					Message:   err.Error(),
					Usage:     toTokenUsage(usage),
					ToolUsage: toToolUsage(usage),
				}
			}
			populateErrorDetails(&runResult.Details.Error, err)
			logger.Error(ctx, logging.LevelError, err, "task finished with error")
		}
	} else {
		logger.Message(ctx, logging.LevelDebug, "using %s for response evaluation", validator.GetName())

		validationResult, err := validator.IsCorrect(ctx, logger, resolvedValidationRules, task.ExpectedResult, result, task.Prompt, task.ResponseResultFormat)
		if err != nil { //nolint:gocritic
			runResult.Kind = Error
			runResult.Got = result.GetFinalAnswerContent()
			runResult.Details.Error = ErrorDetails{
				Title:     "Validation Error",
				Message:   err.Error(),
				Usage:     toTokenUsage(validationResult.Usage),
				ToolUsage: toToolUsage(validationResult.Usage),
			}
			populateErrorDetails(&runResult.Details.Error, err)
		} else {
			if !validationResult.IsCorrect {
				runResult.Kind = Failure
			} else {
				runResult.Kind = Success
			}

			runResult.Got = validator.ToCanonical(resolvedValidationRules, result.GetFinalAnswerContent())
			runResult.Details.Validation = ValidationDetails{
				Title:       validationResult.Title,
				Explanation: utils.SplitLines(validationResult.Explanation),
				Usage:       toTokenUsage(validationResult.Usage),
				ToolUsage:   toToolUsage(validationResult.Usage),
			}
		}

		runResult.Details.Answer = AnswerDetails{
			Title:          result.Title,
			Explanation:    utils.SplitLines(result.Explanation),
			ActualAnswer:   utils.ToLines(result.GetFinalAnswerContent()),
			ExpectedAnswer: toLines(task.ExpectedResult),
			Usage:          toTokenUsage(usage),
			ToolUsage:      toToolUsage(usage),
		}
	}
	runResult.Duration = result.GetDuration()
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

	if r.toolValidator != nil {
		if err := r.toolValidator.Close(); err != nil {
			r.logger.Warn().Err(err).Msg("failed to close tool validator")
		}
	}
}

// populateErrorDetails injects additional error details into the provided ErrorDetails struct
// based on the error type. The Details field is populated with error-specific information.
func populateErrorDetails(errorDetails *ErrorDetails, err error) {
	var unmarshalErr *providers.ErrUnmarshalResponse
	var apiErr *providers.ErrAPIResponse

	switch {
	case errors.As(err, &unmarshalErr):
		errorDetails.Details = map[string][]string{
			"Stop Reason":  {string(unmarshalErr.StopReason)},
			"Raw Response": utils.SplitLines(string(unmarshalErr.RawMessage)),
		}
	case errors.As(err, &apiErr) && apiErr.Body != nil:
		errorDetails.Details = map[string][]string{
			"HTTP Response": utils.SplitLines(string(apiErr.Body)),
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

func toTokenUsage(u providers.Usage) TokenUsage {
	return TokenUsage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens}
}

func toToolUsage(u providers.Usage) (toolUsage map[string]ToolUsage) {
	toolUsage = make(map[string]ToolUsage, len(u.ToolUsage))
	for name, usage := range u.ToolUsage {
		callCount := usage.CallCount
		duration := time.Duration(usage.TotalTimeNs) // nanosecond is the natural unit of time.Duration
		toolUsage[name] = ToolUsage{
			CallCount:     &callCount,
			TotalDuration: &duration,
		}
	}
	return toolUsage
}
