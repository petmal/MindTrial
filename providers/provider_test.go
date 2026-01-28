// Copyright (C) 2025 Petr Malik
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
	"testing"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestResultGetFinalAnswerContent(t *testing.T) {
	tests := []struct {
		name     string
		answer   Answer
		expected interface{}
	}{
		{
			name:     "string value",
			answer:   Answer{Content: "hello world"},
			expected: "hello world",
		},
		{
			name:     "numeric value",
			answer:   Answer{Content: 42.3},
			expected: 42.3,
		},
		{
			name:     "boolean value",
			answer:   Answer{Content: true},
			expected: true,
		},
		{
			name:     "nil value",
			answer:   Answer{Content: nil},
			expected: nil,
		},
		{
			name:     "complex object",
			answer:   Answer{Content: map[string]interface{}{"key": "value", "number": 123}},
			expected: map[string]interface{}{"key": "value", "number": 123},
		},
		{
			name:     "slice value",
			answer:   Answer{Content: []string{"item1", "item2", "item3"}},
			expected: []string{"item1", "item2", "item3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Result{
				Title:       "Test Result",
				Explanation: "Test explanation",
				FinalAnswer: tt.answer,
			}
			assert.Equal(t, tt.expected, result.GetFinalAnswerContent())
		})
	}
}

func TestDefaultAnswerFormatInstruction(t *testing.T) {
	tests := []struct {
		name     string
		task     config.Task
		expected string
	}{
		{
			name: "default format",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("<answer>"),
			},
			expected: "Provide the final answer in exactly this format: <answer>",
		},
		{
			name: "custom system prompt",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("<answer>"),
				SystemPrompt: &config.SystemPrompt{
					Template: testutils.Ptr("You are a helpful assistant. Always respond with clear answers."),
				},
			},
			expected: "You are a helpful assistant. Always respond with clear answers.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.task.ResolveSystemPrompt(config.SystemPrompt{}); err != nil {
				t.Fatalf("failed to resolve system prompt: %v", err)
			}
			assert.Equal(t, tt.expected, DefaultAnswerFormatInstruction(tt.task))
		})
	}
}

func TestDefaultAnswerFormatInstruction_EnableFor(t *testing.T) {
	tests := []struct {
		name     string
		task     config.Task
		expected string
	}{
		{
			name: "EnableForAll with string format and template",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("yes/no"),
				SystemPrompt: &config.SystemPrompt{
					Template:  testutils.Ptr("You are helpful. Format: {{.ResponseResultFormat}}"),
					EnableFor: testutils.Ptr(config.EnableForAll),
				},
			},
			expected: "You are helpful. Format: yes/no",
		},
		{
			name: "EnableForAll with schema format and template",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat(map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"answer": map[string]interface{}{"type": "string"},
					},
				}),
				SystemPrompt: &config.SystemPrompt{
					Template:  testutils.Ptr("Custom prompt. Format: {{.ResponseResultFormat}}"),
					EnableFor: testutils.Ptr(config.EnableForAll),
				},
			},
			expected: "Custom prompt. Format: {\n  \"properties\": {\n    \"answer\": {\n      \"type\": \"string\"\n    }\n  },\n  \"type\": \"object\"\n}",
		},
		{
			name: "EnableForAll with string format and blank template",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("yes/no"),
				SystemPrompt: &config.SystemPrompt{
					EnableFor: testutils.Ptr(config.EnableForAll),
				},
			},
			expected: "Provide the final answer in exactly this format: yes/no",
		},
		{
			name: "EnableForAll with schema format and blank template",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat(map[string]interface{}{
					"type": "object",
				}),
				SystemPrompt: &config.SystemPrompt{
					EnableFor: testutils.Ptr(config.EnableForAll),
				},
			},
			expected: "",
		},
		{
			name: "EnableForNone with string format",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("yes/no"),
				SystemPrompt: &config.SystemPrompt{
					Template:  testutils.Ptr("You are helpful"),
					EnableFor: testutils.Ptr(config.EnableForNone),
				},
			},
			expected: "",
		},
		{
			name: "EnableForNone with schema format",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat(map[string]interface{}{
					"type": "object",
				}),
				SystemPrompt: &config.SystemPrompt{
					Template:  testutils.Ptr("You are helpful"),
					EnableFor: testutils.Ptr(config.EnableForNone),
				},
			},
			expected: "",
		},
		{
			name: "EnableForText with string format and template",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("yes/no"),
				SystemPrompt: &config.SystemPrompt{
					Template:  testutils.Ptr("You are helpful"),
					EnableFor: testutils.Ptr(config.EnableForText),
				},
			},
			expected: "You are helpful",
		},
		{
			name: "EnableForText with string format and blank template",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("yes/no"),
				SystemPrompt: &config.SystemPrompt{
					EnableFor: testutils.Ptr(config.EnableForText),
				},
			},
			expected: "Provide the final answer in exactly this format: yes/no",
		},
		{
			name: "EnableForText with schema format",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat(map[string]interface{}{
					"type": "object",
				}),
				SystemPrompt: &config.SystemPrompt{
					Template:  testutils.Ptr("You are helpful"),
					EnableFor: testutils.Ptr(config.EnableForText),
				},
			},
			expected: "",
		},
		{
			name: "default EnableFor (text) with string format",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat("yes/no"),
			},
			expected: "Provide the final answer in exactly this format: yes/no",
		},
		{
			name: "default EnableFor (text) with schema format",
			task: config.Task{
				ResponseResultFormat: config.NewResponseFormat(map[string]interface{}{
					"type": "object",
				}),
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.task.ResolveSystemPrompt(config.SystemPrompt{}); err != nil {
				t.Fatalf("failed to resolve system prompt: %v", err)
			}
			assert.Equal(t, tt.expected, DefaultAnswerFormatInstruction(tt.task))
		})
	}
}
func TestResultJSONSchemaRaw(t *testing.T) {
	tests := []struct {
		name           string
		responseFormat config.ResponseFormat
		wantSchema     map[string]interface{}
	}{
		{
			name:           "string response format",
			responseFormat: config.NewResponseFormat("Provide a simple answer"),
			wantSchema: map[string]interface{}{
				"$schema":              "https://json-schema.org/draft/2020-12/schema",
				"$id":                  "https://github.com/petmal/mindtrial/providers/result",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"title":       "Response Title",
						"description": "A concise, descriptive title that summarizes what this response is about. Should be brief (typically 3-8 words) and capture the essence of the task or question being answered.",
					},
					"explanation": map[string]interface{}{
						"type":        "string",
						"title":       "Response Explanation",
						"description": "A comprehensive explanation of the reasoning process, methodology, and context behind the final answer. This should provide clear rationale for how the answer was derived, including any relevant analysis, steps taken, or considerations made.",
					},
					"final_answer": map[string]interface{}{
						"type":        "string",
						"title":       "Final Answer",
						"description": "The definitive answer to the task or question, provided as plain text. This should directly address what was asked and strictly follow any formatting instructions provided.",
					},
				},
				"required": []interface{}{"title", "explanation", "final_answer"},
			},
		},
		{
			name: "complex object schema",
			responseFormat: config.NewResponseFormat(map[string]interface{}{
				"type":        "object",
				"title":       "Analysis Response",
				"description": "Detailed analysis with score and recommendation",
				"properties": map[string]interface{}{
					"analysis": map[string]interface{}{
						"type":  "object",
						"title": "Analysis Details",
						"properties": map[string]interface{}{
							"score":     map[string]interface{}{"type": "number", "minimum": 0, "maximum": 100},
							"reasoning": map[string]interface{}{"type": "string", "minLength": 10},
						},
						"required": []string{"score", "reasoning"},
					},
					"recommendation": map[string]interface{}{
						"type":  "string",
						"enum":  []string{"APPROVE", "REJECT", "REVIEW"},
						"title": "Decision",
					},
				},
				"required":             []string{"analysis", "recommendation"},
				"additionalProperties": false,
			}),
			wantSchema: map[string]interface{}{
				"$schema":              "https://json-schema.org/draft/2020-12/schema",
				"$id":                  "https://github.com/petmal/mindtrial/providers/result",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"title":       "Response Title",
						"description": "A concise, descriptive title that summarizes what this response is about. Should be brief (typically 3-8 words) and capture the essence of the task or question being answered.",
					},
					"explanation": map[string]interface{}{
						"type":        "string",
						"title":       "Response Explanation",
						"description": "A comprehensive explanation of the reasoning process, methodology, and context behind the final answer. This should provide clear rationale for how the answer was derived, including any relevant analysis, steps taken, or considerations made.",
					},
					"final_answer": map[string]interface{}{
						"type":                 "object",
						"title":                "Final Answer",
						"description":          "The container holding the definitive answer to the task or question. The answer content must directly address what was asked, strictly follow any formatting instructions provided, and conform to the specified schema.",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"content": map[string]interface{}{
								"type":        "object",
								"title":       "Analysis Response",
								"description": "Detailed analysis with score and recommendation",
								"properties": map[string]interface{}{
									"analysis": map[string]interface{}{
										"type":  "object",
										"title": "Analysis Details",
										"properties": map[string]interface{}{
											"score":     map[string]interface{}{"type": "number", "minimum": 0, "maximum": 100},
											"reasoning": map[string]interface{}{"type": "string", "minLength": 10},
										},
										"required": []string{"score", "reasoning"},
									},
									"recommendation": map[string]interface{}{
										"type":  "string",
										"enum":  []string{"APPROVE", "REJECT", "REVIEW"},
										"title": "Decision",
									},
								},
								"required":             []string{"analysis", "recommendation"},
								"additionalProperties": false,
							},
						},
						"required": []interface{}{"content"},
					},
				},
				"required": []interface{}{"title", "explanation", "final_answer"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := ResultJSONSchemaRaw(tt.responseFormat)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSchema, schema)
		})
	}
}

func TestResultJSONSchema(t *testing.T) {
	tests := []struct {
		name           string
		responseFormat config.ResponseFormat
		wantSchemaRaw  map[string]interface{}
	}{
		{
			name:           "string response format",
			responseFormat: config.NewResponseFormat("Provide a simple answer"),
			wantSchemaRaw: map[string]interface{}{
				"$schema":              "https://json-schema.org/draft/2020-12/schema",
				"$id":                  "https://github.com/petmal/mindtrial/providers/result",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"title":       "Response Title",
						"description": "A concise, descriptive title that summarizes what this response is about. Should be brief (typically 3-8 words) and capture the essence of the task or question being answered.",
					},
					"explanation": map[string]interface{}{
						"type":        "string",
						"title":       "Response Explanation",
						"description": "A comprehensive explanation of the reasoning process, methodology, and context behind the final answer. This should provide clear rationale for how the answer was derived, including any relevant analysis, steps taken, or considerations made.",
					},
					"final_answer": map[string]interface{}{
						"type":        "string",
						"title":       "Final Answer",
						"description": "The definitive answer to the task or question, provided as plain text. This should directly address what was asked and strictly follow any formatting instructions provided.",
					},
				},
				"required": []interface{}{"title", "explanation", "final_answer"},
			},
		},
		{
			name: "complex object schema",
			responseFormat: config.NewResponseFormat(map[string]interface{}{
				"type":        "object",
				"title":       "Analysis Response",
				"description": "Detailed analysis with score and recommendation",
				"properties": map[string]interface{}{
					"analysis": map[string]interface{}{
						"type":  "object",
						"title": "Analysis Details",
						"properties": map[string]interface{}{
							"score":     map[string]interface{}{"type": "number", "minimum": 0, "maximum": 100},
							"reasoning": map[string]interface{}{"type": "string", "minLength": 10},
						},
						"required": []string{"score", "reasoning"},
					},
					"recommendation": map[string]interface{}{
						"type":  "string",
						"enum":  []string{"APPROVE", "REJECT", "REVIEW"},
						"title": "Decision",
					},
				},
				"required":             []string{"analysis", "recommendation"},
				"additionalProperties": false,
			}),
			wantSchemaRaw: map[string]interface{}{
				"$schema":              "https://json-schema.org/draft/2020-12/schema",
				"$id":                  "https://github.com/petmal/mindtrial/providers/result",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"title":       "Response Title",
						"description": "A concise, descriptive title that summarizes what this response is about. Should be brief (typically 3-8 words) and capture the essence of the task or question being answered.",
					},
					"explanation": map[string]interface{}{
						"type":        "string",
						"title":       "Response Explanation",
						"description": "A comprehensive explanation of the reasoning process, methodology, and context behind the final answer. This should provide clear rationale for how the answer was derived, including any relevant analysis, steps taken, or considerations made.",
					},
					"final_answer": map[string]interface{}{
						"type":                 "object",
						"title":                "Final Answer",
						"description":          "The container holding the definitive answer to the task or question. The answer content must directly address what was asked, strictly follow any formatting instructions provided, and conform to the specified schema.",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"content": map[string]interface{}{
								"type":        "object",
								"title":       "Analysis Response",
								"description": "Detailed analysis with score and recommendation",
								"properties": map[string]interface{}{
									"analysis": map[string]interface{}{
										"type":  "object",
										"title": "Analysis Details",
										"properties": map[string]interface{}{
											"score":     map[string]interface{}{"type": "number", "minimum": 0, "maximum": 100},
											"reasoning": map[string]interface{}{"type": "string", "minLength": 10},
										},
										"required": []string{"score", "reasoning"},
									},
									"recommendation": map[string]interface{}{
										"type":  "string",
										"enum":  []string{"APPROVE", "REJECT", "REVIEW"},
										"title": "Decision",
									},
								},
								"required":             []string{"analysis", "recommendation"},
								"additionalProperties": false,
							},
						},
						"required": []interface{}{"content"},
					},
				},
				"required": []interface{}{"title", "explanation", "final_answer"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := ResultJSONSchema(tt.responseFormat)
			require.NoError(t, err)
			require.NotNil(t, schema)

			// Convert expected raw map to jsonschema.Schema for comparison.
			expectedBytes, err := json.Marshal(tt.wantSchemaRaw)
			require.NoError(t, err)

			var expectedSchema jsonschema.Schema
			err = json.Unmarshal(expectedBytes, &expectedSchema)
			require.NoError(t, err)

			assert.Equal(t, expectedSchema, *schema)
		})
	}
}

func TestMapToJSONSchema(t *testing.T) {
	tests := []struct {
		name      string
		schemaMap map[string]interface{}
		wantErr   bool
	}{
		{
			name: "valid simple schema",
			schemaMap: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type": "string",
					},
					"age": map[string]interface{}{
						"type": "integer",
					},
				},
				"required": []string{"name"},
			},
			wantErr: false,
		},
		{
			name: "empty schema map",
			schemaMap: map[string]interface{}{
				"type": "object",
			},
			wantErr: false,
		},
		{
			name: "invalid json structure",
			schemaMap: map[string]interface{}{
				"invalid": make(chan int), // channels cannot be marshaled to JSON
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := MapToJSONSchema(tt.schemaMap)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, schema)
			}
		})
	}
}

func TestAnswerMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		answer   Answer
		jsonData string
	}{
		{
			name:     "string content",
			answer:   Answer{Content: "This is a plain text answer"},
			jsonData: `"This is a plain text answer"`,
		},
		{
			name:     "null string",
			answer:   Answer{Content: "null"},
			jsonData: `"null"`,
		},
		{
			name:     "null content",
			answer:   Answer{Content: nil},
			jsonData: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling.
			marshaled, err := json.Marshal(tt.answer)
			require.NoError(t, err)
			assert.JSONEq(t, tt.jsonData, string(marshaled))

			// Test unmarshaling.
			var unmarshaled Answer
			err = json.Unmarshal([]byte(tt.jsonData), &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, tt.answer.Content, unmarshaled.Content)
		})
	}
}

func TestAnswerUnmarshalEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError bool
		expected    interface{}
	}{
		{
			name:        "empty string",
			jsonData:    `""`,
			expectError: false,
			expected:    "",
		},
		{
			name:        "whitespace string",
			jsonData:    `"   "`,
			expectError: false,
			expected:    "   ",
		},
		{
			name:        "multiline string",
			jsonData:    `"line1\nline2"`,
			expectError: false,
			expected:    "line1\nline2",
		},
		{
			name:        "invalid json",
			jsonData:    `{invalid}`,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "object with content field",
			jsonData:    `{"content":"test"}`,
			expectError: false,
			expected:    "test",
		},
		{
			name:        "object with complex content",
			jsonData:    `{"content":{"key":"value","number":42}}`,
			expectError: false,
			expected:    map[string]interface{}{"key": "value", "number": float64(42)},
		},
		{
			name:        "object with array content",
			jsonData:    `{"content":["item1","item2","item3"]}`,
			expectError: false,
			expected:    []interface{}{"item1", "item2", "item3"},
		},
		{
			name:        "object with null content",
			jsonData:    `{"content":null}`,
			expectError: false,
			expected:    nil,
		},
		{
			name:        "direct array content (not supported)",
			jsonData:    `["item1","item2","item3"]`,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "direct boolean content (not supported)",
			jsonData:    `true`,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "direct number content (not supported)",
			jsonData:    `123.45`,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "direct object content (not supported)",
			jsonData:    `{"data":"value","items":42}`,
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var answer Answer
			err := json.Unmarshal([]byte(tt.jsonData), &answer)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, answer.Content)
			}
		})
	}
}

func TestUnmarshalUnstructuredResponse(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedAnswer any
	}{
		{
			name:           "raw text",
			content:        "hello world",
			expectedAnswer: "hello world",
		},
		{
			name:           "json string literal",
			content:        `"hello world"`,
			expectedAnswer: `"hello world"`,
		},
		{
			name:           "null",
			content:        `null`,
			expectedAnswer: "null",
		},
		{
			name:           "json object with content field",
			content:        `{"content":"extracted value"}`,
			expectedAnswer: `{"content":"extracted value"}`,
		},
		{
			name:           "json object without content field",
			content:        `{"key":"value","number":42}`,
			expectedAnswer: `{"key":"value","number":42}`,
		},
		{
			name:           "json array",
			content:        `["item1","item2","item3"]`,
			expectedAnswer: `["item1","item2","item3"]`,
		},
		{
			name:           "number",
			content:        `123.45`,
			expectedAnswer: `123.45`,
		},
		{
			name:           "boolean",
			content:        `true`,
			expectedAnswer: `true`,
		},
		{
			name:           "malformed json",
			content:        `{invalid}`,
			expectedAnswer: `{invalid}`,
		},
		{
			name:           "empty content",
			content:        "",
			expectedAnswer: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.NewTestLogger(t)
			var result Result

			err := UnmarshalUnstructuredResponse(context.Background(), logger, []byte(tt.content), &result)

			require.NoError(t, err)
			assert.Equal(t, "Unstructured Response", result.Title)
			assert.Equal(t, "Response obtained with structured output disabled.", result.Explanation)
			assert.Equal(t, tt.expectedAnswer, result.FinalAnswer.Content)
		})
	}
}

func TestFormatToolExecutionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "simple error message",
			err:      errors.New("execution failed"), //nolint:err113
			expected: "Tool execution failed: execution failed",
		},
		{
			name:     "wrapped error",
			err:      fmt.Errorf("%w: additional context", ErrToolUse),
			expected: "Tool execution failed: tool use failed: additional context",
		},
		{
			name:     "nil error creates empty result",
			err:      nil,
			expected: "Tool execution failed: <nil>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatToolExecutionError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindToolByName(t *testing.T) {
	tests := []struct {
		name           string
		availableTools []config.ToolConfig
		searchName     string
		want           *config.ToolConfig
		wantFound      bool
	}{
		{
			name:           "empty tools slice",
			availableTools: []config.ToolConfig{},
			searchName:     "test-tool",
			want:           nil,
			wantFound:      false,
		},
		{
			name: "tool found at beginning",
			availableTools: []config.ToolConfig{
				{Name: "tool1", Description: "First tool"},
				{Name: "tool2", Description: "Second tool"},
			},
			searchName: "tool1",
			want:       &config.ToolConfig{Name: "tool1", Description: "First tool"},
			wantFound:  true,
		},
		{
			name: "tool found in middle",
			availableTools: []config.ToolConfig{
				{Name: "tool1", Description: "First tool"},
				{Name: "tool2", Description: "Second tool"},
				{Name: "tool3", Description: "Third tool"},
			},
			searchName: "tool2",
			want:       &config.ToolConfig{Name: "tool2", Description: "Second tool"},
			wantFound:  true,
		},
		{
			name: "tool found at end",
			availableTools: []config.ToolConfig{
				{Name: "tool1", Description: "First tool"},
				{Name: "tool2", Description: "Second tool"},
			},
			searchName: "tool2",
			want:       &config.ToolConfig{Name: "tool2", Description: "Second tool"},
			wantFound:  true,
		},
		{
			name: "tool not found",
			availableTools: []config.ToolConfig{
				{Name: "tool1", Description: "First tool"},
				{Name: "tool2", Description: "Second tool"},
			},
			searchName: "tool3",
			want:       nil,
			wantFound:  false,
		},
		{
			name: "case sensitive match",
			availableTools: []config.ToolConfig{
				{Name: "Tool1", Description: "First tool"},
			},
			searchName: "tool1",
			want:       nil,
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotFound := findToolByName(tt.availableTools, tt.searchName)
			assert.Equal(t, tt.wantFound, gotFound)
			if tt.wantFound {
				assert.Equal(t, tt.want, got)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func TestTaskFilesToDataMap(t *testing.T) {
	ctx := context.Background()

	t.Run("empty files", func(t *testing.T) {
		result, err := taskFilesToDataMap(ctx, []config.TaskFile{})
		require.NoError(t, err)
		assert.Equal(t, map[string][]byte{}, result)
	})

	t.Run("single file", func(t *testing.T) {
		mockData := []byte("test content")
		filePath := testutils.CreateMockFile(t, "test-*.txt", mockData)

		taskFile := mockTaskFile(t, "test", filePath, "text/plain")

		result, err := taskFilesToDataMap(ctx, []config.TaskFile{taskFile})
		require.NoError(t, err)
		assert.Equal(t, map[string][]byte{"test": mockData}, result)
	})

	t.Run("multiple files", func(t *testing.T) {
		mockData1 := []byte("content 1")
		mockData2 := []byte("content 2")
		filePath1 := testutils.CreateMockFile(t, "test1-*.txt", mockData1)
		filePath2 := testutils.CreateMockFile(t, "test2-*.txt", mockData2)

		taskFile1 := mockTaskFile(t, "file1.txt", filePath1, "text/plain")
		taskFile2 := mockTaskFile(t, "file2.txt", filePath2, "text/plain")

		result, err := taskFilesToDataMap(ctx, []config.TaskFile{taskFile1, taskFile2})
		require.NoError(t, err)
		expected := map[string][]byte{
			"file1.txt": mockData1,
			"file2.txt": mockData2,
		}
		assert.Equal(t, expected, result)
	})

	t.Run("file read error", func(t *testing.T) {
		taskFile := mockTaskFile(t, "nonexistent.txt", "/nonexistent/path.txt", "text/plain")

		result, err := taskFilesToDataMap(ctx, []config.TaskFile{taskFile})
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to read content for file \"nonexistent.txt\"")
	})
}

func mockTaskFile(t *testing.T, name string, uri string, mimeType string) config.TaskFile {
	// Use YAML unmarshaling to properly initialize the TaskFile functions.
	yamlStr := fmt.Sprintf("name: %s\nuri: %s\ntype: %s", name, uri, mimeType)
	var file config.TaskFile
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &file))
	return file
}
