// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"testing"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMockResult creates a providers.Result for testing.
func createMockResult(response string) providers.Result {
	return providers.Result{
		Title:       "Mock Title",
		Explanation: "Mock Explanation",
		FinalAnswer: response,
	}
}

func TestValidatorIsCorrect(t *testing.T) {
	tests := []struct {
		name            string
		expected        utils.StringSet
		validationRules config.ValidationRules
		actual          providers.Result
		want            bool
	}{
		// Basic correct and incorrect answers.
		{
			name:            "exact match - correct",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "exact match - incorrect",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("goodbye world"),
			want:            false,
		},

		// Multiple expected values - StringSet scenarios.
		{
			name:            "multiple expected - first match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer1"),
			want:            true,
		},
		{
			name:            "multiple expected - second match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer2"),
			want:            true,
		},
		{
			name:            "multiple expected - third match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer3"),
			want:            true,
		},
		{
			name:            "multiple expected - no match",
			expected:        utils.NewStringSet("answer1", "answer2", "answer3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("answer4"),
			want:            false,
		},

		// Default ValidationRules values (all nil - should be false by default).
		{
			name:            "default rules - case insensitive by default",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "default rules - whitespace trimmed by default",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("  hello world  "),
			want:            true,
		},
		{
			name:            "default rules - internal whitespace preserved by default",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello  world"), // extra space should fail
			want:            false,
		},
		{
			name:            "default rules - tabs/newlines inside text preserved",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello\tworld"), // tab should fail
			want:            false,
		},

		// CaseSensitive testing.
		{
			name:            "case sensitive - exact match",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("Hello World"),
			want:            true,
		},
		{
			name:            "case sensitive - case mismatch",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("hello world"),
			want:            false,
		},
		{
			name:            "case insensitive - case mismatch should pass",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false)},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "case insensitive - mixed case should pass",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false)},
			actual:          createMockResult("HeLLo WoRLd"),
			want:            true,
		},
		{
			name:            "case sensitive - mixed case should fail",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("HeLLo WoRLd"), // same input as above but with case sensitivity
			want:            false,
		},

		// IgnoreWhitespace testing.
		{
			name:            "ignore whitespace - spaces removed",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("helloworld"),
			want:            true,
		},
		{
			name:            "ignore whitespace - tabs and newlines removed",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("hello\t\nworld"),
			want:            true,
		},
		{
			name:            "preserve whitespace - tabs and newlines should fail",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("hello\t\nworld"), // same input but whitespace preserved
			want:            false,
		},
		{
			name:            "ignore whitespace - all whitespace removed",
			expected:        utils.NewStringSet("hello world test"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("  hello\t\n world   test  "),
			want:            true,
		},
		{
			name:            "ignore whitespace - newlines specifically",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1line2"),
			want:            true,
		},
		{
			name:            "preserve whitespace - spaces matter",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("helloworld"),
			want:            false,
		},
		{
			name:            "preserve whitespace - trimmed only",
			expected:        utils.NewStringSet("hello world"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("  hello world  "),
			want:            true,
		},

		// Combined ValidationRules.
		{
			name:            "case sensitive + ignore whitespace",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("Hello\t\nWorld"),
			want:            true,
		},
		{
			name:            "case sensitive + ignore whitespace - case mismatch",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("hello\t\nworld"),
			want:            false,
		},
		{
			name:            "case insensitive + preserve whitespace",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("hello world"),
			want:            true,
		},
		{
			name:            "case insensitive + preserve whitespace - whitespace mismatch",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("hello  world"),
			want:            false,
		},
		{
			name:            "case insensitive + ignore whitespace",
			expected:        utils.NewStringSet("Hello World"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("hello\t\nworld"),
			want:            true,
		},

		// Edge cases and potential false positives.
		{
			name:            "empty strings",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{},
			actual:          createMockResult(""),
			want:            true,
		},
		{
			name:            "empty vs whitespace",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("   "),
			want:            true, // whitespace is trimmed by default
		},
		{
			name:            "empty vs whitespace - ignore whitespace",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult(" \t\n "),
			want:            true,
		},
		{
			name:            "empty vs whitespace with newlines - default trim",
			expected:        utils.NewStringSet(""),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)}, // explicit false
			actual:          createMockResult(" \t\n "),
			want:            true, // default trim should remove newlines too
		},
		{
			name:            "substring false positive prevention - longer actual",
			expected:        utils.NewStringSet("test"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("test this is a longer answer"),
			want:            false,
		},
		{
			name:            "substring false positive prevention - longer expected",
			expected:        utils.NewStringSet("test this is a longer answer"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("test"),
			want:            false,
		},
		{
			name:            "partial word match prevention",
			expected:        utils.NewStringSet("cat"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("concatenate"),
			want:            false,
		},
		{
			name:            "similar but different words",
			expected:        utils.NewStringSet("accept"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("except"),
			want:            false,
		},
		{
			name:            "unicode characters",
			expected:        utils.NewStringSet("café"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("café"),
			want:            true,
		},
		{
			name:            "unicode vs ascii false positive",
			expected:        utils.NewStringSet("café"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("cafe"),
			want:            false,
		},
		{
			name:            "number strings",
			expected:        utils.NewStringSet("123"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("123"),
			want:            true,
		},
		{
			name:            "number vs number-like string",
			expected:        utils.NewStringSet("123"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("123.0"),
			want:            false,
		},
		{
			name:            "punctuation edge case",
			expected:        utils.NewStringSet("hello!"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello!"),
			want:            true,
		},
		{
			name:            "punctuation false positive prevention",
			expected:        utils.NewStringSet("hello!"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("hello"),
			want:            false,
		},
		{
			name:            "mixed line endings in same string",
			expected:        utils.NewStringSet("line1\nline2\r\nline3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("line1\nline2\r\nline3"),
			want:            true,
		},
		{
			name:            "mixed line endings with ignore whitespace",
			expected:        utils.NewStringSet("line1\nline2\r\nline3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1line2line3"),
			want:            true,
		},

		// Multiple lines.
		{
			name:            "multiline exact match",
			expected:        utils.NewStringSet("line1\nline2\nline3"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("line1\nline2\nline3"),
			want:            true,
		},
		{
			name:            "multiline with different line endings",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{},
			actual:          createMockResult("line1\r\nline2"),
			want:            false, // different line endings should not match
		},
		{
			name:            "multiline ignore whitespace",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1line2"),
			want:            true,
		},
		{
			name:            "multiline ignore whitespace - different line endings",
			expected:        utils.NewStringSet("line1\nline2"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("line1\r\nline2"),
			want:            true, // should match when ignoring whitespace
		},

		// Complex combinations with multiple expected values.
		{
			name:            "multiple expected with case sensitivity",
			expected:        utils.NewStringSet("Answer1", "answer2", "ANSWER3"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("answer2"),
			want:            true,
		},
		{
			name:            "multiple expected with case sensitivity - no match",
			expected:        utils.NewStringSet("Answer1", "answer2", "ANSWER3"),
			validationRules: config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			actual:          createMockResult("answer1"), // case doesn't match Answer1
			want:            false,
		},
		{
			name:            "multiple expected with whitespace handling",
			expected:        utils.NewStringSet("answer 1", "answer2", "answer 3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			actual:          createMockResult("answer1"),
			want:            true, // matches "answer 1" with whitespace removed
		},
		{
			name:            "multiple expected with whitespace handling - no match when preserved",
			expected:        utils.NewStringSet("answer 1", "answer2", "answer 3"),
			validationRules: config.ValidationRules{IgnoreWhitespace: testutils.Ptr(false)},
			actual:          createMockResult("answer1"), // doesn't match any with spaces preserved
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValueMatchValidator()
			result, err := validator.IsCorrect(context.Background(), tt.validationRules, tt.expected, tt.actual, "test prompt", "test format")
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.IsCorrect)
			// Value match validator should not have an assessment result.
			assert.Nil(t, result.GetAssessmentResult())
		})
	}
}

func TestValidationResultGetAssessmentResult(t *testing.T) {
	result := ValidationResult{
		IsCorrect:   true,
		Title:       "Test",
		Explanation: "Test explanation",
	}
	assert.Nil(t, result.GetAssessmentResult())

	assessmentResult := providers.Result{
		Title:       "Assessment Title",
		Explanation: "Assessment Explanation",
		FinalAnswer: "Assessment Answer",
	}
	resultWithAssessment := ValidationResult{
		IsCorrect:   true,
		Title:       "Test",
		Explanation: "Test explanation",
		assessment:  &assessmentResult,
	}
	require.NotNil(t, resultWithAssessment.GetAssessmentResult())
	assert.Equal(t, &assessmentResult, resultWithAssessment.GetAssessmentResult())
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
			validator := NewValueMatchValidator()
			assert.Equal(t, tt.want, validator.ToCanonical(tt.validationRules, tt.value))
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
	assert.Equal(t, "mock (test-run) judge", judgeValidator.GetName())
}

func TestJudgeValidatorToCanonical(t *testing.T) {
	judgeValidator := &judgeValidator{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
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
3. If ANY match, the response is correct; else incorrect.`

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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock judge provider.
			mockJudgeProvider, err := providers.NewProvider(context.Background(), config.ProviderConfig{
				Name: "mock-judge",
			})
			require.NoError(t, err)
			defer mockJudgeProvider.Close(context.Background())

			// Create a judge validator with a mock run variant config.
			judgeRunVariant := config.RunConfig{
				Name:        tt.judgeConfigName,
				Model:       "judge-model",
				RetryPolicy: tt.retryPolicy,
			}

			validator := NewJudgeValidator(mockJudgeProvider, judgeRunVariant)

			// Create original task expectedTaskValues result set.
			expectedTaskValues := utils.NewStringSet(tt.originalTaskExpectedResult)

			// Create original task result.
			actualTaskResult := providers.Result{
				Title:       "Original Task Result",
				Explanation: "Original task explanation",
				FinalAnswer: tt.originalTaskActualResponse,
			}

			result, err := validator.IsCorrect(context.Background(), config.ValidationRules{}, expectedTaskValues, actualTaskResult, tt.originalPrompt, "json")

			if tt.expectError {
				require.Error(t, err)
				assert.False(t, result.IsCorrect, "Expected result to be incorrect when error is expected")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectCorrect, result.IsCorrect)

			// Verify that assessment result is populated.
			assessmentResult := result.GetAssessmentResult()
			require.NotNil(t, assessmentResult, "Assessment result should not be nil for judge validator")

			// Verify assessment result contains expected mock data.
			assert.Contains(t, assessmentResult.Explanation, tt.expectedJudgeResp)
			assert.NotEmpty(t, assessmentResult.GetPrompts())
			assert.Equal(t, 7211609999927884*time.Nanosecond, assessmentResult.GetDuration())

			// Verify token usage is populated.
			usage := assessmentResult.GetUsage()
			assert.NotNil(t, usage.InputTokens)
			assert.Equal(t, int64(8200209999917998), *usage.InputTokens)
		})
	}
}
