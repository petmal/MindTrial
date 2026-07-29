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

func TestAnthropic_Run_IncompatibleThinking(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &Anthropic{}

	runCfg := config.RunConfig{
		Name:  "test-run",
		Model: "claude",
		ModelParams: config.AnthropicModelParams{
			Thinking:             testutils.Ptr("disabled"), // incompatible with ThinkingBudgetTokens
			ThinkingBudgetTokens: testutils.Ptr(int64(1024)),
		},
	}
	task := config.Task{Name: "t"}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrInvalidModelParams) // Should error due to ErrInvalidModelParams
}

func TestAnthropic_ConfigurePromptCaching(t *testing.T) {
	t.Run("places breakpoint on last local tool", func(t *testing.T) {
		p := &Anthropic{}
		req := anthropic.MessageNewParams{Model: "claude-opus-5", MaxTokens: 2048}
		req.Tools = append(req.Tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{Name: "t1", InputSchema: anthropic.ToolInputSchemaParam{}},
		})
		req.Tools = append(req.Tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{Name: "t2", InputSchema: anthropic.ToolInputSchemaParam{}},
		})

		p.configurePromptCaching(&req, len(req.Tools)-1, testutils.Ptr("5m"))

		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL5m, req.CacheControl.TTL)
		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL5m, req.Tools[1].OfTool.CacheControl.TTL)
		assert.Zero(t, req.Tools[0].OfTool.CacheControl)
	})

	t.Run("falls back to last system block", func(t *testing.T) {
		p := &Anthropic{}
		req := anthropic.MessageNewParams{Model: "claude-opus-5", MaxTokens: 2048}
		req.System = append(req.System, anthropic.TextBlockParam{Text: "first"})
		req.System = append(req.System, anthropic.TextBlockParam{Text: "last"})

		p.configurePromptCaching(&req, -1, testutils.Ptr("1h"))

		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, req.CacheControl.TTL)
		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, req.System[len(req.System)-1].CacheControl.TTL)
		assert.Zero(t, req.System[0].CacheControl)
	})

	t.Run("falls back to last user text block", func(t *testing.T) {
		p := &Anthropic{}
		req := anthropic.MessageNewParams{Model: "claude-opus-5", MaxTokens: 2048}
		req.Messages = append(req.Messages, anthropic.NewUserMessage(
			anthropic.NewTextBlock("first"),
			anthropic.NewTextBlock("last"),
		))

		p.configurePromptCaching(&req, -1, testutils.Ptr("5m"))

		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL5m, req.CacheControl.TTL)

		content := req.Messages[0].Content
		require.Len(t, content, 2)

		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL5m, content[1].OfText.CacheControl.TTL)
		assert.Zero(t, content[0].OfText.CacheControl)
	})

	t.Run("falls back to last user image block", func(t *testing.T) {
		p := &Anthropic{}
		req := anthropic.MessageNewParams{Model: "claude-opus-5", MaxTokens: 2048}
		req.Messages = append(req.Messages, anthropic.NewUserMessage(
			anthropic.NewTextBlock("describe"),
			anthropic.NewImageBlockBase64("image/png", "abc"),
		))

		p.configurePromptCaching(&req, -1, testutils.Ptr("1h"))

		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, req.CacheControl.TTL)

		content := req.Messages[0].Content
		require.Len(t, content, 2)

		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, content[1].OfImage.CacheControl.TTL)
		assert.Zero(t, content[0].OfText.CacheControl)
	})

	t.Run("always sets the top-level cache control", func(t *testing.T) {
		p := &Anthropic{}
		req := anthropic.MessageNewParams{Model: "claude-opus-5", MaxTokens: 2048}

		p.configurePromptCaching(&req, -1, testutils.Ptr("5m"))
		assert.Equal(t, anthropic.CacheControlEphemeralTTLTTL5m, req.CacheControl.TTL)
	})

	t.Run("nil does not enable cache control", func(t *testing.T) {
		p := &Anthropic{}
		req := anthropic.MessageNewParams{Model: "claude-opus-5", MaxTokens: 2048}
		req.Tools = append(req.Tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{Name: "t1", InputSchema: anthropic.ToolInputSchemaParam{}},
		})

		p.configurePromptCaching(&req, len(req.Tools)-1, nil)

		assert.Zero(t, req.CacheControl)
		assert.Zero(t, req.Tools[0].OfTool.CacheControl)
	})

	t.Run("unknown values fall back to SDK default", func(t *testing.T) {
		p := &Anthropic{}
		req := anthropic.MessageNewParams{Model: "claude-opus-5", MaxTokens: 2048}

		p.configurePromptCaching(&req, -1, testutils.Ptr("unknown"))

		assert.NotZero(t, req.CacheControl)
		assert.Zero(t, req.CacheControl.TTL)
	})
}
