// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import "github.com/petmal/mindtrial/runners"

// effectiveInputTokens resolves a single TokenUsage's input token count according to its
// InputTokenAccounting, mirroring the HTML report's dynamic summary: for
// cache_tokens_included, cache read/write counters are subsets already reflected in
// InputTokens; for cache_tokens_separate (including absent/legacy accounting), they must be
// added to obtain total input usage. Returns nil if no relevant counter was reported.
func effectiveInputTokens(u runners.TokenUsage) *int64 {
	if u.InputTokenAccounting == runners.InputTokenAccountingCacheTokensIncluded {
		return u.InputTokens
	}
	return addInt64(addInt64(u.InputTokens, u.InputCacheWriteTokens), u.InputCacheReadTokens)
}

// addInt64 sums a and b, treating a nil operand as absent rather than zero. Returns nil
// only when both operands are nil.
func addInt64(a, b *int64) *int64 {
	if a == nil && b == nil {
		return nil
	}
	var sum int64
	if a != nil {
		sum += *a
	}
	if b != nil {
		sum += *b
	}
	return &sum
}

// sumToolCallCount totals CallCount across every tool in usage, or nil if none reported it.
func sumToolCallCount(usage map[string]runners.ToolUsage) *int64 {
	var total *int64
	for _, u := range usage {
		total = addInt64(total, u.CallCount)
	}
	return total
}

// candidateInputTokens, candidateOutputTokens and candidateToolCalls sum usage from the
// candidate answer and any subsequent candidate-attributable error (never judge/validation
// usage), matching the HTML report's dynamic summary. An error whose FromValidation is
// true carries the judge's usage from a failed validation attempt, not the candidate's, and
// is excluded here the same way the (also-excluded) Validation section's own usage is.
func candidateInputTokens(r runners.RunResult) *int64 {
	if r.Details.Error.FromValidation {
		return effectiveInputTokens(r.Details.Answer.Usage)
	}
	return addInt64(effectiveInputTokens(r.Details.Answer.Usage), effectiveInputTokens(r.Details.Error.Usage))
}

func candidateOutputTokens(r runners.RunResult) *int64 {
	if r.Details.Error.FromValidation {
		return r.Details.Answer.Usage.OutputTokens
	}
	return addInt64(r.Details.Answer.Usage.OutputTokens, r.Details.Error.Usage.OutputTokens)
}

func candidateToolCalls(r runners.RunResult) *int64 {
	if r.Details.Error.FromValidation {
		return sumToolCallCount(r.Details.Answer.ToolUsage)
	}
	return addInt64(sumToolCallCount(r.Details.Answer.ToolUsage), sumToolCallCount(r.Details.Error.ToolUsage))
}
