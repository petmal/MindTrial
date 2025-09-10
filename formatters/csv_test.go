// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"fmt"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockResults = runners.Results{
	"provider-name": []runners.RunResult{
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-success",
			Kind:     runners.Success,
			Duration: 95 * time.Second,
			Want:     utils.NewValueSet("Quos aut rerum quaerat qui ad culpa."),
			Got:      "Quos aut rerum quaerat qui ad culpa.",
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:          "Responsio Bona",
					Explanation:    []string{"Quis ea voluptatem non aperiam dolor est.", "Alias odit enim fugiat vitae aliquam dolor quo ratione."},
					ActualAnswer:   []string{"Quos aut rerum quaerat qui ad culpa."},
					ExpectedAnswer: [][]string{{"Quos aut rerum quaerat qui ad culpa."}},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(9876543210)),
						OutputTokens: testutils.Ptr(int64(1234567890)),
					},
				},
				Validation: runners.ValidationDetails{
					Title:       "Validatio Perfecta",
					Explanation: []string{"Sed ut perspiciatis unde omnis iste natus error sit voluptatem."},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(11)),
						OutputTokens: testutils.Ptr(int64(5)),
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-failure",
			Kind:     runners.Failure,
			Duration: 10 * time.Second,
			Want:     utils.NewValueSet("Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."),
			Got:      "Ipsam ea et optio explicabo eius et.",
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:          "Generatio Responsi",
					Explanation:    []string{"Ut eos eius modi nihil voluptatem error.", "Veniam omnis at possimus aliquid tempore.", "Ut voluptatem ullam et ea non beatae eos adipisci incidunt.", "Consequatur hic sint laboriosam maiores unde vero ipsum magnam."},
					ActualAnswer:   []string{"Ipsam ea et optio explicabo eius et."},
					ExpectedAnswer: [][]string{{"Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."}},
					Usage: runners.TokenUsage{
						InputTokens: testutils.Ptr(int64(200)),
					},
				},
				Validation: runners.ValidationDetails{
					Title:       "Validatio Defecit",
					Explanation: []string{"At vero eos et accusamus et iusto odio dignissimos ducimus qui."},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-success-multiple-answers",
			Kind:     runners.Success,
			Duration: 17 * time.Second,
			Want:     utils.NewValueSet("Deserunt quo sint minus eos officiis et.", "Quos aut rerum quaerat qui ad culpa."),
			Got:      "Quos aut rerum quaerat qui ad culpa.",
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:          "Multiplex Responsio",
					Explanation:    []string{"Quis ea voluptatem non aperiam.", "Dolor est alias odit enim fugiat vitae aliquam dolore ratione."},
					ActualAnswer:   []string{"Quos aut rerum quaerat qui ad culpa."},
					ExpectedAnswer: [][]string{{"Deserunt quo sint minus eos officiis et."}, {"Quos aut rerum quaerat qui ad culpa."}},
				},
				Validation: runners.ValidationDetails{
					Title: "Selectio Validata",
					Explanation: []string{
						"Blanditiis praesentium voluptatum deleniti atque corrupti quos dolores.",
						"Et quas molestias excepturi sint occaecati cupiditate non provident.",
						"Similique sunt in culpa qui officia deserunt mollitia animi.",
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-failure-multiple-answers",
			Kind:     runners.Failure,
			Duration: 3*time.Minute + 800*time.Millisecond,
			Want:     utils.NewValueSet("Dolores saepe ad sed rerum autem iure minima et.", "Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."),
			Got:      "Ipsam ea et optio explicabo eius et.",
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:          "Responsum Generatum",
					Explanation:    []string{"Ut eos eius modi nihil voluptatem error quidem.", "Veniam omnis at possimus aliquid corporis.", "Ut voluptatem ullam et ea non beatae eos adipisci incidunt tempore.", "Consequatur hic sint laboriosam maiores unde vero ipsum dolorem."},
					ActualAnswer:   []string{"Ipsam ea et optio explicabo eius et."},
					ExpectedAnswer: [][]string{{"Dolores saepe ad sed rerum autem iure minima et."}, {"Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."}},
				},
				Validation: runners.ValidationDetails{
					Title:       "Selectio Rejicienda",
					Explanation: []string{"Et harum quidem rerum facilis est et expedita distinctio nam libero."},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(500)),
						OutputTokens: testutils.Ptr(int64(150)),
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-error",
			Kind:     runners.Error,
			Duration: 0 * time.Second,
			Want:     utils.NewValueSet("Cum et rem."),
			Got:      "error message",
			Details: runners.Details{
				Answer:     runners.AnswerDetails{},
				Validation: runners.ValidationDetails{},
				Error: runners.ErrorDetails{
					Title:   "Errorem Executionis",
					Message: "Temporibus autem quibusdam et aut officiis debitis aut rerum necessitatibus.",
					Details: nil,
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(0)),
						OutputTokens: testutils.Ptr(int64(0)),
					},
				},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-not-supported",
			Kind:     runners.NotSupported,
			Duration: 500 * time.Millisecond,
			Want:     utils.NewValueSet("Animi aut eligendi repellendus debitis harum aut."),
			Got:      "Sequi molestiae iusto sit sit dolorum aut.",
			Details: runners.Details{
				Answer:     runners.AnswerDetails{},
				Validation: runners.ValidationDetails{},
				Error: runners.ErrorDetails{
					Title:   "Functio Non Supporta",
					Message: "Voluptate velit esse cillum dolore eu fugiat nulla pariatur.",
					Details: map[string][]string{
						"Feature Type": {"advanced-reasoning"},
						"Provider":     {"legacy-model-v1"},
						"Suggestion": {
							"Excepteur sint occaecat cupidatat non proident.",
							"Sunt in culpa qui officia deserunt mollit anim.",
						},
					},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(10)),
						OutputTokens: testutils.Ptr(int64(0)),
					},
				},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-validation-error",
			Kind:     runners.Error,
			Duration: 2 * time.Second,
			Want:     utils.NewValueSet("Lorem ipsum dolor sit amet consectetur."),
			Got:      "Adipiscing elit sed do eiusmod tempor.",
			Details: runners.Details{
				Answer:     runners.AnswerDetails{},
				Validation: runners.ValidationDetails{},
				Error: runners.ErrorDetails{
					Title:   "Validatio Deficiens",
					Message: "Ut enim ad minim veniam quis nostrud exercitation ullamco laboris.",
					Details: map[string][]string{
						"Service":  {"validation-service-v2"},
						"Endpoint": {"validate-response"},
						"Raw Response": {
							"Excepteur sint occaecat cupidatat non proident",
							"Sunt in culpa qui officia deserunt mollit anim",
							"Id est laborum et dolorum fuga",
						},
						"Diagnostic": {
							"Nemo enim ipsam voluptatem quia voluptas sit",
						},
					},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(1234)),
						OutputTokens: testutils.Ptr(int64(5678)),
					},
				},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-parsing-error",
			Kind:     runners.Error,
			Duration: 314159 * time.Millisecond,
			Want:     utils.NewValueSet("Sed do eiusmod tempor incididunt ut."),
			Got:      "Invalid JSON: {broken",
			Details: runners.Details{
				Answer:     runners.AnswerDetails{},
				Validation: runners.ValidationDetails{},
				Error: runners.ErrorDetails{
					Title:   "Parsing Errorem Responsi",
					Message: "Duis aute irure dolor in reprehenderit in voluptate velit esse.",
					Details: map[string][]string{
						"Error Position": {"line 3, column 25"},
						"Raw Response": {
							"Invalid JSON: {broken",
							"  \"field1\": \"value1\",",
							"  \"field2\": incomplete...",
							"} // missing closing brace",
						},
						"Parser State": {
							"Expected: closing quote or brace",
							"Found: end of input",
							"Context: within object literal",
						},
						"Recovery": {
							"Cillum dolore eu fugiat nulla pariatur.",
						},
					},
					Usage: runners.TokenUsage{
						OutputTokens: testutils.Ptr(int64(333)),
					},
				},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-structured-success",
			Kind:     runners.Success,
			Duration: 42 * time.Second,
			Want: utils.NewValueSet(
				[]interface{}{
					map[string]interface{}{
						"timestamp": "2025-09-14T10:30:00Z",
						"level":     "INFO",
						"message":   "User 'admin' logged in successfully.",
						"user_id":   "admin",
					},
					map[string]interface{}{
						"timestamp": "2025-09-14T10:31:15Z",
						"level":     "WARN",
						"message":   "System memory usage is high.",
					},
				}),
			Got: []interface{}{
				map[string]interface{}{
					"timestamp": "2025-09-14T10:30:00Z",
					"level":     "INFO",
					"message":   "User 'admin' logged in successfully.",
					"user_id":   "admin",
				},
				map[string]interface{}{
					"timestamp": "2025-09-14T10:31:15Z",
					"level":     "WARN",
					"message":   "System memory usage is high.",
				},
			},
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:       "Log Parsing Success",
					Explanation: []string{"Successfully parsed log entries with structured JSON output.", "Extracted timestamps, levels, messages, and user IDs where present."},
					ActualAnswer: []string{
						"[",
						"  {",
						"    \"timestamp\": \"2025-09-14T10:30:00Z\",",
						"    \"level\": \"INFO\",",
						"    \"message\": \"User 'admin' logged in successfully.\",",
						"    \"user_id\": \"admin\"",
						"  },",
						"  {",
						"    \"timestamp\": \"2025-09-14T10:31:15Z\",",
						"    \"level\": \"WARN\",",
						"    \"message\": \"System memory usage is high.\"",
						"  }",
						"]",
					},
					ExpectedAnswer: [][]string{{
						"[",
						"  {",
						"    \"timestamp\": \"2025-09-14T10:30:00Z\",",
						"    \"level\": \"INFO\",",
						"    \"message\": \"User 'admin' logged in successfully.\",",
						"    \"user_id\": \"admin\"",
						"  },",
						"  {",
						"    \"timestamp\": \"2025-09-14T10:31:15Z\",",
						"    \"level\": \"WARN\",",
						"    \"message\": \"System memory usage is high.\"",
						"  }",
						"]",
					}},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(456)),
						OutputTokens: testutils.Ptr(int64(234)),
					},
				},
				Validation: runners.ValidationDetails{
					Title:       "Structured Validation Success",
					Explanation: []string{"JSON structure matches expected schema.", "All required fields present with correct types.", "Deep equality comparison passed."},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(78)),
						OutputTokens: testutils.Ptr(int64(12)),
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-structured-failure",
			Kind:     runners.Failure,
			Duration: 38 * time.Second,
			Want: utils.NewValueSet(
				map[string]interface{}{
					"timestamp": "2025-09-14T10:30:00Z",
					"level":     "INFO",
					"message":   "User 'admin' logged in successfully.",
					"user_id":   "admin",
				},
				map[string]interface{}{
					"timestamp": "2025-09-14T10:31:15Z",
					"level":     "WARN",
					"message":   "System memory usage is high.",
				},
				map[string]interface{}{
					"timestamp": "2025-09-14T10:30:00Z",
					"level":     "INFO",
					"message":   "User login successful.",
					"user_id":   "admin",
				}),
			Got: map[string]interface{}{
				"timestamp": "2025-09-14T10:30:00Z",
				"level":     "ERROR",
				"message":   "Authentication failed for user 'admin'.",
				"user_id":   "admin",
			},
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:       "Log Parsing with Incorrect Data",
					Explanation: []string{"Parsed log entries but with incorrect content.", "First entry shows authentication failure instead of success.", "Second entry is correct."},
					ActualAnswer: []string{
						"{",
						"  \"level\": \"ERROR\",",
						"  \"message\": \"Authentication failed for user 'admin'.\",",
						"  \"timestamp\": \"2025-09-14T10:30:00Z\",",
						"  \"user_id\": \"admin\"",
						"}",
					},
					ExpectedAnswer: [][]string{
						{
							"{",
							"  \"level\": \"INFO\",",
							"  \"message\": \"User 'admin' logged in successfully.\",",
							"  \"timestamp\": \"2025-09-14T10:30:00Z\",",
							"  \"user_id\": \"admin\"",
							"}",
						},
						{
							"{",
							"  \"level\": \"WARN\",",
							"  \"message\": \"System memory usage is high.\",",
							"  \"timestamp\": \"2025-09-14T10:31:15Z\"",
							"}",
						},
						{
							"{",
							"  \"level\": \"INFO\",",
							"  \"message\": \"User login successful.\",",
							"  \"timestamp\": \"2025-09-14T10:30:00Z\",",
							"  \"user_id\": \"admin\"",
							"}",
						},
					},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(412)),
						OutputTokens: testutils.Ptr(int64(198)),
					},
				},
				Validation: runners.ValidationDetails{
					Title:       "Structured Validation Failure",
					Explanation: []string{"JSON structure is valid but content doesn't match any expected results.", "First log entry shows ERROR level instead of expected INFO level.", "Message content mismatch: authentication failure vs login success."},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(89)),
						OutputTokens: testutils.Ptr(int64(15)),
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
	},
}

func TestCSVFormatterWrite(t *testing.T) {
	tests := []struct {
		name    string
		results runners.Results
		want    string
	}{
		{
			name:    "format no results",
			results: runners.Results{},
			want:    "testdata/empty.csv",
		},
		{
			name:    "format some results",
			results: mockResults,
			want:    "testdata/results.csv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewCSVFormatter()
			assertFormatterOutputFromFile(t, formatter, tt.results, tt.want)
		})
	}
}

func assertFormatterOutputFromFile(t *testing.T, formatter Formatter, results runners.Results, expectedContentsFilePath string) {
	outputFileNamePattern := fmt.Sprintf("*.%s", formatter.FileExt())
	got := testutils.CreateOpenNewTestFile(t, outputFileNamePattern)
	gotFilePath := got.Name()
	require.NoError(t, formatter.Write(results, got))
	require.NoError(t, got.Close())
	t.Logf("Generated formatted file: %s\n", gotFilePath)
	testutils.AssertFileContentsSameAs(t, expectedContentsFilePath, gotFilePath)
}

func TestCSVFormatterFileExt(t *testing.T) {
	formatter := NewCSVFormatter()
	assert.Equal(t, "csv", formatter.FileExt())
}
