// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package stats computes derived, read-only statistics (pass rates, durations, token and
// tool-call distributions, and error diagnostics) over MindTrial result sets, filtered and
// grouped by task/run metadata. It is analytical output derived from canonical results, not
// a canonical result format itself, and is not persisted back into result artifacts.
package stats

import (
	"sort"
	"strings"
	"time"

	"github.com/petmal/mindtrial/formatters"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
)

// Record holds aggregated metrics for one group of results, keyed by the requested
// group-by dimensions.
//
// Reasoning-token and estimated-cost metrics are intentionally absent: MindTrial does not
// yet track per-request reasoning-token accounting or configurable pricing, so there is no
// reliable data source for them. Once that data becomes available, this Record can be
// extended with the corresponding fields.
type Record struct {
	// Dimensions maps each requested group-by dimension name to this record's value for it
	// (e.g. {"provider": "openai", "run": "gpt-4"}).
	Dimensions map[string]string

	// Count is the number of non-skipped results (Passed + Failed + Error).
	Count   int
	Passed  int
	Failed  int
	Error   int
	Skipped int

	// PassRate, Accuracy and ErrorRate are percentages (0-100) rounded to 2 decimals. See
	// formatters.PassRate/AccuracyRate/ErrorRate for the exact formulas; a zero denominator
	// yields 0, not an undefined value.
	PassRate  float64
	Accuracy  float64
	ErrorRate float64

	// Duration metrics are computed over non-skipped results only.
	TotalDuration  *time.Duration
	MedianDuration *time.Duration
	StddevDuration *time.Duration

	// Token/tool-call metrics reflect only the candidate answer and any subsequent error
	// (never judge/validation usage), matching the HTML report's dynamic summary. A metric
	// is nil when no contributing result reported it.
	TotalInputTokens  *int64
	MedianInputTokens *float64
	StddevInputTokens *float64

	TotalOutputTokens  *int64
	MedianOutputTokens *float64
	StddevOutputTokens *float64

	TotalToolCalls  *int64
	MedianToolCalls *float64
	StddevToolCalls *float64

	// TransientErrors and ResponseParsingErrors count Error-kind results whose
	// ErrorDetails.Transient/ResponseParsing flag is explicitly true. The two are not
	// mutually exclusive: a result can set both.
	TransientErrors       int
	ResponseParsingErrors int
}

// ComputeStats filters and groups results according to groupBy and filters, returning one
// Record per distinct combination of group-by dimension values. Records are returned in a
// deterministic order, sorted by dimension values in groupBy order.
func ComputeStats(results runners.Results, groupBy []Dimension, filters Filters) ([]Record, error) {
	allowedStatuses, err := filters.resolvedStatuses()
	if err != nil {
		return nil, err
	}

	type group struct {
		values  []string
		results []runners.RunResult
	}
	groups := make(map[string]*group)

	for _, provider := range utils.SortedKeys(results) {
		for _, r := range results[provider] {
			if !filters.matches(r, allowedStatuses) {
				continue
			}
			for _, combo := range dimensionCombinations(r, groupBy) {
				key := strings.Join(combo, "\x1f")
				g, ok := groups[key]
				if !ok {
					g = &group{values: combo}
					groups[key] = g
				}
				g.results = append(g.results, r)
			}
		}
	}

	records := make([]Record, 0, len(groups))
	for _, g := range groups {
		records = append(records, buildRecord(groupBy, g.values, g.results))
	}

	sort.Slice(records, func(i, j int) bool {
		for _, dim := range groupBy {
			vi, vj := records[i].Dimensions[string(dim)], records[j].Dimensions[string(dim)]
			if vi != vj {
				return vi < vj
			}
		}
		return false
	})

	return records, nil
}

// buildRecord aggregates results already known to share the given dimension values.
func buildRecord(groupBy []Dimension, values []string, results []runners.RunResult) Record {
	dimensions := make(map[string]string, len(groupBy))
	for i, dim := range groupBy {
		dimensions[string(dim)] = values[i]
	}

	byKind := make(map[runners.ResultKind][]runners.RunResult)
	for _, r := range results {
		byKind[r.Kind] = append(byKind[r.Kind], r)
	}

	rec := Record{
		Dimensions: dimensions,
		Count:      len(byKind[runners.Success]) + len(byKind[runners.Failure]) + len(byKind[runners.Error]),
		Passed:     len(byKind[runners.Success]),
		Failed:     len(byKind[runners.Failure]),
		Error:      len(byKind[runners.Error]),
		Skipped:    len(byKind[runners.NotSupported]),
		PassRate:   formatters.Percent(formatters.PassRate(byKind)),
		Accuracy:   formatters.Percent(formatters.AccuracyRate(byKind)),
		ErrorRate:  formatters.Percent(formatters.ErrorRate(byKind)),
	}

	var durationsNS, inputTokens, outputTokens, toolCalls []float64
	var totalDuration time.Duration
	var totalInput, totalOutput, totalCalls int64
	var hasDuration, hasInput, hasOutput, hasCalls bool

	for _, r := range results {
		if r.Kind == runners.NotSupported {
			continue // skipped tasks are excluded from every metric below
		}

		totalDuration += r.Duration
		hasDuration = true
		durationsNS = append(durationsNS, float64(r.Duration.Nanoseconds()))

		if v := candidateInputTokens(r); v != nil {
			totalInput += *v
			hasInput = true
			inputTokens = append(inputTokens, float64(*v))
		}
		if v := candidateOutputTokens(r); v != nil {
			totalOutput += *v
			hasOutput = true
			outputTokens = append(outputTokens, float64(*v))
		}
		if v := candidateToolCalls(r); v != nil {
			totalCalls += *v
			hasCalls = true
			toolCalls = append(toolCalls, float64(*v))
		}

		if r.Kind == runners.Error {
			if r.Details.Error.Transient != nil && *r.Details.Error.Transient {
				rec.TransientErrors++
			}
			if r.Details.Error.ResponseParsing != nil && *r.Details.Error.ResponseParsing {
				rec.ResponseParsingErrors++
			}
		}
	}

	if hasDuration {
		rec.TotalDuration = utils.Ptr(totalDuration)
	}
	rec.MedianDuration = medianDuration(durationsNS)
	rec.StddevDuration = stddevDuration(durationsNS)

	if hasInput {
		rec.TotalInputTokens = utils.Ptr(totalInput)
	}
	rec.MedianInputTokens = median(inputTokens)
	rec.StddevInputTokens = stddev(inputTokens)

	if hasOutput {
		rec.TotalOutputTokens = utils.Ptr(totalOutput)
	}
	rec.MedianOutputTokens = median(outputTokens)
	rec.StddevOutputTokens = stddev(outputTokens)

	if hasCalls {
		rec.TotalToolCalls = utils.Ptr(totalCalls)
	}
	rec.MedianToolCalls = median(toolCalls)
	rec.StddevToolCalls = stddev(toolCalls)

	return rec
}
