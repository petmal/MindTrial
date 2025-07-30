// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package logging_test

import (
	"log/slog"
	"testing"

	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestFormatLogInt64(t *testing.T) {
	tests := []struct {
		name     string
		value    *int64
		expected string
	}{
		{
			name:     "nil pointer",
			value:    nil,
			expected: logging.UnknownLogValue,
		},
		{
			name:     "zero value",
			value:    testutils.Ptr(int64(0)),
			expected: "0",
		},
		{
			name:     "positive value",
			value:    testutils.Ptr(int64(12345)),
			expected: "12345",
		},
		{
			name:     "negative value",
			value:    testutils.Ptr(int64(-789)),
			expected: "-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logging.FormatLogInt64(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatLogText(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{
			name:     "empty slice",
			lines:    []string{},
			expected: "\t" + logging.UnknownLogValue,
		},
		{
			name:     "nil slice",
			lines:    nil,
			expected: "\t" + logging.UnknownLogValue,
		},
		{
			name:     "single line",
			lines:    []string{"line1"},
			expected: "\tline1",
		},
		{
			name:     "multiple lines",
			lines:    []string{"line1", "line2", "line3"},
			expected: "\tline1\n\n\tline2\n\n\tline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logging.FormatLogText(tt.lines)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLogLevels(t *testing.T) {
	assert.Equal(t, slog.Level(-8), logging.LevelTrace) //nolint:testifylint
	assert.Equal(t, slog.LevelDebug, logging.LevelDebug)
	assert.Equal(t, slog.LevelInfo, logging.LevelInfo)
	assert.Equal(t, slog.LevelWarn, logging.LevelWarn)
	assert.Equal(t, slog.LevelError, logging.LevelError)
}
