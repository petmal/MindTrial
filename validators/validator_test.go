// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMockResult creates a providers.Result for testing.
func createMockResult(response interface{}) providers.Result {
	return providers.Result{
		Title:       "Mock Title",
		Explanation: "Mock Explanation",
		FinalAnswer: providers.Answer{Content: response},
	}
}

func TestValidatorIsCorrect(t *testing.T) {
	tests := []struct {
		name            string
		expected        utils.ValueSet
		validationRules config.ValidationRules
		actual          providers.Result
		want            bool
	}{
		// Basic correct and incorrect answers.
		{
			name:            "exact match - correct",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "exact match - incorrect",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("goodbye world"),
			want:            false,
		},

		// Multiple expected values - StringSet scenarios.
		{
			name:            "multiple expected - first match",
			expected:        utils.NewValueSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer1"),
			want:            true,
		},
		{
			name:            "multiple expected - second match",
			expected:        utils.NewValueSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer2"),
			want:            true,
		},
		{
			name:            "multiple expected - third match",
			expected:        utils.NewValueSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer3"),
			want:            true,
		},
		{
			name:            "multiple expected - no match",
			expected:        utils.NewValueSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer4"),
			want:            false,
		},

		// Default ValidationRules values (all nil - should be false by default).
		{
			name:            "default rules - case insensitive by default",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "default rules - whitespace trimmed by default",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("  hello world  "),
			want:            true,
		},
		{
			name:            "default rules - internal whitespace preserved by default",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello  world"), // extra space should fail
			want:            false,
		},
		{
			name:            "default rules - tabs/newlines inside text preserved",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello\tworld"), // tab should fail
			want:            false,
		},

		// CaseSensitive testing.
		{
			name:            "case sensitive - exact match",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("Hello World"),
			want:            true,
		},
		{
			name:            "case sensitive - case mismatch",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("hello world"),
			want:            false,
		},
		{
			name:            "case insensitive - case mismatch should pass",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false)},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "case insensitive - mixed case should pass",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false)},
			actual:          createMockResult("HeLLo WoRLd"),
			want:            true,
		},
		{
			name:            "case sensitive - mixed case should fail",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("HeLLo WoRLd"), // same input as above but with case sensitivity
			want:            false,
		},

		// IgnoreWhitespace testing.
		{
			name:            "ignore whitespace - spaces removed",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("helloworld"),
			want:            true,
		},
		{
			name:            "ignore whitespace - tabs and newlines removed",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("hello\t\nworld"),
			want:            true,
		},
		{
			name:            "preserve whitespace - tabs and newlines should fail",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("hello\t\nworld"), // same input but whitespace preserved
			want:            false,
		},
		{
			name:            "ignore whitespace - all whitespace removed",
			expected:        utils.NewValueSet("hello world test"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("  hello\t\n world   test  "),
			want:            true,
		},
		{
			name:            "ignore whitespace - newlines specifically",
			expected:        utils.NewValueSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1line2"),
			want:            true,
		},
		{
			name:            "preserve whitespace - spaces matter",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("helloworld"),
			want:            false,
		},
		{
			name:            "preserve whitespace - trimmed only",
			expected:        utils.NewValueSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("  hello world  "),
			want:            true,
		},

		// Combined ValidationRules.
		{
			name:            "case sensitive + ignore whitespace",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("Hello\t\nWorld"),
			want:            true,
		},
		{
			name:            "case sensitive + ignore whitespace - case mismatch",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("hello\t\nworld"),
			want:            false,
		},
		{
			name:            "case insensitive + preserve whitespace",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "case insensitive + preserve whitespace - whitespace mismatch",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("hello  world"),
			want:            false,
		},
		{
			name:            "case insensitive + ignore whitespace",
			expected:        utils.NewValueSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("hello\t\nworld"),
			want:            true,
		},

		// Edge cases and potential false positives.
		{
			name:            "empty strings",
			expected:        utils.NewValueSet(""),
			validationRules: config.ValidationRules{},
			actual:          createMockResult(""),
			want:            true,
		},
		{
			name:            "empty vs whitespace",
			expected:        utils.NewValueSet(""),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("   "),
			want:            true, // whitespace is trimmed by default
		},
		{
			name:            "empty vs whitespace - ignore whitespace",
			expected:        utils.NewValueSet(""),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult(" \t\n "),
			want:            true,
		},
		{
			name:            "empty vs whitespace with newlines - default trim",
			expected:        utils.NewValueSet(""),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)}, // explicit false
			actual:          createMockResult(" \t\n "),
			want:            true, // default trim should remove newlines too
		},
		{
			name:            "substring false positive prevention - longer actual",
			expected:        utils.NewValueSet("test"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("test this is a longer answer"),
			want:            false,
		},
		{
			name:            "substring false positive prevention - longer expected",
			expected:        utils.NewValueSet("test this is a longer answer"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("test"),
			want:            false,
		},
		{
			name:            "partial word match prevention",
			expected:        utils.NewValueSet("cat"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("concatenate"),
			want:            false,
		},
		{
			name:            "similar but different words",
			expected:        utils.NewValueSet("accept"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("except"),
			want:            false,
		},
		{
			name:            "unicode characters",
			expected:        utils.NewValueSet("café"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("café"),
			want:            true,
		},
		{
			name:            "unicode vs ascii false positive",
			expected:        utils.NewValueSet("café"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("cafe"),
			want:            false,
		},
		{
			name:            "number strings",
			expected:        utils.NewValueSet("123"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("123"),
			want:            true,
		},
		{
			name:            "number vs number-like string",
			expected:        utils.NewValueSet("123"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("123.0"),
			want:            false,
		},
		{
			name:            "punctuation edge case",
			expected:        utils.NewValueSet("hello!"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello!"),
			want:            true,
		},
		{
			name:            "punctuation false positive prevention",
			expected:        utils.NewValueSet("hello!"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello"),
			want:            false,
		},
		{
			name:            "mixed line endings in same string",
			expected:        utils.NewValueSet("line1\nline2\r\nline3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("line1\nline2\r\nline3"),
			want:            true,
		},
		{
			name:            "mixed line endings with ignore whitespace",
			expected:        utils.NewValueSet("line1\nline2\r\nline3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1line2line3"),
			want:            true,
		},

		// Multiple lines.
		{
			name:            "multiline exact match",
			expected:        utils.NewValueSet("line1\nline2\nline3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("line1\nline2\nline3"),
			want:            true,
		},
		{
			name:            "multiline with different line endings",
			expected:        utils.NewValueSet("line1\nline2"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("line1\r\nline2"),
			want:            false, // different line endings should not match
		},
		{
			name:            "multiline ignore whitespace",
			expected:        utils.NewValueSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1line2"),
			want:            true,
		},
		{
			name:            "multiline ignore whitespace - different line endings",
			expected:        utils.NewValueSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1\r\nline2"),
			want:            true, // should match when ignoring whitespace
		},

		// TrimLines rule tests (per-line edge trimming; preserves internal spaces; normalizes CRLF to \n).
		{
			name:            "trim-lines - trailing spaces per line",
			expected:        utils.NewValueSet("1. a)\n2. b)\n3. c)"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult("1. a)\n2. b) \n3. c)"),
			want:            true,
		},
		{
			name:            "trim-lines - leading spaces per line",
			expected:        utils.NewValueSet("A\nB\nC"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult(" A\n  B\n   C"),
			want:            true,
		},
		{
			name:            "trim-lines with CRLF",
			expected:        utils.NewValueSet("row1\nrow2\nrow3"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult("row1\r\nrow2 \r\n row3"),
			want:            true,
		},
		{
			name:            "trim-lines does not remove internal spaces",
			expected:        utils.NewValueSet("1) a b"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult("1)  a b "),
			want:            false,
		},
		{
			name:            "trim-lines - preserves internal blank lines",
			expected:        utils.NewValueSet("A\n\nB"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult(" A \r\n \r\n B "),
			want:            true,
		},
		{
			name:            "trim-lines - removes leading and trailing blank lines",
			expected:        utils.NewValueSet("A\nB"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult("\r\n A \n B \n"),
			want:            true,
		},
		{
			name:            "trim-lines + ignore whitespace - trim-lines ignored",
			expected:        utils.NewValueSet("AB"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult(" A \n B "),
			want:            true,
		},
		{
			name:            "trim-lines - tabs inside line are preserved",
			expected:        utils.NewValueSet("a b c"),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult("a\t b \t c"),
			want:            false,
		},
		{
			name:            "trim-lines - whitespace-only becomes empty",
			expected:        utils.NewValueSet(""),
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			actual:          createMockResult(" \r\n "),
			want:            true,
		},

		// Complex combinations with multiple expected values.
		{
			name:            "multiple expected with case sensitivity",
			expected:        utils.NewValueSet("Answer1", "answer2", "ANSWER3"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("answer2"),
			want:            true,
		},
		{
			name:            "multiple expected with case sensitivity - no match",
			expected:        utils.NewValueSet("Answer1", "answer2", "ANSWER3"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("answer1"), // case doesn't match Answer1
			want:            false,
		},
		{
			name:            "multiple expected with whitespace handling",
			expected:        utils.NewValueSet("answer 1", "answer2", "answer 3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("answer1"),
			want:            true, // matches "answer 1" with whitespace removed
		},
		{
			name:            "multiple expected with whitespace handling - no match when preserved",
			expected:        utils.NewValueSet("answer 1", "answer2", "answer 3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("answer1"), // doesn't match any with spaces preserved
			want:            false,
		},

		// Structured objects (non-string) tests - should use JSON matching.
		{
			name:            "structured object - exact match",
			expected:        utils.NewValueSet(map[string]interface{}{"answer": "YES", "confidence": 0.95}),
			validationRules: config.ValidationRules{},
			actual:          createMockResult(map[string]interface{}{"answer": "YES", "confidence": 0.95}),
			want:            true,
		},
		{
			name:            "structured object - field mismatch",
			expected:        utils.NewValueSet(map[string]interface{}{"answer": "YES", "confidence": 0.95}),
			validationRules: config.ValidationRules{},
			actual:          createMockResult(map[string]interface{}{"answer": "NO", "confidence": 0.95}),
			want:            false,
		},
		{
			name:            "structured object - extra field in actual",
			expected:        utils.NewValueSet(map[string]interface{}{"answer": "YES"}),
			validationRules: config.ValidationRules{},
			actual:          createMockResult(map[string]interface{}{"answer": "YES", "confidence": 0.95}),
			want:            false,
		},
		{
			name:            "structured object - missing field in actual",
			expected:        utils.NewValueSet(map[string]interface{}{"answer": "YES", "confidence": 0.95}),
			validationRules: config.ValidationRules{},
			actual:          createMockResult(map[string]interface{}{"answer": "YES"}),
			want:            false,
		},
		{
			name: "structured object - multiple expected objects",
			expected: utils.NewValueSet(
				map[string]interface{}{"answer": "YES", "confidence": 0.95},
				map[string]interface{}{"answer": "NO", "confidence": 0.90},
			),
			validationRules: config.ValidationRules{},
			actual:          createMockResult(map[string]interface{}{"answer": "NO", "confidence": 0.90}),
			want:            true,
		},
		{
			name: "structured object - nested objects",
			expected: utils.NewValueSet(map[string]interface{}{
				"result": map[string]interface{}{"status": "success", "code": 200},
				"data":   []interface{}{"item1", "item2"},
			}),
			validationRules: config.ValidationRules{},
			actual: createMockResult(map[string]interface{}{
				"result": map[string]interface{}{"status": "success", "code": 200},
				"data":   []interface{}{"item1", "item2"},
			}),
			want: true,
		},
		{
			name: "structured object - with validation rules (case sensitivity)",
			expected: utils.NewValueSet(map[string]interface{}{
				"status":  "  SUCCESS  ",
				"message": "  Operation Completed  ",
			}),
			validationRules: config.ValidationRules{
				CaseSensitive:    testutils.Ptr(true),
				IgnoreWhitespace: testutils.Ptr(false),
			},
			actual: createMockResult(map[string]interface{}{
				"status":  "  SUCCESS  ",
				"message": "  Operation Completed  ",
			}),
			want: true,
		},
		{
			name: "structured object - with validation rules (case insensitive should normalize)",
			expected: utils.NewValueSet(map[string]interface{}{
				"status":  "  success  ",
				"message": "  operation completed  ",
			}),
			validationRules: config.ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(false),
			},
			actual: createMockResult(map[string]interface{}{
				"status":  "  SUCCESS  ",
				"message": "  Operation Completed  ",
			}),
			want: true,
		},
		{
			name: "structured object - with validation rules (ignore whitespace)",
			expected: utils.NewValueSet(map[string]interface{}{
				"status":  "success",
				"message": "operationcompleted",
			}),
			validationRules: config.ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(true),
			},
			actual: createMockResult(map[string]interface{}{
				"status":  "  SUCCESS  ",
				"message": "  Operation Completed  ",
			}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValueMatchValidator()
			logger := testutils.NewTestLogger(t)
			result, err := validator.IsCorrect(context.Background(), logger, tt.validationRules, tt.expected, tt.actual, "test prompt", config.NewResponseFormat("test format"))
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.IsCorrect)
		})
	}
}

func TestValidatorToCanonical(t *testing.T) {
	tests := []struct {
		name            string
		value           interface{}
		validationRules config.ValidationRules
		want            interface{}
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

		// TrimLines coverage
		{
			name:            "trim-lines - trims per line leading/trailing",
			value:           "\t a \n\t  b  \n c \t ",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want:            "a\nb\nc",
		},
		{
			name:            "trim-lines - preserves internal spaces",
			value:           "  a  b  ",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want:            "a  b",
		},
		{
			name:            "trim-lines - CRLF normalization and trim",
			value:           " row1 \r\n row2  \r\n  row3 ",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want:            "row1\nrow2\nrow3",
		},
		{
			name:            "trim-lines - leading and trailing blank lines removed by final TrimSpace",
			value:           "\n a \n b \n",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want:            "a\nb",
		},
		{
			name:            "trim-lines + ignore whitespace - ignore dominates",
			value:           " a \n b ",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			want:            "ab",
		},
		{
			name:            "trim-lines + case insensitive default",
			value:           "  A \n B ",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want:            "a\nb",
		},
		{
			name:            "trim-lines + case sensitive",
			value:           "  A \n B ",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true), CaseSensitive: testutils.Ptr(true)},
			want:            "A\nB",
		},
		{
			name:            "trim-lines - whitespace-only becomes empty",
			value:           " \r\n \r\n ",
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want:            "",
		},

		// Structured objects (non-string) tests
		{
			name:            "structured object - map with string normalization",
			value:           map[string]interface{}{"Answer": "  YES  ", "confidence": 0.95},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"Answer": "yes", "confidence": float64(0.95)}, // 0.95 is not a whole number, so stays as float64
		},
		{
			name:            "structured object - case sensitive map",
			value:           map[string]interface{}{"Answer": "  YES  "},
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			want:            map[string]interface{}{"Answer": "YES"},
		},
		{
			name: "structured object - nested map",
			value: map[string]interface{}{
				"Result": map[string]interface{}{"Status": "  SUCCESS  ", "Code": 200},
				"Data":   []interface{}{"  Item1  ", "  Item2  "},
			},
			validationRules: config.ValidationRules{},
			want: map[string]interface{}{
				"Data":   []interface{}{"item1", "item2"},
				"Result": map[string]interface{}{"Code": int64(200), "Status": "success"},
			},
		},
		{
			name:            "structured object - array normalization",
			value:           []interface{}{"  Hello  ", "  World  ", 123},
			validationRules: config.ValidationRules{},
			want:            []interface{}{"hello", "world", int64(123)},
		},
		{
			name:            "structured object - null value",
			value:           map[string]interface{}{"value": nil},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"value": nil},
		},
		{
			name:            "structured object - boolean values",
			value:           map[string]interface{}{"enabled": true, "disabled": false},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"disabled": false, "enabled": true},
		},
		{
			name:            "structured object - float to int conversion",
			value:           map[string]interface{}{"whole": 42.0, "decimal": 42.5},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"decimal": float64(42.5), "whole": int64(42)},
		},

		// Number normalization tests
		{
			name:            "number normalization - int types to int64",
			value:           map[string]interface{}{"int": int(42), "int8": int8(42), "int16": int16(42), "int32": int32(42)},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"int": int64(42), "int16": int64(42), "int32": int64(42), "int8": int64(42)},
		},
		{
			name:            "number normalization - unsigned int types to int64",
			value:           map[string]interface{}{"uint": uint(42), "uint8": uint8(42), "uint16": uint16(42), "uint32": uint32(42)},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"uint": int64(42), "uint16": int64(42), "uint32": int64(42), "uint8": int64(42)},
		},
		{
			name:            "number normalization - large uint to uint64",
			value:           map[string]interface{}{"large_uint": uint(18446744073709551615)}, // max uint64
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"large_uint": uint64(18446744073709551615)},
		},
		{
			name:            "number normalization - uint64 unchanged",
			value:           map[string]interface{}{"uint64_val": uint64(42)},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"uint64_val": uint64(42)},
		},
		{
			name:            "number normalization - float32 to float64",
			value:           map[string]interface{}{"float32_val": float32(42.5)},
			validationRules: config.ValidationRules{},
			want:            map[string]interface{}{"float32_val": float64(42.5)},
		},
		{
			name:            "number normalization - mixed number types",
			value:           []interface{}{int(1), int8(2), uint16(3), float32(4.5), uint64(5), float64(6.7)},
			validationRules: config.ValidationRules{},
			want:            []interface{}{int64(1), int64(2), int64(3), float64(4.5), uint64(5), float64(6.7)},
		},
		{
			name: "structured object - with TrimLines validation rule",
			value: map[string]interface{}{
				"message":     "  Line 1  \n  Line 2  \n  Line 3  ",
				"description": "\n  Multi-line text  \n  with spaces  \n",
				"status":      "  SUCCESS  ",
				"count":       42,
			},
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want: map[string]interface{}{
				"count":       int64(42),
				"description": "multi-line text\nwith spaces",
				"message":     "line 1\nline 2\nline 3",
				"status":      "success",
			},
		},
		{
			name: "structured object - TrimLines with nested objects",
			value: map[string]interface{}{
				"result": map[string]interface{}{
					"status":  "\n  SUCCESS  \n",
					"message": "  Operation\nCompleted  ",
				},
				"data": []interface{}{
					"  Item 3  \n",
					"\n  Item 2  ",
					"  Item 1\nwith newline  ",
				},
			},
			validationRules: config.ValidationRules{TrimLines: testutils.Ptr(true)},
			want: map[string]interface{}{
				"data": []interface{}{
					"item 3",
					"item 2",
					"item 1\nwith newline",
				},
				"result": map[string]interface{}{
					"message": "operation\ncompleted",
					"status":  "success",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValueMatchValidator()
			result := validator.ToCanonical(tt.validationRules, tt.value)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestValidatorFactoryGetValidator(t *testing.T) {
	judgeConfigs := []config.JudgeConfig{
		{
			Name: "test-judge-1",
			Provider: config.ProviderConfig{
				Name: "mock",
				ClientConfig: config.OpenAIClientConfig{
					APIKey: "test-key-1",
				},
				Runs: []config.RunConfig{
					{
						Name:  "default",
						Model: "mock-model-1",
					},
				},
			},
		},
		{
			Name: "test-judge-2",
			Provider: config.ProviderConfig{
				Name: "mock",
				ClientConfig: config.OpenAIClientConfig{
					APIKey: "test-key-2",
				},
				Runs: []config.RunConfig{
					{
						Name:  "default",
						Model: "mock-model-2",
					},
				},
			},
		},
	}
	factory := NewFactory(judgeConfigs)

	// Test default validator (no judge specified).
	rules := config.ValidationRules{}

	validator1, err := factory.GetValidator(context.Background(), rules.Judge)
	require.NoError(t, err)
	require.NotNil(t, validator1)

	// Test caching - should return same instance for same judge config.
	validator2, err := factory.GetValidator(context.Background(), rules.Judge)
	require.NoError(t, err)
	assert.Same(t, validator1, validator2, "Should return cached validator instance")

	// Test different validation rules with same judge config - should return same validator.
	rules2 := config.ValidationRules{}

	validator3, err := factory.GetValidator(context.Background(), rules2.Judge)
	require.NoError(t, err)
	assert.Same(t, validator1, validator3, "Same judge config should return same validator")

	rulesWithJudge1 := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("test-judge-1"),
			Variant: testutils.Ptr("default"),
		},
	}

	rulesWithJudge2 := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("test-judge-2"),
			Variant: testutils.Ptr("default"),
		},
	}

	validator4, err := factory.GetValidator(context.Background(), rulesWithJudge1.Judge)
	require.NoError(t, err)
	require.NotNil(t, validator4)

	validator5, err := factory.GetValidator(context.Background(), rulesWithJudge2.Judge)
	require.NoError(t, err)
	require.NotNil(t, validator5)

	// Different judge configs should create different cached instances.
	assert.NotEqual(t, validator1, validator4, "Judge config should not return value match validator")
	assert.NotEqual(t, validator1, validator5, "Judge config should not return value match validator")
	assert.NotEqual(t, validator4, validator5, "Different judge configs should return different validator instances")

	// Test that caching works for the same judge config.
	validator6, err := factory.GetValidator(context.Background(), rulesWithJudge1.Judge)
	require.NoError(t, err)
	assert.Same(t, validator4, validator6, "Same judge config should return same validator instance from cache")

	// Test judge validator without setting judge providers (should fail).
	rulesWithMissingJudge := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("nonexistent-judge"),
			Variant: testutils.Ptr("default"),
		},
	}

	validator, err := factory.GetValidator(context.Background(), rulesWithMissingJudge.Judge)
	require.Error(t, err)
	require.Nil(t, validator)
	assert.Contains(t, err.Error(), "judge not found: nonexistent-judge")

	// Test judge validator with existing judge name but nonexistent run variant (should fail).
	rulesWithMissingVariant := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("test-judge-1"),
			Variant: testutils.Ptr("nonexistent-variant"),
		},
	}

	validator, err = factory.GetValidator(context.Background(), rulesWithMissingVariant.Judge)
	require.Error(t, err)
	require.Nil(t, validator)
	assert.Contains(t, err.Error(), "run variant not found: nonexistent-variant for judge test-judge-1")
}

func TestFactoryAssertExists(t *testing.T) {
	judgeConfigs := []config.JudgeConfig{
		{
			Name: "test-judge",
			Provider: config.ProviderConfig{
				Name: "mock",
				ClientConfig: config.OpenAIClientConfig{
					APIKey: "test-key",
				},
				Runs: []config.RunConfig{
					{
						Name:  "default",
						Model: "mock-model",
					},
				},
			},
		},
	}
	factory := NewFactory(judgeConfigs)

	// Test existing judge
	existingJudge := config.JudgeSelector{
		Enabled: testutils.Ptr(true),
		Name:    testutils.Ptr("test-judge"),
		Variant: testutils.Ptr("default"),
	}
	err := factory.AssertExists(existingJudge)
	assert.NoError(t, err) //nolint:testifylint

	// Test non-existing judge
	nonExistingJudge := config.JudgeSelector{
		Enabled: testutils.Ptr(true),
		Name:    testutils.Ptr("non-existing"),
		Variant: testutils.Ptr("default"),
	}
	err = factory.AssertExists(nonExistingJudge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "judge not found: non-existing")

	// Test non-existing run variant.
	nonExistingVariant := config.JudgeSelector{
		Enabled: testutils.Ptr(true),
		Name:    testutils.Ptr("test-judge"),
		Variant: testutils.Ptr("non-existing"),
	}
	err = factory.AssertExists(nonExistingVariant)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run variant not found: non-existing for judge test-judge")
}

func TestValidatorGetName(t *testing.T) {
	// Test value match validator.
	valueMatchValidator := NewValueMatchValidator()
	assert.Equal(t, "value match", valueMatchValidator.GetName())

	// Test judge validator.
	judgeConfigs := []config.JudgeConfig{
		{
			Name: "test-judge",
			Provider: config.ProviderConfig{
				Name: "mock",
				ClientConfig: config.OpenAIClientConfig{
					APIKey: "test-key",
				},
				Runs: []config.RunConfig{
					{
						Name:  "test-run",
						Model: "mock-model",
					},
				},
			},
		},
	}
	factory := NewFactory(judgeConfigs)

	rules := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("test-judge"),
			Variant: testutils.Ptr("test-run"),
		},
	}

	judgeValidator, err := factory.GetValidator(context.Background(), rules.Judge)
	require.NoError(t, err)
	assert.Equal(t, "test-run test-judge judge", judgeValidator.GetName())
}

func TestJudgeValidatorToCanonical(t *testing.T) {
	judgeValidator := &judgeValidator{}

	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		// String input tests.
		{
			name:  "trims whitespace",
			input: "  hello world  ",
			want:  "hello world",
		},
		{
			name:  "preserves internal whitespace",
			input: "hello\t\nworld",
			want:  "hello\t\nworld",
		},
		{
			name:  "preserves case",
			input: "Hello World",
			want:  "Hello World",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "trims newlines",
			input: "\nhello world\n",
			want:  "hello world",
		},
		{
			name:  "only whitespace",
			input: "   \r\n\t  ",
			want:  "",
		},

		// Non-string input tests - should be returned as-is.
		{
			name:  "structured object returned as-is",
			input: map[string]interface{}{"answer": "YES", "confidence": 0.95},
			want:  map[string]interface{}{"answer": "YES", "confidence": 0.95},
		},
		{
			name:  "array returned as-is",
			input: []interface{}{"item1", "item2", 123},
			want:  []interface{}{"item1", "item2", 123},
		},
		{
			name:  "boolean returned as-is",
			input: true,
			want:  true,
		},
		{
			name:  "number returned as-is",
			input: 42.5,
			want:  42.5,
		},
		{
			name:  "nil returned as-is",
			input: nil,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Judge validator ignores validation rules for ToCanonical.
			result := judgeValidator.ToCanonical(config.ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(true),
			}, tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestJudgeValidator_NewJudgeValidator_Success(t *testing.T) {
	ctx := context.Background()

	judgeConfig := &config.JudgeConfig{
		Name: "test-judge",
		Provider: config.ProviderConfig{
			Name:         "mock",
			ClientConfig: nil,
			Runs: []config.RunConfig{
				{Name: "test-run"},
			},
		},
	}

	runVariant := config.RunConfig{Name: "test-run"}

	validator, err := NewJudgeValidator(ctx, judgeConfig, runVariant)

	require.NoError(t, err)
	require.NotNil(t, validator)
	assert.Equal(t, "test-run test-judge judge", validator.GetName())
}

func TestValidatorFactoryClose(t *testing.T) {
	// Create factory with judge configs so we can create judge validators.
	judgeConfigs := []config.JudgeConfig{
		{
			Name: "test-judge",
			Provider: config.ProviderConfig{
				Name: "mock",
				ClientConfig: config.OpenAIClientConfig{
					APIKey: "test-key",
				},
				Runs: []config.RunConfig{
					{
						Name:  "default",
						Model: "mock-model",
					},
				},
			},
		},
	}
	factory := NewFactory(judgeConfigs)

	// Create and cache a value match validator (default case).
	defaultRules := config.ValidationRules{}
	valueMatchValidator, err := factory.GetValidator(context.Background(), defaultRules.Judge)
	require.NoError(t, err)
	require.NotNil(t, valueMatchValidator)

	// Create and cache a judge validator.
	judgeRules := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("test-judge"),
			Variant: testutils.Ptr("default"),
		},
	}
	judgeValidator, err := factory.GetValidator(context.Background(), judgeRules.Judge)
	require.NoError(t, err)
	require.NotNil(t, judgeValidator)

	// Verify they are different types.
	assert.NotSame(t, valueMatchValidator, judgeValidator, "Value match and judge validators should be different instances")

	// Test closing the factory - should close judge validators but not affect value match validators.
	err = factory.Close(context.Background())
	assert.NoError(t, err) //nolint:testifylint

	// Test closing completely empty factory.
	anotherEmptyFactory := NewFactory([]config.JudgeConfig{})
	err = anotherEmptyFactory.Close(context.Background())
	assert.NoError(t, err)
}

func TestValidatorFactoryJudgeCacheKey(t *testing.T) {
	factory := NewFactory([]config.JudgeConfig{})

	tests := []struct {
		name     string
		selector config.JudgeSelector
		expected string
	}{
		{
			name: "basic judge selector",
			selector: config.JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("test-judge"),
				Variant: testutils.Ptr("default"),
			},
			expected: "judge_test-judge_default",
		},
		{
			name: "empty name and variant",
			selector: config.JudgeSelector{
				Enabled: testutils.Ptr(true),
			},
			expected: "judge__",
		},
		{
			name: "with special characters",
			selector: config.JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("semantic-judge"),
				Variant: testutils.Ptr("fast-v2"),
			},
			expected: "judge_semantic-judge_fast-v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := factory.createJudgeCacheKey(tt.selector)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidatorFactoryWithDisabledJudgeRun(t *testing.T) {
	judgeConfigs := []config.JudgeConfig{
		{
			Name: "judge-with-disabled-run",
			Provider: config.ProviderConfig{
				Name: "mock",
				ClientConfig: config.OpenAIClientConfig{
					APIKey: "test-key",
				},
				Runs: []config.RunConfig{
					{
						Name:     "disabled-run",
						Model:    "mock-model",
						Disabled: testutils.Ptr(true), // this run is disabled
					},
					{
						Name:  "enabled-run",
						Model: "mock-model",
					},
				},
			},
		},
	}
	factory := NewFactory(judgeConfigs)

	// Test accessing disabled run variant.
	rulesWithDisabledRun := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("judge-with-disabled-run"),
			Variant: testutils.Ptr("disabled-run"),
		},
	}

	// AssertExists should pass (disabled runs are still in lookup).
	err := factory.AssertExists(rulesWithDisabledRun.Judge)
	assert.NoError(t, err) //nolint:testifylint

	// Test accessing enabled run variant.
	rulesWithEnabledRun := config.ValidationRules{
		Judge: config.JudgeSelector{
			Enabled: testutils.Ptr(true),
			Name:    testutils.Ptr("judge-with-disabled-run"),
			Variant: testutils.Ptr("enabled-run"),
		},
	}

	err = factory.AssertExists(rulesWithEnabledRun.Judge)
	assert.NoError(t, err)
}

func TestJudgeValidatorCreateJudgePrompt(t *testing.T) {
	judgeValidator := &judgeValidator{}

	rules := config.ValidationRules{}
	expected := utils.NewStringSet("42", "forty-two")
	actualResponse := "The answer is 42"
	originalPrompt := "What is the answer to the ultimate question?"
	expectedResponseFormat := "A single number or spelled-out number"

	prompt, err := judgeValidator.createJudgePrompt(rules, expected, actualResponse, originalPrompt, expectedResponseFormat)
	require.NoError(t, err)

	expectedPrompt := `You are an automatic grader. Decide if the candidate response is semantically equivalent to ANY ONE of the expected answers.

Definitions
- Semantic equivalence: the candidate conveys the same meaning and required facts as an expected answer; wording may differ.
- Extra content: ignore unless it contradicts or changes the meaning.
- Normalization: apply the flags below BEFORE comparing (case/whitespace).

Inputs
Original task prompt:
What is the answer to the ultimate question?

Original answer format instruction:
A single number or spelled-out number

Expected answer(s) (match any one):
- 42
- forty-two

Candidate response:
The answer is 42

Validation flags:
- Case sensitive: no
- Ignore whitespace: no

Procedure
1. Normalize candidate and each expected answer per the flags.  
2. Compare the candidate to each expected answer independently for semantic equivalence.  
3. Set "correct" to true if ANY match, false otherwise.`

	assert.Equal(t, expectedPrompt, prompt)
}

func TestJudgeValidatorCreateJudgePromptWithValidationRules(t *testing.T) {
	judgeValidator := &judgeValidator{}

	tests := []struct {
		name     string
		rules    config.ValidationRules
		expected []string
	}{
		{
			name:  "default rules",
			rules: config.ValidationRules{},
			expected: []string{
				"Case sensitive: no",
				"Ignore whitespace: no",
			},
		},
		{
			name: "case sensitive enabled",
			rules: config.ValidationRules{
				CaseSensitive: testutils.Ptr(true),
			},
			expected: []string{
				"Case sensitive: yes",
				"Ignore whitespace: no",
			},
		},
		{
			name: "ignore whitespace enabled",
			rules: config.ValidationRules{
				IgnoreWhitespace: testutils.Ptr(true),
			},
			expected: []string{
				"Case sensitive: no",
				"Ignore whitespace: yes",
			},
		},
		{
			name: "both enabled",
			rules: config.ValidationRules{
				CaseSensitive:    testutils.Ptr(true),
				IgnoreWhitespace: testutils.Ptr(true),
			},
			expected: []string{
				"Case sensitive: yes",
				"Ignore whitespace: yes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := utils.NewStringSet("test answer")
			actualResponse := "test response"
			originalPrompt := "test prompt"
			expectedResponseFormat := "test format"

			prompt, err := judgeValidator.createJudgePrompt(tt.rules, expected, actualResponse, originalPrompt, expectedResponseFormat)
			require.NoError(t, err)

			for _, expectedText := range tt.expected {
				assert.Contains(t, prompt, expectedText, "Judge prompt should include validation rules")
			}
		})
	}
}

func TestJudgeValidatorIsCorrect(t *testing.T) {
	tests := []struct {
		name                       string
		judgeConfigName            string
		originalTaskExpectedResult string
		originalTaskActualResponse string
		originalPrompt             string
		retryPolicy                *config.RetryPolicy
		expectError                bool
		expectCorrect              bool
		expectedJudgeResp          string
	}{
		{
			name:                       "judge success",
			judgeConfigName:            "judge_evaluation",
			originalTaskExpectedResult: "correct answer",
			originalTaskActualResponse: "correct answer",
			expectError:                false,
			expectCorrect:              true,
			expectedJudgeResp:          "mock success",
		},
		{
			name:                       "judge failure",
			judgeConfigName:            "judge_evaluation",
			originalTaskExpectedResult: "correct answer",
			originalTaskActualResponse: "wrong answer",
			expectError:                false,
			expectCorrect:              false,
			expectedJudgeResp:          "mock success",
		},
		{
			name:                       "judge error",
			judgeConfigName:            "judge_evaluation",
			originalTaskExpectedResult: "correct answer",
			originalTaskActualResponse: "error",
			expectError:                true,
			expectCorrect:              false,
		},
		{
			name:                       "judge success with retry",
			judgeConfigName:            "judge_evaluation",
			originalTaskExpectedResult: "correct answer",
			originalTaskActualResponse: "retry_1: correct answer",
			retryPolicy: &config.RetryPolicy{
				MaxRetryAttempts:    1,
				InitialDelaySeconds: 1,
			},
			expectError:       false,
			expectCorrect:     true,
			expectedJudgeResp: "after 2 attempts",
		},
		{
			name:                       "judge failure with retry",
			judgeConfigName:            "judge_evaluation",
			originalTaskExpectedResult: "correct answer",
			originalTaskActualResponse: "retry_1: wrong answer",
			retryPolicy: &config.RetryPolicy{
				MaxRetryAttempts:    1,
				InitialDelaySeconds: 1,
			},
			expectError:       false,
			expectCorrect:     false,
			expectedJudgeResp: "after 2 attempts",
		},
		{
			name:                       "judge error with retry",
			judgeConfigName:            "judge_evaluation",
			originalTaskExpectedResult: "correct answer",
			originalTaskActualResponse: "retry_1: error",
			retryPolicy: &config.RetryPolicy{
				MaxRetryAttempts:    1,
				InitialDelaySeconds: 1,
			},
			expectError:   true,
			expectCorrect: false,
		},
		{
			name:                       "judge error too many retries",
			judgeConfigName:            "judge_evaluation",
			originalTaskExpectedResult: "correct answer",
			originalTaskActualResponse: "retry_5",
			retryPolicy: &config.RetryPolicy{
				MaxRetryAttempts:    1,
				InitialDelaySeconds: 1,
			},
			expectError:   true,
			expectCorrect: false,
		},
	}

	// Test non-string actual responses (should fail validation).
	nonStringTests := []struct {
		name           string
		actualResponse interface{}
		expectedResult string
		expectedError  bool
	}{
		{
			name:           "structured object response - should fail",
			actualResponse: map[string]interface{}{"answer": "YES", "confidence": 0.95},
			expectedResult: "correct answer",
			expectedError:  false,
		},
		{
			name:           "array response - should fail",
			actualResponse: []interface{}{"answer1", "answer2"},
			expectedResult: "correct answer",
			expectedError:  false,
		},
		{
			name:           "boolean response - should fail",
			actualResponse: true,
			expectedResult: "correct answer",
			expectedError:  false,
		},
		{
			name:           "number response - should fail",
			actualResponse: 42,
			expectedResult: "correct answer",
			expectedError:  false,
		},
	}

	// Test non-plain text expected values (should return error).
	nonPlainTextExpectedTests := []struct {
		name              string
		expectedValues    utils.ValueSet
		actualResponse    string
		shouldReturnError bool
	}{
		{
			name:              "structured object expected - should error",
			expectedValues:    utils.NewValueSet(map[string]interface{}{"answer": "YES"}),
			actualResponse:    "YES",
			shouldReturnError: true,
		},
		{
			name:              "mixed string and object expected - should error",
			expectedValues:    utils.NewValueSet("YES", map[string]interface{}{"answer": "YES"}),
			actualResponse:    "YES",
			shouldReturnError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a judge validator with a mock run variant config.
			judgeRunVariant := config.RunConfig{
				Name:        tt.judgeConfigName,
				Model:       "judge-model",
				RetryPolicy: tt.retryPolicy,
			}

			judgeConfig := &config.JudgeConfig{
				Name: "mock-judge",
				Provider: config.ProviderConfig{
					Name: "mock-judge",
				},
			}

			validator, err := NewJudgeValidator(context.Background(), judgeConfig, judgeRunVariant)
			require.NoError(t, err)

			// Create original task expected result set.
			expectedTaskValues := utils.NewValueSet(tt.originalTaskExpectedResult)

			// Create original task result.
			actualTaskResult := providers.Result{
				Title:       "Original Task Result",
				Explanation: "Original task explanation",
				FinalAnswer: providers.Answer{Content: tt.originalTaskActualResponse},
			}

			logger := testutils.NewTestLogger(t)
			result, err := validator.IsCorrect(context.Background(), logger, config.ValidationRules{}, expectedTaskValues, actualTaskResult, tt.originalPrompt, config.NewResponseFormat("json"))

			if tt.expectError {
				require.Error(t, err)
				assert.False(t, result.IsCorrect, "Expected result to be incorrect when error is expected")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectCorrect, result.IsCorrect)
		})
	}

	// Test non-string actual responses.
	for _, tt := range nonStringTests {
		t.Run(tt.name, func(t *testing.T) {
			judgeRunVariant := config.RunConfig{
				Name:  "judge_evaluation",
				Model: "judge-model",
			}

			judgeConfig := &config.JudgeConfig{
				Name: "mock-judge",
				Provider: config.ProviderConfig{
					Name: "mock-judge",
				},
			}

			validator, err := NewJudgeValidator(context.Background(), judgeConfig, judgeRunVariant)
			require.NoError(t, err)

			expectedTaskValues := utils.NewValueSet(tt.expectedResult)
			actualTaskResult := providers.Result{
				Title:       "Original Task Result",
				Explanation: "Original task explanation",
				FinalAnswer: providers.Answer{Content: tt.actualResponse},
			}

			logger := testutils.NewTestLogger(t)
			result, err := validator.IsCorrect(context.Background(), logger, config.ValidationRules{}, expectedTaskValues, actualTaskResult, "", config.NewResponseFormat("json"))

			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.False(t, result.IsCorrect, "Non-string responses should fail validation")
				assert.Equal(t, "Invalid Response Type", result.Title)
				assert.Contains(t, result.Explanation, "Semantic validation requires plain text responses")
			}
		})
	}

	// Test non-plain text expected values.
	for _, tt := range nonPlainTextExpectedTests {
		t.Run(tt.name, func(t *testing.T) {
			judgeRunVariant := config.RunConfig{
				Name:  "judge_evaluation",
				Model: "judge-model",
			}

			judgeConfig := &config.JudgeConfig{
				Name: "mock-judge",
				Provider: config.ProviderConfig{
					Name: "mock-judge",
				},
			}

			validator, err := NewJudgeValidator(context.Background(), judgeConfig, judgeRunVariant)
			require.NoError(t, err)

			actualTaskResult := providers.Result{
				Title:       "Original Task Result",
				Explanation: "Original task explanation",
				FinalAnswer: providers.Answer{Content: tt.actualResponse},
			}

			logger := testutils.NewTestLogger(t)
			result, err := validator.IsCorrect(context.Background(), logger, config.ValidationRules{}, tt.expectedValues, actualTaskResult, "", config.NewResponseFormat("json"))

			if tt.shouldReturnError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "semantic validation requires plain text responses")
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
