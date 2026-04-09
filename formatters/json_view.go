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
	TraceID    string         `json:"TraceID"`
	Kind       string         `json:"Kind"`
	Task       string         `json:"Task"`
	Provider   string         `json:"Provider"`
	Run        string         `json:"Run"`
	Got        interface{}    `json:"Got"`
	Want       utils.ValueSet `json:"Want"`
	Details    detailsView    `json:"Details"`
	DurationNS int64          `json:"DurationNS"`
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
}

// validationDetailsView is the view model for runners.ValidationDetails.
type validationDetailsView struct {
	Title       string                   `json:"Title,omitempty"`
	Explanation []string                 `json:"Explanation,omitempty"`
	Usage       *runners.TokenUsage      `json:"Usage,omitempty"`
	ToolUsage   map[string]toolUsageView `json:"ToolUsage,omitempty"`
}

// errorDetailsView is the view model for runners.ErrorDetails.
type errorDetailsView struct {
	Title     string                   `json:"Title,omitempty"`
	Message   string                   `json:"Message,omitempty"`
	Details   map[string][]string      `json:"Details,omitempty"`
	Usage     *runners.TokenUsage      `json:"Usage,omitempty"`
	ToolUsage map[string]toolUsageView `json:"ToolUsage,omitempty"`
}

// toolUsageView is the view model for runners.ToolUsage.
type toolUsageView struct {
	CallCount       *int64 `json:"CallCount,omitempty"`
	TotalDurationNS *int64 `json:"TotalDurationNS,omitempty"`
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
		TraceID:    r.TraceID,
		Kind:       ToStatus(r.Kind),
		Task:       r.Task,
		Provider:   r.Provider,
		Run:        r.Run,
		Got:        r.Got,
		Want:       r.Want,
		Details:    newDetailsView(r.Details),
		DurationNS: r.Duration.Nanoseconds(),
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
	}
	if v.Title == "" && len(v.Explanation) == 0 && len(v.ActualAnswer) == 0 &&
		len(v.ExpectedAnswer) == 0 && v.Usage == nil && len(v.ToolUsage) == 0 {
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
	}
	if rv.Title == "" && len(rv.Explanation) == 0 && rv.Usage == nil && len(rv.ToolUsage) == 0 {
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
	}
	if v.Title == "" && v.Message == "" && len(v.Details) == 0 && v.Usage == nil && len(v.ToolUsage) == 0 {
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
		TraceID:  v.TraceID,
		Kind:     kind,
		Task:     v.Task,
		Provider: v.Provider,
		Run:      v.Run,
		Got:      v.Got,
		Want:     v.Want,
		Details:  fromDetailsView(v.Details),
		Duration: time.Duration(v.DurationNS),
	}, nil
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
		}
	}
	if d.Validation != nil {
		result.Validation = runners.ValidationDetails{
			Title:       d.Validation.Title,
			Explanation: d.Validation.Explanation,
			Usage:       tokenUsageFromPtr(d.Validation.Usage),
			ToolUsage:   fromToolUsageMapView(d.Validation.ToolUsage),
		}
	}
	if d.Error != nil {
		result.Error = runners.ErrorDetails{
			Title:     d.Error.Title,
			Message:   d.Error.Message,
			Details:   d.Error.Details,
			Usage:     tokenUsageFromPtr(d.Error.Usage),
			ToolUsage: fromToolUsageMapView(d.Error.ToolUsage),
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

func nsToDurationPtr(n *int64) *time.Duration {
	if n == nil {
		return nil
	}
	d := time.Duration(*n)
	return &d
}
