// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"errors"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
