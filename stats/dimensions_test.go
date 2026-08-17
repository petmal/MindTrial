// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/runners"
)

func TestParseDimensions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Dimension
		wantErr bool
	}{
		{name: "single dimension", input: "provider", want: []Dimension{DimensionProvider}},
		{name: "multiple dimensions", input: "provider,run", want: []Dimension{DimensionProvider, DimensionRun}},
		{name: "trims whitespace and normalizes case", input: " Provider , RUN ", want: []Dimension{DimensionProvider, DimensionRun}},
		{name: "all valid dimensions", input: "provider,run,model,suite,category,difficulty,tag", want: []Dimension{
			DimensionProvider, DimensionRun, DimensionModel, DimensionSuite, DimensionCategory, DimensionDifficulty, DimensionTag,
		}},
		{name: "blank entries ignored", input: "provider,,run,", want: []Dimension{DimensionProvider, DimensionRun}},
		{name: "empty string", input: "", wantErr: true},
		{name: "only blanks", input: " , ,", wantErr: true},
		{name: "invalid dimension", input: "provider,bogus", wantErr: true},
		{name: "duplicate dimension", input: "provider,run,provider", wantErr: true},
		{name: "duplicate dimension after case/whitespace normalization", input: "provider, Provider ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDimensions(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidDimension)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDimensionValues(t *testing.T) {
	result := runners.RunResult{
		Provider: "openai",
		Run:      "gpt-4",
		RunConfig: runners.RunConfigSnapshot{
			Model: "gpt-4o",
		},
		TaskMetadata: runners.TaskMetadata{
			Suite:      "benchmark-1",
			Category:   "math",
			Difficulty: "hard",
			Tags:       []string{"visual", "spatial"},
		},
	}

	assert.Equal(t, []string{"openai"}, dimensionValues(result, DimensionProvider))
	assert.Equal(t, []string{"gpt-4"}, dimensionValues(result, DimensionRun))
	assert.Equal(t, []string{"gpt-4o"}, dimensionValues(result, DimensionModel))
	assert.Equal(t, []string{"benchmark-1"}, dimensionValues(result, DimensionSuite))
	assert.Equal(t, []string{"math"}, dimensionValues(result, DimensionCategory))
	assert.Equal(t, []string{"hard"}, dimensionValues(result, DimensionDifficulty))
	assert.Equal(t, []string{"visual", "spatial"}, dimensionValues(result, DimensionTag))

	t.Run("unspecified metadata", func(t *testing.T) {
		blank := runners.RunResult{}
		assert.Equal(t, []string{unspecifiedValue}, dimensionValues(blank, DimensionProvider))
		assert.Equal(t, []string{unspecifiedValue}, dimensionValues(blank, DimensionModel))
		assert.Equal(t, []string{unspecifiedValue}, dimensionValues(blank, DimensionTag))
	})

	t.Run("duplicate tags are deduplicated", func(t *testing.T) {
		duplicated := runners.RunResult{
			TaskMetadata: runners.TaskMetadata{Tags: []string{"visual", "visual", "spatial", "visual"}},
		}
		assert.Equal(t, []string{"visual", "spatial"}, dimensionValues(duplicated, DimensionTag))
	})
}

func TestComputeStatsGroupsByEveryDimension(t *testing.T) {
	results := runners.Results{
		"openai": {
			{
				Provider: "openai",
				Run:      "run-a",
				RunConfig: runners.RunConfigSnapshot{
					Model: "model-1",
				},
				Kind: runners.Success,
				TaskMetadata: runners.TaskMetadata{
					Suite:      "suite-1",
					Category:   "reasoning",
					Difficulty: "easy",
					Tags:       []string{"visual", "smoke"},
				},
			},
			{
				Provider: "openai",
				Run:      "run-b",
				RunConfig: runners.RunConfigSnapshot{
					Model: "model-2",
				},
				Kind: runners.Failure,
				TaskMetadata: runners.TaskMetadata{
					Suite:      "suite-2",
					Category:   "coding",
					Difficulty: "hard",
					Tags:       []string{"smoke"},
				},
			},
		},
		"anthropic": {
			{
				Provider: "anthropic",
				Run:      "run-a",
				RunConfig: runners.RunConfigSnapshot{
					Model: "model-1",
				},
				Kind: runners.Error,
				TaskMetadata: runners.TaskMetadata{
					Category:   "reasoning",
					Difficulty: "easy",
				},
			},
		},
	}

	tests := []struct {
		name      string
		dimension Dimension
		want      map[string]int
	}{
		{name: "provider", dimension: DimensionProvider, want: map[string]int{"anthropic": 1, "openai": 2}},
		{name: "run", dimension: DimensionRun, want: map[string]int{"run-a": 2, "run-b": 1}},
		{name: "model", dimension: DimensionModel, want: map[string]int{"model-1": 2, "model-2": 1}},
		{name: "suite", dimension: DimensionSuite, want: map[string]int{unspecifiedValue: 1, "suite-1": 1, "suite-2": 1}},
		{name: "category", dimension: DimensionCategory, want: map[string]int{"coding": 1, "reasoning": 2}},
		{name: "difficulty", dimension: DimensionDifficulty, want: map[string]int{"easy": 2, "hard": 1}},
		{name: "tag", dimension: DimensionTag, want: map[string]int{unspecifiedValue: 1, "smoke": 2, "visual": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, err := ComputeStats(results, []Dimension{tt.dimension}, Filters{})
			require.NoError(t, err)
			require.Len(t, records, len(tt.want))

			got := make(map[string]int, len(records))
			for _, record := range records {
				got[record.Dimensions[string(tt.dimension)]] = record.Count
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDimensionCombinations(t *testing.T) {
	t.Run("scalar dimensions produce exactly one combination", func(t *testing.T) {
		result := runners.RunResult{Provider: "openai", Run: "gpt-4"}
		combos := dimensionCombinations(result, []Dimension{DimensionProvider, DimensionRun})
		assert.Equal(t, [][]string{{"openai", "gpt-4"}}, combos)
	})

	t.Run("tag dimension explodes into one combination per tag", func(t *testing.T) {
		result := runners.RunResult{
			Provider:     "openai",
			TaskMetadata: runners.TaskMetadata{Tags: []string{"visual", "spatial", "grayscale"}},
		}
		combos := dimensionCombinations(result, []Dimension{DimensionProvider, DimensionTag})
		assert.ElementsMatch(t, [][]string{
			{"openai", "visual"},
			{"openai", "spatial"},
			{"openai", "grayscale"},
		}, combos)
	})

	t.Run("tag dimension at the start of groupBy", func(t *testing.T) {
		result := runners.RunResult{
			Provider:     "openai",
			Run:          "gpt-4",
			TaskMetadata: runners.TaskMetadata{Tags: []string{"visual", "spatial"}},
		}
		combos := dimensionCombinations(result, []Dimension{DimensionTag, DimensionProvider, DimensionRun})
		assert.ElementsMatch(t, [][]string{
			{"visual", "openai", "gpt-4"},
			{"spatial", "openai", "gpt-4"},
		}, combos)
	})

	t.Run("tag dimension in the middle of groupBy", func(t *testing.T) {
		result := runners.RunResult{
			Provider:     "openai",
			Run:          "gpt-4",
			TaskMetadata: runners.TaskMetadata{Tags: []string{"visual", "spatial"}},
		}
		combos := dimensionCombinations(result, []Dimension{DimensionProvider, DimensionTag, DimensionRun})
		assert.ElementsMatch(t, [][]string{
			{"openai", "visual", "gpt-4"},
			{"openai", "spatial", "gpt-4"},
		}, combos)
	})

	t.Run("two multi-valued positions produce a full cartesian product", func(t *testing.T) {
		// groupBy with DimensionTag appearing on both sides of another dimension isn't a
		// realistic CLI input (each dimension normally appears once), but exercises the
		// general cartesian-product combination logic beyond the single multi-valued case.
		result := runners.RunResult{
			TaskMetadata: runners.TaskMetadata{Tags: []string{"a", "b"}},
		}
		combos := dimensionCombinations(result, []Dimension{DimensionTag, DimensionProvider, DimensionTag})
		assert.ElementsMatch(t, [][]string{
			{"a", unspecifiedValue, "a"},
			{"a", unspecifiedValue, "b"},
			{"b", unspecifiedValue, "a"},
			{"b", unspecifiedValue, "b"},
		}, combos)
	})

	t.Run("no tags falls back to unspecified", func(t *testing.T) {
		result := runners.RunResult{Provider: "openai"}
		combos := dimensionCombinations(result, []Dimension{DimensionTag})
		assert.Equal(t, [][]string{{unspecifiedValue}}, combos)
	})
}
