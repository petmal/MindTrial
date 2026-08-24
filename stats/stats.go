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
	//
	// Input metrics resolve cache tokens according to InputTokenAccounting (see
	// TokenUsage.EffectiveInputTokens), so they are comparable across providers regardless
	// of whether cache reads/writes are already part of InputTokens.
	TotalInputTokens  *int64
	MedianInputTokens *float64
	StddevInputTokens *float64

	// Output metrics resolve reasoning tokens according to OutputTokenAccounting (see
	// TokenUsage.GeneratedTokens), so they are comparable across providers regardless of
	// whether reasoning is already part of OutputTokens.
	TotalOutputTokens  *int64
	MedianOutputTokens *float64
	StddevOutputTokens *float64

	// Reasoning/cache metrics sum only the counts providers actually reported, so they stay
	// nil when none did rather than implying zero.
	TotalReasoningTokens  *int64
	MedianReasoningTokens *float64
	StddevReasoningTokens *float64

	TotalCacheReadTokens  *int64
	MedianCacheReadTokens *float64
	StddevCacheReadTokens *float64

	TotalCacheWriteTokens  *int64
	MedianCacheWriteTokens *float64
	StddevCacheWriteTokens *float64

	TotalToolCalls  *int64
	MedianToolCalls *float64
	StddevToolCalls *float64

	// EstimatedCandidateCost derives from the static prices the candidate run was configured
	// with and is never a billed amount. It is nil when any contributing result reported
	// usage that could not be priced, so a partial sum is never mistaken for a complete total.
	EstimatedCandidateCost *float64

	// CandidateCostCurrency is the ISO 4217 code EstimatedCandidateCost is expressed in,
	// empty when unknown.
	CandidateCostCurrency string

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

	var durations, input, output, reasoning, cacheRead, cacheWrite, toolCalls distribution
	var candidateCost costAccumulator

	for _, r := range results {
		if r.Kind == runners.NotSupported {
			continue // skipped tasks are excluded from every metric below
		}

		durations.add(utils.Ptr(r.Duration.Nanoseconds()))

		candidate := candidateUsage(r)
		input.add(sumUsage(candidate, runners.TokenUsage.EffectiveInputTokens))
		output.add(sumUsage(candidate, runners.TokenUsage.GeneratedTokens))
		reasoning.add(sumUsage(candidate, reasoningTokens))
		cacheRead.add(sumUsage(candidate, cacheReadTokens))
		cacheWrite.add(sumUsage(candidate, cacheWriteTokens))
		toolCalls.add(candidateToolCalls(r))

		candidateCost.add(candidate, r.RunConfig.Pricing)

		if r.Kind == runners.Error {
			if r.Details.Error.Transient != nil && *r.Details.Error.Transient {
				rec.TransientErrors++
			}
			if r.Details.Error.ResponseParsing != nil && *r.Details.Error.ResponseParsing {
				rec.ResponseParsingErrors++
			}
		}
	}

	if total := durations.sum(); total != nil {
		rec.TotalDuration = utils.Ptr(time.Duration(*total))
	}
	rec.MedianDuration = medianDuration(durations.samples)
	rec.StddevDuration = stddevDuration(durations.samples)

	rec.TotalInputTokens = input.sum()
	rec.MedianInputTokens = median(input.samples)
	rec.StddevInputTokens = stddev(input.samples)

	rec.TotalOutputTokens = output.sum()
	rec.MedianOutputTokens = median(output.samples)
	rec.StddevOutputTokens = stddev(output.samples)

	rec.TotalReasoningTokens = reasoning.sum()
	rec.MedianReasoningTokens = median(reasoning.samples)
	rec.StddevReasoningTokens = stddev(reasoning.samples)

	rec.TotalCacheReadTokens = cacheRead.sum()
	rec.MedianCacheReadTokens = median(cacheRead.samples)
	rec.StddevCacheReadTokens = stddev(cacheRead.samples)

	rec.TotalCacheWriteTokens = cacheWrite.sum()
	rec.MedianCacheWriteTokens = median(cacheWrite.samples)
	rec.StddevCacheWriteTokens = stddev(cacheWrite.samples)

	rec.TotalToolCalls = toolCalls.sum()
	rec.MedianToolCalls = median(toolCalls.samples)
	rec.StddevToolCalls = stddev(toolCalls.samples)

	rec.EstimatedCandidateCost = candidateCost.value()
	rec.CandidateCostCurrency = candidateCost.currencyCode()

	return rec
}

// distribution accumulates a metric's total and per-result samples, ignoring results that
// did not report it so that missing values never count as zero.
type distribution struct {
	total    int64
	samples  []float64
	reported bool
}

func (d *distribution) add(value *int64) {
	if value == nil {
		return
	}
	d.total += *value
	d.samples = append(d.samples, float64(*value))
	d.reported = true
}

// sum returns the accumulated total, or nil when no result reported the metric.
func (d distribution) sum() *int64 {
	if !d.reported {
		return nil
	}
	total := d.total
	return &total
}
