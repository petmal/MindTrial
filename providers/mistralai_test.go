// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/mistralai"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/require"
)

func TestMistralApplyReasoningEffort(t *testing.T) {
	request := mistralai.NewChatCompletionRequestWithDefaults()
	provider := &MistralAI{}
	require.NoError(t, provider.applyModelParameters(request, config.MistralAIModelParams{ReasoningEffort: testutils.Ptr("high")}))
	require.Equal(t, mistralai.REASONINGEFFORT_HIGH, request.GetReasoningEffort())
}

func TestMistralUsageCachedTokens(t *testing.T) {
	var usage mistralai.UsageInfo
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":75}}`), &usage))
	details, ok := usage.GetPromptTokensDetailsOk()
	require.True(t, ok)
	require.Equal(t, int32(75), details.GetCachedTokens())
}

func TestMistralGetMessageTextExcludesThinking(t *testing.T) {
	// The reasoning content itself is irrelevant here: getMessageTextChunks only
	// checks the ContentChunk union discriminator (chunk.TextChunk != nil), so an
	// empty ThinkingInner is sufficient to exercise the exclusion.
	thinking := mistralai.ThinkChunkAsContentChunk(mistralai.NewThinkChunk([]mistralai.ThinkingInner{{}}))
	first := mistralai.TextChunkAsContentChunk(mistralai.NewTextChunk("first "))
	second := mistralai.TextChunkAsContentChunk(mistralai.NewTextChunk("second"))
	chunks := []mistralai.ContentChunk{thinking, first, second}
	content := mistralai.Content{ArrayOfContentChunk: &chunks}
	message := mistralai.NewAssistantMessage()
	message.Content = *mistralai.NewNullableContent(&content)

	text, ok := (&MistralAI{}).getMessageText(message)
	require.True(t, ok)
	require.Equal(t, "first second", text)
}

func TestMistral_FileUploadNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MistralAI{} // nil client is sufficient to exercise early check

	runCfg := config.RunConfig{Name: "test-run", Model: "mistral-embed"} // non-vision model
	task := config.Task{
		Name:  "with_file",
		Files: []config.TaskFile{mockTaskFile(t, "img.png", "file://img.png", "image/png")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileUploadNotSupported)
}

func TestMistral_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MistralAI{} // nil client is sufficient to exercise early validation

	// Use a vision-capable model prefix to bypass the isFileUploadSupported() check
	runCfg := config.RunConfig{Name: "test-run", Model: "mistral-large-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}
