// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropic_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &Anthropic{} // nil client is sufficient since error occurs before any API call

	runCfg := config.RunConfig{Name: "test-run", Model: "claude"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestSanitizeAssistantMessage(t *testing.T) {
	tests := []struct {
		name        string
		content     []anthropic.ContentBlockParamUnion
		wantLen     int
		wantTexts   []string // expected Text values of remaining OfText blocks, in order
		wantNonText int      // expected count of non-text blocks preserved
	}{
		{
			name: "no text blocks preserved unchanged",
			content: []anthropic.ContentBlockParamUnion{
				anthropic.NewThinkingBlock("sig", "deep thought"),
			},
			wantLen:     1,
			wantNonText: 1,
		},
		{
			name: "non-empty text blocks preserved",
			content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("hello"),
				anthropic.NewTextBlock("world"),
			},
			wantLen:   2,
			wantTexts: []string{"hello", "world"},
		},
		{
			name: "empty text blocks removed",
			content: []anthropic.ContentBlockParamUnion{
				anthropic.NewThinkingBlock("sig", "thinking"),
				anthropic.NewTextBlock(""),
				anthropic.NewTextBlock("answer"),
				anthropic.NewTextBlock(""),
			},
			wantLen:     2,
			wantTexts:   []string{"answer"},
			wantNonText: 1,
		},
		{
			name: "all empty text blocks removed",
			content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock(""),
				anthropic.NewTextBlock(""),
			},
			wantLen: 0,
		},
		{
			name: "mixed block types with empty text",
			content: []anthropic.ContentBlockParamUnion{
				anthropic.NewThinkingBlock("sig1", "thought1"),
				anthropic.NewTextBlock(""),
				anthropic.NewToolUseBlock("id1", map[string]string{"key": "val"}, "tool1"),
				anthropic.NewTextBlock("result"),
			},
			wantLen:     3,
			wantTexts:   []string{"result"},
			wantNonText: 2,
		},
		{
			name:    "empty content unchanged",
			content: nil,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: tt.content,
			}
			got := sanitizeAssistantMessage(msg)

			require.Len(t, got.Content, tt.wantLen)

			var texts []string
			var nonTextCount int
			for _, block := range got.Content {
				if block.OfText != nil {
					texts = append(texts, block.OfText.Text)
				} else {
					nonTextCount++
				}
			}
			assert.Equal(t, tt.wantTexts, texts)
			assert.Equal(t, tt.wantNonText, nonTextCount)
		})
	}
}
