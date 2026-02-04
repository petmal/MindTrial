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

func TestJudgeConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		judge   JudgeConfig
		wantErr bool
	}{
		{
			name: "valid judge without disable-structured-output",
			judge: JudgeConfig{
				Name: "test-judge",
				Provider: ProviderConfig{
					Name: "openai",
					Runs: []RunConfig{
						{
							Name:  "default",
							Model: "gpt-4o",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid judge with disable-structured-output enabled",
			judge: JudgeConfig{
				Name: "test-judge",
				Provider: ProviderConfig{
					Name: "openai",
					Runs: []RunConfig{
						{
							Name:                    "default",
							Model:                   "gpt-4o",
							DisableStructuredOutput: true,
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid judge with disable-structured-output explicitly false",
			judge: JudgeConfig{
				Name: "test-judge",
				Provider: ProviderConfig{
					Name: "openai",
					Runs: []RunConfig{
						{
							Name:                    "default",
							Model:                   "gpt-4o",
							DisableStructuredOutput: false,
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.judge.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidJudgeVariant)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

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
			name: "provider with duplicate run names",
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
              api-key: "a8b159e5-ee58-47c6-93d2-f31dcf068e8a"
          runs:
              - name: "Duplicate Run"
                model: "gpt-4"
              - name: "Duplicate Run"
                model: "gpt-3.5-turbo"
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
    - name: openrouter
      client-config:
          api-key: "sk-openrouter-test-key"
      runs:
        - name: "Router"
          model: "openai/gpt-4"
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
    - name: xai
      client-config:
          api-key: "49bdde73-d1bf-4a69-8bf6-a73c80fc8008"
      runs:
          - name: "Vision"
            model: "interface"
    - name: alibaba
      client-config:
          api-key: "sk-alibaba-test-key"
      runs:
          - name: "Qwen"
            model: "qwen-turbo"
    - name: moonshotai
      client-config:
          api-key: "sk-moonshot-test-key"
      runs:
          - name: "Kimi"
            model: "kimi-k2"
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
							Name: "openrouter",
							ClientConfig: OpenRouterClientConfig{
								APIKey: "sk-openrouter-test-key",
							},
							Runs: []RunConfig{
								{
									Name:                 "Router",
									Model:                "openai/gpt-4",
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
						{
							Name: "xai",
							ClientConfig: XAIClientConfig{
								APIKey: "49bdde73-d1bf-4a69-8bf6-a73c80fc8008",
							},
							Runs: []RunConfig{
								{
									Name:                 "Vision",
									Model:                "interface",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
						{
							Name: "alibaba",
							ClientConfig: AlibabaClientConfig{
								APIKey: "sk-alibaba-test-key",
							},
							Runs: []RunConfig{
								{
									Name:                 "Qwen",
									Model:                "qwen-turbo",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
						{
							Name: "moonshotai",
							ClientConfig: MoonshotAIClientConfig{
								APIKey: "sk-moonshot-test-key",
							},
							Runs: []RunConfig{
								{
									Name:                 "Kimi",
									Model:                "kimi-k2",
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
                disable-structured-output: true
                model-parameters:
                    reasoning-effort: high
                    text-response-format: true
                    temperature: 0.7
                    top-p: 0.95
                    presence-penalty: 0.1
                    frequency-penalty: 0.1
                    max-completion-tokens: 4096
        - name: openrouter
          client-config:
              api-key: "sk-openrouter-optional-test"
          runs:
              - name: "OpenRouter GPT"
                model: "openai/gpt-5"
                model-parameters:
                    response-format: json-object
                    temperature: 0.7
                    top-p: 0.95
                    top-k: 40
                    min-p: 0.05
                    top-a: 0.1
                    presence-penalty: 0.1
                    frequency-penalty: 0.2
                    repetition-penalty: 1.1
                    max-tokens: 2048
                    seed: 42
                    parallel-tool-calls: false
                    verbosity: low
                    provider:
                        order:
                          - "OpenAI"
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
                    text-response-format-with-tools: true
                    thinking-level: high
                    media-resolution: medium
                    temperature: 0.7
                    top-p: 0.95
                    top-k: 40
                    presence-penalty: 0.1
                    frequency-penalty: 0.1
                    seed: 42
        - name: mistralai
          client-config:
              api-key: "f1a2b3c4-d5e6-7f8g-9h0i-j1k2l3m4n5o6"
          retry-policy:
              max-retry-attempts: 5
              initial-delay-seconds: 10
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
                retry-policy:
                    max-retry-attempts: 3
                    initial-delay-seconds: 1
        - name: xai
          client-config:
              api-key: "b990bc70-169c-4de8-8dd1-fd4253527046"
          runs:
              - name: "Grok 4"
                model: "grok4-latest"
                model-parameters:
                    temperature: 0.5
                    top-p: 0.9
                    max-completion-tokens: 1024
                    presence-penalty: 0.1
                    frequency-penalty: 0.2
                    reasoning-effort: low
                    seed: 42
        - name: alibaba
          client-config:
              api-key: "sk-alibaba-test-endpoint"
              endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1"
          runs:
              - name: "Qwen Beijing"
                model: "qwen-turbo"
                model-parameters:
                    temperature: 0.8
                    top-p: 0.9
                    presence-penalty: 0.1
                    frequency-penalty: 0.2
                    max-tokens: 2048
                    seed: 12345
                    disable-legacy-json-mode: true
                    stream: true
        - name: moonshotai
          client-config:
              api-key: "sk-moonshot-test-key"
              endpoint: "https://api.moonshot.6ee41fa3-1279-4f8c-a348-c9a00c6f5b06.ai/v1"
          runs:
              - name: "Kimi K2"
                model: "kimi-k2"
                text-only: true
                model-parameters:
                    temperature: 0.6
                    top-p: 0.9
                    max-tokens: 2048
                    presence-penalty: 0.1
                    frequency-penalty: 0.2
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
									Name:                    "Sports",
									Model:                   "directional",
									MaxRequestsPerMinute:    3,
									Disabled:                testutils.Ptr(true),
									DisableStructuredOutput: true,
									ModelParams: OpenAIModelParams{
										ReasoningEffort:     testutils.Ptr("high"),
										TextResponseFormat:  true,
										Temperature:         testutils.Ptr(float32(0.7)),
										TopP:                testutils.Ptr(float32(0.95)),
										PresencePenalty:     testutils.Ptr(float32(0.1)),
										FrequencyPenalty:    testutils.Ptr(float32(0.1)),
										MaxCompletionTokens: testutils.Ptr(int32(4096)),
									},
								},
							},
							Disabled: true,
						},
						{
							Name: "openrouter",
							ClientConfig: OpenRouterClientConfig{
								APIKey: "sk-openrouter-optional-test",
							},
							Runs: []RunConfig{
								{
									Name:                 "OpenRouter GPT",
									Model:                "openai/gpt-5",
									MaxRequestsPerMinute: 0,
									ModelParams: OpenRouterModelParams{
										ResponseFormat:    testutils.Ptr(ModelResponseFormatJSONObject),
										Temperature:       testutils.Ptr(float32(0.7)),
										TopP:              testutils.Ptr(float32(0.95)),
										TopK:              testutils.Ptr(int32(40)),
										MinP:              testutils.Ptr(float32(0.05)),
										TopA:              testutils.Ptr(float32(0.1)),
										PresencePenalty:   testutils.Ptr(float32(0.1)),
										FrequencyPenalty:  testutils.Ptr(float32(0.2)),
										RepetitionPenalty: testutils.Ptr(float32(1.1)),
										MaxTokens:         testutils.Ptr(int32(2048)),
										Seed:              testutils.Ptr(int64(42)),
										ParallelToolCalls: testutils.Ptr(false),
										Verbosity:         testutils.Ptr("low"),
										Extra: map[string]any{
											"provider": map[string]any{
												"order": []any{"OpenAI"},
											},
										},
									},
								},
							},
							Disabled: false,
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
										TextResponseFormat:          true,
										TextResponseFormatWithTools: true,
										ThinkingLevel:               testutils.Ptr("high"),
										MediaResolution:             testutils.Ptr("medium"),
										Temperature:                 testutils.Ptr(float32(0.7)),
										TopP:                        testutils.Ptr(float32(0.95)),
										TopK:                        testutils.Ptr(int32(40)),
										PresencePenalty:             testutils.Ptr(float32(0.1)),
										FrequencyPenalty:            testutils.Ptr(float32(0.1)),
										Seed:                        testutils.Ptr(int32(42)),
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
							RetryPolicy: RetryPolicy{
								MaxRetryAttempts:    5,
								InitialDelaySeconds: 10,
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
									RetryPolicy: &RetryPolicy{
										MaxRetryAttempts:    3,
										InitialDelaySeconds: 1,
									},
								},
							},
							Disabled: false,
						},
						{
							Name: "xai",
							ClientConfig: XAIClientConfig{
								APIKey: "b990bc70-169c-4de8-8dd1-fd4253527046",
							},
							Runs: []RunConfig{
								{
									Name:                 "Grok 4",
									Model:                "grok4-latest",
									MaxRequestsPerMinute: 0,
									ModelParams: XAIModelParams{
										Temperature:         testutils.Ptr(float32(0.5)),
										TopP:                testutils.Ptr(float32(0.9)),
										MaxCompletionTokens: testutils.Ptr(int32(1024)),
										PresencePenalty:     testutils.Ptr(float32(0.1)),
										FrequencyPenalty:    testutils.Ptr(float32(0.2)),
										ReasoningEffort:     testutils.Ptr("low"),
										Seed:                testutils.Ptr(int32(42)),
									},
								},
							},
							Disabled: false,
						},
						{
							Name: "alibaba",
							ClientConfig: AlibabaClientConfig{
								APIKey:   "sk-alibaba-test-endpoint",
								Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1",
							},
							Runs: []RunConfig{
								{
									Name:                 "Qwen Beijing",
									Model:                "qwen-turbo",
									MaxRequestsPerMinute: 0,
									ModelParams: AlibabaModelParams{
										Temperature:           testutils.Ptr(float32(0.8)),
										TopP:                  testutils.Ptr(float32(0.9)),
										PresencePenalty:       testutils.Ptr(float32(0.1)),
										FrequencyPenalty:      testutils.Ptr(float32(0.2)),
										MaxTokens:             testutils.Ptr(int32(2048)),
										Seed:                  testutils.Ptr(uint32(12345)),
										DisableLegacyJsonMode: testutils.Ptr(true),
										Stream:                true,
									},
								},
							},
							Disabled: false,
						},
						{
							Name: "moonshotai",
							ClientConfig: MoonshotAIClientConfig{
								APIKey:   "sk-moonshot-test-key",
								Endpoint: "https://api.moonshot.6ee41fa3-1279-4f8c-a348-c9a00c6f5b06.ai/v1",
							},
							Runs: []RunConfig{
								{
									Name:                 "Kimi K2",
									Model:                "kimi-k2",
									MaxRequestsPerMinute: 0,
									TextOnly:             true,
									ModelParams: MoonshotAIModelParams{
										Temperature:      testutils.Ptr(float32(0.6)),
										TopP:             testutils.Ptr(float32(0.9)),
										MaxTokens:        testutils.Ptr(int32(2048)),
										PresencePenalty:  testutils.Ptr(float32(0.1)),
										FrequencyPenalty: testutils.Ptr(float32(0.2)),
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
		{
			name: "config with tool config",
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
              api-key: "test-key"
          runs:
              - name: "test-run"
                model: "gpt-4"
    tools:
        - name: python-code-executor
          image: python:3.9-slim
          description: "Executes Python code in a container"
          parameters:
            code:
              type: string
              description: "Python code to execute"
          command: ["python", "-c"]
          parameter-files:
            code: "/tmp/code.py"
          auxiliary-dir: "/tmp/data"
          shared-dir: "/tmp/shared"
          env:
            PYTHONPATH: "/usr/local/lib/python3.9"
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
								APIKey: "test-key",
							},
							Runs: []RunConfig{
								{
									Name:                 "test-run",
									Model:                "gpt-4",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
					},
					Tools: []ToolConfig{
						{
							Name:        "python-code-executor",
							Image:       "python:3.9-slim",
							Description: "Executes Python code in a container",
							Parameters: map[string]interface{}{
								"code": map[string]interface{}{
									"type":        "string",
									"description": "Python code to execute",
								},
							},
							Command:        []string{"python", "-c"},
							ParameterFiles: map[string]string{"code": "/tmp/code.py"},
							AuxiliaryDir:   "/tmp/data",
							SharedDir:      "/tmp/shared",
							Env:            map[string]string{"PYTHONPATH": "/usr/local/lib/python3.9"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "config with invalid tool parameter schema",
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
              api-key: "test-key"
          runs:
              - name: "test-run"
                model: "gpt-4"
    tools:
        - name: invalid-tool
          image: python:3.9-slim
          description: "Invalid tool"
          parameters:
            code:
              type: string
              description: "Python code to execute"
            required: "code"
          command: ["python", "-c"]
`)),
			},
			wantErr: true,
		},
		{
			name: "config with duplicate tool names",
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
              api-key: "test-key"
          runs:
              - name: "test-run"
                model: "gpt-4"
    tools:
        - name: duplicate-tool
          image: python:3.9-slim
          description: "First tool"
          parameters:
            code:
              type: string
        - name: duplicate-tool
          image: python:3.9-slim
          description: "Second tool"
          parameters:
            code:
              type: string
`)),
			},
			wantErr: true,
		},
		{
			name: "config with duplicate judge names",
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
              api-key: "primary-key"
          runs:
              - name: "primary"
                model: "gpt-4"
    judges:
        - name: "duplicate-judge"
          provider:
              name: openai
              client-config:
                  api-key: "judge-key-1"
              runs:
                  - name: "default"
                    model: "gpt-4o"
        - name: "duplicate-judge"
          provider:
              name: openai
              client-config:
                  api-key: "judge-key-2"
              runs:
                  - name: "default"
                    model: "gpt-4o"
`)),
			},
			wantErr: true,
		},
		{
			name: "config with judge using disable-structured-output",
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
              api-key: "primary-key"
          runs:
              - name: "primary"
                model: "gpt-4"
    judges:
        - name: "invalid-judge"
          provider:
              name: openai
              client-config:
                  api-key: "judge-key"
              runs:
                  - name: "default"
                    model: "gpt-4o"
                    disable-structured-output: true
`)),
			},
			wantErr: true,
		},
		{
			name: "valid file with judges",
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
              api-key: "primary-key"
          runs:
              - name: "primary"
                model: "gpt-4"
    judges:
        - name: "semantic-judge"
          provider:
              name: openai
              client-config:
                  api-key: "judge-key-1"
              runs:
                  - name: "default"
                    model: "gpt-4o"
                    max-requests-per-minute: 10
        - name: "strict-judge"
          provider:
              name: anthropic
              disabled: true
              client-config:
                  api-key: "judge-key-2"
              runs:
                  - name: "claude"
                    model: "claude-3"
                    disabled: true
                  - name: "enabled-claude"
                    model: "claude-3-opus"
                    model-parameters:
                        temperature: 0.1
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
								APIKey: "primary-key",
							},
							Runs: []RunConfig{
								{
									Name:                 "primary",
									Model:                "gpt-4",
									MaxRequestsPerMinute: 0,
								},
							},
							Disabled: false,
						},
					},
					Judges: []JudgeConfig{
						{
							Name: "semantic-judge",
							Provider: ProviderConfig{
								Name: "openai",
								ClientConfig: OpenAIClientConfig{
									APIKey: "judge-key-1",
								},
								Runs: []RunConfig{
									{
										Name:                 "default",
										Model:                "gpt-4o",
										MaxRequestsPerMinute: 10,
									},
								},
								Disabled: false,
							},
						},
						{
							Name: "strict-judge",
							Provider: ProviderConfig{
								Name: "anthropic",
								ClientConfig: AnthropicClientConfig{
									APIKey: "judge-key-2",
								},
								Runs: []RunConfig{
									{
										Name:                 "claude",
										Model:                "claude-3",
										MaxRequestsPerMinute: 0,
										Disabled:             testutils.Ptr(true),
									},
									{
										Name:                 "enabled-claude",
										Model:                "claude-3-opus",
										MaxRequestsPerMinute: 0,
										ModelParams: AnthropicModelParams{
											Temperature: testutils.Ptr(0.1),
										},
									},
								},
								Disabled: true,
							},
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
			name: "task with duplicate task names",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    tasks:
        - name: "Duplicate Task"
          prompt: "First task prompt"
          response-result-format: "Result format"
          expected-result: "Result"
        - name: "Duplicate Task"
          prompt: "Second task prompt"
          response-result-format: "Result format"
          expected-result: "Result"`)),
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
							ResponseResultFormat: NewResponseFormat("Sed unde non.\nVoluptatem quia voluptate id ipsum est rerum quisquam modi pariatur."),
							ExpectedResult:       utils.NewValueSet("Ut quibusdam inventore dolorum velit.\nUllam et dolor laudantium placeat totam dolorem quia.\nEx voluptates et ipsam sunt nulla eos alias sint ad.\n\nDeleniti ducimus natus et omnis expedita."),
							resolvedSystemPrompt: "Provide the final answer in exactly this format: Sed unde non.\nVoluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.",
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
							ResponseResultFormat: NewResponseFormat("Sed unde non.\nVoluptatem quia voluptate id ipsum est rerum quisquam modi pariatur."),
							ExpectedResult:       utils.NewValueSet("Ut quibusdam inventore dolorum velit.\nUllam et dolor laudantium placeat totam dolorem quia.\nEx voluptates et ipsam sunt nulla eos alias sint ad.\n\nDeleniti ducimus natus et omnis expedita."),
							Files: []TaskFile{
								mockTaskFile(t, "local-file", "path/to/file.txt", "text"),
								mockTaskFile(t, "remote-file", "http://example.com/file.txt", "text"),
							},
							Disabled:             testutils.Ptr(false),
							resolvedSystemPrompt: "Provide the final answer in exactly this format: Sed unde non.\nVoluptatem quia voluptate id ipsum est rerum quisquam modi pariatur.",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid file with judge validation rules",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    tasks:
        - name: "Task with judge validation"
          prompt: |-
              What is the capital of France?
          response-result-format: |-
              City name
          expected-result: |-
              Paris
          validation-rules:
            judge:
              enabled: true
              name: "semantic-judge"
              variant: "default"
        - name: "Task with disabled judge validation"
          prompt: |-
              What is 2 + 2?
          response-result-format: |-
              Number
          expected-result: |-
              4
          validation-rules:
            judge:
              enabled: false
              name: "math-judge"
              variant: "strict"`)),
			},
			want: &Tasks{
				TaskConfig: TaskConfig{
					Tasks: []Task{
						{
							Name:                 "Task with judge validation",
							Prompt:               "What is the capital of France?",
							ResponseResultFormat: NewResponseFormat("City name"),
							ExpectedResult:       utils.NewValueSet("Paris"),
							ValidationRules: &ValidationRules{
								Judge: JudgeSelector{
									Enabled: testutils.Ptr(true),
									Name:    testutils.Ptr("semantic-judge"),
									Variant: testutils.Ptr("default"),
								},
							},
							resolvedSystemPrompt: "Provide the final answer in exactly this format: City name",
							resolvedValidationRules: ValidationRules{
								Judge: JudgeSelector{
									Enabled: testutils.Ptr(true),
									Name:    testutils.Ptr("semantic-judge"),
									Variant: testutils.Ptr("default"),
								},
							},
						},
						{
							Name:                 "Task with disabled judge validation",
							Prompt:               "What is 2 + 2?",
							ResponseResultFormat: NewResponseFormat("Number"),
							ExpectedResult:       utils.NewValueSet("4"),
							ValidationRules: &ValidationRules{
								Judge: JudgeSelector{
									Enabled: testutils.Ptr(false),
									Name:    testutils.Ptr("math-judge"),
									Variant: testutils.Ptr("strict"),
								},
							},
							resolvedSystemPrompt: "Provide the final answer in exactly this format: Number",
							resolvedValidationRules: ValidationRules{
								Judge: JudgeSelector{
									Enabled: testutils.Ptr(false),
									Name:    testutils.Ptr("math-judge"),
									Variant: testutils.Ptr("strict"),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid file with basic validation rules",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    tasks:
        - name: "Task with case-sensitive validation"
          prompt: |-
              What is the exact name of the capital city of France?
          response-result-format: |-
              City name (exact case)
          expected-result: |-
              Paris
          validation-rules:
            case-sensitive: true
            ignore-whitespace: false
        - name: "Task with whitespace-ignoring validation"
          prompt: |-
              What is 2 + 2?
          response-result-format: |-
              Number with optional whitespace
          expected-result: |-
              4
          validation-rules:
            case-sensitive: false
            ignore-whitespace: true
        - name: "Task with combined validation rules"
          prompt: |-
              What programming language is this project written in?
          response-result-format: |-
              Programming language name
          expected-result: |-
              Go
          validation-rules:
            case-sensitive: true
            ignore-whitespace: true`)),
			},
			want: &Tasks{
				TaskConfig: TaskConfig{
					Tasks: []Task{
						{
							Name:                 "Task with case-sensitive validation",
							Prompt:               "What is the exact name of the capital city of France?",
							ResponseResultFormat: NewResponseFormat("City name (exact case)"),
							ExpectedResult:       utils.NewValueSet("Paris"),
							ValidationRules: &ValidationRules{
								CaseSensitive:    testutils.Ptr(true),
								IgnoreWhitespace: testutils.Ptr(false),
							},
							resolvedSystemPrompt: "Provide the final answer in exactly this format: City name (exact case)",
							resolvedValidationRules: ValidationRules{
								CaseSensitive:    testutils.Ptr(true),
								IgnoreWhitespace: testutils.Ptr(false),
							},
						},
						{
							Name:                 "Task with whitespace-ignoring validation",
							Prompt:               "What is 2 + 2?",
							ResponseResultFormat: NewResponseFormat("Number with optional whitespace"),
							ExpectedResult:       utils.NewValueSet("4"),
							ValidationRules: &ValidationRules{
								CaseSensitive:    testutils.Ptr(false),
								IgnoreWhitespace: testutils.Ptr(true),
							},
							resolvedSystemPrompt: "Provide the final answer in exactly this format: Number with optional whitespace",
							resolvedValidationRules: ValidationRules{
								CaseSensitive:    testutils.Ptr(false),
								IgnoreWhitespace: testutils.Ptr(true),
							},
						},
						{
							Name:                 "Task with combined validation rules",
							Prompt:               "What programming language is this project written in?",
							ResponseResultFormat: NewResponseFormat("Programming language name"),
							ExpectedResult:       utils.NewValueSet("Go"),
							ValidationRules: &ValidationRules{
								CaseSensitive:    testutils.Ptr(true),
								IgnoreWhitespace: testutils.Ptr(true),
							},
							resolvedSystemPrompt: "Provide the final answer in exactly this format: Programming language name",
							resolvedValidationRules: ValidationRules{
								CaseSensitive:    testutils.Ptr(true),
								IgnoreWhitespace: testutils.Ptr(true),
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid file with system prompt",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    system-prompt:
        template: "You are a helpful assistant. {{.ResponseResultFormat}}"
        enable-for: none
    tasks:
        - name: "Task with custom system prompt"
          prompt: "How many legs does a dog have?"
          response-result-format: "Greeting"
          system-prompt:
              template: "Hello, {{.ResponseResultFormat}}"
              enable-for: all
          expected-result: "4"`)),
			},
			want: &Tasks{
				TaskConfig: TaskConfig{
					SystemPrompt: SystemPrompt{
						Template:  testutils.Ptr("You are a helpful assistant. {{.ResponseResultFormat}}"),
						EnableFor: testutils.Ptr(EnableForNone),
					},
					Tasks: []Task{
						{
							Name:                 "Task with custom system prompt",
							Prompt:               "How many legs does a dog have?",
							ResponseResultFormat: NewResponseFormat("Greeting"),
							ExpectedResult:       utils.NewValueSet("4"),
							SystemPrompt: &SystemPrompt{
								Template:  testutils.Ptr("Hello, {{.ResponseResultFormat}}"),
								EnableFor: testutils.Ptr(EnableForAll),
							},
							resolvedSystemPrompt: "Hello, Greeting",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid file with tool selector",
			args: args{
				ctx: context.Background(),
				path: createMockFile(t,
					[]byte(
						`task-config:
    tool-selector:
        disabled: false
        tools:
            - name: python-code-executor
              max-calls: 10
              timeout: 60s
              max-memory-mb: 512
              cpu-percent: 25
            - name: calculator
              disabled: true
    tasks:
        - name: "Task with tool selector"
          prompt: "Calculate 2 + 2"
          response-result-format: "Result"
          expected-result: "4"
          tool-selector:
              tools:
                  - name: calculator
                    disabled: false
                    max-calls: 5`)),
			},
			want: &Tasks{
				TaskConfig: TaskConfig{
					ToolSelector: ToolSelector{
						Disabled: testutils.Ptr(false),
						Tools: []ToolSelection{
							{
								Name:        "python-code-executor",
								MaxCalls:    testutils.Ptr(10),
								Timeout:     testutils.Ptr(60 * time.Second),
								MaxMemoryMB: testutils.Ptr(512),
								CpuPercent:  testutils.Ptr(25),
							},
							{
								Name:     "calculator",
								Disabled: testutils.Ptr(true),
							},
						},
					},
					Tasks: []Task{
						{
							Name:                 "Task with tool selector",
							Prompt:               "Calculate 2 + 2",
							ResponseResultFormat: NewResponseFormat("Result"),
							ExpectedResult:       utils.NewValueSet("4"),
							ToolSelector: &ToolSelector{
								Tools: []ToolSelection{
									{
										Name:     "calculator",
										Disabled: testutils.Ptr(false),
										MaxCalls: testutils.Ptr(5),
									},
								},
							},
							resolvedSystemPrompt: "Provide the final answer in exactly this format: Result",
							resolvedToolSelector: ToolSelector{
								Disabled: testutils.Ptr(false),
								Tools: []ToolSelection{
									{
										Name:        "python-code-executor",
										MaxCalls:    testutils.Ptr(10),
										Timeout:     testutils.Ptr(60 * time.Second),
										MaxMemoryMB: testutils.Ptr(512),
										CpuPercent:  testutils.Ptr(25),
									},
									{
										Name:     "calculator",
										Disabled: testutils.Ptr(false),
										MaxCalls: testutils.Ptr(5),
									},
								},
							},
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

					assertToolSelectionsMatch(t,
						tt.want.TaskConfig.Tasks[i].resolvedToolSelector.Tools,
						got.TaskConfig.Tasks[i].resolvedToolSelector.Tools)

					// Clear Tools slices to allow assert.Equal to compare the rest.
					got.TaskConfig.Tasks[i].resolvedToolSelector.Tools = nil
					tt.want.TaskConfig.Tasks[i].resolvedToolSelector.Tools = nil
				}
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// assertToolSelectionsMatch compares two []ToolSelection slices, ignoring order.
func assertToolSelectionsMatch(t *testing.T, expected, actual []ToolSelection) {
	t.Helper()

	require.Len(t, actual, len(expected), "ToolSelection slice length mismatch")

	// Create a map of expected tools by name for easy lookup.
	expectedToolsByName := make(map[string]ToolSelection)
	for _, tool := range expected {
		expectedToolsByName[tool.Name] = tool
	}

	// Verify each actual tool matches an expected tool.
	for _, actualTool := range actual {
		expectedTool, found := expectedToolsByName[actualTool.Name]
		require.True(t, found, "Unexpected tool in actual: %s", actualTool.Name)

		assert.Equal(t, expectedTool.Name, actualTool.Name)
		assert.Equal(t, expectedTool.Disabled, actualTool.Disabled)
		assert.Equal(t, expectedTool.MaxCalls, actualTool.MaxCalls)
		assert.Equal(t, expectedTool.Timeout, actualTool.Timeout)
		assert.Equal(t, expectedTool.MaxMemoryMB, actualTool.MaxMemoryMB)
		assert.Equal(t, expectedTool.CpuPercent, actualTool.CpuPercent)
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
						ResponseResultFormat: NewResponseFormat("generating"),
						ExpectedResult:       utils.NewValueSet("Account"),
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
					ResponseResultFormat: NewResponseFormat("generating"),
					ExpectedResult:       utils.NewValueSet("Account"),
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
					Name:        "provider with some configurations enabled",
					Disabled:    true,
					RetryPolicy: RetryPolicy{},
					Runs: []RunConfig{
						{
							Name:        "Human",
							Model:       "back-end",
							Disabled:    testutils.Ptr(false),
							RetryPolicy: &RetryPolicy{},
						},
					},
				},
				{
					Name:        "provider with all configurations enabled",
					Disabled:    false,
					RetryPolicy: RetryPolicy{},
					Runs: []RunConfig{
						{
							Name:        "Executive",
							Model:       "Garden",
							Disabled:    testutils.Ptr(false),
							RetryPolicy: &RetryPolicy{},
						},
						{
							Name:        "Pants",
							Model:       "implement",
							Disabled:    testutils.Ptr(false),
							RetryPolicy: &RetryPolicy{},
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

func TestGetJudgesWithEnabledRuns(t *testing.T) {
	tests := []struct {
		name string
		ac   AppConfig
		want []JudgeConfig
	}{
		{
			name: "no judges",
			ac: AppConfig{
				Judges: []JudgeConfig{},
			},
			want: []JudgeConfig{},
		},
		{
			name: "all judges disabled or have no enabled runs",
			ac: AppConfig{
				Judges: []JudgeConfig{
					{
						Name: "disabled judge",
						Provider: ProviderConfig{
							Name:     "openai",
							Disabled: true,
							Runs: []RunConfig{
								{
									Name:  "judge-run1",
									Model: "gpt-4",
								},
							},
						},
					},
					{
						Name: "judge with all runs disabled",
						Provider: ProviderConfig{
							Name:     "openai",
							Disabled: false,
							Runs: []RunConfig{
								{
									Name:     "disabled-run",
									Model:    "gpt-4",
									Disabled: testutils.Ptr(true),
								},
							},
						},
					},
				},
			},
			want: []JudgeConfig{},
		},
		{
			name: "judges with some runs enabled",
			ac: AppConfig{
				Judges: []JudgeConfig{
					{
						Name: "disabled judge",
						Provider: ProviderConfig{
							Name:     "openai",
							Disabled: true,
							Runs: []RunConfig{
								{
									Name:  "judge-run1",
									Model: "gpt-4",
								},
							},
						},
					},
					{
						Name: "judge with mixed runs",
						Provider: ProviderConfig{
							Name:     "openai",
							Disabled: true,
							Runs: []RunConfig{
								{
									Name:     "disabled-run",
									Model:    "gpt-4",
									Disabled: testutils.Ptr(true),
								},
								{
									Name:     "enabled-run",
									Model:    "gpt-4o",
									Disabled: testutils.Ptr(false),
								},
								{
									Name:  "default-run",
									Model: "gpt-4-turbo",
								},
							},
						},
					},
					{
						Name: "judge with all runs enabled",
						Provider: ProviderConfig{
							Name:     "google",
							Disabled: false,
							Runs: []RunConfig{
								{
									Name:     "primary-run",
									Model:    "gemini-pro",
									Disabled: testutils.Ptr(false),
								},
								{
									Name:  "secondary-run",
									Model: "gemini-ultra",
								},
							},
						},
					},
				},
			},
			want: []JudgeConfig{
				{
					Name: "judge with mixed runs",
					Provider: ProviderConfig{
						Name:        "openai",
						Disabled:    true,
						RetryPolicy: RetryPolicy{},
						Runs: []RunConfig{
							{
								Name:        "enabled-run",
								Model:       "gpt-4o",
								Disabled:    testutils.Ptr(false),
								RetryPolicy: &RetryPolicy{},
							},
						},
					},
				},
				{
					Name: "judge with all runs enabled",
					Provider: ProviderConfig{
						Name:        "google",
						Disabled:    false,
						RetryPolicy: RetryPolicy{},
						Runs: []RunConfig{
							{
								Name:        "primary-run",
								Model:       "gemini-pro",
								Disabled:    testutils.Ptr(false),
								RetryPolicy: &RetryPolicy{},
							},
							{
								Name:        "secondary-run",
								Model:       "gemini-ultra",
								Disabled:    testutils.Ptr(false),
								RetryPolicy: &RetryPolicy{},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.ac.GetJudgesWithEnabledRuns())
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

func TestGetRunsResolved(t *testing.T) {
	tests := []struct {
		name string
		pc   ProviderConfig
		want []RunConfig
	}{
		{
			name: "no runs",
			pc: ProviderConfig{
				Name:        "test-provider",
				Disabled:    false,
				RetryPolicy: RetryPolicy{MaxRetryAttempts: 3},
				Runs:        []RunConfig{},
			},
			want: []RunConfig{},
		},
		{
			name: "runs with nil retry policy and disabled flag",
			pc: ProviderConfig{
				Name:     "test-provider",
				Disabled: false,
				RetryPolicy: RetryPolicy{
					MaxRetryAttempts:    3,
					InitialDelaySeconds: 1,
				},
				Runs: []RunConfig{
					{
						Name:        "run1",
						Model:       "model1",
						Disabled:    nil,
						RetryPolicy: nil,
					},
					{
						Name:        "run2",
						Model:       "model2",
						Disabled:    nil,
						RetryPolicy: nil,
					},
				},
			},
			want: []RunConfig{
				{
					Name:     "run1",
					Model:    "model1",
					Disabled: testutils.Ptr(false),
					RetryPolicy: &RetryPolicy{
						MaxRetryAttempts:    3,
						InitialDelaySeconds: 1,
					},
				},
				{
					Name:     "run2",
					Model:    "model2",
					Disabled: testutils.Ptr(false),
					RetryPolicy: &RetryPolicy{
						MaxRetryAttempts:    3,
						InitialDelaySeconds: 1,
					},
				},
			},
		},
		{
			name: "runs with explicit retry policy and disabled flag",
			pc: ProviderConfig{
				Name:        "test-provider",
				Disabled:    false,
				RetryPolicy: RetryPolicy{MaxRetryAttempts: 3},
				Runs: []RunConfig{
					{
						Name:        "run1",
						Model:       "model1",
						Disabled:    testutils.Ptr(true),
						RetryPolicy: &RetryPolicy{MaxRetryAttempts: 1},
					},
					{
						Name:        "run2",
						Model:       "model2",
						Disabled:    testutils.Ptr(false),
						RetryPolicy: &RetryPolicy{MaxRetryAttempts: 5},
					},
				},
			},
			want: []RunConfig{
				{
					Name:        "run1",
					Model:       "model1",
					Disabled:    testutils.Ptr(true),
					RetryPolicy: &RetryPolicy{MaxRetryAttempts: 1},
				},
				{
					Name:        "run2",
					Model:       "model2",
					Disabled:    testutils.Ptr(false),
					RetryPolicy: &RetryPolicy{MaxRetryAttempts: 5},
				},
			},
		},
		{
			name: "mixed resolved and explicit values",
			pc: ProviderConfig{
				Name:        "test-provider",
				Disabled:    true,
				RetryPolicy: RetryPolicy{MaxRetryAttempts: 2},
				Runs: []RunConfig{
					{
						Name:        "run1",
						Model:       "model1",
						Disabled:    nil,
						RetryPolicy: nil,
					},
					{
						Name:        "run2",
						Model:       "model2",
						Disabled:    testutils.Ptr(false),
						RetryPolicy: &RetryPolicy{MaxRetryAttempts: 4},
					},
				},
			},
			want: []RunConfig{
				{
					Name:        "run1",
					Model:       "model1",
					Disabled:    testutils.Ptr(true),
					RetryPolicy: &RetryPolicy{MaxRetryAttempts: 2},
				},
				{
					Name:        "run2",
					Model:       "model2",
					Disabled:    testutils.Ptr(false),
					RetryPolicy: &RetryPolicy{MaxRetryAttempts: 4},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pc.GetRunsResolved())
		})
	}
}

func TestAlibabaClientConfig_GetEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		config   AlibabaClientConfig
		expected string
	}{
		{
			name: "empty endpoint defaults to Singapore",
			config: AlibabaClientConfig{
				APIKey: "test-key",
				// Endpoint not set.
			},
			expected: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		},
		{
			name: "explicit Beijing endpoint",
			config: AlibabaClientConfig{
				APIKey:   "test-key",
				Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			},
			expected: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		},
		{
			name: "explicit Singapore endpoint",
			config: AlibabaClientConfig{
				APIKey:   "test-key",
				Endpoint: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
			},
			expected: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetEndpoint())
		})
	}
}
