// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	deepseek "github.com/cohesion-org/deepseek-go"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepseek_NativeFileInputSupported(t *testing.T) {
	p := &Deepseek{}
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{
			name:     "flash allowed",
			model:    "deepseek-flash",
			expected: true,
		},
		{
			name:     "pro denied",
			model:    "deepseek-v4-pro",
			expected: false,
		},
		{
			name:     "unknown future model allowed by default",
			model:    "deepseek-future-model",
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, p.isNativeFileInputSupported(tt.model))
		})
	}
}

func TestDeepseek_CreatePromptMessageParts_SingleImage(t *testing.T) {
	ctx := context.Background()
	path := testutils.CreateMockFile(t, "img-*.png", []byte("fake-png-bytes"))
	file := mockTaskFileWithAccess(t, "img.png", path, "image/png", []config.FileAccess{config.FileAccessNative})

	p := &Deepseek{}
	result := &Result{}
	parts, err := p.createPromptMessageParts(ctx, "the prompt", []config.TaskFile{file}, result)
	require.NoError(t, err)
	require.Len(t, parts, 3)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "[file: img.png]", parts[0].Text)
	require.Equal(t, "image_url", parts[1].Type)
	require.NotNil(t, parts[1].Image)
	imageURL, ok := parts[1].Image.URL.(string)
	require.True(t, ok, "image URL must be a string data URL")
	assert.True(t, strings.HasPrefix(imageURL, "data:image/png;base64,"))
	assert.Equal(t, "text", parts[2].Type)
	assert.Equal(t, "the prompt", parts[2].Text)
}

func TestDeepseek_ImageRequestJSONSerialization(t *testing.T) {
	ctx := context.Background()
	path := testutils.CreateMockFile(t, "img-*.png", []byte("fake-png-bytes"))
	file := mockTaskFileWithAccess(t, "img.png", path, "image/png", []config.FileAccess{config.FileAccessNative})

	p := &Deepseek{}
	parts, err := p.createPromptMessageParts(ctx, "the prompt", []config.TaskFile{file}, &Result{})
	require.NoError(t, err)

	req := &deepseek.ChatCompletionRequestWithImage{
		Model: "deepseek-flash",
		Messages: []deepseek.ChatCompletionMessageWithImage{
			{
				Role:    deepseek.ChatMessageRoleUser,
				Content: parts,
			},
		},
	}
	raw, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	messages, ok := decoded["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 1)
	message, ok := messages[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user", message["role"])
	content, ok := message["content"].([]any)
	require.True(t, ok, "user message content must serialize as an array of content blocks")
	require.Len(t, content, 3)
	imageBlock, ok := content[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image_url", imageBlock["type"])
	imageURL, ok := imageBlock["image_url"].(map[string]any)
	require.True(t, ok, "image block must serialize the nested image_url object")
	url, ok := imageURL["url"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(url, "data:image/png;base64,"), "unexpected image URL %q", url)
}

func TestDeepseek_SupportedImageMimeTypes(t *testing.T) {
	ctx := context.Background()
	mimeTypes := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
	}
	for _, mimeType := range mimeTypes {
		t.Run(mimeType, func(t *testing.T) {
			path := testutils.CreateMockFile(t, "img-*.bin", []byte("fake-image-bytes"))
			file := mockTaskFileWithAccess(t, "img", path, mimeType, []config.FileAccess{config.FileAccessNative})

			p := &Deepseek{}
			parts, err := p.createPromptMessageParts(ctx, "the prompt", []config.TaskFile{file}, &Result{})
			require.NoError(t, err)
			require.Len(t, parts, 3)
			assert.Equal(t, "image_url", parts[1].Type)
			require.NotNil(t, parts[1].Image)
		})
	}
}

func TestDeepseek_UnsupportedNativeFileType(t *testing.T) {
	ctx := context.Background()
	for _, mimeType := range []string{"application/pdf", "text/plain"} {
		t.Run(mimeType, func(t *testing.T) {
			path := testutils.CreateMockFile(t, "doc-*.bin", []byte("fake-bytes"))
			file := mockTaskFileWithAccess(t, "doc", path, mimeType, []config.FileAccess{config.FileAccessNative})

			p := &Deepseek{}
			_, err := p.createPromptMessageParts(ctx, "the prompt", []config.TaskFile{file}, &Result{})
			require.ErrorIs(t, err, ErrFileNotSupported)
		})
	}
}

func TestDeepseek_CreatePromptMessageParts_MultipleImagesOrdering(t *testing.T) {
	ctx := context.Background()
	path1 := testutils.CreateMockFile(t, "img1-*.png", []byte("fake-png-1"))
	path2 := testutils.CreateMockFile(t, "img2-*.png", []byte("fake-png-2"))
	file1 := mockTaskFileWithAccess(t, "img1.png", path1, "image/png", []config.FileAccess{config.FileAccessNative})
	file2 := mockTaskFileWithAccess(t, "img2.png", path2, "image/png", []config.FileAccess{config.FileAccessNative})

	p := &Deepseek{}
	parts, err := p.createPromptMessageParts(ctx, "the prompt", []config.TaskFile{file1, file2}, &Result{})
	require.NoError(t, err)
	require.Len(t, parts, 5)
	assert.Equal(t, "[file: img1.png]", parts[0].Text)
	assert.Equal(t, "image_url", parts[1].Type)
	assert.Equal(t, "[file: img2.png]", parts[2].Text)
	assert.Equal(t, "image_url", parts[3].Type)
	assert.Equal(t, "the prompt", parts[4].Text)
	require.NotNil(t, parts[1].Image)
	require.NotNil(t, parts[3].Image)
	url1, ok := parts[1].Image.URL.(string)
	require.True(t, ok, "first image URL must be a string data URL")
	url2, ok := parts[3].Image.URL.(string)
	require.True(t, ok, "second image URL must be a string data URL")
	assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("fake-png-1")), url1)
	assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("fake-png-2")), url2)
}

func TestDeepseekApplyModelParameters(t *testing.T) {
	provider := &Deepseek{}

	t.Run("ChatCompletionRequest: numeric parameters applied", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequest{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			Temperature:      utils.Ptr(float32(0.7)),
			TopP:             utils.Ptr(float32(0.9)),
			PresencePenalty:  utils.Ptr(float32(0.5)),
			FrequencyPenalty: utils.Ptr(float32(0.3)),
		})
		assert.InDelta(t, float32(0.7), req.Temperature, 0.0001)
		assert.InDelta(t, float32(0.9), req.TopP, 0.0001)
		assert.InDelta(t, float32(0.5), req.PresencePenalty, 0.0001)
		assert.InDelta(t, float32(0.3), req.FrequencyPenalty, 0.0001)
	})

	t.Run("ChatCompletionRequest: thinking sets ThinkingConfig", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequest{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			Thinking: utils.Ptr("enabled"),
		})
		require.NotNil(t, req.Thinking)
		assert.Equal(t, "enabled", req.Thinking.Type)
	})

	t.Run("ChatCompletionRequest: reasoning-effort sets typed field", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequest{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			ReasoningEffort: utils.Ptr("max"),
		})
		assert.Equal(t, "max", req.ReasoningEffort)
		assert.Nil(t, req.ExtraFields)
	})

	t.Run("ChatCompletionRequest: max-tokens sets MaxTokens", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequest{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			MaxTokens: utils.Ptr(int32(65536)),
		})
		assert.Equal(t, 65536, req.MaxTokens)
	})

	t.Run("ChatCompletionRequest: nil parameters leave fields at zero", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequest{}
		provider.applyModelParameters(req, config.DeepseekModelParams{})
		assert.Zero(t, req.Temperature)
		assert.Zero(t, req.TopP)
		assert.Zero(t, req.PresencePenalty)
		assert.Zero(t, req.FrequencyPenalty)
		assert.Zero(t, req.MaxTokens)
		assert.Nil(t, req.Thinking)
		assert.Nil(t, req.ExtraFields)
	})

	// ChatCompletionRequestWithImage does not expose Thinking in the
	// deepseek-go library, so only ReasoningEffort can be forwarded on this path.
	t.Run("ChatCompletionRequestWithImage: numeric parameters applied", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequestWithImage{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			Temperature:      utils.Ptr(float32(0.7)),
			TopP:             utils.Ptr(float32(0.9)),
			PresencePenalty:  utils.Ptr(float32(0.5)),
			FrequencyPenalty: utils.Ptr(float32(0.3)),
		})
		assert.InDelta(t, float32(0.7), req.Temperature, 0.0001)
		assert.InDelta(t, float32(0.9), req.TopP, 0.0001)
		assert.InDelta(t, float32(0.5), req.PresencePenalty, 0.0001)
		assert.InDelta(t, float32(0.3), req.FrequencyPenalty, 0.0001)
	})

	t.Run("ChatCompletionRequestWithImage: reasoning-effort forwarded", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequestWithImage{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			ReasoningEffort: utils.Ptr("max"),
		})
		assert.Equal(t, "max", req.ReasoningEffort)
	})

	t.Run("ChatCompletionRequestWithImage: thinking not applied", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequestWithImage{}
		// Thinking is not exposed by ChatCompletionRequestWithImage — verify no panic
		// occurs and that no other fields are mutated.
		provider.applyModelParameters(req, config.DeepseekModelParams{
			Thinking: utils.Ptr("enabled"),
		})
		assert.Zero(t, req.Temperature)
		assert.Zero(t, req.TopP)
		assert.Zero(t, req.PresencePenalty)
		assert.Zero(t, req.FrequencyPenalty)
		assert.Empty(t, req.ReasoningEffort)
	})

	t.Run("ChatCompletionRequestWithImage: max-tokens sets MaxTokens", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequestWithImage{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			MaxTokens: utils.Ptr(int32(65536)),
		})
		assert.Equal(t, 65536, req.MaxTokens)
	})
}
