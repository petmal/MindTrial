// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	providertools "github.com/petmal/mindtrial/providers/tools"
)

const validCustomValidatorResponse = `{"correct":true,"title":"Valid","explanation":"The trusted validator accepted the result."}`

func TestFactorySelectsCustomValidator(t *testing.T) {
	factory := NewFactoryWithCustomValidators(nil, []config.ValidatorConfig{{Name: "stateful", Image: "validator:test"}})
	rules := config.ValidationRules{CustomValidator: testutils.Ptr("stateful")}

	validator, err := factory.GetValidator(t.Context(), rules)
	require.NoError(t, err)
	assert.Equal(t, "custom validator stateful", validator.GetName())
	assert.Implements(t, (*CustomValidator)(nil), validator)

	cachedValidator, err := factory.GetValidator(t.Context(), rules)
	require.NoError(t, err)
	assert.Same(t, validator, cachedValidator)
	require.NoError(t, factory.AssertCustomValidatorExists("stateful"))
	require.ErrorIs(t, factory.AssertCustomValidatorExists("missing"), ErrCustomValidatorNotFound)

	_, err = factory.GetValidator(t.Context(), config.ValidationRules{CustomValidator: testutils.Ptr("missing")})
	require.ErrorIs(t, err, ErrCustomValidatorNotFound)
}

// validateWithCustomValidator runs cfg against candidate the way the runner does after a successful attempt.
func validateWithCustomValidator(t *testing.T, cfg config.ValidatorConfig, expected utils.ValueSet, candidate interface{}) (ValidationResult, error) {
	t.Helper()
	factory := NewFactoryWithCustomValidators(nil, []config.ValidatorConfig{cfg})
	rules := config.ValidationRules{CustomValidator: testutils.Ptr(cfg.Name), CaseSensitive: testutils.Ptr(true)}
	validator, err := factory.GetValidator(t.Context(), rules)
	require.NoError(t, err)
	customValidator, ok := validator.(CustomValidator)
	require.True(t, ok)

	return customValidator.IsCorrectWithEnvironment(t.Context(), testutils.NewTestLogger(t), rules, expected,
		providers.Result{FinalAnswer: providers.Answer{Content: candidate}},
		"Win the world.", config.NewResponseFormat("winning code"), nil,
		providertools.ExecutionTemplateData{
			Evaluation: providertools.EvaluationTemplateData{Seed: "eval-seed"},
			Task:       providertools.NameTemplateData{Name: "world-task"},
			Provider:   providertools.NameTemplateData{Name: "provider-a"},
			Run:        providertools.NameTemplateData{Name: "run-a"},
		})
}

func TestCustomValidatorResponseProtocol(t *testing.T) {
	tests := []struct {
		name            string
		result          testutils.DockerFakeResult
		wantCorrect     bool
		wantTitle       string
		wantExplanation string
		wantErr         error
	}{
		{
			name:            "accepted answer",
			result:          testutils.DockerFakeResult{Stdout: validCustomValidatorResponse},
			wantCorrect:     true,
			wantTitle:       "Valid",
			wantExplanation: "The trusted validator accepted the result.",
		},
		{
			name:            "rejected answer is not an error",
			result:          testutils.DockerFakeResult{Stdout: "\n" + `{"correct":false,"title":"Not won","explanation":"The world is not in a winning state."}` + "\n"},
			wantTitle:       "Not won",
			wantExplanation: "The world is not in a winning state.",
		},
		{
			name:    "unknown field",
			result:  testutils.DockerFakeResult{Stdout: `{"correct":true,"title":"Valid","explanation":"ok","score":1}`},
			wantErr: ErrCustomValidatorResponse,
		},
		{
			name:    "missing field",
			result:  testutils.DockerFakeResult{Stdout: `{"title":"Valid","explanation":"ok"}`},
			wantErr: ErrCustomValidatorResponse,
		},
		{
			name:    "blank explanation",
			result:  testutils.DockerFakeResult{Stdout: `{"correct":true,"title":"Valid","explanation":"  "}`},
			wantErr: ErrCustomValidatorResponse,
		},
		{
			name:    "trailing output",
			result:  testutils.DockerFakeResult{Stdout: validCustomValidatorResponse + "\nextra"},
			wantErr: ErrCustomValidatorResponse,
		},
		{
			name:    "malformed JSON",
			result:  testutils.DockerFakeResult{Stdout: `{"correct":true,`},
			wantErr: ErrCustomValidatorResponse,
		},
		{
			name:    "non-zero exit",
			result:  testutils.DockerFakeResult{Stdout: validCustomValidatorResponse, ExitCode: 3},
			wantErr: ErrCustomValidatorExecution,
		},
		{
			name:    "no output",
			result:  testutils.DockerFakeResult{},
			wantErr: ErrCustomValidatorExecution,
		},
		{
			name:    "Docker infrastructure error",
			result:  testutils.DockerFakeResult{CreateError: "No such image: validator:test"},
			wantErr: ErrCustomValidatorExecution,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutils.NewDockerFake(t, tt.result)

			result, err := validateWithCustomValidator(t, config.ValidatorConfig{Name: "world-state", Image: "validator:test"}, utils.NewValueSet(), "NX-1")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCorrect, result.IsCorrect)
			assert.Equal(t, tt.wantTitle, result.Title)
			assert.Equal(t, tt.wantExplanation, result.Explanation)
		})
	}
}

func TestCustomValidatorTimeout(t *testing.T) {
	testutils.NewDockerFake(t, testutils.DockerFakeResult{Hang: true})

	_, err := validateWithCustomValidator(t, config.ValidatorConfig{
		Name:    "slow",
		Image:   "validator:test",
		Timeout: testutils.Ptr(50 * time.Millisecond),
	}, utils.NewValueSet(), "NX-1")

	require.ErrorIs(t, err, ErrCustomValidatorExecution)
	require.ErrorIs(t, err, providertools.ErrToolTimeout)
}

func TestCustomValidatorRendersValidationContext(t *testing.T) {
	cfg := config.ValidatorConfig{
		Name:  "world-state",
		Image: "validator:test",
		Command: []string{
			"validate", "--code={{ .Candidate.Response }}", "--format={{ .OriginalTask.ResponseResultFormat }}",
		},
		Env: map[string]string{
			"EXPECTED":  "{{ json .OriginalTask.ExpectedResults }}",
			"PROMPT":    "{{ .OriginalTask.Prompt }}",
			"CONTEXT":   "{{ .Evaluation.Seed }}/{{ .Task.Name }}/{{ .Provider.Name }}/{{ .Run.Name }}",
			"RULES":     "{{ .Rules.CaseSensitive }}/{{ .Rules.IgnoreWhitespace }}/{{ .Rules.TrimLines }}",
			"WORLD_KEY": "{{ hash .Evaluation.Seed .Task.Name }}",
		},
		TemplateFiles: []config.ValidatorTemplateFile{
			{Path: "/input/candidate", Template: "{{ .Candidate.Response }}"},
			{Path: "/input/./expected.json", Template: "{{ json .OriginalTask.ExpectedResults }}"},
		},
	}
	worldKey, err := utils.ExpandTemplate("key", `{{ hash "eval-seed" "world-task" }}`, nil)
	require.NoError(t, err)

	tests := []struct {
		name         string
		expected     utils.ValueSet
		wantExpected string
	}{
		{
			name:         "dynamic task without expected result",
			expected:     utils.ValueSet{},
			wantExpected: "[]",
		},
		{
			name:         "static task with structured reference data",
			expected:     utils.NewValueSet(map[string]interface{}{"items": map[string]interface{}{"apple": 2}}),
			wantExpected: `[{"items":{"apple":2}}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker := testutils.NewDockerFake(t, testutils.DockerFakeResult{Stdout: validCustomValidatorResponse})

			_, err := validateWithCustomValidator(t, cfg, tt.expected, "NX-1")
			require.NoError(t, err)

			containers := docker.Containers()
			require.Len(t, containers, 1)
			created := containers[0]
			assert.Equal(t, []string{"validate", "--code=NX-1", "--format=winning code"}, created.Cmd)
			assert.ElementsMatch(t, []string{
				"EXPECTED=" + tt.wantExpected,
				"PROMPT=Win the world.",
				"CONTEXT=eval-seed/world-task/provider-a/run-a",
				"RULES=true/false/false",
				"WORLD_KEY=" + worldKey,
			}, created.Env)
			assert.Equal(t, "none", created.NetworkMode, "a validator without dependencies has no network")
			assert.Equal(t, map[string]testutils.DockerFakeMount{
				"/input/candidate":     {Content: []byte("NX-1"), ReadOnly: true},
				"/input/expected.json": {Content: []byte(tt.wantExpected), ReadOnly: true},
			}, created.Mounts)
		})
	}
}

func TestCustomValidatorTemplateFilesCarryArbitraryCandidates(t *testing.T) {
	docker := testutils.NewDockerFake(t, testutils.DockerFakeResult{Stdout: validCustomValidatorResponse})
	candidate := strings.Repeat("x", 256*1024) + "\x00-tail"

	_, err := validateWithCustomValidator(t, config.ValidatorConfig{
		Name:    "world-state",
		Image:   "validator:test",
		Command: []string{"validate", "--candidate-file", "/input/candidate", "--task={{ .Task.Name }}"},
		TemplateFiles: []config.ValidatorTemplateFile{
			{Path: "/input/candidate", Template: "{{ .Candidate.Response }}"},
			{Path: "/input/candidate.json", Template: "{{ json .Candidate.Response }}"},
		},
	}, utils.NewValueSet(), candidate)
	require.NoError(t, err)

	containers := docker.Containers()
	require.Len(t, containers, 1)
	assert.Equal(t, []string{"validate", "--candidate-file", "/input/candidate", "--task=world-task"}, containers[0].Cmd)
	assert.Equal(t, []byte(candidate), containers[0].Mounts["/input/candidate"].Content, "file content must be exact bytes")
	assert.JSONEq(t, `"`+strings.Repeat("x", 256*1024)+`\u0000-tail"`, string(containers[0].Mounts["/input/candidate.json"].Content))
}

func TestCustomValidatorTemplateRenderingErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ValidatorConfig
	}{
		{
			name: "command argument",
			cfg:  config.ValidatorConfig{Name: "invalid", Image: "validator:test", Command: []string{"{{ .Candidate.Missing }}"}},
		},
		{
			name: "environment variable",
			cfg:  config.ValidatorConfig{Name: "invalid", Image: "validator:test", Env: map[string]string{"SEED": "{{ .Evaluation.Missing }}"}},
		},
		{
			name: "template file",
			cfg: config.ValidatorConfig{Name: "invalid", Image: "validator:test", TemplateFiles: []config.ValidatorTemplateFile{
				{Path: "/input/candidate", Template: "{{ .Missing }}"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker := testutils.NewDockerFake(t, testutils.DockerFakeResult{Stdout: validCustomValidatorResponse})

			_, err := validateWithCustomValidator(t, tt.cfg, utils.NewValueSet(), "NX-1")

			require.ErrorIs(t, err, ErrCustomValidatorTemplate)
			assert.Empty(t, docker.Containers(), "no validator container runs with an unrendered template")
		})
	}
}

func TestCompileCustomValidatorTemplates(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.ValidatorConfig
		wantErrText string
	}{
		{
			name: "valid templates",
			cfg: config.ValidatorConfig{
				Command:       []string{"validate", "{{ .Candidate.Response }}"},
				Env:           map[string]string{"EXPECTED": "{{ json .OriginalTask.ExpectedResults }}", "KEY": "{{ hash .Evaluation.Seed }}"},
				TemplateFiles: []config.ValidatorTemplateFile{{Path: "/input/candidate", Template: "{{ .Candidate.Response }}"}},
			},
		},
		{
			name:        "malformed command argument",
			cfg:         config.ValidatorConfig{Command: []string{"validate", "{{ .Candidate.Response"}},
			wantErrText: "command argument 1",
		},
		{
			name:        "malformed environment value",
			cfg:         config.ValidatorConfig{Env: map[string]string{"EXPECTED": "{{ json }"}},
			wantErrText: `environment variable "EXPECTED"`,
		},
		{
			name:        "malformed template file",
			cfg:         config.ValidatorConfig{TemplateFiles: []config.ValidatorTemplateFile{{Path: "/input/candidate", Template: "{{ unknown .Candidate }}"}}},
			wantErrText: `template file "/input/candidate"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompileCustomValidatorTemplates(tt.cfg)
			if tt.wantErrText == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrCustomValidatorTemplate)
			assert.Contains(t, err.Error(), tt.wantErrText)
		})
	}
}
