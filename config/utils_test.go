// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
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
			name: "unsupported run model extra params",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`config:
    task-source: "tasks.yaml"
    output-dir: "."
    providers:
        - name: google
          client-config:
              api-key: "604de01b-4b84-4727-8922-0d675541fe7b"
          runs:
              - name: "Wooden"
                model: "quantifying"
                model-parameters:
                    reasoning-effort: "high"
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
									Disabled:             testutils.BoolPtr(true),
									ModelParams: OpenAIModelParams{
										ReasoningEffort:    testutils.StrPtr("high"),
										TextResponseFormat: true,
									},
								},
							},
							Disabled: true,
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
							ExpectedResult:       "Ut quibusdam inventore dolorum velit.\nUllam et dolor laudantium placeat totam dolorem quia.\nEx voluptates et ipsam sunt nulla eos alias sint ad.\n\nDeleniti ducimus natus et omnis expedita.",
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

              Deleniti ducimus natus et omnis expedita.`)),
			},
			want: &Tasks{
				TaskConfig: TaskConfig{
					Disabled: true,
					Tasks: []Task{
						{
							Name:                 "Books neural Automotive",
							Prompt:               "Commodi enim magni.\nEos modi id omnis exercitationem debitis doloremque.\n\nEt atque eius ut.",
							ResponseResultFormat: "Sed unde non.\nVoluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.",
							ExpectedResult:       "Ut quibusdam inventore dolorum velit.\nUllam et dolor laudantium placeat totam dolorem quia.\nEx voluptates et ipsam sunt nulla eos alias sint ad.\n\nDeleniti ducimus natus et omnis expedita.",
							Disabled:             testutils.BoolPtr(false),
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
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func createMockFile(t *testing.T, contents []byte) string {
	return testutils.CreateMockFile(t, "*.test.yaml", contents)
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
						Disabled: testutils.BoolPtr(true),
					},
					{
						Name:     "Security",
						Prompt:   "Liaison",
						Disabled: testutils.BoolPtr(true),
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
						Name:     "Rapid",
						Prompt:   "enable",
						Disabled: testutils.BoolPtr(false),
					},
					{
						Name:     "payment",
						Prompt:   "archive",
						Disabled: testutils.BoolPtr(true),
					},
				},
			},
			want: []Task{
				{
					Name:     "Rapid",
					Prompt:   "enable",
					Disabled: testutils.BoolPtr(false),
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
								Disabled: testutils.BoolPtr(true),
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
								Disabled: testutils.BoolPtr(true),
							},
							{
								Name:     "Human",
								Model:    "back-end",
								Disabled: testutils.BoolPtr(false),
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
								Disabled: testutils.BoolPtr(false),
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
							Disabled: testutils.BoolPtr(false),
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
							Disabled: testutils.BoolPtr(false),
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
				override:    testutils.BoolPtr(false),
				parentValue: false,
			},
			want: false,
		},
		{
			name: "override true, parent value false",
			args: args{
				override:    testutils.BoolPtr(true),
				parentValue: false,
			},
			want: true,
		},
		{
			name: "override false, parent value true",
			args: args{
				override:    testutils.BoolPtr(false),
				parentValue: true,
			},
			want: false,
		},
		{
			name: "override true, parent value true",
			args: args{
				override:    testutils.BoolPtr(true),
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
