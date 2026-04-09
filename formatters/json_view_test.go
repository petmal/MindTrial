// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"testing"

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
