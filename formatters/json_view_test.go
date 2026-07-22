// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultKindMappingConsistency(t *testing.T) {
	allKinds := []runners.ResultKind{
		runners.Success,
		runners.Failure,
		runners.Error,
		runners.NotSupported,
	}

	t.Run("every ResultKind has a stringToResultKind entry", func(t *testing.T) {
		for _, kind := range allKinds {
			status := ToStatus(kind)
			assert.Contains(t, stringToResultKind, status,
				"ToStatus(%d) = %q is missing from stringToResultKind", kind, status)
		}
	})

	t.Run("every stringToResultKind key is produced by ToStatus", func(t *testing.T) {
		toStatusValues := make(map[string]bool, len(allKinds))
		for _, kind := range allKinds {
			toStatusValues[ToStatus(kind)] = true
		}
		for key := range stringToResultKind {
			assert.True(t, toStatusValues[key],
				"stringToResultKind key %q is not produced by any ToStatus call", key)
		}
	})

	t.Run("round-trip preserves ResultKind", func(t *testing.T) {
		for _, kind := range allKinds {
			status := ToStatus(kind)
			roundTripped, ok := stringToResultKind[status]
			if assert.True(t, ok, "status %q not in stringToResultKind", status) {
				assert.Equal(t, kind, roundTripped,
					"round-trip failed: ResultKind %d -> %q -> %d", kind, status, roundTripped)
			}
		}
	})
}

func TestEmptyNonNilContainerSuppression(t *testing.T) {
	t.Run("answer details with empty non-nil fields", func(t *testing.T) {
		details := runners.Details{
			Answer: runners.AnswerDetails{
				Explanation:    []string{},
				ActualAnswer:   []string{},
				ExpectedAnswer: [][]string{},
				ToolUsage:      map[string]runners.ToolUsage{},
			},
		}
		view := newDetailsView(details)
		assert.Nil(t, view.Answer)
		assert.Nil(t, view.Validation)
		assert.Nil(t, view.Error)
	})

	t.Run("validation details with empty non-nil fields", func(t *testing.T) {
		details := runners.Details{
			Validation: runners.ValidationDetails{
				Explanation: []string{},
				ToolUsage:   map[string]runners.ToolUsage{},
			},
		}
		view := newDetailsView(details)
		assert.Nil(t, view.Validation)
	})

	t.Run("error details with empty non-nil fields", func(t *testing.T) {
		details := runners.Details{
			Error: runners.ErrorDetails{
				Details:   map[string][]string{},
				ToolUsage: map[string]runners.ToolUsage{},
			},
		}
		view := newDetailsView(details)
		assert.Nil(t, view.Error)
	})

	t.Run("non-empty fields are preserved", func(t *testing.T) {
		details := runners.Details{
			Answer: runners.AnswerDetails{
				Title:       "Test",
				Explanation: []string{},
				ToolUsage:   map[string]runners.ToolUsage{},
			},
		}
		view := newDetailsView(details)
		require.NotNil(t, view.Answer)
		assert.Equal(t, "Test", view.Answer.Title)
		assert.Empty(t, view.Answer.Explanation)
		assert.Nil(t, view.Answer.ToolUsage)
	})
}

func TestTaskMetadataViewRoundTrip(t *testing.T) {
	t.Run("empty metadata maps to nil view", func(t *testing.T) {
		view := newTaskMetadataView(runners.TaskMetadata{})
		assert.Nil(t, view)
		assert.Equal(t, runners.TaskMetadata{}, fromTaskMetadataView(view))
	})

	t.Run("non-empty metadata round-trips", func(t *testing.T) {
		metadata := runners.TaskMetadata{
			Suite:      "core-suite",
			Category:   "reasoning",
			Difficulty: "hard",
			Tags:       []string{"nightly", "regression"},
		}
		view := newTaskMetadataView(metadata)
		require.NotNil(t, view)
		assert.Equal(t, metadata, fromTaskMetadataView(view))
	})

	t.Run("partial metadata is not treated as empty", func(t *testing.T) {
		metadata := runners.TaskMetadata{Tags: []string{"smoke"}}
		view := newTaskMetadataView(metadata)
		require.NotNil(t, view)
		assert.Equal(t, metadata, fromTaskMetadataView(view))
	})
}

func TestToolUsageViewRoundTrip(t *testing.T) {
	t.Run("round-trips CallCount and TotalDuration", func(t *testing.T) {
		duration := 45 * time.Second
		usage := runners.ToolUsage{
			CallCount:     testutils.Ptr(int64(1)),
			TotalDuration: &duration,
		}
		view := newToolUsageView(usage)
		assert.Equal(t, usage, fromToolUsageView(view))
	})

	t.Run("zero-value round-trips", func(t *testing.T) {
		usage := runners.ToolUsage{}
		view := newToolUsageView(usage)
		assert.Equal(t, usage, fromToolUsageView(view))
	})
}

func TestToolCallSummaryViewsRoundTrip(t *testing.T) {
	t.Run("empty input maps to nil view", func(t *testing.T) {
		view := newToolCallSummaryViews([]runners.ToolCallSummary{})
		assert.Nil(t, view)
		assert.Nil(t, fromToolCallSummaryViews(view))
	})

	t.Run("calls round-trip including nil and populated previews", func(t *testing.T) {
		preview := "some output"
		exitCode := int64(1)
		startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		completedAt := startedAt.Add(1500 * time.Nanosecond)
		calls := []runners.ToolCallSummary{
			{
				Tool:             "calculator",
				CallID:           "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				ConversationTurn: 2,
				StartedAt:        startedAt,
				CompletedAt:      completedAt,
				Duration:         testutils.Ptr(1500 * time.Nanosecond),
				Status:           "success",
				Stdout:           &runners.ToolCallOutput{Bytes: 42},
			},
			{
				Tool:         "calculator",
				Duration:     testutils.Ptr(2500 * time.Nanosecond),
				ExitCode:     &exitCode,
				TimedOut:     false,
				Status:       "nonzero_exit",
				Stdout:       nil,
				Stderr:       &runners.ToolCallOutput{Bytes: int64(len(preview)), Preview: &preview, Truncated: true},
				ErrorMessage: "tool container exited with code 1",
			},
			{
				Tool:         "web_search",
				Duration:     nil,
				WallTime:     60 * time.Second,
				TimedOut:     true,
				Status:       "timeout",
				ErrorMessage: "tool execution timeout: execution timed out after 1m0s",
			},
		}
		view := newToolCallSummaryViews(calls)
		require.Len(t, view, 3)
		assert.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", view[0].CallID)
		assert.Equal(t, 2, view[0].ConversationTurn)
		assert.Equal(t, startedAt, view[0].StartedAt)
		assert.Equal(t, completedAt, view[0].CompletedAt)
		assert.Zero(t, view[2].ConversationTurn, "omitted when the caller did not provide one")
		assert.Nil(t, view[0].Stderr, "no stderr was ever captured for this call")
		require.NotNil(t, view[1].Stderr)
		assert.Nil(t, view[2].DurationNS, "the process never ran, so DurationNS is omitted rather than zero")
		assert.Equal(t, calls, fromToolCallSummaryViews(view))
	})
}
