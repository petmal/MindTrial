// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/runners"
)

func TestComputeStatsRatesAndCounts(t *testing.T) {
	results := runners.Results{"openai": {}}
	addN := func(kind runners.ResultKind, n int) {
		for i := 0; i < n; i++ {
			results["openai"] = append(results["openai"], runners.RunResult{Provider: "openai", Run: "r1", Kind: kind})
		}
	}
	addN(runners.Success, 6)
	addN(runners.Failure, 2)
	addN(runners.Error, 2)
	addN(runners.NotSupported, 3)

	records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)

	rec := records[0]
	assert.Equal(t, 10, rec.Count)
	assert.Equal(t, 6, rec.Passed)
	assert.Equal(t, 2, rec.Failed)
	assert.Equal(t, 2, rec.Error)
	assert.Equal(t, 3, rec.Skipped)
	assert.InDelta(t, 60.0, rec.PassRate, 0.001)
	assert.InDelta(t, 75.0, rec.Accuracy, 0.001)
	assert.InDelta(t, 20.0, rec.ErrorRate, 0.001)
}

func TestComputeStatsAllSkippedRatesAreZero(t *testing.T) {
	results := runners.Results{"openai": {
		{Provider: "openai", Run: "r1", Kind: runners.NotSupported},
		{Provider: "openai", Run: "r1", Kind: runners.NotSupported},
	}}

	records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)

	rec := records[0]
	assert.Equal(t, 0, rec.Count)
	assert.Equal(t, 2, rec.Skipped)
	assert.InDelta(t, 0.0, rec.PassRate, 0.001)
	assert.InDelta(t, 0.0, rec.Accuracy, 0.001)
	assert.InDelta(t, 0.0, rec.ErrorRate, 0.001)
	assert.Nil(t, rec.TotalDuration)
	assert.Nil(t, rec.MedianDuration)
	assert.Nil(t, rec.StddevDuration)
	assert.Nil(t, rec.TotalInputTokens)
	assert.Nil(t, rec.MedianInputTokens)
	assert.Nil(t, rec.StddevInputTokens)
	assert.Nil(t, rec.TotalOutputTokens)
	assert.Nil(t, rec.MedianOutputTokens)
	assert.Nil(t, rec.StddevOutputTokens)
	assert.Nil(t, rec.TotalToolCalls)
	assert.Nil(t, rec.MedianToolCalls)
	assert.Nil(t, rec.StddevToolCalls)
	assert.Zero(t, rec.TransientErrors)
	assert.Zero(t, rec.ResponseParsingErrors)
}

func TestComputeStatsCalculatesEveryExposedValue(t *testing.T) {
	results := runners.Results{
		"openai": {
			{
				Provider: "openai",
				Run:      "r1",
				Kind:     runners.Success,
				Duration: time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:           testutils.Ptr(int64(10)),
							InputCacheWriteTokens: testutils.Ptr(int64(2)),
							InputCacheReadTokens:  testutils.Ptr(int64(3)),
							OutputTokens:          testutils.Ptr(int64(5)),
						},
						ToolUsage: map[string]runners.ToolUsage{
							"tool": {CallCount: testutils.Ptr(int64(1))},
						},
					},
				},
			},
			{
				Provider: "openai",
				Run:      "r1",
				Kind:     runners.Failure,
				Duration: 2 * time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:          testutils.Ptr(int64(20)),
							InputCacheReadTokens: testutils.Ptr(int64(7)),
							OutputTokens:         testutils.Ptr(int64(10)),
							InputTokenAccounting: runners.InputTokenAccountingCacheTokensIncluded,
						},
						ToolUsage: map[string]runners.ToolUsage{
							"tool": {CallCount: testutils.Ptr(int64(3))},
						},
					},
				},
			},
			{
				Provider: "openai",
				Run:      "r1",
				Kind:     runners.Error,
				Duration: 3 * time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(5)),
							OutputTokens: testutils.Ptr(int64(2)),
						},
						ToolUsage: map[string]runners.ToolUsage{
							"tool": {CallCount: testutils.Ptr(int64(2))},
						},
					},
					Error: runners.ErrorDetails{
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(25)),
							OutputTokens: testutils.Ptr(int64(13)),
						},
						ToolUsage: map[string]runners.ToolUsage{
							"tool": {CallCount: testutils.Ptr(int64(4))},
						},
						Transient:       testutils.Ptr(true),
						ResponseParsing: testutils.Ptr(true),
					},
				},
			},
			{
				Provider: "openai",
				Run:      "r1",
				Kind:     runners.Success,
				Duration: 4 * time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(40)),
							OutputTokens: testutils.Ptr(int64(20)),
						},
						ToolUsage: map[string]runners.ToolUsage{
							"tool": {CallCount: testutils.Ptr(int64(10))},
						},
					},
					Validation: runners.ValidationDetails{
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(9999)),
							OutputTokens: testutils.Ptr(int64(9999)),
						},
					},
				},
			},
			{
				Provider: "openai",
				Run:      "r1",
				Kind:     runners.NotSupported,
				Duration: 100 * time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(9999)),
							OutputTokens: testutils.Ptr(int64(9999)),
						},
					},
				},
			},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionProvider, DimensionRun}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	rec := records[0]

	assert.Equal(t, map[string]string{"provider": "openai", "run": "r1"}, rec.Dimensions)
	assert.Equal(t, 4, rec.Count)
	assert.Equal(t, 2, rec.Passed)
	assert.Equal(t, 1, rec.Failed)
	assert.Equal(t, 1, rec.Error)
	assert.Equal(t, 1, rec.Skipped)
	assert.InDelta(t, 50.0, rec.PassRate, 0.001)
	assert.InDelta(t, 66.67, rec.Accuracy, 0.001)
	assert.InDelta(t, 25.0, rec.ErrorRate, 0.001)

	require.NotNil(t, rec.TotalDuration)
	assert.Equal(t, 10*time.Second, *rec.TotalDuration)
	require.NotNil(t, rec.MedianDuration)
	assert.Equal(t, 2500*time.Millisecond, *rec.MedianDuration)
	require.NotNil(t, rec.StddevDuration)
	assert.InDelta(t, 1_118_033_988, rec.StddevDuration.Nanoseconds(), 1)

	require.NotNil(t, rec.TotalInputTokens)
	assert.Equal(t, int64(105), *rec.TotalInputTokens)
	require.NotNil(t, rec.MedianInputTokens)
	assert.InDelta(t, 25.0, *rec.MedianInputTokens, 0.001)
	require.NotNil(t, rec.StddevInputTokens)
	assert.InDelta(t, 9.601432, *rec.StddevInputTokens, 0.000001)

	require.NotNil(t, rec.TotalOutputTokens)
	assert.Equal(t, int64(50), *rec.TotalOutputTokens)
	require.NotNil(t, rec.MedianOutputTokens)
	assert.InDelta(t, 12.5, *rec.MedianOutputTokens, 0.001)
	require.NotNil(t, rec.StddevOutputTokens)
	assert.InDelta(t, 5.590170, *rec.StddevOutputTokens, 0.000001)

	require.NotNil(t, rec.TotalToolCalls)
	assert.Equal(t, int64(20), *rec.TotalToolCalls)
	require.NotNil(t, rec.MedianToolCalls)
	assert.InDelta(t, 4.5, *rec.MedianToolCalls, 0.001)
	require.NotNil(t, rec.StddevToolCalls)
	assert.InDelta(t, 3.391165, *rec.StddevToolCalls, 0.000001)
	assert.Equal(t, 1, rec.TransientErrors)
	assert.Equal(t, 1, rec.ResponseParsingErrors)
}

func TestComputeStatsGroupingByProviderAndRun(t *testing.T) {
	results := runners.Results{
		"openai": {
			{Provider: "openai", Run: "gpt-4", Kind: runners.Success},
			{Provider: "openai", Run: "gpt-4", Kind: runners.Failure},
			{Provider: "openai", Run: "gpt-3", Kind: runners.Success},
		},
		"anthropic": {
			{Provider: "anthropic", Run: "claude", Kind: runners.Success},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionProvider, DimensionRun}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 3)

	// Deterministic ordering: sorted by provider then run.
	assert.Equal(t, "anthropic", records[0].Dimensions["provider"])
	assert.Equal(t, "claude", records[0].Dimensions["run"])
	assert.Equal(t, "openai", records[1].Dimensions["provider"])
	assert.Equal(t, "gpt-3", records[1].Dimensions["run"])
	assert.Equal(t, "openai", records[2].Dimensions["provider"])
	assert.Equal(t, "gpt-4", records[2].Dimensions["run"])
	assert.Equal(t, 2, records[2].Count)
}

func TestComputeStatsTagExplosionOverlapsNotAdditive(t *testing.T) {
	results := runners.Results{
		"openai": {
			{Provider: "openai", Kind: runners.Success, TaskMetadata: runners.TaskMetadata{Tags: []string{"visual", "spatial", "grayscale"}}},
			{Provider: "openai", Kind: runners.Failure, TaskMetadata: runners.TaskMetadata{Tags: []string{"visual"}}},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionTag}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 3) // visual, spatial, grayscale

	byTag := make(map[string]Record)
	for _, r := range records {
		byTag[r.Dimensions["tag"]] = r
	}
	require.Contains(t, byTag, "visual")
	require.Contains(t, byTag, "spatial")
	require.Contains(t, byTag, "grayscale")

	assert.Equal(t, 2, byTag["visual"].Count) // both results carry "visual"
	assert.Equal(t, 1, byTag["spatial"].Count)
	assert.Equal(t, 1, byTag["grayscale"].Count)
}

func TestComputeStatsDuplicateTagsAreNotDoubleCounted(t *testing.T) {
	results := runners.Results{
		"openai": {
			{Provider: "openai", Kind: runners.Success, Duration: time.Second, TaskMetadata: runners.TaskMetadata{Tags: []string{"visual", "visual"}}},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionTag}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1) // duplicate tag must not create/inflate a second group

	rec := records[0]
	assert.Equal(t, "visual", rec.Dimensions["tag"])
	assert.Equal(t, 1, rec.Count)
	assert.Equal(t, 1, rec.Passed)
	require.NotNil(t, rec.TotalDuration)
	assert.Equal(t, time.Second, *rec.TotalDuration)
}

func TestComputeStatsUnspecifiedMetadataGrouping(t *testing.T) {
	results := runners.Results{
		"openai": {
			{Provider: "openai", Kind: runners.Success, TaskMetadata: runners.TaskMetadata{Suite: ""}},
			{Provider: "openai", Kind: runners.Success, TaskMetadata: runners.TaskMetadata{Suite: "bench"}},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionSuite}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 2)

	suites := []string{records[0].Dimensions["suite"], records[1].Dimensions["suite"]}
	assert.Contains(t, suites, unspecifiedValue)
	assert.Contains(t, suites, "bench")
}

func TestComputeStatsFiltering(t *testing.T) {
	results := runners.Results{
		"openai": {
			{Provider: "openai", Run: "gpt-4", Kind: runners.Success, TaskMetadata: runners.TaskMetadata{Tags: []string{"visual"}}},
			{Provider: "openai", Run: "gpt-3", Kind: runners.Failure, TaskMetadata: runners.TaskMetadata{Tags: []string{"text"}}},
		},
		"anthropic": {
			{Provider: "anthropic", Run: "claude", Kind: runners.Success},
		},
	}

	t.Run("provider filter", func(t *testing.T) {
		records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{Providers: []string{"openai"}})
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, "openai", records[0].Dimensions["provider"])
		assert.Equal(t, 2, records[0].Count)
	})

	t.Run("status filter", func(t *testing.T) {
		records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{Statuses: []string{"failed"}})
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, 1, records[0].Failed)
	})

	t.Run("invalid status returns error", func(t *testing.T) {
		_, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{Statuses: []string{"bogus"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatus)
	})

	t.Run("tag filter", func(t *testing.T) {
		records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{Tags: []string{"visual"}})
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, 1, records[0].Count)
	})
}

func TestComputeStatsCandidateTokenAndDurationAggregation(t *testing.T) {
	results := runners.Results{
		"openai": {
			{
				Provider: "openai", Run: "r1", Kind: runners.Success, Duration: 1 * time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:          testutils.Ptr(int64(100)),
							InputCacheReadTokens: testutils.Ptr(int64(20)),
							OutputTokens:         testutils.Ptr(int64(50)),
							InputTokenAccounting: runners.InputTokenAccountingCacheTokensSeparate,
						},
					},
				},
			},
			{
				Provider: "openai", Run: "r1", Kind: runners.Success, Duration: 3 * time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:          testutils.Ptr(int64(200)),
							OutputTokens:         testutils.Ptr(int64(80)),
							InputTokenAccounting: runners.InputTokenAccountingCacheTokensIncluded,
						},
					},
				},
			},
			{
				// Error kind: usage should still contribute (candidate + error usage summed).
				Provider: "openai", Run: "r1", Kind: runners.Error, Duration: 2 * time.Second,
				Details: runners.Details{
					Error: runners.ErrorDetails{
						Message: "boom",
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(30)),
							OutputTokens: testutils.Ptr(int64(10)),
						},
					},
				},
			},
			{
				// Validation-only usage must be excluded from candidate token totals.
				Provider: "openai", Run: "r1", Kind: runners.Success, Duration: 1 * time.Second,
				Details: runners.Details{
					Validation: runners.ValidationDetails{
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(9999)),
							OutputTokens: testutils.Ptr(int64(9999)),
						},
					},
				},
			},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	rec := records[0]

	// Input: (100+20) + 200 + 30 + 0(validation excluded, no answer/error usage on 4th result) = 350
	require.NotNil(t, rec.TotalInputTokens)
	assert.Equal(t, int64(350), *rec.TotalInputTokens)

	// Output: 50 + 80 + 10 = 140 (validation excluded)
	require.NotNil(t, rec.TotalOutputTokens)
	assert.Equal(t, int64(140), *rec.TotalOutputTokens)

	// Duration: sum of all 4 non-skipped results = 1+3+2+1 = 7s
	require.NotNil(t, rec.TotalDuration)
	assert.Equal(t, 7*time.Second, *rec.TotalDuration)

	require.NotNil(t, rec.MedianDuration)
	require.NotNil(t, rec.StddevDuration)
}

func TestComputeStatsFromValidationErrorUsageNotCounted(t *testing.T) {
	results := runners.Results{
		"openai": {
			{
				// Mirrors a failed validation attempt: Answer holds the candidate's own
				// usage, Error holds the judge's usage but is flagged FromValidation.
				Provider: "openai", Run: "r1", Kind: runners.Error, Duration: time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(40)),
							OutputTokens: testutils.Ptr(int64(15)),
						},
						ToolUsage: map[string]runners.ToolUsage{
							"tool": {CallCount: testutils.Ptr(int64(2))},
						},
					},
					Error: runners.ErrorDetails{
						Message: "judge evaluation failed",
						Usage: runners.TokenUsage{
							InputTokens:  testutils.Ptr(int64(9999)),
							OutputTokens: testutils.Ptr(int64(9999)),
						},
						ToolUsage: map[string]runners.ToolUsage{
							"judge-tool": {CallCount: testutils.Ptr(int64(9999))},
						},
						FromValidation: true,
					},
				},
			},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	rec := records[0]

	require.NotNil(t, rec.TotalInputTokens)
	assert.Equal(t, int64(40), *rec.TotalInputTokens) // judge's 9999 excluded

	require.NotNil(t, rec.TotalOutputTokens)
	assert.Equal(t, int64(15), *rec.TotalOutputTokens) // judge's 9999 excluded

	require.NotNil(t, rec.TotalToolCalls)
	assert.Equal(t, int64(2), *rec.TotalToolCalls) // judge's 9999 calls excluded
}

func TestComputeStatsSkippedResultsExcludedFromMetrics(t *testing.T) {
	results := runners.Results{
		"openai": {
			{
				Provider: "openai", Run: "r1", Kind: runners.NotSupported, Duration: 100 * time.Second,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						Usage: runners.TokenUsage{InputTokens: testutils.Ptr(int64(99999)), OutputTokens: testutils.Ptr(int64(99999))},
					},
				},
			},
			{Provider: "openai", Run: "r1", Kind: runners.Success, Duration: 1 * time.Second},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	rec := records[0]

	assert.Nil(t, rec.TotalInputTokens)
	assert.Nil(t, rec.TotalOutputTokens)
	require.NotNil(t, rec.TotalDuration)
	assert.Equal(t, 1*time.Second, *rec.TotalDuration)
}

func TestComputeStatsToolCallAggregation(t *testing.T) {
	results := runners.Results{
		"openai": {
			{
				Provider: "openai", Run: "r1", Kind: runners.Success,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						ToolUsage: map[string]runners.ToolUsage{
							"tool-a": {CallCount: testutils.Ptr(int64(2))},
							"tool-b": {CallCount: testutils.Ptr(int64(3))},
						},
					},
				},
			},
			{
				Provider: "openai", Run: "r1", Kind: runners.Success,
				Details: runners.Details{
					Answer: runners.AnswerDetails{
						ToolUsage: map[string]runners.ToolUsage{
							"tool-a": {CallCount: testutils.Ptr(int64(1))},
						},
					},
				},
			},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	rec := records[0]

	require.NotNil(t, rec.TotalToolCalls)
	assert.Equal(t, int64(6), *rec.TotalToolCalls) // (2+3) + 1
	require.NotNil(t, rec.MedianToolCalls)
	assert.InDelta(t, 3.0, *rec.MedianToolCalls, 0.001) // median of [5, 1]
}

func TestComputeStatsErrorDiagnostics(t *testing.T) {
	results := runners.Results{
		"openai": {
			{Provider: "openai", Run: "r1", Kind: runners.Error, Details: runners.Details{
				Error: runners.ErrorDetails{Message: "transient", Transient: testutils.Ptr(true)},
			}},
			{Provider: "openai", Run: "r1", Kind: runners.Error, Details: runners.Details{
				Error: runners.ErrorDetails{Message: "parsing", ResponseParsing: testutils.Ptr(true)},
			}},
			{Provider: "openai", Run: "r1", Kind: runners.Error, Details: runners.Details{
				Error: runners.ErrorDetails{Message: "both", Transient: testutils.Ptr(true), ResponseParsing: testutils.Ptr(true)},
			}},
			{Provider: "openai", Run: "r1", Kind: runners.Error, Details: runners.Details{
				Error: runners.ErrorDetails{Message: "permanent", Transient: testutils.Ptr(false)},
			}},
		},
	}

	records, err := ComputeStats(results, []Dimension{DimensionProvider}, Filters{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	rec := records[0]

	assert.Equal(t, 2, rec.TransientErrors)       // "transient" + "both"
	assert.Equal(t, 2, rec.ResponseParsingErrors) // "parsing" + "both"
}

func TestComputeStatsInvalidGroupBy(t *testing.T) {
	results := runners.Results{"openai": {{Provider: "openai", Kind: runners.Success}}}
	records, err := ComputeStats(results, []Dimension{"bogus"}, Filters{})
	// ComputeStats itself doesn't validate dimension names (that's ParseDimensions'
	// responsibility): an unknown dimension simply contributes no values, so no result
	// belongs to any group and the returned record set is empty rather than erroring.
	require.NoError(t, err)
	assert.Empty(t, records)
}
