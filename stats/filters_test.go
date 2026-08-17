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

func TestParseTagMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    TagMode
		wantErr bool
	}{
		{name: "blank defaults to all", input: "", want: TagModeAll},
		{name: "all", input: "all", want: TagModeAll},
		{name: "any", input: "any", want: TagModeAny},
		{name: "case insensitive", input: "ANY", want: TagModeAny},
		{name: "trims whitespace", input: " all ", want: TagModeAll},
		{name: "invalid", input: "bogus", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTagMode(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidTagMode)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFiltersValidate(t *testing.T) {
	assert.NoError(t, Filters{}.Validate())
	assert.NoError(t, Filters{Statuses: []string{"passed"}}.Validate())
	assert.ErrorIs(t, Filters{Statuses: []string{"bogus"}}.Validate(), ErrInvalidStatus)
}

func TestFiltersResolvedStatuses(t *testing.T) {
	t.Run("no statuses configured", func(t *testing.T) {
		kinds, err := Filters{}.resolvedStatuses()
		require.NoError(t, err)
		assert.Nil(t, kinds)
	})

	t.Run("all valid statuses", func(t *testing.T) {
		kinds, err := Filters{Statuses: []string{" Passed ", "FAILED", "Error", "skipped"}}.resolvedStatuses()
		require.NoError(t, err)
		assert.Equal(t, map[runners.ResultKind]bool{
			runners.Success:      true,
			runners.Failure:      true,
			runners.Error:        true,
			runners.NotSupported: true,
		}, kinds)
	})

	t.Run("invalid status", func(t *testing.T) {
		_, err := Filters{Statuses: []string{"bogus"}}.resolvedStatuses()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestComputeStatsFiltersProduceExpectedGroups(t *testing.T) {
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
					Suite:      "core",
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
					Suite:      "extended",
					Category:   "coding",
					Difficulty: "hard",
					Tags:       []string{"text", "smoke"},
				},
			},
		},
		"anthropic": {
			{
				Provider: "anthropic",
				Run:      "run-a",
				RunConfig: runners.RunConfigSnapshot{
					Model: "model-3",
				},
				Kind: runners.Error,
				TaskMetadata: runners.TaskMetadata{
					Suite:      "core",
					Category:   "coding",
					Difficulty: "medium",
					Tags:       []string{"visual", "nightly"},
				},
			},
			{
				Provider: "anthropic",
				Run:      "run-b",
				RunConfig: runners.RunConfigSnapshot{
					Model: "model-4",
				},
				Kind: runners.NotSupported,
				TaskMetadata: runners.TaskMetadata{
					Suite:      "legacy",
					Category:   "vision",
					Difficulty: "hard",
				},
			},
		},
	}

	type counts struct {
		count   int
		skipped int
	}
	tests := []struct {
		name    string
		filters Filters
		want    map[string]counts
	}{
		{name: "provider", filters: Filters{Providers: []string{"OPENAI"}}, want: map[string]counts{"openai/run-a": {count: 1}, "openai/run-b": {count: 1}}},
		{name: "run", filters: Filters{Runs: []string{"RUN-A"}}, want: map[string]counts{"anthropic/run-a": {count: 1}, "openai/run-a": {count: 1}}},
		{name: "model with OR values", filters: Filters{Models: []string{"missing", "MODEL-2"}}, want: map[string]counts{"openai/run-b": {count: 1}}},
		{name: "suite", filters: Filters{Suites: []string{"CORE"}}, want: map[string]counts{"anthropic/run-a": {count: 1}, "openai/run-a": {count: 1}}},
		{name: "category", filters: Filters{Categories: []string{"CODING"}}, want: map[string]counts{"anthropic/run-a": {count: 1}, "openai/run-b": {count: 1}}},
		{name: "difficulty", filters: Filters{Difficulties: []string{"HARD"}}, want: map[string]counts{"anthropic/run-b": {skipped: 1}, "openai/run-b": {count: 1}}},
		{name: "passed status", filters: Filters{Statuses: []string{"passed"}}, want: map[string]counts{"openai/run-a": {count: 1}}},
		{name: "failed status", filters: Filters{Statuses: []string{"failed"}}, want: map[string]counts{"openai/run-b": {count: 1}}},
		{name: "error status", filters: Filters{Statuses: []string{"error"}}, want: map[string]counts{"anthropic/run-a": {count: 1}}},
		{name: "skipped status", filters: Filters{Statuses: []string{"skipped"}}, want: map[string]counts{"anthropic/run-b": {skipped: 1}}},
		{name: "tags use default all mode", filters: Filters{Tags: []string{"VISUAL", "SMOKE"}}, want: map[string]counts{"openai/run-a": {count: 1}}},
		{name: "tags all mode", filters: Filters{Tags: []string{"visual", "nightly"}, TagMode: TagModeAll}, want: map[string]counts{"anthropic/run-a": {count: 1}}},
		{name: "tags any mode", filters: Filters{Tags: []string{"nightly", "text"}, TagMode: TagModeAny}, want: map[string]counts{"anthropic/run-a": {count: 1}, "openai/run-b": {count: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, err := ComputeStats(results, []Dimension{DimensionProvider, DimensionRun}, tt.filters)
			require.NoError(t, err)
			require.Len(t, records, len(tt.want))

			got := make(map[string]counts, len(records))
			for _, record := range records {
				key := record.Dimensions[string(DimensionProvider)] + "/" + record.Dimensions[string(DimensionRun)]
				got[key] = counts{count: record.Count, skipped: record.Skipped}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFiltersMatches(t *testing.T) {
	result := runners.RunResult{
		Kind:     runners.Success,
		Provider: "OpenAI",
		Run:      "GPT-4",
		RunConfig: runners.RunConfigSnapshot{
			Model: "gpt-4o",
		},
		TaskMetadata: runners.TaskMetadata{
			Suite:      "Benchmark-1",
			Category:   "Math",
			Difficulty: "Hard",
			Tags:       []string{"Visual", "Spatial"},
		},
	}

	tests := []struct {
		name    string
		filters Filters
		want    bool
	}{
		{name: "no filters matches everything", filters: Filters{}, want: true},
		{name: "provider match case-insensitive", filters: Filters{Providers: []string{"openai"}}, want: true},
		{name: "provider mismatch", filters: Filters{Providers: []string{"anthropic"}}, want: false},
		{name: "run match", filters: Filters{Runs: []string{"gpt-4"}}, want: true},
		{name: "run mismatch", filters: Filters{Runs: []string{"gpt-3"}}, want: false},
		{name: "model match", filters: Filters{Models: []string{"GPT-4O"}}, want: true},
		{name: "model mismatch", filters: Filters{Models: []string{"gpt-3"}}, want: false},
		{name: "suite match", filters: Filters{Suites: []string{"benchmark-1"}}, want: true},
		{name: "suite mismatch", filters: Filters{Suites: []string{"benchmark-2"}}, want: false},
		{name: "category match", filters: Filters{Categories: []string{"math"}}, want: true},
		{name: "category mismatch", filters: Filters{Categories: []string{"coding"}}, want: false},
		{name: "difficulty match", filters: Filters{Difficulties: []string{"hard"}}, want: true},
		{name: "difficulty mismatch", filters: Filters{Difficulties: []string{"easy"}}, want: false},
		{name: "tag all mode matches subset present", filters: Filters{Tags: []string{"visual"}, TagMode: TagModeAll}, want: true},
		{name: "tag all mode requires every tag", filters: Filters{Tags: []string{"visual", "missing"}, TagMode: TagModeAll}, want: false},
		{name: "tag any mode matches if one present", filters: Filters{Tags: []string{"missing", "spatial"}, TagMode: TagModeAny}, want: true},
		{name: "tag any mode fails if none present", filters: Filters{Tags: []string{"missing"}, TagMode: TagModeAny}, want: false},
		{name: "combined filters all match", filters: Filters{Providers: []string{"openai"}, Suites: []string{"benchmark-1"}}, want: true},
		{name: "combined filters one mismatches", filters: Filters{Providers: []string{"openai"}, Suites: []string{"other"}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := tt.filters.resolvedStatuses()
			require.NoError(t, err)
			assert.Equal(t, tt.want, tt.filters.matches(result, allowed))
		})
	}

	t.Run("status filter restricts by kind", func(t *testing.T) {
		filters := Filters{Statuses: []string{"failed"}}
		allowed, err := filters.resolvedStatuses()
		require.NoError(t, err)
		assert.False(t, filters.matches(result, allowed)) // result Kind is Success

		filters = Filters{Statuses: []string{"passed"}}
		allowed, err = filters.resolvedStatuses()
		require.NoError(t, err)
		assert.True(t, filters.matches(result, allowed))
	})
}

func TestFiltersMatchesUnspecifiedPlaceholder(t *testing.T) {
	blank := runners.RunResult{Kind: runners.Success}
	withSuite := runners.RunResult{Kind: runners.Success, TaskMetadata: runners.TaskMetadata{Suite: "bench"}}

	tests := []struct {
		name    string
		result  runners.RunResult
		filters Filters
		want    bool
	}{
		{name: "suite unspecified matches blank suite", result: blank, filters: Filters{Suites: []string{unspecifiedValue}}, want: true},
		{name: "category unspecified matches blank category", result: blank, filters: Filters{Categories: []string{unspecifiedValue}}, want: true},
		{name: "difficulty unspecified matches blank difficulty", result: blank, filters: Filters{Difficulties: []string{unspecifiedValue}}, want: true},
		{name: "provider unspecified matches blank provider", result: blank, filters: Filters{Providers: []string{unspecifiedValue}}, want: true},
		{name: "run unspecified matches blank run", result: blank, filters: Filters{Runs: []string{unspecifiedValue}}, want: true},
		{name: "model unspecified matches blank model", result: blank, filters: Filters{Models: []string{unspecifiedValue}}, want: true},
		{name: "tag unspecified matches untagged result", result: blank, filters: Filters{Tags: []string{unspecifiedValue}}, want: true},
		{name: "suite unspecified does not match a real suite value", result: withSuite, filters: Filters{Suites: []string{unspecifiedValue}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := tt.filters.resolvedStatuses()
			require.NoError(t, err)
			assert.Equal(t, tt.want, tt.filters.matches(tt.result, allowed))
		})
	}
}
