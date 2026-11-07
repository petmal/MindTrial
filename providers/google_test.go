// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
	"google.golang.org/genai/interactions/models/apierrors"
	"google.golang.org/genai/interactions/models/components"
	"google.golang.org/genai/interactions/models/interactions"
)

func TestGoogle_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &GoogleAI{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "gemini-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "application/octet-stream")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task, nil)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestGoogle_InvalidModelParams(t *testing.T) {
	p := &GoogleAI{}
	runCfg := config.RunConfig{Name: "test-run", Model: "gemini-test", ModelParams: config.OpenAIModelParams{}}
	_, err := p.Run(context.Background(), testutils.NewTestLogger(t), runCfg, config.Task{Name: "t"}, nil)
	require.ErrorIs(t, err, ErrInvalidModelParams)
}

func TestGoogle_RemoveFunctionTool(t *testing.T) {
	p := &GoogleAI{}
	newRequest := func(names ...string) *interactions.CreateModelInteraction {
		request := &interactions.CreateModelInteraction{
			GenerationConfig: &interactions.GenerationConfig{
				ToolChoice: genai.Ptr(interactions.NewToolChoice(interactions.ToolChoiceTypeAuto)),
			},
		}
		for _, name := range names {
			request.Tools = append(request.Tools, interactions.NewTool(interactions.Function{Name: genai.Ptr(name)}))
		}
		return request
	}
	toolNames := func(request *interactions.CreateModelInteraction) (names []string) {
		for _, tool := range request.Tools {
			names = append(names, *tool.Function.Name)
		}
		return
	}

	tests := []struct {
		name           string
		request        *interactions.CreateModelInteraction
		remove         string
		wantTools      []string
		wantToolChoice bool
	}{
		{
			name:           "removes matching function",
			request:        newRequest("tool_a", "tool_b"),
			remove:         "tool_a",
			wantTools:      []string{"tool_b"},
			wantToolChoice: true,
		},
		{
			name:           "unknown function removal is noop",
			request:        newRequest("tool_a", "tool_b"),
			remove:         "tool_missing",
			wantTools:      []string{"tool_a", "tool_b"},
			wantToolChoice: true,
		},
		{
			name:           "removing last function clears tool choice",
			request:        newRequest("tool_a"),
			remove:         "tool_a",
			wantTools:      nil,
			wantToolChoice: false,
		},
		{
			name: "skips tools without function or name",
			request: &interactions.CreateModelInteraction{
				Tools: []interactions.Tool{
					interactions.NewTool(interactions.Function{}),
					interactions.NewTool(interactions.Function{Name: genai.Ptr("tool_a")}),
				},
			},
			remove: "tool_a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.removeFunctionTool(tt.request, tt.remove)
			if tt.request.GenerationConfig == nil {
				require.Len(t, tt.request.Tools, 1)
				assert.Nil(t, tt.request.Tools[0].Function.Name)
				return
			}
			assert.Equal(t, tt.wantTools, toolNames(tt.request))
			assert.Equal(t, tt.wantToolChoice, tt.request.GenerationConfig.ToolChoice != nil)
		})
	}
}

func TestGoogle_IsTransientResponse(t *testing.T) {
	httpMeta := func(statusCode int) components.HTTPMetadata {
		return components.HTTPMetadata{Response: &http.Response{StatusCode: statusCode}}
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "server error 503", err: &apierrors.CreateInteractionServerError{HTTPMeta: httpMeta(http.StatusServiceUnavailable)}, want: true},
		{name: "server error 500", err: &apierrors.CreateInteractionServerError{HTTPMeta: httpMeta(http.StatusInternalServerError)}, want: true},
		{name: "server error 502", err: &apierrors.CreateInteractionServerError{HTTPMeta: httpMeta(http.StatusBadGateway)}, want: false},
		{name: "client error 429", err: &apierrors.CreateInteractionClientError{HTTPMeta: httpMeta(http.StatusTooManyRequests)}, want: true},
		{name: "client error 400", err: &apierrors.CreateInteractionClientError{HTTPMeta: httpMeta(http.StatusBadRequest)}, want: false},
		{name: "client error without response", err: &apierrors.CreateInteractionClientError{}, want: false},
		{name: "api error 503", err: &apierrors.APIError{StatusCode: http.StatusServiceUnavailable}, want: true},
		{name: "api error 404", err: &apierrors.APIError{StatusCode: http.StatusNotFound}, want: false},
		{name: "validation error 429", err: &apierrors.ResponseValidationError{StatusCode: http.StatusTooManyRequests}, want: true},
		{name: "validation error 200", err: &apierrors.ResponseValidationError{StatusCode: http.StatusOK}, want: false},
		{name: "wrapped error", err: fmt.Errorf("outer: %w", &apierrors.APIError{StatusCode: http.StatusTooManyRequests}), want: true},
		{name: "continuation interrupted by transient error", err: &apierrors.ContinuationInterruptedError{Cause: &apierrors.APIError{StatusCode: http.StatusServiceUnavailable}}, want: true},
		{name: "context error", err: context.DeadlineExceeded, want: false},
		{name: "plain error", err: errors.ErrUnsupported, want: false},
	}
	p := &GoogleAI{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.isTransientResponse(tt.err))
		})
	}
}

func TestResolveGoogleImageResolution(t *testing.T) {
	defaultResolution := genai.Ptr(interactions.MediaResolutionMedium)
	tests := []struct {
		name              string
		detail            *config.ImageDetail
		defaultResolution *interactions.MediaResolution
		want              *interactions.MediaResolution
	}{
		{name: "nil detail and default", want: nil},
		{name: "nil detail uses default", defaultResolution: defaultResolution, want: defaultResolution},
		{name: "auto lets the model decide despite default", detail: testutils.Ptr(config.ImageDetailAuto), defaultResolution: defaultResolution, want: nil},
		{name: "low overrides default", detail: testutils.Ptr(config.ImageDetailLow), defaultResolution: defaultResolution, want: genai.Ptr(interactions.MediaResolutionLow)},
		{name: "medium", detail: testutils.Ptr(config.ImageDetailMedium), want: genai.Ptr(interactions.MediaResolutionMedium)},
		{name: "high overrides default", detail: testutils.Ptr(config.ImageDetailHigh), defaultResolution: defaultResolution, want: genai.Ptr(interactions.MediaResolutionHigh)},
		{name: "original maps to ultra high", detail: testutils.Ptr(config.ImageDetailOriginal), want: genai.Ptr(interactions.MediaResolutionUltraHigh)},
		{name: "unknown falls back to default", detail: testutils.Ptr(config.ImageDetail("unknown")), defaultResolution: defaultResolution, want: defaultResolution},
	}
	p := &GoogleAI{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.resolveImageResolution(context.Background(), testutils.NewTestLogger(t), tt.detail, tt.defaultResolution)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGoogle_MapModelParamValues(t *testing.T) {
	p := &GoogleAI{}
	assert.Equal(t, genai.Ptr(interactions.ThinkingLevelMinimal), p.mapThinkingLevel("minimal"))
	assert.Equal(t, genai.Ptr(interactions.ThinkingLevelLow), p.mapThinkingLevel("low"))
	assert.Equal(t, genai.Ptr(interactions.ThinkingLevelMedium), p.mapThinkingLevel("medium"))
	assert.Equal(t, genai.Ptr(interactions.ThinkingLevelHigh), p.mapThinkingLevel("high"))
	assert.Nil(t, p.mapThinkingLevel("unknown"))

	assert.Equal(t, genai.Ptr(interactions.ThinkingSummariesAuto), p.mapThinkingSummaries("auto"))
	assert.Equal(t, genai.Ptr(interactions.ThinkingSummariesNone), p.mapThinkingSummaries("none"))
	assert.Nil(t, p.mapThinkingSummaries("unknown"))

	assert.Equal(t, genai.Ptr(interactions.MediaResolutionLow), p.mapMediaResolution("low"))
	assert.Equal(t, genai.Ptr(interactions.MediaResolutionMedium), p.mapMediaResolution("medium"))
	assert.Equal(t, genai.Ptr(interactions.MediaResolutionHigh), p.mapMediaResolution("high"))
	assert.Equal(t, genai.Ptr(interactions.MediaResolutionUltraHigh), p.mapMediaResolution("ultra_high"))
	assert.Nil(t, p.mapMediaResolution("unknown"))
}

func TestGoogle_CreatePromptContent(t *testing.T) {
	ctx := context.Background()

	t.Run("image uses registered mime type and resolution precedence", func(t *testing.T) {
		p := &GoogleAI{}
		jpgPath := testutils.CreateMockFile(t, "photo-*.jpg", []byte("jpg-bytes"))
		jpgFile := mockTaskFileWithAccess(t, "photo.jpg", jpgPath, "image/jpg", []config.FileAccess{config.FileAccessNative})
		jpgFile.Options.ImageDetail = testutils.Ptr(config.ImageDetailOriginal)
		jpgFile.ResolveFileOptions(config.FileOptions{})
		pngPath := testutils.CreateMockFile(t, "chart-*.png", []byte("png-bytes"))
		pngFile := mockTaskFileWithAccess(t, "chart.png", pngPath, "image/png", []config.FileAccess{config.FileAccessNative})

		contents, err := p.createPromptContent(ctx, testutils.NewTestLogger(t), "the prompt", []config.TaskFile{jpgFile, pngFile}, genai.Ptr(interactions.MediaResolutionLow), &Result{})
		require.NoError(t, err)
		require.Len(t, contents, 5)

		require.NotNil(t, contents[1].ImageContent)
		assert.Equal(t, interactions.ImageContentMimeTypeImageJpeg, *contents[1].ImageContent.MimeType)
		assert.Equal(t, interactions.MediaResolutionUltraHigh, *contents[1].ImageContent.Resolution)
		require.NotNil(t, contents[1].ImageContent.Data)
		assert.NotEmpty(t, *contents[1].ImageContent.Data)

		require.NotNil(t, contents[3].ImageContent)
		assert.Equal(t, interactions.ImageContentMimeTypeImagePng, *contents[3].ImageContent.MimeType)
		assert.Equal(t, interactions.MediaResolutionLow, *contents[3].ImageContent.Resolution)

		require.NotNil(t, contents[4].TextContent)
		assert.Equal(t, "the prompt", contents[4].TextContent.Text)
	})

	t.Run("plain text document is sent as text", func(t *testing.T) {
		p := &GoogleAI{}
		path := testutils.CreateMockFile(t, "notes-*.txt", []byte("line one\nline two"))
		file := mockTaskFileWithAccess(t, "notes.txt", path, "text/plain", []config.FileAccess{config.FileAccessNative})

		contents, err := p.createPromptContent(ctx, testutils.NewTestLogger(t), "the prompt", []config.TaskFile{file}, genai.Ptr(interactions.MediaResolutionHigh), &Result{})
		require.NoError(t, err)
		require.Len(t, contents, 3)
		require.NotNil(t, contents[1].TextContent)
		assert.Equal(t, "line one\nline two", contents[1].TextContent.Text)
	})

	t.Run("best effort document keeps original mime type", func(t *testing.T) {
		p := &GoogleAI{}
		path := testutils.CreateMockFile(t, "config-*.json", []byte(`{"a":1}`))
		file := mockTaskFileWithAccess(t, "config.json", path, "application/json", []config.FileAccess{config.FileAccessNative})

		contents, err := p.createPromptContent(ctx, testutils.NewTestLogger(t), "the prompt", []config.TaskFile{file}, genai.Ptr(interactions.MediaResolutionHigh), &Result{})
		require.NoError(t, err)
		require.Len(t, contents, 3)
		require.NotNil(t, contents[1].DocumentContent)
		assert.Equal(t, interactions.DocumentContentMimeType("application/json"), *contents[1].DocumentContent.MimeType)
	})

	t.Run("unsupported type is rejected", func(t *testing.T) {
		p := &GoogleAI{}
		file := mockTaskFileWithAccess(t, "notes.md", "file://notes.md", "text/markdown", []config.FileAccess{config.FileAccessNative})

		_, err := p.createPromptContent(ctx, testutils.NewTestLogger(t), "the prompt", []config.TaskFile{file}, nil, &Result{})
		require.ErrorIs(t, err, ErrFileNotSupported)
	})
}

func TestGoogle_NewFunctionResultStep(t *testing.T) {
	p := &GoogleAI{}
	functionCall := &interactions.FunctionCallStep{ID: "call_1", Name: "lookup"}
	tests := []struct {
		name     string
		response map[string]interface{}
		isError  bool
		want     string
	}{
		{
			name:     "success result",
			response: map[string]interface{}{"result": "42"},
			want:     `{"call_id":"call_1","name":"lookup","result":{"result":"42"},"type":"function_result"}`,
		},
		{
			name:     "error result",
			response: map[string]interface{}{"error": "Tool execution failed"},
			isError:  true,
			want:     `{"call_id":"call_1","is_error":true,"name":"lookup","result":{"error":"Tool execution failed"},"type":"function_result"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step, err := p.newFunctionResultStep(functionCall, tt.response, tt.isError)
			require.NoError(t, err)
			got, err := json.Marshal(step)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

// googleFakeResponse is a canned HTTP response of the fake Interactions API server.
type googleFakeResponse struct {
	statusCode int
	body       string
}

// googleFakeServer is a fake Interactions API server returning canned responses in order.
type googleFakeServer struct {
	mu        sync.Mutex
	responses []googleFakeResponse
	requests  []map[string]any
	paths     []string
}

func newGoogleFakeServer(t *testing.T, responses ...googleFakeResponse) (*googleFakeServer, *httptest.Server) {
	fake := &googleFakeServer{responses: responses}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var request map[string]any
		assert.NoError(t, json.Unmarshal(body, &request))

		fake.mu.Lock()
		index := len(fake.requests)
		fake.requests = append(fake.requests, request)
		fake.paths = append(fake.paths, r.URL.Path)
		fake.mu.Unlock()

		response := googleFakeResponse{statusCode: http.StatusInternalServerError, body: `{"error":{"code":"INTERNAL","message":"unexpected request"}}`}
		if index < len(fake.responses) {
			response = fake.responses[index]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.statusCode)
		_, _ = io.WriteString(w, response.body)
	}))
	t.Cleanup(server.Close)
	return fake, server
}

func (f *googleFakeServer) recordedRequests() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func newGoogleTestProvider(t *testing.T, serverURL string, availableTools []config.ToolConfig) *GoogleAI {
	client, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey:      "test",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: serverURL},
	})
	require.NoError(t, err)
	return &GoogleAI{client: client, availableTools: availableTools}
}

func googleInteractionJSON(t *testing.T, id string, status string, steps ...map[string]any) string {
	interaction := map[string]any{
		"id":     id,
		"status": status,
		"steps":  steps,
		"usage": map[string]any{
			"total_input_tokens":   10,
			"total_output_tokens":  5,
			"total_thought_tokens": 3,
			"total_cached_tokens":  2,
		},
	}
	data, err := json.Marshal(interaction)
	require.NoError(t, err)
	return string(data)
}

func googleTextStep(text string) map[string]any {
	return map[string]any{"type": "model_output", "content": []map[string]any{{"type": "text", "text": text}}}
}

const googleTestFinalAnswerJSON = `{"title":"t","explanation":"e","final_answer":{"content":"done"}}`

func TestGoogle_Run_SingleTurn(t *testing.T) {
	fake, server := newGoogleFakeServer(t, googleFakeResponse{
		statusCode: http.StatusOK,
		body: googleInteractionJSON(t, "int-1", "completed",
			map[string]any{"type": "user_input", "content": []map[string]any{{"type": "text", "text": "the prompt"}}},
			map[string]any{"type": "thought", "summary": []map[string]any{{"type": "text", "text": "thinking"}}},
			googleTextStep(`{"title":"t","explanation":"e",`),
			googleTextStep(`"final_answer":{"content":"done"}}`),
		),
	})
	provider := newGoogleTestProvider(t, server.URL, nil)

	runCfg := config.RunConfig{
		Name:  "test-run",
		Model: "gemini-test",
		ModelParams: config.GoogleAIModelParams{
			ThinkingLevel:     testutils.Ptr("high"),
			ThinkingSummaries: testutils.Ptr("auto"),
			MaxOutputTokens:   testutils.Ptr(int32(1024)),
			Temperature:       testutils.Ptr(float32(0.5)),
			TopP:              testutils.Ptr(float32(0.25)),
			TopK:              testutils.Ptr(int32(40)),
			PresencePenalty:   testutils.Ptr(float32(0.1)),
			FrequencyPenalty:  testutils.Ptr(float32(0.1)),
			Seed:              testutils.Ptr(int32(7)),
		},
	}
	task := config.Task{Name: "t", Prompt: "the prompt", ResponseResultFormat: config.NewResponseFormat("text")}

	result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", result.GetFinalAnswerContent())

	usage := result.GetUsage()
	assert.Equal(t, testutils.Ptr(int64(10)), usage.InputTokens)
	assert.Equal(t, testutils.Ptr(int64(5)), usage.OutputTokens)
	assert.Equal(t, testutils.Ptr(int64(3)), usage.ReasoningTokens)
	assert.Equal(t, testutils.Ptr(int64(2)), usage.InputCacheReadTokens)

	requests := fake.recordedRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, "/v1beta/interactions", fake.paths[0])
	request := requests[0]
	assert.Equal(t, "gemini-test", request["model"])
	assert.Equal(t, true, request["store"])
	assert.NotContains(t, request, "previous_interaction_id")
	assert.NotContains(t, request, "tools")
	assert.NotContains(t, request, "system_instruction", "no system instruction is needed for JSON schema mode without answer format")

	responseFormat := request["response_format"].(map[string]any)
	assert.Equal(t, "text", responseFormat["type"])
	assert.Equal(t, "application/json", responseFormat["mime_type"])
	assert.NotEmpty(t, responseFormat["schema"])

	generationConfig := request["generation_config"].(map[string]any)
	assert.Equal(t, map[string]any{
		"thinking_level":     "high",
		"thinking_summaries": "auto",
		"max_output_tokens":  float64(1024),
		"temperature":        0.5,
		"top_p":              0.25,
		"seed":               float64(7),
	}, generationConfig, "unsupported parameters must not be sent")

	input := request["input"].([]any)
	require.Len(t, input, 1)
	assert.Equal(t, map[string]any{"type": "text", "text": "the prompt"}, input[0])
}

func TestGoogle_Run_TextResponseFormat(t *testing.T) {
	fake, server := newGoogleFakeServer(t, googleFakeResponse{
		statusCode: http.StatusOK,
		body:       googleInteractionJSON(t, "int-1", "completed", googleTextStep("```json\n"+googleTestFinalAnswerJSON+"\n```")),
	})
	provider := newGoogleTestProvider(t, server.URL, nil)

	runCfg := config.RunConfig{Name: "test-run", Model: "gemini-test", ModelParams: config.GoogleAIModelParams{TextResponseFormat: true}}
	task := config.Task{Name: "t", Prompt: "the prompt", ResponseResultFormat: config.NewResponseFormat("text")}

	result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", result.GetFinalAnswerContent())

	requests := fake.recordedRequests()
	require.Len(t, requests, 1)
	responseFormat := requests[0]["response_format"].(map[string]any)
	assert.Equal(t, "text/plain", responseFormat["mime_type"])
	assert.NotContains(t, responseFormat, "schema")
	assert.NotEmpty(t, requests[0]["system_instruction"], "response format instruction must be sent as system instruction")
}

func TestGoogle_Run_ToolConversation(t *testing.T) {
	worldClient := config.ToolConfig{
		Name:         "world-client",
		Image:        "world-client:latest",
		Description:  "Inspects the world.",
		Parameters:   map[string]interface{}{"type": "object"},
		Dependencies: []config.ServiceDependency{{Service: "world"}},
	}
	functionCallStep := func(name string) map[string]any {
		return map[string]any{"type": "function_call", "id": "call_1", "name": name, "arguments": map[string]any{"q": "x"}}
	}

	tests := []struct {
		name         string
		calledTool   string
		wantErr      error
		wantRequests int
	}{
		{
			name:         "ordinary tool error is returned to the model",
			calledTool:   "unknown-tool",
			wantRequests: 2,
		},
		{
			name:         "task runtime error ends the task",
			calledTool:   worldClient.Name,
			wantErr:      ErrToolUse,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, server := newGoogleFakeServer(t,
				googleFakeResponse{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "requires_action",
					map[string]any{"type": "user_input", "content": []map[string]any{{"type": "text", "text": "Inspect the world."}}},
					googleTextStep("Let me check."),
					functionCallStep(tt.calledTool),
				)},
				googleFakeResponse{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-2", "completed",
					map[string]any{"type": "function_result", "call_id": "call_1", "result": map[string]any{"error": "x"}},
					googleTextStep("done"),
				)},
			)
			provider := newGoogleTestProvider(t, server.URL, []config.ToolConfig{worldClient})
			task := config.Task{
				Name:                 "inspect world",
				Prompt:               "Inspect the world.",
				ResponseResultFormat: config.NewResponseFormat("text"),
				ToolSelector:         &config.ToolSelector{Tools: []config.ToolSelection{{Name: worldClient.Name}}},
			}
			task.ResolveToolSelector(config.ToolSelector{})
			runCfg := config.RunConfig{Name: "test-run", Model: "gemini-test", DisableStructuredOutput: true}

			// No environment is active, so the service-backed tool fails with a task runtime error.
			result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)

			requests := fake.recordedRequests()
			require.Len(t, requests, tt.wantRequests)
			first := requests[0]
			assert.Equal(t, []any{map[string]any{
				"type":        "function",
				"name":        worldClient.Name,
				"description": worldClient.Description,
				"parameters":  map[string]any{"type": "object"},
			}}, first["tools"])
			assert.Equal(t, "auto", first["generation_config"].(map[string]any)["tool_choice"])
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "done", result.GetFinalAnswerContent())

			second := requests[1]
			assert.Equal(t, "int-1", second["previous_interaction_id"])
			assert.Equal(t, first["tools"], second["tools"], "tools must be sent on every turn")
			assert.Equal(t, first["system_instruction"], second["system_instruction"], "system instruction must be sent on every turn")
			assert.Equal(t, first["response_format"], second["response_format"], "response format must be sent on every turn")
			assert.Equal(t, first["generation_config"], second["generation_config"], "generation config must be sent on every turn")

			input := second["input"].([]any)
			require.Len(t, input, 1)
			functionResult := input[0].(map[string]any)
			assert.Equal(t, "function_result", functionResult["type"])
			assert.Equal(t, "call_1", functionResult["call_id"])
			assert.Equal(t, tt.calledTool, functionResult["name"])
			assert.Equal(t, true, functionResult["is_error"])
			resultObject := functionResult["result"].(map[string]any)
			assert.Contains(t, resultObject["error"], "Tool execution failed")
			assert.NotContains(t, resultObject, "result")

			usage := result.GetUsage()
			assert.Equal(t, testutils.Ptr(int64(20)), usage.InputTokens, "usage must be accumulated across turns")
		})
	}
}

func TestGoogle_Run_ResponseHandling(t *testing.T) {
	tests := []struct {
		name          string
		responses     []googleFakeResponse
		wantErr       error
		wantRetryable bool
		wantNoContent bool
		wantRequests  int
	}{
		{
			name:          "transient HTTP error is retryable and not retried by the SDK",
			responses:     []googleFakeResponse{{statusCode: http.StatusServiceUnavailable, body: `{"error":{"code":"UNAVAILABLE","message":"overloaded"}}`}},
			wantErr:       ErrGenerateResponse,
			wantRetryable: true,
			wantRequests:  1,
		},
		{
			name:          "transient HTTP error with numeric code is retryable",
			responses:     []googleFakeResponse{{statusCode: http.StatusTooManyRequests, body: `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`}},
			wantErr:       ErrGenerateResponse,
			wantRetryable: true,
			wantRequests:  1,
		},
		{
			name:         "client HTTP error is not retryable",
			responses:    []googleFakeResponse{{statusCode: http.StatusBadRequest, body: `{"error":{"code":"INVALID_ARGUMENT","message":"bad"}}`}},
			wantErr:      ErrGenerateResponse,
			wantRequests: 1,
		},
		{
			name:          "failed interaction with transient interaction error is retryable",
			responses:     []googleFakeResponse{{statusCode: http.StatusOK, body: `{"id":"int-1","status":"failed","errors":[{"code":"rate_limit_exceeded","message":"quota"}]}`}},
			wantErr:       ErrGenerateResponse,
			wantRetryable: true,
			wantRequests:  1,
		},
		{
			name:         "failed interaction with permanent error is not retryable",
			responses:    []googleFakeResponse{{statusCode: http.StatusOK, body: `{"id":"int-1","status":"failed","errors":[{"code":"invalid_request","message":"bad"}]}`}},
			wantErr:      ErrGenerateResponse,
			wantRequests: 1,
		},
		{
			name:         "cancelled interaction",
			responses:    []googleFakeResponse{{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "cancelled")}},
			wantErr:      ErrNoResponseCandidates,
			wantRequests: 1,
		},
		{
			name:          "unfinished interaction is retryable",
			responses:     []googleFakeResponse{{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "in_progress")}},
			wantErr:       ErrGenerateResponse,
			wantRetryable: true,
			wantRequests:  1,
		},
		{
			name: "interaction requiring continuation without progress is retryable",
			responses: []googleFakeResponse{{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "requires_continuation",
				googleTextStep("partial"),
			)}},
			wantErr:       ErrGenerateResponse,
			wantRetryable: true,
			wantRequests:  1,
		},
		{
			name:          "incomplete interaction without text",
			responses:     []googleFakeResponse{{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "incomplete")}},
			wantNoContent: true,
			wantRequests:  1,
		},
		{
			name: "incomplete interaction with function calls is terminal",
			responses: []googleFakeResponse{{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "incomplete",
				map[string]any{"type": "function_call", "id": "call_1", "name": "tool", "arguments": map[string]any{}},
			)}},
			wantNoContent: true,
			wantRequests:  1,
		},
		{
			name: "completed interaction with function calls is terminal",
			responses: []googleFakeResponse{{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "completed",
				map[string]any{"type": "function_call", "id": "call_1", "name": "tool", "arguments": map[string]any{}},
			)}},
			wantNoContent: true,
			wantRequests:  1,
		},
		{
			name:         "incomplete interaction with text is parsed",
			responses:    []googleFakeResponse{{statusCode: http.StatusOK, body: googleInteractionJSON(t, "int-1", "incomplete", googleTextStep(googleTestFinalAnswerJSON))}},
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, server := newGoogleFakeServer(t, tt.responses...)
			provider := newGoogleTestProvider(t, server.URL, nil)
			runCfg := config.RunConfig{Name: "test-run", Model: "gemini-test"}
			task := config.Task{Name: "t", Prompt: "the prompt", ResponseResultFormat: config.NewResponseFormat("text")}

			result, err := provider.Run(t.Context(), testutils.NewTestLogger(t), runCfg, task, nil)

			assert.Len(t, fake.recordedRequests(), tt.wantRequests)
			switch {
			case tt.wantNoContent:
				var noContentErr *ErrNoActionableContent
				require.ErrorAs(t, err, &noContentErr)
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.wantRetryable, errors.Is(err, ErrRetryable))
			default:
				require.NoError(t, err)
				assert.Equal(t, "done", result.GetFinalAnswerContent())
			}
		})
	}
}
