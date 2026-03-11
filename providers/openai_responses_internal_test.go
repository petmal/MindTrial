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

	"github.com/openai/openai-go/v3/responses"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponses_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAIResponsesProvider{}
	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "gpt-test",
		DisableStructuredOutput: true,
		ModelParams: openAIV3ModelParams{
			ResponseFormat: ResponseFormatJSONObject.Ptr(),
		},
	}
	_, err := p.Run(context.Background(), logger, runCfg, config.Task{Name: "t"})
	require.ErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestOpenAIResponses_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAIResponsesProvider{}

	runCfg := config.RunConfig{Name: "test-run", Model: "gpt-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestDefaultResponseHandler_AddEvent(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NewTestLogger(t)

	t.Run("captures response on completed event", func(t *testing.T) {
		handler := &defaultResponseHandler{}
		event := responses.ResponseStreamEventUnion{
			Type: "response.completed",
			Response: responses.Response{
				Status: responses.ResponseStatusCompleted,
			},
		}
		err := handler.AddEvent(ctx, logger, event)
		require.NoError(t, err)
		require.NotNil(t, handler.GetResponse())
		assert.Equal(t, responses.ResponseStatusCompleted, handler.GetResponse().Status)
	})

	t.Run("captures response on failed event", func(t *testing.T) {
		handler := &defaultResponseHandler{}
		event := responses.ResponseStreamEventUnion{
			Type: "response.failed",
			Response: responses.Response{
				Status: responses.ResponseStatusFailed,
			},
		}
		err := handler.AddEvent(ctx, logger, event)
		require.NoError(t, err)
		require.NotNil(t, handler.GetResponse())
		assert.Equal(t, responses.ResponseStatusFailed, handler.GetResponse().Status)
	})

	t.Run("captures response on incomplete event", func(t *testing.T) {
		handler := &defaultResponseHandler{}
		event := responses.ResponseStreamEventUnion{
			Type: "response.incomplete",
			Response: responses.Response{
				Status: responses.ResponseStatusIncomplete,
			},
		}
		err := handler.AddEvent(ctx, logger, event)
		require.NoError(t, err)
		require.NotNil(t, handler.GetResponse())
		assert.Equal(t, responses.ResponseStatusIncomplete, handler.GetResponse().Status)
	})

	t.Run("returns retryable error on transient error event", func(t *testing.T) {
		handler := &defaultResponseHandler{}
		var event responses.ResponseStreamEventUnion
		require.NoError(t, json.Unmarshal([]byte(`{"type":"error","code":"server_error","message":"internal server error","param":"test_param"}`), &event))
		err := handler.AddEvent(ctx, logger, event)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrGenerateResponse)
		assert.ErrorIs(t, err, ErrRetryable)
	})

	t.Run("returns non-retryable error on permanent error event", func(t *testing.T) {
		handler := &defaultResponseHandler{}
		var event responses.ResponseStreamEventUnion
		require.NoError(t, json.Unmarshal([]byte(`{"type":"error","code":"invalid_prompt","message":"invalid prompt"}`), &event))
		err := handler.AddEvent(ctx, logger, event)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrGenerateResponse)
		assert.NotErrorIs(t, err, ErrRetryable)
	})

	t.Run("ignores non-terminal events", func(t *testing.T) {
		handler := &defaultResponseHandler{}
		for _, eventType := range []string{
			"response.created",
			"response.in_progress",
			"response.output_text.delta",
			"response.output_item.added",
		} {
			err := handler.AddEvent(ctx, logger, responses.ResponseStreamEventUnion{Type: eventType})
			require.NoError(t, err)
		}
		assert.Nil(t, handler.GetResponse())
	})
}

func TestIsRetryableResponseErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		code     responses.ResponseErrorCode
		expected bool
	}{
		{name: "server_error is retryable", code: responses.ResponseErrorCodeServerError, expected: true},
		{name: "rate_limit_exceeded is retryable", code: responses.ResponseErrorCodeRateLimitExceeded, expected: true},
		{name: "vector_store_timeout is retryable", code: responses.ResponseErrorCodeVectorStoreTimeout, expected: true},
		{name: "invalid_prompt is not retryable", code: responses.ResponseErrorCodeInvalidPrompt, expected: false},
		{name: "invalid_image is not retryable", code: responses.ResponseErrorCodeInvalidImage, expected: false},
		{name: "unknown code is not retryable", code: responses.ResponseErrorCode("unknown_error"), expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isRetryableResponseErrorCode(tt.code))
		})
	}
}

func TestIsTerminalResponseStatus(t *testing.T) {
	provider := &openAIResponsesProvider{}

	tests := []struct {
		name       string
		resp       *responses.Response
		isTerminal bool
	}{
		{
			name:       "completed without function calls is terminal",
			resp:       &responses.Response{Status: responses.ResponseStatusCompleted},
			isTerminal: true,
		},
		{
			name: "completed with message output is terminal",
			resp: &responses.Response{
				Status: responses.ResponseStatusCompleted,
				Output: []responses.ResponseOutputItemUnion{
					{Type: "message"},
				},
			},
			isTerminal: true,
		},
		{
			name: "completed with function call is non-terminal",
			resp: &responses.Response{
				Status: responses.ResponseStatusCompleted,
				Output: []responses.ResponseOutputItemUnion{
					{Type: "function_call"},
				},
			},
			isTerminal: false,
		},
		{
			name: "completed with mixed output including function call is non-terminal",
			resp: &responses.Response{
				Status: responses.ResponseStatusCompleted,
				Output: []responses.ResponseOutputItemUnion{
					{Type: "message"},
					{Type: "function_call"},
				},
			},
			isTerminal: false,
		},
		{
			name:       "failed is terminal",
			resp:       &responses.Response{Status: responses.ResponseStatusFailed},
			isTerminal: true,
		},
		{
			name:       "cancelled is terminal",
			resp:       &responses.Response{Status: responses.ResponseStatusCancelled},
			isTerminal: true,
		},
		{
			name:       "incomplete is terminal",
			resp:       &responses.Response{Status: responses.ResponseStatusIncomplete},
			isTerminal: true,
		},
		{
			name:       "empty status is non-terminal",
			resp:       &responses.Response{Status: ""},
			isTerminal: false,
		},
		{
			name:       "queued is non-terminal",
			resp:       &responses.Response{Status: responses.ResponseStatusQueued},
			isTerminal: false,
		},
		{
			name:       "in_progress is non-terminal",
			resp:       &responses.Response{Status: responses.ResponseStatusInProgress},
			isTerminal: false,
		},
		{
			name:       "unknown status is terminal",
			resp:       &responses.Response{Status: "some_new_status"},
			isTerminal: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isTerminal, provider.isTerminalResponseStatus(tt.resp))
		})
	}
}

func TestResponseOutputText(t *testing.T) {
	makeMessage := func(phase responses.ResponseOutputMessagePhase, text string) responses.ResponseOutputItemUnion {
		return responses.ResponseOutputItemUnion{
			Type:  "message",
			Phase: phase,
			Content: []responses.ResponseOutputMessageContentUnion{
				{Type: "output_text", Text: text},
			},
		}
	}

	tests := []struct {
		name      string
		output    []responses.ResponseOutputItemUnion
		wantOther string
		wantFinal string
	}{
		{
			name:      "empty output",
			output:    nil,
			wantOther: "",
			wantFinal: "",
		},
		{
			name: "no phase set (pre-5.3 models)",
			output: []responses.ResponseOutputItemUnion{
				makeMessage("", "hello world"),
			},
			wantOther: "hello world",
			wantFinal: "",
		},
		{
			name: "final_answer only",
			output: []responses.ResponseOutputItemUnion{
				makeMessage(responses.ResponseOutputMessagePhaseFinalAnswer, "the answer"),
			},
			wantOther: "",
			wantFinal: "the answer",
		},
		{
			name: "commentary only",
			output: []responses.ResponseOutputItemUnion{
				makeMessage(responses.ResponseOutputMessagePhaseCommentary, "thinking..."),
			},
			wantOther: "thinking...",
			wantFinal: "",
		},
		{
			name: "commentary then final_answer",
			output: []responses.ResponseOutputItemUnion{
				makeMessage(responses.ResponseOutputMessagePhaseCommentary, "let me check"),
				makeMessage(responses.ResponseOutputMessagePhaseFinalAnswer, `{"answer":42}`),
			},
			wantOther: "let me check",
			wantFinal: `{"answer":42}`,
		},
		{
			name: "non-message items are ignored",
			output: []responses.ResponseOutputItemUnion{
				{Type: "reasoning"},
				makeMessage(responses.ResponseOutputMessagePhaseFinalAnswer, "result"),
				{Type: "function_call"},
			},
			wantOther: "",
			wantFinal: "result",
		},
		{
			name: "multiple messages concatenated per phase",
			output: []responses.ResponseOutputItemUnion{
				makeMessage(responses.ResponseOutputMessagePhaseCommentary, "step 1"),
				makeMessage(responses.ResponseOutputMessagePhaseCommentary, "step 2"),
				makeMessage(responses.ResponseOutputMessagePhaseFinalAnswer, "part A"),
				makeMessage(responses.ResponseOutputMessagePhaseFinalAnswer, "part B"),
			},
			wantOther: "step 1step 2",
			wantFinal: "part Apart B",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &openAIResponsesProvider{}
			resp := &responses.Response{Output: tt.output}
			otherText, finalAnswer := p.responseOutputText(resp)
			assert.Equal(t, tt.wantOther, otherText)
			assert.Equal(t, tt.wantFinal, finalAnswer)
		})
	}
}

func TestMapImageDetailToResponses(t *testing.T) {
	logger := testutils.NewTestLogger(t)

	tests := []struct {
		name     string
		detail   *config.ImageDetail
		expected responses.ResponseInputImageDetail
	}{
		{name: "nil defaults to auto", detail: nil, expected: responses.ResponseInputImageDetailAuto},
		{name: "auto", detail: testutils.Ptr(config.ImageDetailAuto), expected: responses.ResponseInputImageDetailAuto},
		{name: "low", detail: testutils.Ptr(config.ImageDetailLow), expected: responses.ResponseInputImageDetailLow},
		{name: "medium maps to high", detail: testutils.Ptr(config.ImageDetailMedium), expected: responses.ResponseInputImageDetailHigh},
		{name: "high", detail: testutils.Ptr(config.ImageDetailHigh), expected: responses.ResponseInputImageDetailHigh},
		{name: "original", detail: testutils.Ptr(config.ImageDetailOriginal), expected: responses.ResponseInputImageDetailOriginal},
		{name: "unknown falls back to auto", detail: testutils.Ptr(config.ImageDetail("unknown")), expected: responses.ResponseInputImageDetailAuto},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapImageDetailToResponses(context.Background(), logger, tt.detail)
			assert.Equal(t, tt.expected, result)
		})
	}
}
