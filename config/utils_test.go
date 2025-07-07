// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockDirPathWithPlaceholders = filepath.Join(".", "base", "{{.Year}}", "{{.Month}}", "{{.Day}}", "{{.Hour}}", "{{.Minute}}", "{{.Second}}")

func TestLoadConfigFromFile(t *testing.T) {
	type args struct {
		ctx  context.Context
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    *Config
		wantErr bool
	}{
		{
			name: "file does not exist",
			args: args{
				ctx:  context.Background(),
				path: t.TempDir() + "/unknown.yaml",
			},
			wantErr: true,
		},
		{
			name: "malformed file",
			args: args{
				ctx:  context.Background(),
				path: createMockFile(t, []byte(`{[][][]}`)),
			},
			wantErr: true,
		},
		{
			name: "invalid file",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t, []byte(`config:
    task-source: "tasks.yaml"
    output-dir: "."`)),
			},
			wantErr: true,
		},
		{
			name: "unknown provider",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`config:
    task-source: "tasks.yaml"
    output-dir: "."
    providers:
        - name: unknown
          client-config:
              api-key: "5223bcbd-6939-42d5-989e-23376d12a512"
          runs:
              - name: "repudiandae"
                model: "Profound"
`)),
			},
			wantErr: true,
		},
		{
			name: "invalid run model extra params",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`config:
    task-source: "tasks.yaml"
    output-dir: "."
    providers:
        - name: openai
          client-config:
              api-key: "93e8f51a-89d6-483a-9268-0ec2d0a4c8a2"
          runs:
              - name: "Developer"
                model: "partnerships"
                model-parameters:
                    reasoning-effort: "cdfe8a37-bb9a-4564-a593-67df8f3810e5"
`)),
			},
			wantErr: true,
		},
		{
			name: "extra top-level field",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`config:
    task-source: "tasks.yaml"
    output-dir: "."
    unknown: "solutions"
    providers:
        - name: openai
          client-config:
              api-key: "a8b159e5-ee58-47c6-93d2-f31dcf068e8a"
          runs:
              - name: "Cape"
                model: "Baby"
`)),
			},
			wantErr: true,
		},
		{
			name: "valid file with multiple providers",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`config:
    task-source: "tasks.yaml"
    output-dir: "`+strings.ReplaceAll(mockDirPathWithPlaceholders, `\`, `\\`)+`"
    providers:
        - name: openai
          client-config:
              api-key: "09eca6f7-d51e-45bd-bc5d-2023c624c428"
          runs:
              - name: "Avon"
                model: "protocol"
        - name: google
          client-config:
              api-key: "df2270f9-d4e1-4761-b809-bee219390d00"
          runs:
              - name: "didactic"
                model: "connecting"
        - name: anthropic
          client-config:
              api-key: "c86be894-ad2e-4c7f-b0bd-4397df9f234f"
          runs:
              - name: "innovative"
                model: "Nevada"
        - name: deepseek
          client-config:
              api-key: "b8d40c7c-b169-49a9-9a5c-291741e86daa"
          runs:
              - name: "Afghani"
                model: "Euro"
        - name: mistralai
          client-config:
              api-key: "f1a2b3c4-d5e6-7f8g-9h0i-j1k2l3m4n5o6"
          runs:
              - name: "bypass"
                model: "impactful"
`)),
			},
			want: &Config{
				Config: AppConfig{
					TaskSource: "tasks.yaml",
					OutputDir:  mockDirPathWithPlaceholders,
					Providers: []ProviderConfig{
						{
							Name: "openai",
							ClientConfig: OpenAIClientConfig{
								APIKey: "09eca6f7-d51e-45bd-bc5d-2023c624c428",
							},
							Runs: []RunConfig{
								{
									Name:                 "Avon",
									Model:                "protocol",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
						{
							Name: "google",
							ClientConfig: GoogleAIClientConfig{
								APIKey: "df2270f9-d4e1-4761-b809-bee219390d00",
							},
							Runs: []RunConfig{
								{
									Name:                 "didactic",
									Model:                "connecting",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
						{
							Name: "anthropic",
							ClientConfig: AnthropicClientConfig{
								APIKey: "c86be894-ad2e-4c7f-b0bd-4397df9f234f",
							},
							Runs: []RunConfig{
								{
									Name:                 "innovative",
									Model:                "Nevada",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
						{
							Name: "deepseek",
							ClientConfig: DeepseekClientConfig{
								APIKey: "b8d40c7c-b169-49a9-9a5c-291741e86daa",
							},
							Runs: []RunConfig{
								{
									Name:                 "Afghani",
									Model:                "Euro",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
						{
							Name: "mistralai",
							ClientConfig: MistralAIClientConfig{
								APIKey: "f1a2b3c4-d5e6-7f8g-9h0i-j1k2l3m4n5o6",
							},
							Runs: []RunConfig{
								{
									Name:                 "bypass",
									Model:                "impactful",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid file with optional values",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`config:
    task-source: "tasks.yaml"
    output-dir: "."
    providers:
        - name: openai
          disabled: true
          client-config:
              api-key: "fb6a325d-03c8-4b22-9bf1-ed0950dcfe34"
          runs:
              - name: "Sports"
                disabled: true
                model: "directional"
                max-requests-per-minute: 3
                model-parameters:
                    reasoning-effort: high
                    text-response-format: true
                    temperature: 0.7
                    top-p: 0.95
                    presence-penalty: 0.1
                    frequency-penalty: 0.1
        - name: anthropic
          client-config:
              api-key: "c86be894-ad2e-4c7f-b0bd-4397df9f234f"
              request-timeout: 30s
          runs:
              - name: "Claude"
                model: "claude-3"
                model-parameters:
                    max-tokens: 4096
                    thinking-budget-tokens: 2048
                    temperature: 0.7
                    top-p: 0.95
                    top-k: 40
        - name: deepseek
          client-config:
              api-key: "b8d40c7c-b169-49a9-9a5c-291741e86daa"
              request-timeout: 45s
          runs:
              - name: "DeepSeek"
                model: "deepseek-coder"
                model-parameters:
                    temperature: 0.7
                    top-p: 0.95
                    presence-penalty: 0.1
                    frequency-penalty: 0.1
        - name: google
          client-config:
              api-key: "df2270f9-d4e1-4761-b809-bee219390d00"
          runs:
              - name: "Gemini"
                model: "gemini-pro"
                model-parameters:
                    text-response-format: true
                    temperature: 0.7
                    top-p: 0.95
                    top-k: 40
        - name: mistralai
          client-config:
              api-key: "f1a2b3c4-d5e6-7f8g-9h0i-j1k2l3m4n5o6"
          runs:
              - name: "Mistral"
                model: "mistral-large"
                model-parameters:
                    temperature: 0.8
                    top-p: 0.9
                    max-tokens: 2048
                    presence-penalty: 0.2
                    frequency-penalty: 0.2
                    random-seed: 42
                    prompt-mode: reasoning
                    safe-prompt: true
`)),
			},
			want: &Config{
				Config: AppConfig{
					TaskSource: "tasks.yaml",
					OutputDir:  ".",
					Providers: []ProviderConfig{
						{
							Name: "openai",
							ClientConfig: OpenAIClientConfig{
								APIKey: "fb6a325d-03c8-4b22-9bf1-ed0950dcfe34",
							},
							Runs: []RunConfig{
								{
									Name:                 "Sports",
									Model:                "directional",
									MaxRequestsPerMinute: 3,
									Disabled:             testutils.Ptr(true),
									ModelParams: OpenAIModelParams{
										ReasoningEffort:    testutils.Ptr("high"),
										TextResponseFormat: true,
										Temperature:        testutils.Ptr(float32(0.7)),
										TopP:               testutils.Ptr(float32(0.95)),
										PresencePenalty:    testutils.Ptr(float32(0.1)),
										FrequencyPenalty:   testutils.Ptr(float32(0.1)),
									},
								},
							},
							Disabled: true,
						},
						{
							Name: "anthropic",
							ClientConfig: AnthropicClientConfig{
								APIKey:         "c86be894-ad2e-4c7f-b0bd-4397df9f234f",
								RequestTimeout: testutils.Ptr(30 * time.Second),
							},
							Runs: []RunConfig{
								{
									Name:                 "Claude",
									Model:                "claude-3",
									MaxRequestsPerMinute: 0,
									ModelParams: AnthropicModelParams{
										MaxTokens:            testutils.Ptr(int64(4096)),
										ThinkingBudgetTokens: testutils.Ptr(int64(2048)),
										Temperature:          testutils.Ptr(float64(0.7)),
										TopP:                 testutils.Ptr(float64(0.95)),
										TopK:                 testutils.Ptr(int64(40)),
									},
								},
							},
							Disabled: false,
						},
						{
							Name: "deepseek",
							ClientConfig: DeepseekClientConfig{
								APIKey:         "b8d40c7c-b169-49a9-9a5c-291741e86daa",
								RequestTimeout: testutils.Ptr(45 * time.Second),
							},
							Runs: []RunConfig{
								{
									Name:                 "DeepSeek",
									Model:                "deepseek-coder",
									MaxRequestsPerMinute: 0,
									ModelParams: DeepseekModelParams{
										Temperature:      testutils.Ptr(float32(0.7)),
										TopP:             testutils.Ptr(float32(0.95)),
										PresencePenalty:  testutils.Ptr(float32(0.1)),
										FrequencyPenalty: testutils.Ptr(float32(0.1)),
									},
								},
							},
							Disabled: false,
						},
						{
							Name: "google",
							ClientConfig: GoogleAIClientConfig{
								APIKey: "df2270f9-d4e1-4761-b809-bee219390d00",
							},
							Runs: []RunConfig{
								{
									Name:                 "Gemini",
									Model:                "gemini-pro",
									MaxRequestsPerMinute: 0,
									ModelParams: GoogleAIModelParams{
										TextResponseFormat: true,
										Temperature:        testutils.Ptr(float32(0.7)),
										TopP:               testutils.Ptr(float32(0.95)),
										TopK:               testutils.Ptr(int32(40)),
									},
								},
							},
							Disabled: false,
						},
						{
							Name: "mistralai",
							ClientConfig: MistralAIClientConfig{
								APIKey: "f1a2b3c4-d5e6-7f8g-9h0i-j1k2l3m4n5o6",
							},
							Runs: []RunConfig{
								{
									Name:                 "Mistral",
									Model:                "mistral-large",
									MaxRequestsPerMinute: 0,
									ModelParams: MistralAIModelParams{
										Temperature:      testutils.Ptr(float32(0.8)),
										TopP:             testutils.Ptr(float32(0.9)),
										MaxTokens:        testutils.Ptr(int32(2048)),
										PresencePenalty:  testutils.Ptr(float32(0.2)),
										FrequencyPenalty: testutils.Ptr(float32(0.2)),
										RandomSeed:       testutils.Ptr(int32(42)),
										PromptMode:       testutils.Ptr("reasoning"),
										SafePrompt:       testutils.Ptr(true),
									},
								},
							},
							Disabled: false,
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadConfigFromFile(tt.args.ctx, tt.args.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestLoadTasksFromFile(t *testing.T) {
	type args struct {
		ctx  context.Context
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    *Tasks
		wantErr bool
	}{
		{
			name: "file does not exist",
			args: args{
				ctx:  context.Background(),
				path: t.TempDir() + "/unknown.yaml",
			},
			wantErr: true,
		},
		{
			name: "malformed file",
			args: args{
				ctx:  context.Background(),
				path: createMockFile(t, []byte(`{[][][]}`)),
			},
			wantErr: true,
		},
		{
			name: "invalid file",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t, []byte(`task-config:
    tasks:
	    - name: ""`)),
			},
			wantErr: true,
		},
		{
			name: "task with duplicate file names",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    tasks:
        - name: "Books neural Automotive"
          prompt: |-
              Commodi enim magni.
              Eos modi id omnis exercitationem debitis doloremque.

              Et atque eius ut.
          response-result-format: |-
              Sed unde non.
              Voluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.
          expected-result: |-
              Ut quibusdam inventore dolorum velit.
              Ullam et dolor laudantium placeat totam dolorem quia.
              Ex voluptates et ipsam sunt nulla eos alias sint ad.

              Deleniti ducimus natus et omnis expedita.
          files:
            - name: "file"
              url: "path/to/file.txt"
            - name: "file"
              url: "http://example.com/file.txt"`)),
			},
			wantErr: true,
		},
		{
			name: "valid file",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    tasks:
        - name: "Books neural Automotive"
          prompt: |-
              Commodi enim magni.
              Eos modi id omnis exercitationem debitis doloremque.

              Et atque eius ut.
          response-result-format: |-
              Sed unde non.
              Voluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.
          expected-result: |-
              Ut quibusdam inventore dolorum velit.
              Ullam et dolor laudantium placeat totam dolorem quia.
              Ex voluptates et ipsam sunt nulla eos alias sint ad.

              Deleniti ducimus natus et omnis expedita.`)),
			},
			want: &Tasks{
				TaskConfig: TaskConfig{
					Tasks: []Task{
						{
							Name:                 "Books neural Automotive",
							Prompt:               "Commodi enim magni.\nEos modi id omnis exercitationem debitis doloremque.\n\nEt atque eius ut.",
							ResponseResultFormat: "Sed unde non.\nVoluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.",
							ExpectedResult:       utils.NewStringSet("Ut quibusdam inventore dolorum velit.\nUllam et dolor laudantium placeat totam dolorem quia.\nEx voluptates et ipsam sunt nulla eos alias sint ad.\n\nDeleniti ducimus natus et omnis expedita."),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid file with optional values",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    disabled: true
    tasks:
        - name: "Books neural Automotive"
          disabled: false
          prompt: |-
              Commodi enim magni.
              Eos modi id omnis exercitationem debitis doloremque.

              Et atque eius ut.
          response-result-format: |-
              Sed unde non.
              Voluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.
          expected-result: |-
              Ut quibusdam inventore dolorum velit.
              Ullam et dolor laudantium placeat totam dolorem quia.
              Ex voluptates et ipsam sunt nulla eos alias sint ad.

              Deleniti ducimus natus et omnis expedita.
          files:
            - name: "local-file"
              uri: "path/to/file.txt"
              type: "text"
            - name: "remote-file"
              uri: "http://example.com/file.txt"
              type: "text"`)),
			},
			want: &Tasks{
				TaskConfig: TaskConfig{
					Disabled: true,
					Tasks: []Task{
						{
							Name:                 "Books neural Automotive",
							Prompt:               "Commodi enim magni.\nEos modi id omnis exercitationem debitis doloremque.\n\nEt atque eius ut.",
							ResponseResultFormat: "Sed unde non.\nVoluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.",
							ExpectedResult:       utils.NewStringSet("Ut quibusdam inventore dolorum velit.\nUllam et dolor laudantium placeat totam dolorem quia.\nEx voluptates et ipsam sunt nulla eos alias sint ad.\n\nDeleniti ducimus natus et omnis expedita."),
							Files: []TaskFile{
								mockTaskFile(t, "local-file", "path/to/file.txt", "text"),
								mockTaskFile(t, "remote-file", "http://example.com/file.txt", "text"),
							},
							Disabled: testutils.Ptr(false),
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadTasksFromFile(tt.args.ctx, tt.args.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				for i := range got.TaskConfig.Tasks {
					for j := range got.TaskConfig.Tasks[i].Files {
						assert.NotNil(t, got.TaskConfig.Tasks[i].Files[j].content)
						assert.NotNil(t, got.TaskConfig.Tasks[i].Files[j].base64)
						assert.NotNil(t, got.TaskConfig.Tasks[i].Files[j].typeValue)

						// Reset the private fields to nil for comparison.
						got.TaskConfig.Tasks[i].Files[j].content = nil
						got.TaskConfig.Tasks[i].Files[j].base64 = nil
						got.TaskConfig.Tasks[i].Files[j].typeValue = nil
					}
				}
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func createMockFile(t *testing.T, contents []byte) string {
	return testutils.CreateMockFile(t, "*.test.yaml", contents)
}

func mockTaskFile(t *testing.T, name string, uri string, mimeType string) TaskFile {
	file := TaskFile{
		Name: name,
		Type: mimeType,
	}
	require.NoError(t, file.URI.Parse(uri))
	return file
}

func TestIsNotBlank(t *testing.T) {
	type args struct {
		value string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "empty string",
			args: args{
				value: "",
			},
			want: false,
		},
		{
			name: "space",
			args: args{
				value: " ",
			},
			want: false,
		},
		{
			name: "multi-space",
			args: args{
				value: " \t \t  ",
			},
			want: false,
		},
		{
			name: "value",
			args: args{
				value: "Ball Networked",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNotBlank(tt.args.value))
		})
	}
}

func TestResolveFileNamePattern(t *testing.T) {
	mockTimeRef := time.Date(2025, 03, 04, 22, 10, 0, 0, time.Local)
	type args struct {
		pattern string
		timeRef time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "year-month-day",
			args: args{
				pattern: "{{.Year}}-{{.Month}}-{{.Day}}",
				timeRef: mockTimeRef,
			},
			want: mockTimeRef.Format("2006-01-02"),
		},
		{
			name: "full date and time",
			args: args{
				pattern: "{{.Year}}-{{.Month}}-{{.Day}}_{{.Hour}}-{{.Minute}}-{{.Second}}",
				timeRef: mockTimeRef,
			},
			want: mockTimeRef.Format("2006-01-02_15-04-05"),
		},
		{
			name: "no format pattern",
			args: args{
				pattern: "results.txt",
				timeRef: mockTimeRef,
			},
			want: "results.txt",
		},
		{
			name: "custom format",
			args: args{
				pattern: "results-{{.Year}}-{{.Month}}-{{.Day}}_{{.Hour}}-{{.Minute}}.txt",
				timeRef: mockTimeRef,
			},
			want: mockTimeRef.Format("results-2006-01-02_15-04.txt"),
		},
		{
			name: "custom format in path",
			args: args{
				pattern: "/data/2006/20250208/{{.Year}}/{{.Month}}-{{.Day}}/results-{{.Hour}}-{{.Minute}}.txt",
				timeRef: mockTimeRef,
			},
			want: "/data/2006/20250208/" + mockTimeRef.Format("2006/01-02/results-15-04.txt"),
		},
		{
			name: "invalid format",
			args: args{
				pattern: "results-{{.Unknown}}.txt",
				timeRef: mockTimeRef,
			},
			want: "results-{{.Unknown}}.txt",
		},
		{
			name: "invalid template",
			args: args{
				pattern: "results-{.Year}.txt",
				timeRef: mockTimeRef,
			},
			want: "results-{.Year}.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveFileNamePattern(tt.args.pattern, tt.args.timeRef))
		})
	}
}

func TestGetEnabledTasks(t *testing.T) {
	tests := []struct {
		name string
		ts   TaskConfig
		want []Task
	}{
		{
			name: "no tasks",
			ts: TaskConfig{
				Tasks: []Task{},
			},
			want: []Task{},
		},
		{
			name: "all tasks disabled",
			ts: TaskConfig{
				Disabled: true,
				Tasks: []Task{
					{
						Name:   "Lev",
						Prompt: "Czech",
					},
					{
						Name:   "Arkansas",
						Prompt: "orchestrate",
					},
				},
			},
			want: []Task{},
		},
		{
			name: "all tasks disabled individually",
			ts: TaskConfig{
				Disabled: false,
				Tasks: []Task{
					{
						Name:     "management",
						Prompt:   "Ball",
						Disabled: testutils.Ptr(true),
					},
					{
						Name:     "Security",
						Prompt:   "Liaison",
						Disabled: testutils.Ptr(true),
					},
				},
			},
			want: []Task{},
		},
		{
			name: "some tasks enabled",
			ts: TaskConfig{
				Disabled: true,
				Tasks: []Task{
					{
						Name:   "Markets",
						Prompt: "SMS",
					},
					{
						Name:                 "Rapid",
						Prompt:               "enable",
						ResponseResultFormat: "generating",
						ExpectedResult:       utils.NewStringSet("Account"),
						Disabled:             testutils.Ptr(false),
						Files: []TaskFile{
							mockTaskFile(t, "mock file", "http://example.com/file.txt", "text"),
						},
					},
					{
						Name:     "payment",
						Prompt:   "archive",
						Disabled: testutils.Ptr(true),
					},
				},
			},
			want: []Task{
				{
					Name:                 "Rapid",
					Prompt:               "enable",
					ResponseResultFormat: "generating",
					ExpectedResult:       utils.NewStringSet("Account"),
					Disabled:             testutils.Ptr(false),
					Files: []TaskFile{
						mockTaskFile(t, "mock file", "http://example.com/file.txt", "text"),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.ts.GetEnabledTasks())
		})
	}
}

func TestGetProvidersWithEnabledRuns(t *testing.T) {
	tests := []struct {
		name string
		ac   AppConfig
		want []ProviderConfig
	}{
		{
			name: "no providers",
			ac: AppConfig{
				Providers: []ProviderConfig{},
			},
			want: []ProviderConfig{},
		},
		{
			name: "some providers",
			ac: AppConfig{
				Providers: []ProviderConfig{
					{
						Name:     "disabled provider",
						Disabled: true,
						Runs: []RunConfig{
							{
								Name:  "Berkshire",
								Model: "West",
							},
						},
					},
					{
						Name:     "provider with all configurations disabled",
						Disabled: false,
						Runs: []RunConfig{
							{
								Name:     "cross-platform",
								Model:    "invoice",
								Disabled: testutils.Ptr(true),
							},
						},
					},
					{
						Name:     "provider with some configurations enabled",
						Disabled: true,
						Runs: []RunConfig{
							{
								Name:     "Danish",
								Model:    "Soft",
								Disabled: testutils.Ptr(true),
							},
							{
								Name:     "Human",
								Model:    "back-end",
								Disabled: testutils.Ptr(false),
							},
							{
								Name:  "Colorado",
								Model: "extranet",
							},
						},
					},
					{
						Name:     "provider with all configurations enabled",
						Disabled: false,
						Runs: []RunConfig{
							{
								Name:     "Executive",
								Model:    "Garden",
								Disabled: testutils.Ptr(false),
							},
							{
								Name:  "Pants",
								Model: "implement",
							},
						},
					},
				},
			},
			want: []ProviderConfig{
				{
					Name:     "provider with some configurations enabled",
					Disabled: true,
					Runs: []RunConfig{
						{
							Name:     "Human",
							Model:    "back-end",
							Disabled: testutils.Ptr(false),
						},
					},
				},
				{
					Name:     "provider with all configurations enabled",
					Disabled: false,
					Runs: []RunConfig{
						{
							Name:     "Executive",
							Model:    "Garden",
							Disabled: testutils.Ptr(false),
						},
						{
							Name:  "Pants",
							Model: "implement",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.ac.GetProvidersWithEnabledRuns())
		})
	}
}
func TestResolveFlagOverride(t *testing.T) {
	type args struct {
		override    *bool
		parentValue bool
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nil override, parent value false",
			args: args{
				override:    nil,
				parentValue: false,
			},
			want: false,
		},
		{
			name: "nil override, parent value true",
			args: args{
				override:    nil,
				parentValue: true,
			},
			want: true,
		},
		{
			name: "override false, parent value false",
			args: args{
				override:    testutils.Ptr(false),
				parentValue: false,
			},
			want: false,
		},
		{
			name: "override true, parent value false",
			args: args{
				override:    testutils.Ptr(true),
				parentValue: false,
			},
			want: true,
		},
		{
			name: "override false, parent value true",
			args: args{
				override:    testutils.Ptr(false),
				parentValue: true,
			},
			want: false,
		},
		{
			name: "override true, parent value true",
			args: args{
				override:    testutils.Ptr(true),
				parentValue: true,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveFlagOverride(tt.args.override, tt.args.parentValue))
		})
	}
}

func TestMakeAbs(t *testing.T) {
	tests := []struct {
		name       string
		baseDir    string
		filePath   string
		wantResult string
	}{
		{
			name:       "absolute file path",
			baseDir:    os.TempDir(),
			filePath:   filepath.Join(os.TempDir(), "absolute", "path", "file.txt"),
			wantResult: filepath.Join(os.TempDir(), "absolute", "path", "file.txt"),
		},
		{
			name:       "relative file path",
			baseDir:    os.TempDir(),
			filePath:   filepath.Join("relative", "path", "file.txt"),
			wantResult: filepath.Join(os.TempDir(), "relative", "path", "file.txt"),
		},
		{
			name:       "blank file path",
			baseDir:    os.TempDir(),
			filePath:   "",
			wantResult: "",
		},
		{
			name:       "file path with spaces",
			baseDir:    os.TempDir(),
			filePath:   "  ",
			wantResult: "  ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantResult, MakeAbs(tt.baseDir, tt.filePath))
		})
	}
}

func TestCleanIfNotBlank(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "empty string",
			filePath: "",
			want:     "",
		},
		{
			name:     "blank string",
			filePath: "   ",
			want:     "   ",
		},
		{
			name:     "valid path",
			filePath: "path/to/file.txt",
			want:     filepath.Join("path", "to", "file.txt"),
		},
		{
			name:     "path with redundant elements",
			filePath: "path/./to/../to/file.txt",
			want:     filepath.Join("path", "to", "file.txt"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CleanIfNotBlank(tt.filePath))
		})
	}
}

//nolint:staticcheck,errcheck,err113
func TestOnceWithContext(t *testing.T) {
	newOnceFunc := func() func(context.Context, *int) (int, error) {
		return OnceWithContext(func(ctx context.Context, state *int) (int, error) {
			if e := ctx.Value("error"); e != nil {
				return *state, e.(error)
			} else if p := ctx.Value("panic"); p != nil {
				panic(p.(string))
			}

			*state++
			return *state, nil
		})
	}

	ctx := context.Background()

	t.Run("with result", func(t *testing.T) {
		counter := testutils.Ptr(0)

		wrapped := newOnceFunc()
		got, err := wrapped(ctx, counter)
		require.NoError(t, err)
		require.Equal(t, 1, got)

		got, err = wrapped(ctx, counter)
		require.NoError(t, err)
		require.Equal(t, 1, got)

		got, err = wrapped(context.WithValue(ctx, "error", errors.New("mock error")), counter)
		require.NoError(t, err)
		require.Equal(t, 1, got)

		got, err = wrapped(context.WithValue(ctx, "panic", "mock panic"), counter)
		require.NoError(t, err)
		require.Equal(t, 1, got)

		assert.Equal(t, 1, *counter)
	})

	t.Run("with error", func(t *testing.T) {
		counter := testutils.Ptr(17)

		wantErr := errors.New("mock error")
		wrapped := newOnceFunc()
		got, err := wrapped(context.WithValue(ctx, "error", wantErr), counter)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, 17, got)

		got, err = wrapped(ctx, counter)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, 17, got)

		got, err = wrapped(context.WithValue(ctx, "error", errors.New("other error")), counter)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, 17, got)

		got, err = wrapped(context.WithValue(ctx, "panic", "mock panic"), counter)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, 17, got)

		assert.Equal(t, 17, *counter)
	})

	t.Run("with panic", func(t *testing.T) {
		counter := testutils.Ptr(-1)

		wantPanic := "mock panic"
		wrapped := newOnceFunc()
		require.PanicsWithValue(t, wantPanic, func() {
			wrapped(context.WithValue(ctx, "panic", wantPanic), counter)
		})

		require.PanicsWithValue(t, wantPanic, func() {
			wrapped(ctx, counter)
		})

		require.PanicsWithValue(t, wantPanic, func() {
			wrapped(context.WithValue(ctx, "error", errors.New("mock error")), counter)
		})

		require.PanicsWithValue(t, wantPanic, func() {
			wrapped(context.WithValue(ctx, "panic", "other panic"), counter)
		})

		assert.Equal(t, -1, *counter)
	})
}
