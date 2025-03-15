package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatorIsCorrect(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   Result
		want     bool
	}{
		{
			name:     "correct answer",
			expected: "correct answer",
			actual:   Result{FinalAnswer: "correct answer"},
			want:     true,
		},
		{
			name:     "incorrect answer",
			expected: "correct answer",
			actual:   Result{FinalAnswer: "wrong answer"},
			want:     false,
		},
		{
			name:     "case insensitive",
			expected: "Correct Answer",
			actual:   Result{FinalAnswer: "correct answer"},
			want:     true,
		},
		{
			name:     "trim spaces",
			expected: "correct answer",
			actual:   Result{FinalAnswer: " correct answer "},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NewDefaultValidator(tt.expected).IsCorrect(context.Background(), tt.actual))
		})
	}
}

func TestValidatorToCanonical(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "lowercase",
			value: "Correct Answer",
			want:  "correct answer",
		},
		{
			name:  "trim spaces",
			value: " correct answer ",
			want:  "correct answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NewDefaultValidator("").ToCanonical(tt.value))
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
