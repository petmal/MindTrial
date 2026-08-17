// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update-golden", false, "update golden test files")

type goldenFileTestCase struct {
	path    string
	results runners.Results
}

func updateGoldenFiles(t *testing.T, formatter Formatter, cases []goldenFileTestCase) {
	t.Helper()
	if !*updateGolden {
		t.Skip("use -update-golden to regenerate golden files")
	}
	for _, tc := range cases {
		withFixedMetadata(t, func() {
			f, err := os.Create(tc.path)
			require.NoError(t, err)
			require.NoError(t, formatter.Write(tc.results, f))
			require.NoError(t, f.Close())
			t.Logf("Updated %s", tc.path)
		})
	}
}

var mockResults = runners.Results{
	"provider-name": []runners.RunResult{
		{
			TraceID:  "01JEDE7Z8X0000000000000001",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-success",
			Kind:     runners.Success,
			Duration: 95 * time.Second,
			RunConfig: runners.RunConfigSnapshot{
				Name:                 "run-success",
				Model:                "gpt-4o-mini",
				MaxRequestsPerMinute: 60,
				RetryPolicy: runners.RetryPolicy{
					MaxRetryAttempts:    3,
					InitialDelaySeconds: 5,
				},
			},
			Want: utils.NewValueSet("Quos aut rerum quaerat qui ad culpa."),
			Got:  "Quos aut rerum quaerat qui ad culpa.",
			TaskMetadata: runners.TaskMetadata{
				Suite:      "core-suite",
				Category:   "reasoning",
				Difficulty: "hard",
				Tags:       []string{"nightly", "regression"},
			},
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:          "Responsio Bona",
					Explanation:    []string{"Quis ea voluptatem non aperiam dolor est.", "Alias odit enim fugiat vitae aliquam dolor quo ratione."},
					ActualAnswer:   []string{"Quos aut rerum quaerat qui ad culpa."},
					ExpectedAnswer: [][]string{{"Quos aut rerum quaerat qui ad culpa."}},
					Usage: runners.TokenUsage{
						InputTokens:           testutils.Ptr(int64(9876543210)),
						InputCacheWriteTokens: testutils.Ptr(int64(9757309999902428)),
						InputCacheReadTokens:  testutils.Ptr(int64(91835)),
						InputTokenAccounting:  runners.InputTokenAccountingCacheTokensSeparate,
						OutputTokens:          testutils.Ptr(int64(1234567890)),
					},
					ToolUsage: map[string]runners.ToolUsage{
						"calculator": {
							CallCount:     testutils.Ptr(int64(127)),
							TotalDuration: testutils.Ptr(45 * time.Second),
						},
						"web_search": {
							CallCount:     testutils.Ptr(int64(1)),
							TotalDuration: testutils.Ptr(2*time.Minute + 15*time.Second),
						},
						"memory_tool": {
							CallCount: testutils.Ptr(int64(8)),
							// Only call count, no duration
						},
						"file_processor": {
							TotalDuration: testutils.Ptr(350 * time.Millisecond),
							// Only duration, no call count
						},
					},
					ToolCalls: []runners.ToolCallSummary{
						{
							Tool:             "calculator",
							CallID:           "call_9f8c2e1a4b3d",
							ConversationTurn: 1,
							StartedAt:        time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
							CompletedAt:      time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC).Add(145 * time.Millisecond),
							Duration:         testutils.Ptr(120 * time.Millisecond),
							WallTime:         145 * time.Millisecond,
							ExitCode:         testutils.Ptr(int64(0)),
							Status:           "success",
							Stdout:           &runners.ToolCallOutput{Bytes: 12},
						},
						{
							Tool:             "calculator",
							CallID:           "call_1a2b3c4d5e6f",
							ConversationTurn: 2,
							StartedAt:        time.Date(2025, 6, 1, 12, 0, 1, 0, time.UTC),
							CompletedAt:      time.Date(2025, 6, 1, 12, 0, 1, 0, time.UTC).Add(110 * time.Millisecond),
							Duration:         testutils.Ptr(85 * time.Millisecond),
							WallTime:         110 * time.Millisecond,
							ExitCode:         testutils.Ptr(int64(1)),
							Status:           "nonzero_exit",
							Stderr:           &runners.ToolCallOutput{Bytes: 22, Preview: testutils.Ptr("division by zero"), Truncated: false},
							ErrorMessage:     "tool container exited with code 1: division by zero",
						},
						{
							Tool:         "web_search",
							CallID:       "01JEDE7Z8X0000000000000ABC",
							StartedAt:    time.Date(2025, 6, 1, 12, 0, 2, 0, time.UTC),
							CompletedAt:  time.Date(2025, 6, 1, 12, 1, 2, 0, time.UTC),
							WallTime:     60 * time.Second,
							TimedOut:     true,
							Status:       "timeout",
							ErrorMessage: "tool execution timeout: execution timed out after 1m0s",
						},
					},
				},
				Validation: runners.ValidationDetails{
					Title:       "Validatio Perfecta",
					Explanation: []string{"Sed ut perspiciatis unde omnis iste natus error sit voluptatem."},
					Usage: runners.TokenUsage{
						InputTokens:           testutils.Ptr(int64(11)),
						InputCacheWriteTokens: testutils.Ptr(int64(55)),
						InputCacheReadTokens:  testutils.Ptr(int64(99)),
						InputTokenAccounting:  runners.InputTokenAccountingCacheTokensIncluded,
						OutputTokens:          testutils.Ptr(int64(5)),
					},
					ToolUsage: map[string]runners.ToolUsage{
						"validator": {
							CallCount:     testutils.Ptr(int64(15)),
							TotalDuration: testutils.Ptr(7*time.Second + 250*time.Millisecond),
						},
						"data_formatter": {
							CallCount: testutils.Ptr(int64(1)),
							// Only call count
						},
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000002",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-failure",
			Kind:     runners.Failure,
			Duration: 10 * time.Second,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-failure",
				Model: "gpt-4o-mini",
			},
			Want: utils.NewValueSet("Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."),
			Got:  "Ipsam ea et optio explicabo eius et.",
			Details: runners.Details{
				Answer: runners.AnswerDetails{
					Title:          "Generatio Responsi",
					Explanation:    []string{"Ut eos eius modi nihil voluptatem error.", "Veniam omnis at possimus aliquid tempore.", "Ut voluptatem ullam et ea non beatae eos adipisci incidunt.", "Consequatur hic sint laboriosam maiores unde vero ipsum magnam."},
					ActualAnswer:   []string{"Ipsam ea et optio explicabo eius et."},
					ExpectedAnswer: [][]string{{"Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."}},
					Usage: runners.TokenUsage{
						InputTokens:           testutils.Ptr(int64(200)),
						InputCacheWriteTokens: testutils.Ptr(int64(100)),
					},
					ToolUsage: map[string]runners.ToolUsage{
						"json_parser": {
							TotalDuration: testutils.Ptr(55 * time.Millisecond),
							// Only duration
						},
					},
				},
				Validation: runners.ValidationDetails{
					Title:       "Validatio Defecit",
					Explanation: []string{"At vero eos et accusamus et iusto odio dignissimos ducimus qui."},
					Semantic: &runners.SemanticValidationDetails{
						Verdict:   map[string]interface{}{"correct": false},
						JudgeName: "mistral-judge",
						Provider:  "mistralai",
						Variant:   "reasoning",
						VariantConfig: runners.RunConfigSnapshot{
							Name:  "reasoning",
							Model: "mistral-large-latest",
						},
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000003",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-success-multiple-answers",
			Kind:     runners.Success,
			Duration: 17 * time.Second,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-success-multiple-answers",
				Model: "gpt-4o",
			},
			Want: utils.NewValueSet("Deserunt quo sint minus eos officiis et.", "Quos aut rerum quaerat qui ad culpa."),
			Got:  "Quos aut rerum quaerat qui ad culpa.",
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
			TraceID:  "01JEDE7Z8X0000000000000004",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-failure-multiple-answers",
			Kind:     runners.Failure,
			Duration: 3*time.Minute + 800*time.Millisecond,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-failure-multiple-answers",
				Model: "gpt-4o-mini",
			},
			Want: utils.NewValueSet("Dolores saepe ad sed rerum autem iure minima et.", "Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."),
			Got:  "Ipsam ea et optio explicabo eius et.",
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
					ToolUsage: map[string]runners.ToolUsage{
						"spell_checker": {
							CallCount:     testutils.Ptr(int64(847)),
							TotalDuration: testutils.Ptr(3*time.Minute + 42*time.Second + 300*time.Millisecond),
						},
						"grammar_analyzer": {
							TotalDuration: testutils.Ptr(12 * time.Second),
							// Only duration
						},
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000005",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-error",
			Kind:     runners.Error,
			Duration: 0 * time.Second,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-error",
				Model: "claude-3-7-sonnet",
			},
			Want: utils.NewValueSet("Cum et rem."),
			Got:  "error message",
			Details: runners.Details{
				Answer:     runners.AnswerDetails{},
				Validation: runners.ValidationDetails{},
				Error: runners.ErrorDetails{
					Title:   "Errorem Executionis",
					Message: "Temporibus autem quibusdam et aut officiis debitis aut rerum necessitatibus.",
					Details: nil,
					Usage: runners.TokenUsage{
						InputTokens:           testutils.Ptr(int64(0)),
						InputCacheWriteTokens: testutils.Ptr(int64(0)),
						InputCacheReadTokens:  testutils.Ptr(int64(0)),
						InputTokenAccounting:  runners.InputTokenAccountingCacheTokensSeparate,
						OutputTokens:          testutils.Ptr(int64(0)),
					},
					ToolUsage: map[string]runners.ToolUsage{
						"file_reader": {
							CallCount:     testutils.Ptr(int64(3456)),
							TotalDuration: testutils.Ptr(25*time.Minute + 12*time.Second + 100*time.Millisecond),
						},
						"config_loader": {
							CallCount: testutils.Ptr(int64(92)),
							// Only call count
						},
					},
					Transient: testutils.Ptr(true),
				},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000006",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-not-supported",
			Kind:     runners.NotSupported,
			Duration: 500 * time.Millisecond,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-not-supported",
				Model: "claude-4-sonnet",
			},
			Want: utils.NewValueSet("Animi aut eligendi repellendus debitis harum aut."),
			Got:  "Sequi molestiae iusto sit sit dolorum aut.",
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
					Transient: testutils.Ptr(false),
				},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000007",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-validation-error",
			Kind:     runners.Error,
			Duration: 2 * time.Second,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-validation-error",
				Model: "gemini-2.5-flash",
			},
			Want: utils.NewValueSet("Lorem ipsum dolor sit amet consectetur."),
			Got:  "Adipiscing elit sed do eiusmod tempor.",
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
					FromValidation: true,
				},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000008",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-parsing-error",
			Kind:     runners.Error,
			Duration: 314159 * time.Millisecond,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-parsing-error",
				Model: "mistral-small",
			},
			Want: utils.NewValueSet("Sed do eiusmod tempor incididunt ut."),
			Got:  "Invalid JSON: {broken",
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
					ResponseParsing: testutils.Ptr(true),
				},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000009",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-structured-success",
			Kind:     runners.Success,
			Duration: 42 * time.Second,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-structured-success",
				Model: "gpt-4.1",
			},
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
					ToolUsage: map[string]runners.ToolUsage{
						"json_parser": {
							CallCount:     testutils.Ptr(int64(2)),
							TotalDuration: testutils.Ptr(50 * time.Millisecond),
						},
						"validator": {
							CallCount:     testutils.Ptr(int64(1)),
							TotalDuration: testutils.Ptr(25 * time.Millisecond),
						},
					},
				},
				Validation: runners.ValidationDetails{
					Title:       "Structured Validation Success",
					Explanation: []string{"JSON structure matches expected schema.", "All required fields present with correct types.", "Deep equality comparison passed."},
					Usage: runners.TokenUsage{
						InputTokens:  testutils.Ptr(int64(78)),
						OutputTokens: testutils.Ptr(int64(12)),
					},
					ToolUsage: map[string]runners.ToolUsage{
						"schema_validator": {
							CallCount:     testutils.Ptr(int64(1)),
							TotalDuration: testutils.Ptr(15 * time.Millisecond),
						},
					},
				},
				Error: runners.ErrorDetails{},
			},
		},
		{
			TraceID:  "01JEDE7Z8X0000000000000010",
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-structured-failure",
			Kind:     runners.Failure,
			Duration: 38 * time.Second,
			RunConfig: runners.RunConfigSnapshot{
				Name:  "run-structured-failure",
				Model: "grok-3-mini",
			},
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

func TestUpdateGoldenCSV(t *testing.T) {
	updateGoldenFiles(t, NewCSVFormatter(), []goldenFileTestCase{
		{"testdata/empty.csv", runners.Results{}},
		{"testdata/results.csv", mockResults},
	})
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
