// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/version"
)

const (
	testOutputFileBasename = "results"
	mockConfig             = `config:
            log-file: ""
            output-dir: "/"
            output-basename: ""
            task-source: "/usr/include/bedfordshire_incredible.pcf.vcard"
            providers:
              - name: "openai"
                client-config:
                  api-key: "37ce2f83-ff15-4772-acbb-fb519185f6d6"
                runs:
                  - name: "p1 run1"
                    model: "p1-model-1"
                    max-requests-per-minute: 10
                  - name: "p1 run2"
                    model: "p1-model-2"
                  - name: "p1 run3"
                    model: "p1-model-3"
                    disabled: true
              - name: "openai"
                client-config:
                  api-key: "d474d964-8fa0-4330-b171-e9af7d4e173b"
                runs:
                  - name: "p2 run1"
                    model: "p2-model-1"
                    model-parameters:
                      reasoning-effort: high
              - name: "google"
                disabled: true
                client-config:
                  api-key: "63add9c7-3329-4a3e-bd4e-b251256a848c"
                runs:
                  - name: "p3 run1"
                    model: "p3-model-1"
            judges:
              - name: "semantic-judge"
                provider:
                  name: "openai"
                  client-config:
                    api-key: "judge-key-1"
                  runs:
                    - name: "default"
                      model: "judge-model-1"
                      model-parameters:
                        reasoning-effort: high
              - name: "disabled-judge"
                provider:
                  name: "openai"
                  disabled: true
                  client-config:
                    api-key: "judge-key-2"
                  runs:
                    - name: "default"
                      model: "judge-model-2"
              - name: "judge-with-disabled-run"
                provider:
                  name: "openai"
                  client-config:
                    api-key: "judge-key-3"
                  runs:
                    - name: "disabled-run"
                      model: "judge-model-3"
                      disabled: true`
	mockTasks = `task-config:
  tasks:
    - name: "unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37"
      prompt: |-
        Aut earum consectetur dicta facere nobis qui et id.
        Aut eveniet consequatur incidunt velit ea incidunt eos quas.
        
        Ut ut et doloremque debitis non minus quasi.
      response-result-format: |-
        A, <color>, <number>
        B, <color>, <number>
        C, <color>, <number>
      expected-result: |-
        A, pink, 13
        B, green, 0
        C, red, 7
    - name: "failure"
      prompt: |-
        Dolorum omnis ea et.
      response-result-format: |-
        Veniam recusandae sed error aut laudantium ut vitae.
      expected-result: |-
        Sapiente pariatur ipsam commodi non praesentium voluptates sunt deleniti perspiciatis.
    - name: "error"
      prompt: |-
        Laboriosam sit quam totam.
      response-result-format: |-
        Et sunt sint sequi necessitatibus et.
      expected-result: |-
        Quia error officia et aliquid voluptas fugiat nihil.
    - name: "disabled task"
      disabled: true
      prompt: |-
        Quia fuga error eligendi soluta reiciendis in.
      response-result-format: |-
        Iusto sequi qui omnis esse odit neque voluptas.
      expected-result: |-
        Libero sed nulla.`
)

var (
	allOutputFormatsEnabled = map[string]bool{
		"csv":  true,
		"html": true,
	}
	noOutputFormatsEnabled = map[string]bool{
		"csv":  false,
		"html": false,
	}
	expectedStdoutMessages = []string{
		"Current working directory:",
		"Configuration directory:",
		"Loading configuration from file:",
		"Loading tasks from file:",
	}
)

func TestCommands(t *testing.T) {
	tests := []struct {
		name               string
		commands           []string
		wantStdoutContains []string
	}{
		{
			name:               "display help",
			commands:           []string{"help"},
			wantStdoutContains: []string{"Usage:"},
		},
		{
			name:               "display version",
			commands:           []string{"version"},
			wantStdoutContains: []string{fmt.Sprintf("%s %s", version.Name, version.GetVersion())},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sout := testutils.CaptureStdout(t, func() { testutils.WithArgs(t, main, tt.commands...) })
			testutils.AssertContainsAll(t, sout, tt.wantStdoutContains)
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name                  string
		config                []byte
		tasks                 []byte
		logFilePath           string
		outputFileBasename    string
		outputFormats         map[string]bool
		verbose               bool
		debug                 bool
		initOutputContent     []byte
		wantStdoutContains    []string
		wantStdoutNotContains []string
		wantOutputContains    []string
		wantOutputNotContains []string
		wantLogContains       []string
		wantLogNotContains    []string
	}{
		{
			name:   "judge validation with valid judge",
			config: []byte(mockConfig),
			tasks: []byte(`task-config:
                    tasks:
                      - name: "task-with-valid-judge"
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
                            variant: "default"`),
			outputFileBasename: testOutputFileBasename,
			outputFormats:      allOutputFormatsEnabled,
			debug:              true,
			wantStdoutContains: append([]string{
				"Log messages will be saved to:",
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			}, expectedStdoutMessages...),
			wantStdoutNotContains: []string{
				"judge not found",
				"judge run variant not found",
			},
			wantOutputContains: []string{
				"task-with-valid-judge",
			},
			wantLogContains: []string{
				"all tasks in all configurations have finished on all providers",
				"openai: p1 run1: task-with-valid-judge: using openai (default) judge for response evaluation",
				"openai: p1 run2: task-with-valid-judge: using openai (default) judge for response evaluation",
				"openai: p2 run1: task-with-valid-judge: using openai (default) judge for response evaluation",
				"openai: p1 run1: task-with-valid-judge: response evaluation token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run2: task-with-valid-judge: response evaluation token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p2 run1: task-with-valid-judge: response evaluation token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run1: task-with-valid-judge: response evaluation duration:",
				"openai: p1 run2: task-with-valid-judge: response evaluation duration:",
				"openai: p2 run1: task-with-valid-judge: response evaluation duration:",
				"openai: p1 run1: task-with-valid-judge: response evaluation prompts:",
				"openai: p1 run2: task-with-valid-judge: response evaluation prompts:",
				"openai: p2 run1: task-with-valid-judge: response evaluation prompts:",
			},
		},
		{
			name: "fail on no enabled targets",
			config: []byte(`config:
                    log-file: ""
                    output-dir: "/"
                    output-basename: ""
                    task-source: "/usr/include/bedfordshire_incredible.pcf.vcard"
                    providers:
                      - name: "openai"
                        client-config:
                          api-key: "021cfc61-3c24-4d3e-9419-c3d26bb01d84"
                        runs:
                          - name: "disabled run"
                            disabled: true
                            model: "channels"
                      - name: "google"
                        disabled: true
                        client-config:
                          api-key: "451588d2-2595-4e34-a617-edce26e7943c"
                        runs:
                          - name: "enabled run of disabled provider"
                            model: "capacitor"`),
			tasks:              []byte(mockTasks),
			outputFileBasename: testOutputFileBasename,
			outputFormats:      allOutputFormatsEnabled,
			wantStdoutContains: append([]string{
				"Nothing to run: all providers are disabled or have no enabled run configurations.",
			}, expectedStdoutMessages...),
			wantStdoutNotContains: []string{
				"Log messages will be saved to:",
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			},
			wantOutputContains: nil,
			wantLogContains:    nil,
		},
		{
			name:   "fail on no enabled tasks",
			config: []byte(mockConfig),
			tasks: []byte(`task-config:
          disabled: true
          tasks:
            - name: "task #1"
              prompt: |-
                Aut earum consectetur dicta facere nobis qui et id.
                Aut eveniet consequatur incidunt velit ea incidunt eos quas.

                Ut ut et doloremque debitis non minus quasi.
              response-result-format: |-
                A, <color>, <number>
                B, <color>, <number>
                C, <color>, <number>
              expected-result: |-
                A, pink, 13
                B, green, 0
                C, red, 7
            - name: "failure"
              prompt: |-
                Dolorum omnis ea et.
              response-result-format: |-
                Veniam recusandae sed error aut laudantium ut vitae.
              expected-result: |-
                Sapiente pariatur ipsam commodi non praesentium voluptates sunt deleniti perspiciatis.
            - name: "error"
              prompt: |-
                Laboriosam sit quam totam.
              response-result-format: |-
                Et sunt sint sequi necessitatibus et.
              expected-result: |-
                Quia error officia et aliquid voluptas fugiat nihil.`),
			outputFileBasename: testOutputFileBasename,
			outputFormats:      allOutputFormatsEnabled,
			wantStdoutContains: append([]string{
				"Nothing to run: all tasks are disabled.",
			}, expectedStdoutMessages...),
			wantStdoutNotContains: []string{
				"Log messages will be saved to:",
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			},
			wantOutputContains: nil,
			wantLogContains:    nil,
		},
		{
			name:               "pre-existing output files",
			config:             []byte(mockConfig),
			tasks:              []byte(mockTasks),
			logFilePath:        testutils.CreateMockFile(t, "*.messages.log", []byte("e8787ca3-12e4-47b9-a06f-4b81ad15c304")),
			outputFileBasename: testOutputFileBasename,
			outputFormats:      allOutputFormatsEnabled,
			initOutputContent:  []byte("95db2195-5a95-4e4b-9a0d-61f38e639491"),
			wantStdoutContains: append([]string{
				"Log messages will be saved to:",
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			}, expectedStdoutMessages...),
			wantOutputContains: []string{
				"unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37",
			},
			wantOutputNotContains: []string{
				"95db2195-5a95-4e4b-9a0d-61f38e639491",
			}, // output file should get overwritten
			wantLogContains: []string{
				"e8787ca3-12e4-47b9-a06f-4b81ad15c304", // log file should get appended to
				"all tasks in all configurations have finished on all providers",
			},
			wantLogNotContains: []string{
				"google:",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
			},
		},
		{
			name:               "non-existing output artifacts",
			config:             []byte(mockConfig),
			tasks:              []byte(mockTasks),
			outputFileBasename: testOutputFileBasename,
			outputFormats:      allOutputFormatsEnabled,
			wantStdoutContains: append([]string{
				"Log messages will be saved to:",
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			}, expectedStdoutMessages...),
			wantOutputContains: []string{
				"unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37",
			},
			wantLogContains: []string{
				"all tasks in all configurations have finished on all providers",
			},
			wantLogNotContains: []string{
				"google:",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
			},
		},
		{
			name:               "output to stdout",
			config:             []byte(mockConfig),
			tasks:              []byte(mockTasks),
			outputFileBasename: "",
			outputFormats:      allOutputFormatsEnabled,
			wantStdoutContains: append([]string{
				"Log messages will be saved to:",
				"unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37",
			}, expectedStdoutMessages...),
			wantStdoutNotContains: []string{
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			},
			wantOutputContains: []string{},
			wantLogContains: []string{
				"all tasks in all configurations have finished on all providers",
			},
			wantLogNotContains: []string{
				"google:",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\nPorro laudantium quam voluptas.\n\nEt magnam velit unde.\n\nDolore odio esse et esse.",
			},
		},
		{
			name:               "verbose logging",
			config:             []byte(mockConfig),
			tasks:              []byte(mockTasks),
			outputFileBasename: "",
			outputFormats:      noOutputFormatsEnabled, // no output will be generated
			verbose:            true,
			wantStdoutContains: append([]string{
				"Log messages will be saved to:",
				"unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37",
			}, expectedStdoutMessages...),
			wantStdoutNotContains: []string{
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			},
			wantOutputContains: []string{},
			wantLogContains: []string{
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"all tasks in all configurations have finished on all providers",
			},
			wantLogNotContains: []string{
				"google:",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\n\tPorro laudantium quam voluptas.\n\n\tEt magnam velit unde.\n\n\tDolore odio esse et esse.",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\n\tPorro laudantium quam voluptas.\n\n\tEt magnam velit unde.\n\n\tDolore odio esse et esse.",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\n\tPorro laudantium quam voluptas.\n\n\tEt magnam velit unde.\n\n\tDolore odio esse et esse.",
			},
		},
		{
			name:               "debug logging",
			config:             []byte(mockConfig),
			tasks:              []byte(mockTasks),
			outputFileBasename: "",
			outputFormats:      noOutputFormatsEnabled, // no output will be generated
			debug:              true,
			wantStdoutContains: append([]string{
				"Log messages will be saved to:",
				"unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37",
			}, expectedStdoutMessages...),
			wantStdoutNotContains: []string{
				"Results in HTML format will be saved to:",
				"Results in CSV format will be saved to:",
			},
			wantOutputContains: []string{},
			wantLogContains: []string{
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: token usage: [in:8200209999917998, out:<unknown>]",
				"openai: p1 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\n\tPorro laudantium quam voluptas.\n\n\tEt magnam velit unde.\n\n\tDolore odio esse et esse.",
				"openai: p1 run2: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\n\tPorro laudantium quam voluptas.\n\n\tEt magnam velit unde.\n\n\tDolore odio esse et esse.",
				"openai: p2 run1: unique-enabled-task-name-68315b95-de8c-4f19-9f76-d70829ec0e37: prompts:\n\tPorro laudantium quam voluptas.\n\n\tEt magnam velit unde.\n\n\tDolore odio esse et esse.",
				"all tasks in all configurations have finished on all providers",
			},
			wantLogNotContains: []string{
				"google:",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFilePath := testutils.CreateMockFile(t, "*.config.yaml", tt.config)
			tasksFilePath := testutils.CreateMockFile(t, "*.tasks.yaml", tt.tasks)

			// Any necessary parent directories should be created automatically.
			logFilePath := tt.logFilePath
			if logFilePath == "" {
				logFilePath = filepath.Join(os.TempDir(), uuid.NewString(), "messages.log")
			}
			outBasePath := filepath.Join(os.TempDir(), uuid.NewString())

			outputFiles := make(map[string]bool)
			for name, enabled := range tt.outputFormats {
				require.NoError(t, flag.Set(name, strconv.FormatBool(enabled)))
				if tt.outputFileBasename != "" {
					outputFilePath := filepath.Join(outBasePath, fmt.Sprintf("%s.%s", tt.outputFileBasename, name))
					outputFiles[outputFilePath] = enabled
					if enabled && tt.initOutputContent != nil {
						createFile(t, outputFilePath, tt.initOutputContent)
					}
				}
			}

			require.NoError(t, flag.Set("config", configFilePath))
			require.NoError(t, flag.Set("tasks", tasksFilePath))
			require.NoError(t, flag.Set("output-dir", outBasePath))
			require.NoError(t, flag.Set("output-basename", tt.outputFileBasename))
			require.NoError(t, flag.Set("log", logFilePath))
			require.NoError(t, flag.Set("verbose", strconv.FormatBool(tt.verbose)))
			require.NoError(t, flag.Set("debug", strconv.FormatBool(tt.debug)))

			sout := testutils.CaptureStdout(t, func() { testutils.WithArgs(t, main, "run") })

			testutils.AssertContainsAll(t, sout, tt.wantStdoutContains)
			testutils.AssertContainsNone(t, sout, tt.wantStdoutNotContains)
			assertTestArtifact(t, logFilePath, tt.wantLogContains, tt.wantLogNotContains)
			for filePath, isWant := range outputFiles {
				if isWant {
					assertTestArtifact(t, filePath, tt.wantOutputContains, tt.wantOutputNotContains)
				} else {
					assert.NoFileExists(t, filePath)
				}
			}
		})
	}
}

func createFile(t *testing.T, filePath string, contents []byte) {
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), os.ModePerm))
	require.NoError(t, os.WriteFile(filePath, contents, 0600))
}

func assertTestArtifact(t *testing.T, filePath string, want []string, notWant []string) {
	if want != nil {
		require.FileExists(t, filePath)
		t.Logf("test artifact: %s\n", filePath)
		testutils.AssertFileContains(t, filePath, want, notWant)
	} else {
		require.NoFileExists(t, filePath)
	}
}
