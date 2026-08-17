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
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"
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
// in parallel. The individual runs on a single provider are executed sequentially by default,
// or in parallel when the provider's MaxParallelRequestsPerMinute is set to a value greater than 0.
// It returns an error if any provider initialization fails.
func NewDefaultRunner(ctx context.Context, cfg []config.ProviderConfig, judges []config.JudgeConfig, tools []config.ToolConfig, logger zerolog.Logger) (Runner, error) {
	toolValidator, err := providertools.NewDockerToolExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tool validator: %w", err)
	}

	targets := make(map[providers.Provider]config.ProviderConfig, len(cfg))
	totalTargetCount := 0
	for _, providerConfig := range cfg {
		client, err := providers.NewProvider(ctx, providerConfig, tools)
		if err != nil {
			if cleanupErr := toolValidator.Close(); cleanupErr != nil {
				logger.Warn().Err(cleanupErr).Msg("failed to close tool validator")
			}

			return nil, fmt.Errorf("failed to initialize task runner: %w", err)
		}
		targets[client] = providerConfig
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
	targets          map[providers.Provider]config.ProviderConfig // All tasks will be executed against all run configurations of each target provider.
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
	for provider, providerConfig := range r.targets {
		wg.Add(1)
		go func(p providers.Provider, c config.ProviderConfig) {
			defer wg.Done()
			r.runTasks(ctx, logger, p, c, tasks, rs)
		}(provider, providerConfig)
	}
	wg.Wait()
	logger.Message(ctx, logging.LevelInfo, "all tasks in all configurations have finished on all providers in %s.", time.Since(start))
	return
}

func (r *defaultRunner) runTasks(ctx context.Context, logger logging.Logger, provider providers.Provider, providerConfig config.ProviderConfig, tasks []config.Task, rs resultCollector) {
	runs := providerConfig.Runs
	logger.Message(ctx, logging.LevelInfo, "%s: starting %d task%s on this provider in %d configuration%s...", pluralize(provider.Name(), countable(len(tasks)), countable(len(runs)))...)
	providerStart := time.Now()

	var sharedLimiter *rate.Limiter
	parallelRunsEnabled := providerConfig.MaxParallelRequestsPerMinute > 0
	if parallelRunsEnabled {
		ratePerSecond := rate.Limit(providerConfig.MaxParallelRequestsPerMinute) / 60
		sharedLimiter = rate.NewLimiter(ratePerSecond, providerConfig.MaxParallelRequestsPerMinute)
		logger.Message(ctx, logging.LevelInfo, "%s: parallel run execution enabled, aggregate rate limited to %d requests/min.", provider.Name(), providerConfig.MaxParallelRequestsPerMinute)
	}

	executeRun := func(run config.RunConfig) {
		if run.MaxRequestsPerMinute > 0 {
			logger.Message(ctx, logging.LevelInfo, "%s: %s: request rate limited to %d requests/min.", provider.Name(), run.Name, run.MaxRequestsPerMinute)
		}
		skipTasksWithSchemaResultFormat := run.DisableStructuredOutput
		if skipTasksWithSchemaResultFormat {
			logger.Message(ctx, logging.LevelInfo, "%s: %s: structured output disabled for this configuration.", provider.Name(), run.Name)
		}
		skipTasksWithFiles := run.TextOnly
		if skipTasksWithFiles {
			logger.Message(ctx, logging.LevelInfo, "%s: %s: text-only mode enabled for this configuration.", provider.Name(), run.Name)
		}
		executor := execution.NewExecutor(provider, run, sharedLimiter)

		for _, task := range tasks {
			runResult := RunResult{TraceID: ulid.Make().String()}

			// Create prefixed logger for this specific task.
			taskLogger := logger.WithContext(fmt.Sprintf("[%s] %s: %s: %s: ", runResult.TraceID, provider.Name(), run.Name, task.Name))

			taskLogger.Message(ctx, logging.LevelInfo, "starting task...")
			runStart := time.Now()
			r.runTask(ctx, taskLogger, executor, task, skipTasksWithSchemaResultFormat, skipTasksWithFiles, &runResult)
			taskLogger.Message(ctx, logging.LevelInfo, "task has finished in %s.", time.Since(runStart))
			rs.appendResult(runResult)
			rs.emitProgressEvent()
		}
	}

	if parallelRunsEnabled {
		var wg sync.WaitGroup
		for _, run := range runs {
			wg.Add(1)
			go func(rc config.RunConfig) {
				defer wg.Done()
				executeRun(rc)
			}(run)
		}
		wg.Wait()
	} else {
		for _, run := range runs {
			executeRun(run)
		}
	}

	logger.Message(ctx, logging.LevelInfo, "%s: all tasks in all configurations have finished on this provider in %s.", provider.Name(), time.Since(providerStart))
}

func (r *defaultRunner) runTask(ctx context.Context, logger logging.Logger, executor *execution.Executor, task config.Task, skipTasksWithSchemaResultFormat bool, skipTasksWithFiles bool, runResult *RunResult) {
	runResult.Task = task.Name
	runResult.Provider = executor.Provider.Name()
	runResult.Run = executor.RunConfig.Name
	runResult.RunConfig = snapshotRunConfig(ctx, logger, executor.RunConfig)
	runResult.TaskMetadata = TaskMetadata{
		Suite:      task.Suite,
		Category:   task.Category,
		Difficulty: task.Difficulty,
		Tags:       task.Tags,
	}

	// Skip tasks with schema response format when structured output is disabled.
	if skipTasksWithSchemaResultFormat {
		if _, isSchema := task.ResponseResultFormat.AsSchema(); isSchema {
			runResult.Kind = NotSupported
			runResult.Got = "task requires schema response format but disable-structured-output is enabled for this configuration"
			runResult.Details.Error = ErrorDetails{
				Title:     "Incompatible Response Format",
				Message:   "task requires schema response format but disable-structured-output is enabled for this configuration",
				Transient: utils.Ptr(false),
			}
			return
		}
	}

	// Skip tasks with file attachments when text-only mode is enabled.
	if skipTasksWithFiles && len(task.Files) > 0 {
		runResult.Kind = NotSupported
		runResult.Got = "task requires file attachments but text-only mode is enabled for this configuration"
		runResult.Details.Error = ErrorDetails{
			Title:     "Feature Disabled",
			Message:   "task requires file attachments but text-only mode is enabled for this configuration",
			Transient: utils.Ptr(false),
		}
		return
	}

	// Resolve validation rules for this task.
	resolvedValidationRules := task.GetResolvedValidationRules()

	// Create validator selected for this task.
	validator, err := r.validatorFactory.GetValidator(ctx, resolvedValidationRules.Judge)
	if err != nil {
		runResult.Kind = Error
		runResult.Got = err.Error()
		runResult.Details.Error = ErrorDetails{
			Title:     "Configuration Error",
			Message:   err.Error(),
			Transient: utils.Ptr(false),
		}
		return
	}

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
	toolCalls := result.GetToolCalls()
	logger.Message(ctx, logging.LevelDebug, "token usage: [in:%s, out:%s]", logging.FormatLogInt64(usage.InputTokens), logging.FormatLogInt64(usage.OutputTokens))
	if usage.InputCacheWriteTokens != nil || usage.InputCacheReadTokens != nil {
		logger.Message(ctx, logging.LevelDebug, "cache token usage: [write:%s, read:%s, accounting:%s]", logging.FormatLogInt64(usage.InputCacheWriteTokens), logging.FormatLogInt64(usage.InputCacheReadTokens), usage.InputTokenAccounting)
	}
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
				ToolCalls: toToolCallSummaries(toolCalls),
				Transient: utils.Ptr(false),
			}
		default:
			var unmarshalErr *providers.ErrUnmarshalResponse
			if errors.As(err, &unmarshalErr) {
				runResult.Details.Error = ErrorDetails{
					Title:           "Response Parsing Error",
					Message:         unmarshalErr.Cause.Error(),
					Usage:           toTokenUsage(usage),
					ToolUsage:       toToolUsage(usage),
					ToolCalls:       toToolCallSummaries(toolCalls),
					ResponseParsing: utils.Ptr(true),
					Transient:       transientFlagFor(err),
				}
			} else {
				runResult.Details.Error = ErrorDetails{
					Title:     "Execution Error",
					Message:   err.Error(),
					Usage:     toTokenUsage(usage),
					ToolUsage: toToolUsage(usage),
					ToolCalls: toToolCallSummaries(toolCalls),
					Transient: transientFlagFor(err),
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

			var unmarshalErr *providers.ErrUnmarshalResponse
			if errors.As(err, &unmarshalErr) {
				runResult.Details.Error = ErrorDetails{
					Title:           "Validation Response Parsing Error",
					Message:         unmarshalErr.Cause.Error(),
					Usage:           toTokenUsage(validationResult.Usage),
					ToolUsage:       toToolUsage(validationResult.Usage),
					ToolCalls:       toToolCallSummaries(validationResult.ToolCalls),
					ResponseParsing: utils.Ptr(true),
					Transient:       transientFlagFor(err),
					FromValidation:  true, // Usage above is the judge's, not the candidate's.
				}
			} else {
				runResult.Details.Error = ErrorDetails{
					Title:          "Validation Error",
					Message:        err.Error(),
					Usage:          toTokenUsage(validationResult.Usage),
					ToolUsage:      toToolUsage(validationResult.Usage),
					ToolCalls:      toToolCallSummaries(validationResult.ToolCalls),
					Transient:      transientFlagFor(err),
					FromValidation: true, // Usage above is the judge's, not the candidate's.
				}
			}
			populateErrorDetails(&runResult.Details.Error, err)
			// Preserve judge provenance/verdict even when validation itself errored out.
			runResult.Details.Validation = ValidationDetails{
				Semantic: toSemanticValidationDetails(ctx, logger, validationResult.Semantic),
			}
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
				ToolCalls:   toToolCallSummaries(validationResult.ToolCalls),
				Semantic:    toSemanticValidationDetails(ctx, logger, validationResult.Semantic),
			}
		}

		runResult.Details.Answer = AnswerDetails{
			Title:          result.Title,
			Explanation:    utils.SplitLines(result.Explanation),
			ActualAnswer:   utils.ToLines(result.GetFinalAnswerContent()),
			ExpectedAnswer: toLines(task.ExpectedResult),
			Usage:          toTokenUsage(usage),
			ToolUsage:      toToolUsage(usage),
			ToolCalls:      toToolCallSummaries(toolCalls),
		}
	}
	runResult.Duration = result.GetDuration()
}

// snapshotRunConfig returns an artifact-safe projection of cfg, suitable for persisting in
// results without leaking API keys or other secrets. cfg is always loaded from
// validated YAML, so re-marshaling ModelParams back through YAML is not expected to fail;
// if it somehow does, ModelParameters is left nil and the error is logged rather than
// propagated, since there is no unsafe state to report to the caller.
func snapshotRunConfig(ctx context.Context, logger logging.Logger, cfg config.RunConfig) RunConfigSnapshot {
	snapshot := RunConfigSnapshot{
		Name:                    cfg.Name,
		Model:                   cfg.Model,
		MaxRequestsPerMinute:    cfg.MaxRequestsPerMinute,
		TextOnly:                cfg.TextOnly,
		DisableStructuredOutput: cfg.DisableStructuredOutput,
	}
	if cfg.RetryPolicy != nil {
		snapshot.RetryPolicy = RetryPolicy{
			MaxRetryAttempts:    cfg.RetryPolicy.MaxRetryAttempts,
			InitialDelaySeconds: cfg.RetryPolicy.InitialDelaySeconds,
		}
	}
	if cfg.ModelParams == nil {
		return snapshot
	}

	data, err := yaml.Marshal(cfg.ModelParams)
	if err != nil {
		logger.Error(ctx, logging.LevelWarn, err, "%s: failed to marshal model parameters for run config snapshot", cfg.Name)
		return snapshot
	}

	var modelParams map[string]interface{}
	if err := yaml.Unmarshal(data, &modelParams); err != nil {
		logger.Error(ctx, logging.LevelWarn, err, "%s: failed to unmarshal model parameters for run config snapshot", cfg.Name)
		return snapshot
	}

	snapshot.ModelParameters = modelParams
	return snapshot
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
	var noActionableContentErr *providers.ErrNoActionableContent

	switch {
	case errors.As(err, &unmarshalErr):
		errorDetails.Details = map[string][]string{
			"Stop Reason":  {string(unmarshalErr.StopReason)},
			"Raw Response": utils.SplitLines(string(unmarshalErr.RawMessage)),
		}
	case errors.As(err, &noActionableContentErr):
		errorDetails.Details = map[string][]string{
			"Stop Reason": {string(noActionableContentErr.StopReason)},
		}
		if noActionableContentErr.StopDetails != nil {
			errorDetails.Details["Stop Details"] = utils.ToLines(noActionableContentErr.StopDetails)
		}
	case errors.As(err, &apiErr) && apiErr.Body != nil:
		errorDetails.Details = map[string][]string{
			"HTTP Response": utils.SplitLines(string(apiErr.Body)),
		}
	}
}

// transientFlagFor returns a pointer to true when err is known to be a transient/retryable
// error, or nil when transience is unknown. This is a best-effort classification based on
// the existing retry signal, not a complete error taxonomy.
func transientFlagFor(err error) *bool {
	if errors.Is(err, providers.ErrRetryable) {
		return utils.Ptr(true)
	}
	return nil
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

// toSemanticValidationDetails converts a validators.SemanticValidationDetails to its
// artifact-safe runners.SemanticValidationDetails projection, using the same
// snapshotRunConfig logic applied to the primary variant configuration. Returns nil when
// details is nil (validation was not performed by a judge).
func toSemanticValidationDetails(ctx context.Context, logger logging.Logger, details *validators.SemanticValidationDetails) *SemanticValidationDetails {
	if details == nil {
		return nil
	}
	return &SemanticValidationDetails{
		Verdict:       details.Verdict,
		JudgeName:     details.JudgeName,
		Provider:      details.Provider,
		Variant:       details.Variant,
		VariantConfig: snapshotRunConfig(ctx, logger, details.VariantConfig),
	}
}

func toTokenUsage(u providers.Usage) TokenUsage {
	return TokenUsage{
		InputTokens:           u.InputTokens,
		OutputTokens:          u.OutputTokens,
		InputCacheWriteTokens: u.InputCacheWriteTokens,
		InputCacheReadTokens:  u.InputCacheReadTokens,
		InputTokenAccounting:  InputTokenAccounting(u.InputTokenAccounting),
	}
}

func toToolUsage(u providers.Usage) (toolUsage map[string]ToolUsage) {
	toolUsage = make(map[string]ToolUsage, len(u.ToolUsage))
	for name, usage := range u.ToolUsage {
		callCount := usage.CallCount
		duration := time.Duration(usage.TotalDurationNs) // nanosecond is the natural unit of time.Duration
		toolUsage[name] = ToolUsage{
			CallCount:     &callCount,
			TotalDuration: &duration,
		}
	}
	return toolUsage
}

// toToolCallSummaries converts tool-package call summaries into the runners package's own,
// executor-agnostic ToolCallSummary shape (e.g. converting raw nanosecond counts to
// time.Duration), decoupling runners/formatters from any specific tool executor
// implementation. Returns nil for an empty input so a result with no recorded calls omits
// the field entirely, consistent with the rest of this package's optional-field conventions.
func toToolCallSummaries(calls []providertools.ToolCallSummary) []ToolCallSummary {
	if len(calls) == 0 {
		return nil
	}
	result := make([]ToolCallSummary, len(calls))
	for i, c := range calls {
		result[i] = ToolCallSummary{
			Tool:             c.Tool,
			CallID:           c.CallID,
			ConversationTurn: c.ConversationTurn,
			StartedAt:        c.StartedAt,
			CompletedAt:      c.CompletedAt,
			Duration:         nsPtrToDurationPtr(c.DurationNs),
			WallTime:         time.Duration(c.WallTimeNs),
			ExitCode:         c.ExitCode,
			TimedOut:         c.TimedOut,
			Status:           c.Status,
			Stdout:           toToolCallOutput(c.Stdout),
			Stderr:           toToolCallOutput(c.Stderr),
			ErrorMessage:     c.ErrorMessage,
		}
	}
	return result
}

// nsPtrToDurationPtr converts a nilable nanosecond count into a nilable time.Duration,
// preserving the nil (process never ran) vs zero distinction.
func nsPtrToDurationPtr(ns *int64) *time.Duration {
	if ns == nil {
		return nil
	}
	d := time.Duration(*ns)
	return &d
}

// toToolCallOutput converts a tool-package output capture into the public
// runners.ToolCallOutput shape. Returns nil when o is nil (the stream was never captured).
func toToolCallOutput(o *providertools.OutputCapture) *ToolCallOutput {
	if o == nil {
		return nil
	}
	return &ToolCallOutput{
		Bytes:     o.Bytes,
		Preview:   o.Preview,
		Truncated: o.Truncated,
	}
}
