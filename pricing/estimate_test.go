// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package pricing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/runners"
)

const million = int64(1_000_000)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name      string
		usage     runners.TokenUsage
		prices    *runners.Pricing
		want      *Estimate
		wantNotOk bool
		wantErr   error
	}{
		{
			name: "input and output at their own rates",
			usage: runners.TokenUsage{
				InputTokens:  testutils.Ptr(million),
				OutputTokens: testutils.Ptr(million / 2),
			},
			prices: &runners.Pricing{
				Currency:         "USD",
				InputPerMillion:  testutils.Ptr(2.0),
				OutputPerMillion: testutils.Ptr(6.0),
			},
			want: &Estimate{Currency: "USD", Input: 2.0, Output: 3.0, Total: 5.0},
		},
		{
			name: "absent output accounting still prices legacy data even with a distinct reasoning rate",
			usage: runners.TokenUsage{
				OutputTokens: testutils.Ptr(million),
			},
			prices: &runners.Pricing{
				OutputPerMillion:    testutils.Ptr(10.0),
				ReasoningPerMillion: testutils.Ptr(20.0),
			},
			want: &Estimate{Output: 10.0, Total: 10.0},
		},
		{
			name: "separate cache accounting prices cache on top of input",
			usage: runners.TokenUsage{
				InputTokens:           testutils.Ptr(million),
				InputCacheReadTokens:  testutils.Ptr(million / 2),
				InputCacheWriteTokens: testutils.Ptr(million / 5),
				InputTokenAccounting:  runners.InputTokenAccountingCacheTokensSeparate,
			},
			prices: &runners.Pricing{
				InputPerMillion:      testutils.Ptr(2.0),
				CacheReadPerMillion:  testutils.Ptr(0.5),
				CacheWritePerMillion: testutils.Ptr(2.5),
			},
			want: &Estimate{Input: 2.0, CacheRead: 0.25, CacheWrite: 0.5, Total: 2.75},
		},
		{
			name: "included cache accounting prices only the uncached remainder at input rate",
			usage: runners.TokenUsage{
				InputTokens:           testutils.Ptr(million),
				InputCacheReadTokens:  testutils.Ptr(million / 2),
				InputCacheWriteTokens: testutils.Ptr(million / 5),
				InputTokenAccounting:  runners.InputTokenAccountingCacheTokensIncluded,
			},
			prices: &runners.Pricing{
				InputPerMillion:      testutils.Ptr(2.0),
				CacheReadPerMillion:  testutils.Ptr(0.5),
				CacheWritePerMillion: testutils.Ptr(2.5),
			},
			want: &Estimate{Input: 0.6, CacheRead: 0.25, CacheWrite: 0.5, Total: 1.35},
		},
		{
			name: "cache rates fall back to the input rate",
			usage: runners.TokenUsage{
				InputTokens:           testutils.Ptr(million),
				InputCacheReadTokens:  testutils.Ptr(million / 2),
				InputCacheWriteTokens: testutils.Ptr(int64(0)),
				InputTokenAccounting:  runners.InputTokenAccountingCacheTokensSeparate,
			},
			prices: &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)},
			want:   &Estimate{Input: 2.0, CacheRead: 1.0, Total: 3.0},
		},
		{
			name: "included reasoning without a distinct rate costs the same as plain output",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million),
				ReasoningTokens:       testutils.Ptr(million * 2 / 5),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensIncluded,
			},
			prices: &runners.Pricing{OutputPerMillion: testutils.Ptr(10.0)},
			want:   &Estimate{Output: 6.0, Reasoning: 4.0, Total: 10.0},
		},
		{
			name: "included reasoning at a distinct rate splits the output total",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million),
				ReasoningTokens:       testutils.Ptr(million * 2 / 5),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensIncluded,
			},
			prices: &runners.Pricing{
				OutputPerMillion:    testutils.Ptr(10.0),
				ReasoningPerMillion: testutils.Ptr(20.0),
			},
			want: &Estimate{Output: 6.0, Reasoning: 8.0, Total: 14.0},
		},
		{
			name: "separate reasoning adds to output at the output rate by default",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million),
				ReasoningTokens:       testutils.Ptr(million * 2 / 5),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensSeparate,
			},
			prices: &runners.Pricing{OutputPerMillion: testutils.Ptr(10.0)},
			want:   &Estimate{Output: 10.0, Reasoning: 4.0, Total: 14.0},
		},
		{
			name: "separate reasoning at a distinct rate adds on top of full output",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million),
				ReasoningTokens:       testutils.Ptr(million * 2 / 5),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensSeparate,
			},
			prices: &runners.Pricing{
				OutputPerMillion:    testutils.Ptr(10.0),
				ReasoningPerMillion: testutils.Ptr(20.0),
			},
			want: &Estimate{Output: 10.0, Reasoning: 8.0, Total: 18.0},
		},
		{
			name: "separate reasoning may exceed visible output",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million / 10),
				ReasoningTokens:       testutils.Ptr(million),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensSeparate,
			},
			prices: &runners.Pricing{OutputPerMillion: testutils.Ptr(10.0)},
			want:   &Estimate{Output: 1.0, Reasoning: 10.0, Total: 11.0},
		},
		{
			name: "explicit zero rates are free rather than unknown",
			usage: runners.TokenUsage{
				InputTokens:  testutils.Ptr(million),
				OutputTokens: testutils.Ptr(million),
			},
			prices: &runners.Pricing{
				InputPerMillion:  testutils.Ptr(0.0),
				OutputPerMillion: testutils.Ptr(0.0),
			},
			want: &Estimate{},
		},
		{
			name:   "an empty bucket costs nothing even without a rate",
			usage:  runners.TokenUsage{InputTokens: testutils.Ptr(int64(0))},
			prices: &runners.Pricing{OutputPerMillion: testutils.Ptr(5.0)},
			want:   &Estimate{},
		},
		{
			name:      "reported tokens without a resolved rate are unknown",
			usage:     runners.TokenUsage{InputTokens: testutils.Ptr(int64(1000))},
			prices:    &runners.Pricing{OutputPerMillion: testutils.Ptr(5.0)},
			wantNotOk: true,
		},
		{
			name: "included accounting cannot apply a distinct reasoning rate without a reasoning count",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensIncluded,
			},
			prices: &runners.Pricing{
				OutputPerMillion:    testutils.Ptr(10.0),
				ReasoningPerMillion: testutils.Ptr(20.0),
			},
			wantNotOk: true,
		},
		{
			name: "separate accounting without a reasoning count is unknown",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensSeparate,
			},
			prices:    &runners.Pricing{OutputPerMillion: testutils.Ptr(10.0)},
			wantNotOk: true,
		},
		{
			name: "separate accounting without a cache-read count is unknown",
			usage: runners.TokenUsage{
				InputTokens:           testutils.Ptr(million),
				InputCacheWriteTokens: testutils.Ptr(int64(0)),
				InputTokenAccounting:  runners.InputTokenAccountingCacheTokensSeparate,
			},
			prices:    &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)},
			wantNotOk: true,
		},
		{
			name: "separate accounting without a cache-write count is unknown",
			usage: runners.TokenUsage{
				InputTokens:          testutils.Ptr(million),
				InputCacheReadTokens: testutils.Ptr(int64(0)),
				InputTokenAccounting: runners.InputTokenAccountingCacheTokensSeparate,
			},
			prices:    &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)},
			wantNotOk: true,
		},
		{
			name: "included cache accounting cannot apply a distinct cache-read rate without a cache-read count",
			usage: runners.TokenUsage{
				InputTokens:           testutils.Ptr(million),
				InputCacheWriteTokens: testutils.Ptr(million / 5),
				InputTokenAccounting:  runners.InputTokenAccountingCacheTokensIncluded,
			},
			prices: &runners.Pricing{
				InputPerMillion:     testutils.Ptr(2.0),
				CacheReadPerMillion: testutils.Ptr(0.5),
			},
			wantNotOk: true,
		},
		{
			name: "included cache accounting cannot apply a distinct cache-write rate without a cache-write count",
			usage: runners.TokenUsage{
				InputTokens:          testutils.Ptr(million),
				InputCacheReadTokens: testutils.Ptr(million / 2),
				InputTokenAccounting: runners.InputTokenAccountingCacheTokensIncluded,
			},
			prices: &runners.Pricing{
				InputPerMillion:      testutils.Ptr(2.0),
				CacheWritePerMillion: testutils.Ptr(2.5),
			},
			wantNotOk: true,
		},
		{
			name: "included cache accounting without a cache split is fine when cache rates fall back to the input rate",
			usage: runners.TokenUsage{
				InputTokens:          testutils.Ptr(million),
				InputTokenAccounting: runners.InputTokenAccountingCacheTokensIncluded,
			},
			prices: &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)},
			want:   &Estimate{Input: 2.0, Total: 2.0},
		},
		{
			name:      "no prices configured",
			usage:     runners.TokenUsage{InputTokens: testutils.Ptr(million)},
			prices:    nil,
			wantNotOk: true,
		},
		{
			name:   "no usage reported",
			usage:  runners.TokenUsage{},
			prices: &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)},
		},
		{
			name: "unrecognized input accounting is unknown",
			usage: runners.TokenUsage{
				InputTokens:          testutils.Ptr(million),
				InputTokenAccounting: runners.InputTokenAccounting("future_mode"),
			},
			prices:    &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)},
			wantNotOk: true,
		},
		{
			name: "unrecognized output accounting is unknown",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(million),
				OutputTokenAccounting: runners.OutputTokenAccounting("future_mode"),
			},
			prices:    &runners.Pricing{OutputPerMillion: testutils.Ptr(2.0)},
			wantNotOk: true,
		},
		{
			name: "cached input exceeding the included total is inconsistent",
			usage: runners.TokenUsage{
				InputTokens:          testutils.Ptr(int64(100)),
				InputCacheReadTokens: testutils.Ptr(int64(200)),
				InputTokenAccounting: runners.InputTokenAccountingCacheTokensIncluded,
			},
			prices:  &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)},
			wantErr: ErrInconsistentUsage,
		},
		{
			name: "reasoning exceeding the included output total is inconsistent",
			usage: runners.TokenUsage{
				OutputTokens:          testutils.Ptr(int64(100)),
				ReasoningTokens:       testutils.Ptr(int64(200)),
				OutputTokenAccounting: runners.OutputTokenAccountingReasoningTokensIncluded,
			},
			prices:  &runners.Pricing{OutputPerMillion: testutils.Ptr(2.0)},
			wantErr: ErrInconsistentUsage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := Calculate(tt.usage, tt.prices)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, !tt.wantNotOk, ok)

			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want.Currency, got.Currency)
			assert.InDelta(t, tt.want.Input, got.Input, 0.000001)
			assert.InDelta(t, tt.want.CacheRead, got.CacheRead, 0.000001)
			assert.InDelta(t, tt.want.CacheWrite, got.CacheWrite, 0.000001)
			assert.InDelta(t, tt.want.Output, got.Output, 0.000001)
			assert.InDelta(t, tt.want.Reasoning, got.Reasoning, 0.000001)
			assert.InDelta(t, tt.want.Total, got.Total, 0.000001)
		})
	}
}

func TestCalculateNeverProducesNegativeCost(t *testing.T) {
	// Cached counters equal to the included input total leave nothing uncached.
	usage := runners.TokenUsage{
		InputTokens:           testutils.Ptr(million),
		InputCacheReadTokens:  testutils.Ptr(million / 2),
		InputCacheWriteTokens: testutils.Ptr(million / 2),
		InputTokenAccounting:  runners.InputTokenAccountingCacheTokensIncluded,
	}

	got, ok, err := Calculate(usage, &runners.Pricing{InputPerMillion: testutils.Ptr(2.0)})
	require.NoError(t, err)
	assert.True(t, ok)
	require.NotNil(t, got)
	assert.InDelta(t, 0.0, got.Input, 0.000001)
	assert.InDelta(t, 2.0, got.Total, 0.000001)
}
