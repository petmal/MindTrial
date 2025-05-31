// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatorIsCorrect(t *testing.T) {
	tests := []struct {
		name            string
		expected        utils.StringSet
		validationRules config.ValidationRules
		actual          Result
		want            bool
	}{
		// Basic correct and incorrect answers.
		{
			name:            "exact match - correct",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "hello world"},
			want:            true,
		},
		{
			name:            "exact match - incorrect",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "goodbye world"},
			want:            false,
		},

		// Multiple expected values - StringSet scenarios.
		{
			name:            "multiple expected - first match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "answer1"},
			want:            true,
		},
		{
			name:            "multiple expected - second match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "answer2"},
			want:            true,
		},
		{
			name:            "multiple expected - third match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "answer3"},
			want:            true,
		},
		{
			name:            "multiple expected - no match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "answer4"},
			want:            false,
		},

		// Default ValidationRules values (all nil - should be false by default).
		{
			name:            "default rules - case insensitive by default",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "hello world"},
			want:            true,
		},
		{
			name:            "default rules - whitespace trimmed by default",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "  hello world  "},
			want:            true,
		},
		{
			name:            "default rules - internal whitespace preserved by default",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "hello  world"}, // extra space should fail
			want:            false,
		},
		{
			name:            "default rules - tabs/newlines inside text preserved",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "hello\tworld"}, // tab should fail
			want:            false,
		},

		// CaseSensitive testing.
		{
			name:            "case sensitive - exact match",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "Hello World"},
			want:            true,
		},
		{
			name:            "case sensitive - case mismatch",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "hello world"},
			want:            false,
		},
		{
			name:            "case insensitive - case mismatch should pass",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "hello world"},
			want:            true,
		},
		{
			name:            "case insensitive - mixed case should pass",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "HeLLo WoRLd"},
			want:            true,
		},
		{
			name:            "case sensitive - mixed case should fail",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "HeLLo WoRLd"}, // same input as above but with case sensitivity
			want:            false,
		},

		// IgnoreWhitespace testing.
		{
			name:            "ignore whitespace - spaces removed",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "helloworld"},
			want:            true,
		},
		{
			name:            "ignore whitespace - tabs and newlines removed",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "hello\t\nworld"},
			want:            true,
		},
		{
			name:            "preserve whitespace - tabs and newlines should fail",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "hello\t\nworld"}, // same input but whitespace preserved
			want:            false,
		},
		{
			name:            "ignore whitespace - all whitespace removed",
			expected:        utils.NewStringSet("hello world test"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "  hello\t\n world   test  "},
			want:            true,
		},
		{
			name:            "ignore whitespace - newlines specifically",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "line1line2"},
			want:            true,
		},
		{
			name:            "preserve whitespace - spaces matter",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "helloworld"},
			want:            false,
		},
		{
			name:            "preserve whitespace - trimmed only",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "  hello world  "},
			want:            true,
		},

		// Combined ValidationRules.
		{
			name:            "case sensitive + ignore whitespace",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "Hello\t\nWorld"},
			want:            true,
		},
		{
			name:            "case sensitive + ignore whitespace - case mismatch",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "hello\t\nworld"},
			want:            false,
		},
		{
			name:            "case insensitive + preserve whitespace",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "hello world"},
			want:            true,
		},
		{
			name:            "case insensitive + preserve whitespace - whitespace mismatch",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "hello  world"},
			want:            false,
		},
		{
			name:            "case insensitive + ignore whitespace",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "hello\t\nworld"},
			want:            true,
		},

		// Edge cases and potential false positives.
		{
			name:            "empty strings",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: ""},
			want:            true,
		},
		{
			name:            "empty vs whitespace",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "   "},
			want:            true, // whitespace is trimmed by default
		},
		{
			name:            "empty vs whitespace - ignore whitespace",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: " \t\n "},
			want:            true,
		},
		{
			name:            "empty vs whitespace with newlines - default trim",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)}, // explicit false
			actual:          Result{FinalAnswer: " \t\n "},
			want:            true, // default trim should remove newlines too
		},
		{
			name:            "substring false positive prevention - longer actual",
			expected:        utils.NewStringSet("test"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "test this is a longer answer"},
			want:            false,
		},
		{
			name:            "substring false positive prevention - longer expected",
			expected:        utils.NewStringSet("test this is a longer answer"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "test"},
			want:            false,
		},
		{
			name:            "partial word match prevention",
			expected:        utils.NewStringSet("cat"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "concatenate"},
			want:            false,
		},
		{
			name:            "similar but different words",
			expected:        utils.NewStringSet("accept"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "except"},
			want:            false,
		},
		{
			name:            "unicode characters",
			expected:        utils.NewStringSet("café"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "café"},
			want:            true,
		},
		{
			name:            "unicode vs ascii false positive",
			expected:        utils.NewStringSet("café"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "cafe"},
			want:            false,
		},
		{
			name:            "number strings",
			expected:        utils.NewStringSet("123"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "123"},
			want:            true,
		},
		{
			name:            "number vs number-like string",
			expected:        utils.NewStringSet("123"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "123.0"},
			want:            false,
		},
		{
			name:            "punctuation edge case",
			expected:        utils.NewStringSet("hello!"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "hello!"},
			want:            true,
		},
		{
			name:            "punctuation false positive prevention",
			expected:        utils.NewStringSet("hello!"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "hello"},
			want:            false,
		},
		{
			name:            "mixed line endings in same string",
			expected:        utils.NewStringSet("line1\nline2\r\nline3"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "line1\nline2\r\nline3"},
			want:            true,
		},
		{
			name:            "mixed line endings with ignore whitespace",
			expected:        utils.NewStringSet("line1\nline2\r\nline3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "line1line2line3"},
			want:            true,
		},

		// Multiple lines.
		{
			name:            "multiline exact match",
			expected:        utils.NewStringSet("line1\nline2\nline3"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "line1\nline2\nline3"},
			want:            true,
		},
		{
			name:            "multiline with different line endings",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{},
			actual:          Result{FinalAnswer: "line1\r\nline2"},
			want:            false, // different line endings should not match
		},
		{
			name:            "multiline ignore whitespace",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "line1line2"},
			want:            true,
		},
		{
			name:            "multiline ignore whitespace - different line endings",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "line1\r\nline2"},
			want:            true, // should match when ignoring whitespace
		},

		// Complex combinations with multiple expected values.
		{
			name:            "multiple expected with case sensitivity",
			expected:        utils.NewStringSet("Answer1", "answer2", "ANSWER3"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "answer2"},
			want:            true,
		},
		{
			name:            "multiple expected with case sensitivity - no match",
			expected:        utils.NewStringSet("Answer1", "answer2", "ANSWER3"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "answer1"}, // case doesn't match Answer1
			want:            false,
		},
		{
			name:            "multiple expected with whitespace handling",
			expected:        utils.NewStringSet("answer 1", "answer2", "answer 3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          Result{FinalAnswer: "answer1"},
			want:            true, // matches "answer 1" with whitespace removed
		},
		{
			name:            "multiple expected with whitespace handling - no match when preserved",
			expected:        utils.NewStringSet("answer 1", "answer2", "answer 3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          Result{FinalAnswer: "answer1"}, // doesn't match any with spaces preserved
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewDefaultValidator(tt.expected, tt.validationRules)
			result := validator.IsCorrect(context.Background(), tt.actual)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestValidatorToCanonical(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		validationRules config.ValidationRules
		want            string
	}{
		// Default behavior (case insensitive, trim spaces)
		{
			name:            "default - lowercase and trim",
			value:           "  Correct Answer  ",
			validationRules: config.ValidationRules{},
			want:            "correct answer",
		},
		{
			name:            "default - mixed case",
			value:           "HeLLo WoRLd",
			validationRules: config.ValidationRules{},
			want:            "hello world",
		},

		// Case sensitivity testing
		{
			name:            "case sensitive - preserve case",
			value:           "Correct Answer",
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			want:            "Correct Answer",
		},
		{
			name:            "case sensitive - with trim",
			value:           "  Correct Answer  ",
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			want:            "Correct Answer",
		},
		{
			name:            "case insensitive - explicit",
			value:           "Correct Answer",
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false)},
			want:            "correct answer",
		},

		// Whitespace handling
		{
			name:            "ignore whitespace - spaces",
			value:           "hello world",
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			want:            "helloworld",
		},
		{
			name:            "ignore whitespace - tabs and newlines",
			value:           "hello\t\nworld",
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			want:            "helloworld",
		},
		{
			name:            "ignore whitespace - multiple spaces",
			value:           "  hello   world  test  ",
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			want:            "helloworldtest",
		},
		{
			name:            "preserve whitespace - trim only",
			value:           "  hello world  ",
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			want:            "hello world",
		},

		// Combined rules
		{
			name:            "case sensitive + ignore whitespace",
			value:           "  Hello\t\nWorld  ",
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			want:            "HelloWorld",
		},
		{
			name:            "case insensitive + ignore whitespace",
			value:           "  Hello\t\nWorld  ",
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(true)},
			want:            "helloworld",
		},
		{
			name:            "case sensitive + preserve whitespace",
			value:           "  Hello World  ",
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(false)},
			want:            "Hello World",
		},

		// Edge cases
		{
			name:            "empty string",
			value:           "",
			validationRules: config.ValidationRules{},
			want:            "",
		},
		{
			name:            "only whitespace - default",
			value:           "   \t\n   ",
			validationRules: config.ValidationRules{},
			want:            "",
		},
		{
			name:            "only whitespace - ignore whitespace",
			value:           "   \t\n   ",
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			want:            "",
		},
		{
			name:            "single character",
			value:           "A",
			validationRules: config.ValidationRules{},
			want:            "a",
		},
		{
			name:            "unicode characters",
			value:           "  Café  ",
			validationRules: config.ValidationRules{},
			want:            "café",
		},
		{
			name:            "numbers and symbols",
			value:           "  Test-123!  ",
			validationRules: config.ValidationRules{},
			want:            "test-123!",
		},
		{
			name:            "multiline text",
			value:           "line1\nline2\nline3",
			validationRules: config.ValidationRules{},
			want:            "line1\nline2\nline3",
		},
		{
			name:            "multiline text - ignore whitespace",
			value:           "line1\nline2\nline3",
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			want:            "line1line2line3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewDefaultValidator(utils.StringSet{}, tt.validationRules)
			assert.Equal(t, tt.want, validator.ToCanonical(tt.value))
		})
	}
}

func TestTimed(t *testing.T) {
	sleepDuration := 100 * time.Millisecond
	f := func() (string, error) {
		time.Sleep(sleepDuration)
		return "Administrator", errors.ErrUnsupported
	}

	var duration time.Duration
	result, err := timed(f, &duration)

	require.Equal(t, "Administrator", result)
	require.ErrorIs(t, err, errors.ErrUnsupported)
	assert.GreaterOrEqual(t, duration, sleepDuration)
}

func TestResultGetPrompts(t *testing.T) {
	tests := []struct {
		name    string
		prompts []string
		want    []string
	}{
		{
			name:    "empty prompts",
			prompts: []string{},
			want:    nil,
		},
		{
			name:    "single prompt",
			prompts: []string{"Test prompt"},
			want:    []string{"Test prompt"},
		},
		{
			name:    "multiple prompts",
			prompts: []string{"Prompt 1", "Prompt 2", "Prompt 3"},
			want:    []string{"Prompt 1", "Prompt 2", "Prompt 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Result{}
			for _, prompt := range tt.prompts {
				result.recordPrompt(prompt)
			}
			assert.Equal(t, tt.want, result.GetPrompts())
		})
	}
}

func TestResultGetUsage(t *testing.T) {
	tests := []struct {
		name         string
		init         Usage
		inputTokens  *int64
		outputTokens *int64
		want         Usage
	}{
		{
			name: "zero usage",
			want: Usage{},
		},
		{
			name:        "input tokens only",
			inputTokens: testutils.Ptr(int64(100)),
			want:        Usage{InputTokens: testutils.Ptr(int64(100))},
		},
		{
			name:         "output tokens only",
			outputTokens: testutils.Ptr(int64(200)),
			want:         Usage{OutputTokens: testutils.Ptr(int64(200))},
		},
		{
			name:         "both input and output tokens",
			inputTokens:  testutils.Ptr(int64(300)),
			outputTokens: testutils.Ptr(int64(400)),
			want:         Usage{InputTokens: testutils.Ptr(int64(300)), OutputTokens: testutils.Ptr(int64(400))},
		},
		{
			name:         "both input and output tokens with initial values",
			init:         Usage{InputTokens: testutils.Ptr(int64(50)), OutputTokens: testutils.Ptr(int64(75))},
			inputTokens:  testutils.Ptr(int64(500)),
			outputTokens: testutils.Ptr(int64(600)),
			want:         Usage{InputTokens: testutils.Ptr(int64(550)), OutputTokens: testutils.Ptr(int64(675))},
		},
		{
			name:         "large tokens",
			inputTokens:  testutils.Ptr(int64(9313009999906870)),
			outputTokens: testutils.Ptr(int64(6440809999935592)),
			want:         Usage{InputTokens: testutils.Ptr(int64(9313009999906870)), OutputTokens: testutils.Ptr(int64(6440809999935592))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Result{usage: tt.init}
			recordUsage(tt.inputTokens, tt.outputTokens, &result.usage)
			assert.Equal(t, tt.want, result.GetUsage())
		})
	}
}
