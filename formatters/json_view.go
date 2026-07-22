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
	TraceID      string            `json:"TraceID"`
	Kind         string            `json:"Kind"`
	Task         string            `json:"Task"`
	Provider     string            `json:"Provider"`
	Run          string            `json:"Run"`
	Got          interface{}       `json:"Got"`
	Want         utils.ValueSet    `json:"Want"`
	TaskMetadata *taskMetadataView `json:"TaskMetadata,omitempty"`
	Details      detailsView       `json:"Details"`
	DurationNS   int64             `json:"DurationNS"`
}

// taskMetadataView is the view model for runners.TaskMetadata.
type taskMetadataView struct {
	Suite      string   `json:"Suite,omitempty"`
	Category   string   `json:"Category,omitempty"`
	Difficulty string   `json:"Difficulty,omitempty"`
	Tags       []string `json:"Tags,omitempty"`
}

// detailsView is the view model for runners.Details.
type detailsView struct {
	Answer     *answerDetailsView     `json:"Answer,omitempty"`
	Validation *validationDetailsView `json:"Validation,omitempty"`
	Error      *errorDetailsView      `json:"Error,omitempty"`
}

// answerDetailsView is the view model for runners.AnswerDetails.
type answerDetailsView struct {
	Title          string                   `json:"Title,omitempty"`
	Explanation    []string                 `json:"Explanation,omitempty"`
	ActualAnswer   []string                 `json:"ActualAnswer,omitempty"`
	ExpectedAnswer [][]string               `json:"ExpectedAnswer,omitempty"`
	Usage          *runners.TokenUsage      `json:"Usage,omitempty"`
	ToolUsage      map[string]toolUsageView `json:"ToolUsage,omitempty"`
	ToolCalls      []toolCallSummaryView    `json:"ToolCalls,omitempty"`
}

// validationDetailsView is the view model for runners.ValidationDetails.
type validationDetailsView struct {
	Title       string                   `json:"Title,omitempty"`
	Explanation []string                 `json:"Explanation,omitempty"`
	Usage       *runners.TokenUsage      `json:"Usage,omitempty"`
	ToolUsage   map[string]toolUsageView `json:"ToolUsage,omitempty"`
	ToolCalls   []toolCallSummaryView    `json:"ToolCalls,omitempty"`
}

// errorDetailsView is the view model for runners.ErrorDetails.
type errorDetailsView struct {
	Title     string                   `json:"Title,omitempty"`
	Message   string                   `json:"Message,omitempty"`
	Details   map[string][]string      `json:"Details,omitempty"`
	Usage     *runners.TokenUsage      `json:"Usage,omitempty"`
	ToolUsage map[string]toolUsageView `json:"ToolUsage,omitempty"`
	ToolCalls []toolCallSummaryView    `json:"ToolCalls,omitempty"`
	Transient *bool                    `json:"Transient,omitempty"`
}

// toolUsageView is the view model for runners.ToolUsage.
type toolUsageView struct {
	CallCount       *int64 `json:"CallCount,omitempty"`
	TotalDurationNS *int64 `json:"TotalDurationNS,omitempty"`
}

// toolCallSummaryView is the view model for runners.ToolCallSummary.
type toolCallSummaryView struct {
	Tool             string              `json:"Tool"`
	CallID           string              `json:"CallID"`
	ConversationTurn int                 `json:"ConversationTurn,omitempty"`
	StartedAt        time.Time           `json:"StartedAt"`
	CompletedAt      time.Time           `json:"CompletedAt"`
	DurationNS       *int64              `json:"DurationNS,omitempty"`
	WallTimeNS       int64               `json:"WallTimeNS"`
	ExitCode         *int64              `json:"ExitCode,omitempty"`
	TimedOut         bool                `json:"TimedOut,omitempty"`
	Status           string              `json:"Status,omitempty"`
	Stdout           *toolCallOutputView `json:"Stdout,omitempty"`
	Stderr           *toolCallOutputView `json:"Stderr,omitempty"`
	ErrorMessage     string              `json:"ErrorMessage,omitempty"`
}

// toolCallOutputView is the view model for runners.ToolCallOutput.
type toolCallOutputView struct {
	Bytes     int64   `json:"Bytes"`
	Preview   *string `json:"Preview,omitempty"`
	Truncated bool    `json:"Truncated,omitempty"`
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
		Got:          r.Got,
		Want:         r.Want,
		TaskMetadata: newTaskMetadataView(r.TaskMetadata),
		Details:      newDetailsView(r.Details),
		DurationNS:   r.Duration.Nanoseconds(),
	}
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
	}
	if rv.Title == "" && len(rv.Explanation) == 0 && rv.Usage == nil && len(rv.ToolUsage) == 0 && len(rv.ToolCalls) == 0 {
		return nil
	}
	return &rv
}

func newErrorDetailsView(e runners.ErrorDetails) *errorDetailsView {
	v := errorDetailsView{
		Title:     e.Title,
		Message:   e.Message,
		Details:   e.Details,
		Usage:     tokenUsageToPtr(e.Usage),
		ToolUsage: newToolUsageMapView(e.ToolUsage),
		ToolCalls: newToolCallSummaryViews(e.ToolCalls),
		Transient: e.Transient,
	}
	if v.Title == "" && v.Message == "" && len(v.Details) == 0 && v.Usage == nil && len(v.ToolUsage) == 0 && len(v.ToolCalls) == 0 && v.Transient == nil {
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
	if u.InputTokens == nil && u.OutputTokens == nil {
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
	return runners.RunResult{
		TraceID:      v.TraceID,
		Kind:         kind,
		Task:         v.Task,
		Provider:     v.Provider,
		Run:          v.Run,
		Got:          v.Got,
		Want:         v.Want,
		TaskMetadata: fromTaskMetadataView(v.TaskMetadata),
		Details:      fromDetailsView(v.Details),
		Duration:     time.Duration(v.DurationNS),
	}, nil
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
		}
	}
	if d.Error != nil {
		result.Error = runners.ErrorDetails{
			Title:     d.Error.Title,
			Message:   d.Error.Message,
			Details:   d.Error.Details,
			Usage:     tokenUsageFromPtr(d.Error.Usage),
			ToolUsage: fromToolUsageMapView(d.Error.ToolUsage),
			ToolCalls: fromToolCallSummaryViews(d.Error.ToolCalls),
			Transient: d.Error.Transient,
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
