// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"testing"

	deepseek "github.com/cohesion-org/deepseek-go"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepseek_FileUploadNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &Deepseek{} // nil client is sufficient to test early error
	runCfg := config.RunConfig{Name: "test-run", Model: "directional"}
	task := config.Task{
		Name:  "with_file",
		Files: []config.TaskFile{mockTaskFile(t, "img.png", "file://img.png", "image/png")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileUploadNotSupported)
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

	t.Run("ChatCompletionRequest: reasoning-effort sets ExtraFields", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequest{}
		provider.applyModelParameters(req, config.DeepseekModelParams{
			ReasoningEffort: utils.Ptr("max"),
		})
		require.NotNil(t, req.ExtraFields)
		assert.Equal(t, "max", req.ExtraFields["reasoning_effort"])
	})

	t.Run("ChatCompletionRequest: nil parameters leave fields at zero", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequest{}
		provider.applyModelParameters(req, config.DeepseekModelParams{})
		assert.Zero(t, req.Temperature)
		assert.Zero(t, req.TopP)
		assert.Zero(t, req.PresencePenalty)
		assert.Zero(t, req.FrequencyPenalty)
		assert.Nil(t, req.Thinking)
		assert.Nil(t, req.ExtraFields)
	})

	// ChatCompletionRequestWithImage does not expose Thinking or ExtraFields in the
	// deepseek-go library, so those parameters cannot be forwarded on this path.
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

	t.Run("ChatCompletionRequestWithImage: thinking and reasoning-effort not applied", func(t *testing.T) {
		req := &deepseek.ChatCompletionRequestWithImage{}
		// These fields do not exist on ChatCompletionRequestWithImage — verify no panic
		// occurs and that no other fields are mutated.
		provider.applyModelParameters(req, config.DeepseekModelParams{
			Thinking:        utils.Ptr("enabled"),
			ReasoningEffort: utils.Ptr("max"),
		})
		assert.Zero(t, req.Temperature)
		assert.Zero(t, req.TopP)
		assert.Zero(t, req.PresencePenalty)
		assert.Zero(t, req.FrequencyPenalty)
	})
}
