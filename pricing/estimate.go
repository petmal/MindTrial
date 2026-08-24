// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package pricing computes estimated costs from recorded token usage and a static price
// list. Every value it produces is an estimate derived from configured rates, never a
// billed amount, and an unknown rate is never assumed to be free.
package pricing

import (
	"errors"
	"fmt"

	"github.com/petmal/mindtrial/runners"
)

// ErrInconsistentUsage is returned when token counters contradict their accounting mode,
// such as cached input tokens exceeding the input total they are declared to be part of.
var ErrInconsistentUsage = errors.New("inconsistent token usage")

// tokensPerMillion converts a token count to the per-million unit prices are quoted in.
const tokensPerMillion = 1_000_000

// Estimate breaks the estimated cost down by token category, expressed in Currency.
type Estimate struct {
	Currency   string
	Input      float64
	CacheRead  float64
	CacheWrite float64
	Output     float64
	Reasoning  float64
	Total      float64
}

// Calculate returns the estimated cost of usage under prices.
//
// A nil estimate is ambiguous on its own: it means either nothing was consumed (ok is
// true) or usage was reported but its cost could not be determined reliably - no prices
// were configured, a reported token bucket has no resolved rate, or a required
// reasoning/cache split is unknown and would change the total (ok is false). Callers that
// need to tell "nothing to price" apart from "could not price it" should check ok rather
// than inferring it from usage themselves. An error is returned only for internally
// inconsistent usage, which is a data defect rather than missing information.
func Calculate(usage runners.TokenUsage, prices *runners.Pricing) (estimate *Estimate, ok bool, err error) {
	buckets, err := resolveBuckets(usage)
	if err != nil {
		return nil, false, err
	}
	if buckets == nil {
		return nil, !hasReportedTokens(usage), nil // unrecognized accounting mode
	}
	if buckets.empty() {
		return nil, true, nil // nothing was consumed, so nothing to price
	}
	if prices == nil {
		return nil, false, nil // usage was reported but no prices are configured
	}

	// Cache and reasoning rates fall back to the bucket they are a variant of.
	cacheReadRate := firstNonNil(prices.CacheReadPerMillion, prices.InputPerMillion)
	cacheWriteRate := firstNonNil(prices.CacheWritePerMillion, prices.InputPerMillion)
	reasoningRate := firstNonNil(prices.ReasoningPerMillion, prices.OutputPerMillion)

	if !canComputeAccuratePrice(usage, prices) {
		return nil, false, nil
	}

	result := Estimate{Currency: prices.Currency}
	for _, bucket := range []struct {
		tokens *int64
		rate   *float64
		cost   *float64
	}{
		{buckets.input, prices.InputPerMillion, &result.Input},
		{buckets.cacheRead, cacheReadRate, &result.CacheRead},
		{buckets.cacheWrite, cacheWriteRate, &result.CacheWrite},
		{buckets.output, prices.OutputPerMillion, &result.Output},
		{buckets.reasoning, reasoningRate, &result.Reasoning},
	} {
		cost, ok := priceBucket(bucket.tokens, bucket.rate)
		if !ok {
			return nil, false, nil
		}
		*bucket.cost = cost
		result.Total += cost
	}

	return &result, true, nil
}

// hasReportedTokens reports whether any token counter was recorded, distinguishing usage
// that could not be priced from usage that never occurred.
func hasReportedTokens(u runners.TokenUsage) bool {
	return u.InputTokens != nil || u.OutputTokens != nil || u.ReasoningTokens != nil ||
		u.InputCacheReadTokens != nil || u.InputCacheWriteTokens != nil
}

// buckets holds token counts already apportioned to the rate each is priced at, so that
// no category is counted twice.
type buckets struct {
	input      *int64
	cacheRead  *int64
	cacheWrite *int64
	output     *int64
	reasoning  *int64
}

func (b buckets) empty() bool {
	return b.input == nil && b.cacheRead == nil && b.cacheWrite == nil && b.output == nil && b.reasoning == nil
}

// canComputeAccuratePrice reports whether every unreported subset counter resolveBuckets
// assumed to be zero is safe to price that way. Under separate accounting, a subset is
// additive and independent of its parent total, so a missing count may hide real,
// unpriced activity and always blocks pricing. Under included accounting, a subset is
// already part of a known total, so a missing count only matters if its rate would
// actually differ from the total's - otherwise the split is irrelevant to the price.
// Absent accounting ("") is exempt from both checks: it predates the field that would
// have reported the subset at all, so a missing count there is expected, not suspicious.
func canComputeAccuratePrice(usage runners.TokenUsage, prices *runners.Pricing) bool {
	switch usage.OutputTokenAccounting {
	case runners.OutputTokenAccountingReasoningTokensSeparate:
		if usage.OutputTokens != nil && usage.ReasoningTokens == nil {
			return false // reasoning may have run unreported; this is not a splittable total
		}
	case runners.OutputTokenAccountingReasoningTokensIncluded:
		if usage.OutputTokens != nil && usage.ReasoningTokens == nil && ratesDiffer(prices.OutputPerMillion, prices.ReasoningPerMillion) {
			return false // unknown subset, and it would be priced differently from output
		}
	default: // "" predates reasoning tracking entirely; a missing counter was simply never recorded
	}

	switch usage.InputTokenAccounting {
	case runners.InputTokenAccountingCacheTokensSeparate:
		if usage.InputTokens != nil && usage.InputCacheReadTokens == nil {
			return false // cache reads may have run unreported; this is not a splittable total
		}
		if usage.InputTokens != nil && usage.InputCacheWriteTokens == nil {
			return false // cache writes may have run unreported; this is not a splittable total
		}
	case runners.InputTokenAccountingCacheTokensIncluded:
		if usage.InputTokens != nil && usage.InputCacheReadTokens == nil && ratesDiffer(prices.InputPerMillion, prices.CacheReadPerMillion) {
			return false // unknown subset, and it would be priced differently from input
		}
		if usage.InputTokens != nil && usage.InputCacheWriteTokens == nil && ratesDiffer(prices.InputPerMillion, prices.CacheWritePerMillion) {
			return false // unknown subset, and it would be priced differently from input
		}
	default: // "" predates cache tracking entirely; a missing counter was simply never recorded
	}

	return true
}

// ratesDiffer reports whether two rates would price the same tokens differently. Either
// rate being unset means Calculate's fallback makes them resolve to the same value.
func ratesDiffer(a, b *float64) bool {
	return a != nil && b != nil && *a != *b
}

// resolveBuckets apportions usage into disjoint priced categories according to its
// accounting modes. Any subset counter left unreported (cache reads/writes, reasoning
// tokens) is assumed to be zero and folded into the total it is a variant of; whether
// that assumption is safe to price is decided separately by canComputeAccuratePrice. A
// nil bucket set is returned only when an accounting mode is not recognized.
func resolveBuckets(u runners.TokenUsage) (*buckets, error) {
	resolved := buckets{
		cacheRead:  u.InputCacheReadTokens,
		cacheWrite: u.InputCacheWriteTokens,
	}

	switch u.InputTokenAccounting {
	case runners.InputTokenAccountingCacheTokensSeparate, "":
		resolved.input = u.InputTokens // already independent of the cache counters
	case runners.InputTokenAccountingCacheTokensIncluded:
		if u.InputTokens != nil {
			uncached := *u.InputTokens - deref(u.InputCacheReadTokens) - deref(u.InputCacheWriteTokens)
			if uncached < 0 {
				return nil, fmt.Errorf("%w: cached input tokens exceed the reported input total", ErrInconsistentUsage)
			}
			resolved.input = &uncached // a missing cache counter is assumed to be zero here
		}
	default:
		return nil, nil // unrecognized accounting mode makes the split undeterminable
	}

	switch u.OutputTokenAccounting {
	case runners.OutputTokenAccountingReasoningTokensIncluded, "":
		if u.OutputTokens != nil && u.ReasoningTokens != nil {
			if *u.ReasoningTokens > *u.OutputTokens {
				return nil, fmt.Errorf("%w: reasoning tokens exceed the reported output total", ErrInconsistentUsage)
			}
			visible := *u.OutputTokens - *u.ReasoningTokens
			resolved.output = &visible // reasoning is a known subset, split it out
			resolved.reasoning = u.ReasoningTokens
		} else {
			resolved.output = u.OutputTokens // a missing reasoning count is assumed to be zero here
		}
	case runners.OutputTokenAccountingReasoningTokensSeparate:
		resolved.output = u.OutputTokens       // already independent of ReasoningTokens
		resolved.reasoning = u.ReasoningTokens // may be nil; see canComputeAccuratePrice
	default:
		return nil, nil // unrecognized accounting mode makes the split undeterminable
	}

	return &resolved, nil
}

// priceBucket returns the cost of tokens at ratePerMillion. An empty bucket costs nothing
// regardless of its rate; a nonempty one with an unknown rate cannot be priced at all.
func priceBucket(tokens *int64, ratePerMillion *float64) (cost float64, ok bool) {
	if tokens == nil || *tokens == 0 {
		return 0, true
	}
	if ratePerMillion == nil {
		return 0, false
	}
	return float64(*tokens) / tokensPerMillion * *ratePerMillion, true
}

func firstNonNil(preferred *float64, fallback *float64) *float64 {
	if preferred != nil {
		return preferred
	}
	return fallback
}

func deref(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
