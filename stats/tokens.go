// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/pricing"
	"github.com/petmal/mindtrial/runners"
)

// candidateUsage returns the token usage records attributable to the candidate response:
// the answer, plus any candidate-attributable error. This matches the HTML report's
// dynamic summary, which excludes the Validation section. An error flagged FromValidation
// carries the judge's usage from a failed validation attempt, not the candidate's, and is
// excluded here the same way.
func candidateUsage(r runners.RunResult) []runners.TokenUsage {
	usage := []runners.TokenUsage{r.Details.Answer.Usage}
	if !r.Details.Error.FromValidation {
		usage = append(usage, r.Details.Error.Usage)
	}
	return usage
}

// sumUsage totals metric across usage records, treating an unreported metric as absent
// rather than zero. Returns nil when no record reports it.
func sumUsage(usage []runners.TokenUsage, metric func(runners.TokenUsage) *int64) *int64 {
	var total *int64
	for _, u := range usage {
		total = utils.SumPtr(total, metric(u))
	}
	return total
}

func reasoningTokens(u runners.TokenUsage) *int64 { return u.ReasoningTokens }

func cacheReadTokens(u runners.TokenUsage) *int64 { return u.InputCacheReadTokens }

func cacheWriteTokens(u runners.TokenUsage) *int64 { return u.InputCacheWriteTokens }

// sumToolCallCount totals CallCount across every tool in usage, or nil if none reported it.
func sumToolCallCount(usage map[string]runners.ToolUsage) *int64 {
	var total *int64
	for _, u := range usage {
		total = utils.SumPtr(total, u.CallCount)
	}
	return total
}

// candidateToolCalls totals tool invocations attributable to the candidate response,
// mirroring candidateUsage's attribution rules.
func candidateToolCalls(r runners.RunResult) *int64 {
	if r.Details.Error.FromValidation {
		return sumToolCallCount(r.Details.Answer.ToolUsage)
	}
	return utils.SumPtr(sumToolCallCount(r.Details.Answer.ToolUsage), sumToolCallCount(r.Details.Error.ToolUsage))
}

// costAccumulator totals estimated costs across a group. Unknown propagates: if any
// contributing result reported usage that could not be priced, or the group mixes
// currencies, the total is unavailable rather than a deceptively low partial sum.
type costAccumulator struct {
	total    float64
	currency string
	reported bool
	unknown  bool
}

func (c *costAccumulator) add(usage []runners.TokenUsage, prices *runners.Pricing) {
	for _, u := range usage {
		estimate, ok, err := pricing.Calculate(u, prices)
		if err != nil || !ok {
			c.unknown = true
			continue
		}
		if estimate == nil {
			continue // nothing was consumed, so nothing contributes
		}
		if c.currency != "" && c.currency != estimate.Currency {
			c.unknown = true
			continue
		}
		c.currency = estimate.Currency
		c.total += estimate.Total
		c.reported = true
	}
}

// value returns the accumulated cost, or nil when it is unknown or nothing was priced.
func (c costAccumulator) value() *float64 {
	var total *float64
	if !c.unknown && c.reported {
		total = utils.Ptr(c.total)
	}
	return total
}

// currencyCode returns the currency value() is expressed in, or "" whenever value() is nil.
func (c costAccumulator) currencyCode() string {
	if c.value() == nil {
		return ""
	}
	return c.currency
}
