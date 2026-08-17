// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"errors"
	"fmt"
	"time"

	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
)

// errUnknownResultKind indicates an unrecognized result kind string during deserialization.
var errUnknownResultKind = errors.New("unknown result kind")

// stringToResultKind maps status strings (as produced by ToStatus) back to ResultKind values.
var stringToResultKind = map[string]runners.ResultKind{
	Passed:  runners.Success,
	Failed:  runners.Failure,
	Error:   runners.Error,
	Skipped: runners.NotSupported,
}

// resultsView is the view model for runners.Results used in JSON serialization.
type resultsView map[string][]resultView

// resultView is the view model for runners.RunResult.
type resultView struct {
	TraceID      string            `json:"TraceID" jsonschema:"title=Trace ID" jsonschema_description:"A globally unique identifier for this specific task result, used for tracing and correlation."`
	Kind         string            `json:"Kind" jsonschema:"title=Result Kind" jsonschema_description:"The result status: Passed (answer accepted), Failed (answer rejected), Error (task execution failed), or Skipped (task not supported/attempted). An \"Unknown (n)\" fallback is possible but not expected in practice."`
	Task         string            `json:"Task" jsonschema:"title=Task Name" jsonschema_description:"The name of the executed task."`
	Provider     string            `json:"Provider" jsonschema:"title=Provider Name" jsonschema_description:"The name of the AI provider that executed the task."`
	Run          string            `json:"Run" jsonschema:"title=Run Name" jsonschema_description:"The name of the provider's run configuration used."`
	RunConfig    *runConfigView    `json:"RunConfig,omitempty" jsonschema:"title=Run Configuration" jsonschema_description:"The effective run configuration used to produce this result (e.g. model, rate limits, retry policy), with any API keys or other secrets omitted."`
	Got          interface{}       `json:"Got" jsonschema:"title=Actual Answer" jsonschema_description:"The actual answer received from the AI model. For plain text response format, a string that follows the format instruction precisely. For structured schema-based response format, any object that conforms to the task's response schema."`
	Want         utils.ValueSet    `json:"Want" jsonschema:"title=Expected Answer(s)" jsonschema_description:"The accepted valid answer(s) for the task, as a single value or an array of values. For plain text response format: string values that should follow the format instruction precisely. For structured schema-based response format: object values that conform to the task's response schema."`
	TaskMetadata *taskMetadataView `json:"TaskMetadata,omitempty" jsonschema:"title=Task Metadata" jsonschema_description:"Optional descriptive labels copied from the originating task."`
	Details      detailsView       `json:"Details" jsonschema:"title=Details" jsonschema_description:"Comprehensive information about the generated response and validation assessment."`
	DurationNS   int64             `json:"DurationNS" jsonschema:"title=Duration (ns)" jsonschema_description:"The cumulative time the AI model itself spent generating a response, in nanoseconds, summed across every conversation turn's model request (network + inference). Excludes local tool execution time (see ToolCalls/ToolUsage) and any subsequent validation time, so this is not the total wall-clock time spent processing the task."`
}

// runConfigView is the view model for runners.RunConfigSnapshot. Shared by the top-level
// task run configuration and a judge's variant configuration, so field descriptions are
// deliberately configuration-context-agnostic rather than saying "run" or "task".
type runConfigView struct {
	Model                   string                 `json:"Model,omitempty" jsonschema:"title=Model" jsonschema_description:"The provider model identifier used."`
	MaxRequestsPerMinute    int                    `json:"MaxRequestsPerMinute,omitempty" jsonschema:"title=Max Requests Per Minute" jsonschema_description:"The maximum number of API requests per minute that was permitted for this configuration. A value of 0 (or absent) means no rate limiting was applied."`
	TextOnly                bool                   `json:"TextOnly,omitempty" jsonschema:"title=Text Only" jsonschema_description:"Whether this configuration was restricted to text-only tasks (i.e. tasks without file attachments)."`
	DisableStructuredOutput bool                   `json:"DisableStructuredOutput,omitempty" jsonschema:"title=Disable Structured Output" jsonschema_description:"Whether this configuration used plain text responses instead of the structured title/explanation/final-answer format, skipping tasks that require schema-based output."`
	ModelParameters         map[string]interface{} `json:"ModelParameters,omitempty" jsonschema:"title=Model Parameters" jsonschema_description:"The public subset of model parameters used by this configuration, with secrets omitted."`
	RetryPolicy             *retryPolicyView       `json:"RetryPolicy,omitempty" jsonschema:"title=Retry Policy" jsonschema_description:"The effective retry policy for this configuration, if configured."`
}

// retryPolicyView is the view model for runners.RetryPolicy.
type retryPolicyView struct {
	MaxRetryAttempts    uint `json:"MaxRetryAttempts,omitempty" jsonschema:"title=Max Retry Attempts" jsonschema_description:"The maximum number of retry attempts for a transient failure."`
	InitialDelaySeconds int  `json:"InitialDelaySeconds,omitempty" jsonschema:"title=Initial Delay Seconds" jsonschema_description:"The initial backoff delay before the first retry attempt."`
}

// taskMetadataView is the view model for runners.TaskMetadata.
type taskMetadataView struct {
	Suite      string   `json:"Suite,omitempty" jsonschema:"title=Suite" jsonschema_description:"An optional grouping label for organizing related tasks (e.g. a benchmark suite name)."`
	Category   string   `json:"Category,omitempty" jsonschema:"title=Category" jsonschema_description:"An optional classification label for the task (e.g. \"math\", \"coding\")."`
	Difficulty string   `json:"Difficulty,omitempty" jsonschema:"title=Difficulty" jsonschema_description:"An optional free-form difficulty label for the task (e.g. \"easy\", \"hard\")."`
	Tags       []string `json:"Tags,omitempty" jsonschema:"title=Tags" jsonschema_description:"An optional set of free-form labels for filtering and grouping tasks."`
}

// detailsView is the view model for runners.Details.
type detailsView struct {
	Answer     *answerDetailsView     `json:"Answer,omitempty" jsonschema:"title=Answer Details" jsonschema_description:"Details about the AI model's response and reasoning process."`
	Validation *validationDetailsView `json:"Validation,omitempty" jsonschema:"title=Validation Details" jsonschema_description:"Details about the answer verification and assessment."`
	Error      *errorDetailsView      `json:"Error,omitempty" jsonschema:"title=Error Details" jsonschema_description:"Details about any errors that occurred during task execution."`
}

// answerDetailsView is the view model for runners.AnswerDetails.
type answerDetailsView struct {
	Title          string                   `json:"Title,omitempty" jsonschema:"title=Title" jsonschema_description:"A descriptive header for the response produced by the target AI model."`
	Explanation    []string                 `json:"Explanation,omitempty" jsonschema:"title=Explanation" jsonschema_description:"Explanation of the answer produced by the target AI model, split into lines."`
	ActualAnswer   []string                 `json:"ActualAnswer,omitempty" jsonschema:"title=Actual Answer Lines" jsonschema_description:"The raw answer from the target AI model split into lines."`
	ExpectedAnswer [][]string               `json:"ExpectedAnswer,omitempty" jsonschema:"title=Expected Answer Lines" jsonschema_description:"A set of all acceptable correct answers, each being an array of lines."`
	Usage          *runners.TokenUsage      `json:"Usage,omitempty" jsonschema:"title=Token Usage" jsonschema_description:"Token usage statistics for generating the answer."`
	ToolUsage      map[string]toolUsageView `json:"ToolUsage,omitempty" jsonschema:"title=Tool Usage" jsonschema_description:"Aggregated execution statistics, keyed by tool name, for any tools invoked while producing the answer."`
	ToolCalls      []toolCallSummaryView    `json:"ToolCalls,omitempty" jsonschema:"title=Tool Calls" jsonschema_description:"A log of every individual invocation attempt made while producing the answer, including attempts that never actually ran. Tracked separately from ToolUsage, which only reflects invocations that actually ran."`
}

// validationDetailsView is the view model for runners.ValidationDetails.
type validationDetailsView struct {
	Title       string                         `json:"Title,omitempty" jsonschema:"title=Title" jsonschema_description:"Identifies the type of validation assessment performed."`
	Explanation []string                       `json:"Explanation,omitempty" jsonschema:"title=Explanation" jsonschema_description:"Detailed analysis of why the validation succeeded or failed, split into lines."`
	Usage       *runners.TokenUsage            `json:"Usage,omitempty" jsonschema:"title=Token Usage" jsonschema_description:"Token usage statistics for the response validation step. Typically populated when using an LLM judge validator."`
	ToolUsage   map[string]toolUsageView       `json:"ToolUsage,omitempty" jsonschema:"title=Tool Usage" jsonschema_description:"Aggregated execution statistics, keyed by tool name, for any tools invoked during validation."`
	ToolCalls   []toolCallSummaryView          `json:"ToolCalls,omitempty" jsonschema:"title=Tool Calls" jsonschema_description:"A log of every individual invocation attempt made during validation, including attempts that never actually ran. Tracked separately from ToolUsage, which only reflects invocations that actually ran."`
	Semantic    *semanticValidationDetailsView `json:"Semantic,omitempty" jsonschema:"title=Semantic Validation" jsonschema_description:"The judge's raw verdict and variant provenance, present only when validation was performed by an LLM judge."`
}

// semanticValidationDetailsView is the view model for runners.SemanticValidationDetails.
type semanticValidationDetailsView struct {
	Verdict       interface{}    `json:"Verdict,omitempty" jsonschema:"title=Judge Verdict" jsonschema_description:"The raw verdict produced by the judge."`
	JudgeName     string         `json:"JudgeName,omitempty" jsonschema:"title=Judge Name" jsonschema_description:"The name of the judge configuration used."`
	Provider      string         `json:"Provider,omitempty" jsonschema:"title=Provider Name" jsonschema_description:"The name of the AI provider that executed the judge task."`
	Variant       string         `json:"Variant,omitempty" jsonschema:"title=Variant Name" jsonschema_description:"The name of the judge's configuration variant used."`
	VariantConfig *runConfigView `json:"VariantConfig,omitempty" jsonschema:"title=Variant Configuration" jsonschema_description:"The effective variant configuration used by the judge, with any API keys or other secrets omitted."`
}

// errorDetailsView is the view model for runners.ErrorDetails.
type errorDetailsView struct {
	Title           string                   `json:"Title,omitempty" jsonschema:"title=Title" jsonschema_description:"A summary description of the error."`
	Message         string                   `json:"Message,omitempty" jsonschema:"title=Message" jsonschema_description:"The primary error message."`
	Details         map[string][]string      `json:"Details,omitempty" jsonschema:"title=Details" jsonschema_description:"Any additional error information in a generic structure."`
	Usage           *runners.TokenUsage      `json:"Usage,omitempty" jsonschema:"title=Token Usage" jsonschema_description:"Token usage statistics if available even in error scenarios. Typically populated if the error occurs when parsing the generated response."`
	ToolUsage       map[string]toolUsageView `json:"ToolUsage,omitempty" jsonschema:"title=Tool Usage" jsonschema_description:"Aggregated execution statistics, keyed by tool name, for any tools invoked prior to the error."`
	ToolCalls       []toolCallSummaryView    `json:"ToolCalls,omitempty" jsonschema:"title=Tool Calls" jsonschema_description:"A log of every individual invocation attempt made prior to the error, including attempts that never actually ran. Tracked separately from ToolUsage, which only reflects invocations that actually ran."`
	ResponseParsing *bool                    `json:"ResponseParsing,omitempty" jsonschema:"title=Response Parsing" jsonschema_description:"True when the model response was received but failed to parse into the expected structure; absent otherwise. When Transient is also absent, this failure should generally be treated as non-transient (retrying with the same input is unlikely to help)."`
	Transient       *bool                    `json:"Transient,omitempty" jsonschema:"title=Transient" jsonschema_description:"Whether the error appears temporary/external (true), appears permanent/hard (false), or is unknown (field absent). A best-effort classification, not a complete error taxonomy."`
	FromValidation  bool                     `json:"FromValidation,omitempty" jsonschema:"title=From Validation" jsonschema_description:"True when Usage/ToolUsage/ToolCalls above belong to the judge's validation attempt rather than the candidate response, so aggregate consumers must not attribute them to the candidate."`
}

// toolUsageView is the view model for runners.ToolUsage.
type toolUsageView struct {
	CallCount       *int64 `json:"CallCount,omitempty" jsonschema:"title=Call Count" jsonschema_description:"The number of times the tool's underlying process actually ran."`
	TotalDurationNS *int64 `json:"TotalDurationNS,omitempty" jsonschema:"title=Total Duration (ns)" jsonschema_description:"The cumulative execution time for the tool's underlying process, in nanoseconds."`
}

// toolCallSummaryView is the view model for runners.ToolCallSummary.
type toolCallSummaryView struct {
	Tool             string              `json:"Tool" jsonschema:"title=Tool Name" jsonschema_description:"The name of the tool this call invoked."`
	CallID           string              `json:"CallID" jsonschema:"title=Call ID" jsonschema_description:"Identifies this call, letting a specific invocation be correlated between this summary, the corresponding tool-call log lines, and - when the calling provider's API assigns its own tool-call ID and that ID was reused here - the provider's own API error messages. Never empty, but its shape/format is not guaranteed to be consistent across providers."`
	ConversationTurn int                 `json:"ConversationTurn,omitempty" jsonschema:"title=Conversation Turn" jsonschema_description:"The 1-based conversation turn this call was made during, or absent/0 if unknown."`
	StartedAt        time.Time           `json:"StartedAt" jsonschema:"title=Started At" jsonschema_description:"When this call began (start of setup, before the underlying process runs)."`
	CompletedAt      time.Time           `json:"CompletedAt" jsonschema:"title=Completed At" jsonschema_description:"When this call finished, successfully or not."`
	DurationNS       *int64              `json:"DurationNS,omitempty" jsonschema:"title=Duration (ns)" jsonschema_description:"The wall-clock duration of the underlying process's runtime, in nanoseconds, not including setup/teardown overhead. Absent when no process ever ran (e.g. an infrastructure_error)."`
	WallTimeNS       int64               `json:"WallTimeNS" jsonschema:"title=Wall Time (ns)" jsonschema_description:"The wall-clock duration of the entire call attempt, in nanoseconds, from setup through output retrieval - i.e. DurationNS plus setup/teardown overhead. Unlike DurationNS, this is always set, even for calls whose underlying process never ran."`
	ExitCode         *int64              `json:"ExitCode,omitempty" jsonschema:"title=Exit Code" jsonschema_description:"The underlying process's exit code, or absent if no exit code is known."`
	TimedOut         bool                `json:"TimedOut,omitempty" jsonschema:"title=Timed Out" jsonschema_description:"Whether the call was aborted due to exceeding its configured timeout."`
	Status           string              `json:"Status,omitempty" jsonschema:"title=Status,enum=success,enum=nonzero_exit,enum=empty_output,enum=timeout,enum=invalid_arguments,enum=infrastructure_error" jsonschema_description:"The outcome of this call."`
	Stdout           *toolCallOutputView `json:"Stdout,omitempty" jsonschema:"title=Standard Output" jsonschema_description:"A size-limited capture of the call's standard output, or absent if no output was ever captured."`
	Stderr           *toolCallOutputView `json:"Stderr,omitempty" jsonschema:"title=Standard Error" jsonschema_description:"A size-limited capture of the call's standard error, or absent if no output was ever captured."`
	ErrorMessage     string              `json:"ErrorMessage,omitempty" jsonschema:"title=Error Message" jsonschema_description:"A short explanation of the failure when Status is not \"success\"."`
}

// toolCallOutputView is the view model for runners.ToolCallOutput.
type toolCallOutputView struct {
	Bytes     int64   `json:"Bytes" jsonschema:"title=Bytes" jsonschema_description:"The total size of the output stream, in bytes, regardless of Truncated."`
	Preview   *string `json:"Preview,omitempty" jsonschema:"title=Preview" jsonschema_description:"A truncated prefix of the output stream, or absent if not captured or empty."`
	Truncated bool    `json:"Truncated,omitempty" jsonschema:"title=Truncated" jsonschema_description:"Whether Preview was cut short of the full output."`
}

func toResultsView(results runners.Results) resultsView {
	rv := make(resultsView, len(results))
	for provider, runResults := range results {
		views := make([]resultView, len(runResults))
		for i, r := range runResults {
			views[i] = newResultView(r)
		}
		rv[provider] = views
	}
	return rv
}

func newResultView(r runners.RunResult) resultView {
	return resultView{
		TraceID:      r.TraceID,
		Kind:         ToStatus(r.Kind),
		Task:         r.Task,
		Provider:     r.Provider,
		Run:          r.Run,
		RunConfig:    newRunConfigView(r.RunConfig),
		Got:          r.Got,
		Want:         r.Want,
		TaskMetadata: newTaskMetadataView(r.TaskMetadata),
		Details:      newDetailsView(r.Details),
		DurationNS:   r.Duration.Nanoseconds(),
	}
}

func newRunConfigView(rc runners.RunConfigSnapshot) *runConfigView {
	if rc.Model == "" && len(rc.ModelParameters) == 0 && rc.RetryPolicy == (runners.RetryPolicy{}) && !rc.TextOnly && !rc.DisableStructuredOutput && rc.MaxRequestsPerMinute == 0 {
		return nil
	}
	view := &runConfigView{
		Model:                   rc.Model,
		MaxRequestsPerMinute:    rc.MaxRequestsPerMinute,
		TextOnly:                rc.TextOnly,
		DisableStructuredOutput: rc.DisableStructuredOutput,
		ModelParameters:         rc.ModelParameters,
	}
	if rc.RetryPolicy != (runners.RetryPolicy{}) {
		view.RetryPolicy = &retryPolicyView{
			MaxRetryAttempts:    rc.RetryPolicy.MaxRetryAttempts,
			InitialDelaySeconds: rc.RetryPolicy.InitialDelaySeconds,
		}
	}
	return view
}

// newTaskMetadataView converts runners.TaskMetadata to its view model.
// Returns nil when there is no metadata to report, matching the nil-when-empty
// convention used by the other optional detail views in this file.
func newTaskMetadataView(m runners.TaskMetadata) *taskMetadataView {
	if m.Suite == "" && m.Category == "" && m.Difficulty == "" && len(m.Tags) == 0 {
		return nil
	}
	return &taskMetadataView{
		Suite:      m.Suite,
		Category:   m.Category,
		Difficulty: m.Difficulty,
		Tags:       m.Tags,
	}
}

func newDetailsView(d runners.Details) detailsView {
	return detailsView{
		Answer:     newAnswerDetailsView(d.Answer),
		Validation: newValidationDetailsView(d.Validation),
		Error:      newErrorDetailsView(d.Error),
	}
}

func newAnswerDetailsView(a runners.AnswerDetails) *answerDetailsView {
	v := answerDetailsView{
		Title:          a.Title,
		Explanation:    a.Explanation,
		ActualAnswer:   a.ActualAnswer,
		ExpectedAnswer: a.ExpectedAnswer,
		Usage:          tokenUsageToPtr(a.Usage),
		ToolUsage:      newToolUsageMapView(a.ToolUsage),
		ToolCalls:      newToolCallSummaryViews(a.ToolCalls),
	}
	if v.Title == "" && len(v.Explanation) == 0 && len(v.ActualAnswer) == 0 &&
		len(v.ExpectedAnswer) == 0 && v.Usage == nil && len(v.ToolUsage) == 0 && len(v.ToolCalls) == 0 {
		return nil
	}
	return &v
}

func newValidationDetailsView(v runners.ValidationDetails) *validationDetailsView {
	rv := validationDetailsView{
		Title:       v.Title,
		Explanation: v.Explanation,
		Usage:       tokenUsageToPtr(v.Usage),
		ToolUsage:   newToolUsageMapView(v.ToolUsage),
		ToolCalls:   newToolCallSummaryViews(v.ToolCalls),
		Semantic:    newSemanticValidationDetailsView(v.Semantic),
	}
	if rv.Title == "" && len(rv.Explanation) == 0 && rv.Usage == nil && len(rv.ToolUsage) == 0 &&
		len(rv.ToolCalls) == 0 && rv.Semantic == nil {
		return nil
	}
	return &rv
}

// newSemanticValidationDetailsView converts a runners.SemanticValidationDetails to its
// view model. Returns nil when s is nil (validation was not performed by a judge).
func newSemanticValidationDetailsView(s *runners.SemanticValidationDetails) *semanticValidationDetailsView {
	if s == nil {
		return nil
	}
	return &semanticValidationDetailsView{
		Verdict:       s.Verdict,
		JudgeName:     s.JudgeName,
		Provider:      s.Provider,
		Variant:       s.Variant,
		VariantConfig: newRunConfigView(s.VariantConfig),
	}
}

func newErrorDetailsView(e runners.ErrorDetails) *errorDetailsView {
	v := errorDetailsView{
		Title:           e.Title,
		Message:         e.Message,
		Details:         e.Details,
		Usage:           tokenUsageToPtr(e.Usage),
		ToolUsage:       newToolUsageMapView(e.ToolUsage),
		ToolCalls:       newToolCallSummaryViews(e.ToolCalls),
		ResponseParsing: e.ResponseParsing,
		Transient:       e.Transient,
		FromValidation:  e.FromValidation,
	}
	if v.Title == "" && v.Message == "" && len(v.Details) == 0 && v.Usage == nil && len(v.ToolUsage) == 0 && len(v.ToolCalls) == 0 && v.ResponseParsing == nil && v.Transient == nil && !v.FromValidation {
		return nil
	}
	return &v
}

func newToolUsageMapView(m map[string]runners.ToolUsage) map[string]toolUsageView {
	if len(m) == 0 {
		return nil
	}
	rv := make(map[string]toolUsageView, len(m))
	for name, u := range m {
		rv[name] = newToolUsageView(u)
	}
	return rv
}

func newToolUsageView(u runners.ToolUsage) toolUsageView {
	return toolUsageView{
		CallCount:       u.CallCount,
		TotalDurationNS: durationToNsPtr(u.TotalDuration),
	}
}

// newToolCallSummaryViews converts runners.ToolCallSummary values to their view model.
// Returns nil for an empty input so the field is omitted entirely, consistent with the
// other optional-field conventions in this file.
func newToolCallSummaryViews(calls []runners.ToolCallSummary) []toolCallSummaryView {
	if len(calls) == 0 {
		return nil
	}
	views := make([]toolCallSummaryView, len(calls))
	for i, c := range calls {
		views[i] = toolCallSummaryView{
			Tool:             c.Tool,
			CallID:           c.CallID,
			ConversationTurn: c.ConversationTurn,
			StartedAt:        c.StartedAt,
			CompletedAt:      c.CompletedAt,
			DurationNS:       durationToNsPtr(c.Duration),
			WallTimeNS:       c.WallTime.Nanoseconds(),
			ExitCode:         c.ExitCode,
			TimedOut:         c.TimedOut,
			Status:           c.Status,
			Stdout:           newToolCallOutputView(c.Stdout),
			Stderr:           newToolCallOutputView(c.Stderr),
			ErrorMessage:     c.ErrorMessage,
		}
	}
	return views
}

// newToolCallOutputView converts a runners.ToolCallOutput to its view model.
// Returns nil when o is nil (the stream was never captured for this call).
func newToolCallOutputView(o *runners.ToolCallOutput) *toolCallOutputView {
	if o == nil {
		return nil
	}
	return &toolCallOutputView{
		Bytes:     o.Bytes,
		Preview:   o.Preview,
		Truncated: o.Truncated,
	}
}

func durationToNsPtr(d *time.Duration) *int64 {
	if d == nil {
		return nil
	}
	ns := d.Nanoseconds()
	return &ns
}

func tokenUsageToPtr(u runners.TokenUsage) *runners.TokenUsage {
	if u.InputTokens == nil &&
		u.OutputTokens == nil &&
		u.InputCacheWriteTokens == nil &&
		u.InputCacheReadTokens == nil &&
		u.InputTokenAccounting == "" {
		return nil
	}
	return &u
}

func tokenUsageFromPtr(u *runners.TokenUsage) runners.TokenUsage {
	if u == nil {
		return runners.TokenUsage{}
	}
	return *u
}

// fromResultsView converts a resultsView back to runners.Results.
func fromResultsView(rv resultsView) (runners.Results, error) {
	results := make(runners.Results, len(rv))
	for provider, views := range rv {
		runResults := make([]runners.RunResult, len(views))
		for i, v := range views {
			r, err := fromResultView(v)
			if err != nil {
				return nil, err
			}
			if r.Provider != provider {
				return nil, fmt.Errorf("%w: provider key %q does not match entry provider %q", ErrReadResults, provider, r.Provider)
			}
			runResults[i] = r
		}
		results[provider] = runResults
	}
	return results, nil
}

func fromResultView(v resultView) (runners.RunResult, error) {
	kind, ok := stringToResultKind[v.Kind]
	if !ok {
		return runners.RunResult{}, fmt.Errorf("%w: %q", errUnknownResultKind, v.Kind)
	}
	result := runners.RunResult{
		TraceID:      v.TraceID,
		Kind:         kind,
		Task:         v.Task,
		Provider:     v.Provider,
		Run:          v.Run,
		RunConfig:    fromRunConfigView(v.RunConfig, v.Run),
		Got:          v.Got,
		Want:         v.Want,
		TaskMetadata: fromTaskMetadataView(v.TaskMetadata),
		Details:      fromDetailsView(v.Details),
		Duration:     time.Duration(v.DurationNS),
	}
	return result, nil
}

// fromRunConfigView converts a runConfigView back to runners.RunConfigSnapshot.
// fallbackName rehydrates Name, which the view never serializes since it always
// matches the enclosing resultView's Run field.
func fromRunConfigView(v *runConfigView, fallbackName string) runners.RunConfigSnapshot {
	if v == nil {
		return runners.RunConfigSnapshot{Name: fallbackName}
	}
	out := runners.RunConfigSnapshot{
		Name:                    fallbackName,
		Model:                   v.Model,
		MaxRequestsPerMinute:    v.MaxRequestsPerMinute,
		TextOnly:                v.TextOnly,
		DisableStructuredOutput: v.DisableStructuredOutput,
		ModelParameters:         v.ModelParameters,
	}
	if v.RetryPolicy != nil {
		out.RetryPolicy = runners.RetryPolicy{
			MaxRetryAttempts:    v.RetryPolicy.MaxRetryAttempts,
			InitialDelaySeconds: v.RetryPolicy.InitialDelaySeconds,
		}
	}
	return out
}

// fromSemanticValidationDetailsView converts a semanticValidationDetailsView back to
// runners.SemanticValidationDetails. Returns nil when v is nil. Variant rehydrates
// VariantConfig.Name, which the nested runConfigView never serializes, mirroring how the
// top-level resultView.Run rehydrates RunConfig.Name.
func fromSemanticValidationDetailsView(v *semanticValidationDetailsView) *runners.SemanticValidationDetails {
	if v == nil {
		return nil
	}
	return &runners.SemanticValidationDetails{
		Verdict:       v.Verdict,
		JudgeName:     v.JudgeName,
		Provider:      v.Provider,
		Variant:       v.Variant,
		VariantConfig: fromRunConfigView(v.VariantConfig, v.Variant),
	}
}

// fromTaskMetadataView converts a taskMetadataView back to runners.TaskMetadata.
// A nil view produces a zero-value TaskMetadata.
func fromTaskMetadataView(v *taskMetadataView) runners.TaskMetadata {
	if v == nil {
		return runners.TaskMetadata{}
	}
	return runners.TaskMetadata{
		Suite:      v.Suite,
		Category:   v.Category,
		Difficulty: v.Difficulty,
		Tags:       v.Tags,
	}
}

func fromDetailsView(d detailsView) runners.Details {
	var result runners.Details
	if d.Answer != nil {
		result.Answer = runners.AnswerDetails{
			Title:          d.Answer.Title,
			Explanation:    d.Answer.Explanation,
			ActualAnswer:   d.Answer.ActualAnswer,
			ExpectedAnswer: d.Answer.ExpectedAnswer,
			Usage:          tokenUsageFromPtr(d.Answer.Usage),
			ToolUsage:      fromToolUsageMapView(d.Answer.ToolUsage),
			ToolCalls:      fromToolCallSummaryViews(d.Answer.ToolCalls),
		}
	}
	if d.Validation != nil {
		result.Validation = runners.ValidationDetails{
			Title:       d.Validation.Title,
			Explanation: d.Validation.Explanation,
			Usage:       tokenUsageFromPtr(d.Validation.Usage),
			ToolUsage:   fromToolUsageMapView(d.Validation.ToolUsage),
			ToolCalls:   fromToolCallSummaryViews(d.Validation.ToolCalls),
			Semantic:    fromSemanticValidationDetailsView(d.Validation.Semantic),
		}
	}
	if d.Error != nil {
		result.Error = runners.ErrorDetails{
			Title:           d.Error.Title,
			Message:         d.Error.Message,
			Details:         d.Error.Details,
			Usage:           tokenUsageFromPtr(d.Error.Usage),
			ToolUsage:       fromToolUsageMapView(d.Error.ToolUsage),
			ToolCalls:       fromToolCallSummaryViews(d.Error.ToolCalls),
			ResponseParsing: d.Error.ResponseParsing,
			Transient:       d.Error.Transient,
			FromValidation:  d.Error.FromValidation,
		}
	}
	return result
}

func fromToolUsageMapView(m map[string]toolUsageView) map[string]runners.ToolUsage {
	if m == nil {
		return nil
	}
	rv := make(map[string]runners.ToolUsage, len(m))
	for name, v := range m {
		rv[name] = fromToolUsageView(v)
	}
	return rv
}

func fromToolUsageView(v toolUsageView) runners.ToolUsage {
	return runners.ToolUsage{
		CallCount:     v.CallCount,
		TotalDuration: nsToDurationPtr(v.TotalDurationNS),
	}
}

// fromToolCallSummaryViews converts view models back to runners.ToolCallSummary.
// Returns nil for an empty input, matching newToolCallSummaryViews's nil-when-empty
// convention.
func fromToolCallSummaryViews(views []toolCallSummaryView) []runners.ToolCallSummary {
	if len(views) == 0 {
		return nil
	}
	calls := make([]runners.ToolCallSummary, len(views))
	for i, v := range views {
		calls[i] = runners.ToolCallSummary{
			Tool:             v.Tool,
			CallID:           v.CallID,
			ConversationTurn: v.ConversationTurn,
			StartedAt:        v.StartedAt,
			CompletedAt:      v.CompletedAt,
			Duration:         nsToDurationPtr(v.DurationNS),
			WallTime:         time.Duration(v.WallTimeNS),
			ExitCode:         v.ExitCode,
			TimedOut:         v.TimedOut,
			Status:           v.Status,
			Stdout:           fromToolCallOutputView(v.Stdout),
			Stderr:           fromToolCallOutputView(v.Stderr),
			ErrorMessage:     v.ErrorMessage,
		}
	}
	return calls
}

// fromToolCallOutputView converts a toolCallOutputView back to runners.ToolCallOutput.
// Returns nil when v is nil (the stream was never captured for this call).
func fromToolCallOutputView(v *toolCallOutputView) *runners.ToolCallOutput {
	if v == nil {
		return nil
	}
	return &runners.ToolCallOutput{
		Bytes:     v.Bytes,
		Preview:   v.Preview,
		Truncated: v.Truncated,
	}
}

func nsToDurationPtr(n *int64) *time.Duration {
	if n == nil {
		return nil
	}
	d := time.Duration(*n)
	return &d
}
