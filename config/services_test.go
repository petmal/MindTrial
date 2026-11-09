// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskConfigValidateCustomValidation(t *testing.T) {
	customRules := &ValidationRules{CustomValidator: testutils.Ptr("nexus")}
	tests := []struct {
		name        string
		task        Task
		wantErrText string
	}{
		{
			name: "dynamic custom validator without expected result",
			task: Task{
				Name:                 "dynamic",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat("validation stamp"),
				ValidationRules:      customRules,
			},
		},
		{
			name: "static custom validator with expected answer",
			task: Task{
				Name:                 "static",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat("validation stamp"),
				ExpectedResult:       utils.NewValueSet("NX-STATIC"),
				ValidationRules:      customRules,
			},
		},
		{
			name: "custom validator with structured reference data for a text answer",
			task: Task{
				Name:                 "cart",
				Prompt:               "fill the cart",
				ResponseResultFormat: NewResponseFormat("the word done"),
				ExpectedResult: utils.NewValueSet(map[string]interface{}{
					"items": map[string]interface{}{"apple": 2, "bread": 1},
				}),
				ValidationRules: customRules,
			},
		},
		{
			name: "custom validator still validates the response format",
			task: Task{
				Name:                 "invalid format",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat(42),
				ValidationRules:      customRules,
			},
			wantErrText: "response-result-format must be either plain text or a JSON schema object",
		},
		{
			name: "missing expected result without custom validator",
			task: Task{
				Name:                 "missing",
				Prompt:               "answer",
				ResponseResultFormat: NewResponseFormat("text"),
			},
			wantErrText: "expected-result is required unless custom-validator is configured",
		},
		{
			name: "custom validator combined with judge",
			task: Task{
				Name:                 "judge conflict",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat("text"),
				ValidationRules: &ValidationRules{
					CustomValidator: testutils.Ptr("nexus"),
					Judge:           JudgeSelector{Enabled: testutils.Ptr(true)},
				},
			},
			wantErrText: "custom-validator is mutually exclusive with schema-validation and judge validation",
		},
		{
			name: "custom validator combined with schema validation",
			task: Task{
				Name:                 "schema conflict",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat("text"),
				ValidationRules: &ValidationRules{
					CustomValidator:  testutils.Ptr("nexus"),
					SchemaValidation: testutils.Ptr(true),
				},
			},
			wantErrText: "custom-validator is mutually exclusive with schema-validation and judge validation",
		},
		{
			name: "scalar service inputs",
			task: Task{
				Name:                 "scalar inputs",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat("text"),
				ValidationRules:      customRules,
				ToolSelector: &ToolSelector{ServiceInputs: map[string]map[string]interface{}{
					"world": {"seed": "{{ .Evaluation.Seed }}", "rooms": 6, "ratio": 0.5, "hard": true},
				}},
			},
		},
		{
			name: "object service input",
			task: Task{
				Name:                 "object input",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat("text"),
				ValidationRules:      customRules,
				ToolSelector: &ToolSelector{ServiceInputs: map[string]map[string]interface{}{
					"world": {"layout": map[string]interface{}{"rooms": 6}},
				}},
			},
			wantErrText: "service-inputs world.layout must be a scalar value",
		},
		{
			name: "list service input",
			task: Task{
				Name:                 "list input",
				Prompt:               "solve",
				ResponseResultFormat: NewResponseFormat("text"),
				ValidationRules:      customRules,
				ToolSelector: &ToolSelector{ServiceInputs: map[string]map[string]interface{}{
					"world": {"rooms": []interface{}{1, 2}},
				}},
			},
			wantErrText: "service-inputs world.rooms must be a scalar value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (TaskConfig{Tasks: []Task{tt.task}}).Validate()
			if tt.wantErrText != "" {
				require.ErrorIs(t, err, ErrInvalidTaskProperty)
				assert.Contains(t, err.Error(), tt.wantErrText)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestToolSelectorInheritsServiceInputs(t *testing.T) {
	base := ToolSelector{ServiceInputs: map[string]map[string]interface{}{
		"world": {"rooms": 6, "machines": 1},
		"shop":  {"currency": "CAD"},
	}}
	override := ToolSelector{ServiceInputs: map[string]map[string]interface{}{
		"world": {"machines": 2, "seed": 17},
	}}

	inherited := base.MergeWith(nil)
	assert.Equal(t, base.ServiceInputs, inherited.ServiceInputs)

	merged := base.MergeWith(&override)
	assert.Equal(t, map[string]map[string]interface{}{
		"world": {"rooms": 6, "machines": 2, "seed": 17},
		"shop":  {"currency": "CAD"},
	}, merged.ServiceInputs)
	assert.Equal(t, map[string]interface{}{"rooms": 6, "machines": 1}, base.ServiceInputs["world"], "merging must not mutate inherited inputs")
}

func TestServiceConfigGetStartupTimeout(t *testing.T) {
	assert.Equal(t, DefaultServiceStartupTimeout, ServiceConfig{}.GetStartupTimeout())
	assert.Equal(t, 90*time.Second, ServiceConfig{StartupTimeout: testutils.Ptr(90 * time.Second)}.GetStartupTimeout())
}

func TestLoadConfigFromFileBindMountTargetsAndPaths(t *testing.T) {
	const baseConfig = `config:
  task-source: tasks.yaml
  output-dir: .
  providers:
    - name: openai
      client-config:
        api-key: test-key
      runs:
        - name: default
          model: test-model
`
	loadConfig := func(t *testing.T, section string) (*Config, string, error) {
		t.Helper()
		configPath := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(baseConfig+section), 0o600))
		cfg, err := LoadConfigFromFile(t.Context(), configPath)
		return cfg, configPath, err
	}

	tests := []struct {
		name        string
		firstTarget string
		target      string
		sharedDir   string
		wantErr     bool
	}{
		{
			name:    "ancestor",
			target:  "/mounted",
			wantErr: true,
		},
		{
			name:   "sibling",
			target: "/mounted/child",
		},
		{
			name:    "same cleaned target",
			target:  "/mounted/nested/",
			wantErr: true,
		},
		{
			name:    "descendant",
			target:  "/mounted/nested/log",
			wantErr: true,
		},
		{
			name:    "cleaned descendant",
			target:  "/other/../mounted/nested/log",
			wantErr: true,
		},
		{
			name:    "root target",
			target:  "/",
			wantErr: true,
		},
		{
			name:    "relative target",
			target:  "relative",
			wantErr: true,
		},
		{
			name:    "shared directory",
			target:  "/shared/log",
			wantErr: true,
		},
		{
			name:    "parameter file",
			target:  "/param",
			wantErr: true,
		},
		{
			name:    "auxiliary directory",
			target:  "/aux/file",
			wantErr: true,
		},
		{
			name:      "reserved root",
			target:    "/other",
			sharedDir: "/",
			wantErr:   true,
		},
		{
			name:        "component prefix",
			firstTarget: "/exports",
			target:      "/exports2/file",
		},
		{
			name:        "case sensitive paths",
			firstTarget: "/exports",
			target:      "/EXPORTS/file",
		},
		{
			name:        "backslash is not a separator",
			firstTarget: "/exports",
			target:      `/exports\file`,
		},
		{
			name:        "Windows reserved name is a child",
			firstTarget: "/exports",
			target:      "/exports/CON",
			wantErr:     true,
		},
		{
			name:        "colon in a POSIX child",
			firstTarget: "/exports",
			target:      "/exports/name:part",
			wantErr:     true,
		},
		{
			name:        "cleaned unrelated target",
			firstTarget: "/exports",
			target:      "/exports/../outside",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstTarget := tt.firstTarget
			if firstTarget == "" {
				firstTarget = "/mounted/nested"
			}
			sharedDir := tt.sharedDir
			if sharedDir == "" {
				sharedDir = "/shared"
			}
			section := fmt.Sprintf(`  tools:
    - name: tool
      image: tool:test
      description: Test tool
      parameters: {type: object}
      shared-dir: %q
      auxiliary-dir: /aux
      parameter-files: {p: /param}
      bind-mounts:
        - source: one
          target: %q
        - source: two
          target: %q
`, sharedDir, firstTarget, tt.target)
			cfg, configPath, err := loadConfig(t, section)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidConfigProperty)
				return
			}

			require.NoError(t, err)
			require.Len(t, cfg.Config.Tools, 1)
			mounts := cfg.Config.Tools[0].BindMounts
			require.Len(t, mounts, 2)
			assert.Equal(t, filepath.Join(filepath.Dir(configPath), "one"), mounts[0].Source)
			assert.Equal(t, filepath.Join(filepath.Dir(configPath), "two"), mounts[1].Source)
			assert.Equal(t, path.Clean(tt.target), mounts[1].Target)
			assert.NoDirExists(t, mounts[0].Source, "loading configuration must not create host directories")
		})
	}

	for _, tt := range []struct {
		name    string
		section string
	}{
		{
			name: "overlapping service mounts",
			section: `  services:
    - name: service
      image: service:test
      endpoint: {port: 8000}
      bind-mounts:
        - source: one
          target: /exports
        - source: two
          target: /exports/child
`,
		},
		{
			name: "validator template file",
			section: `  validators:
    - name: validator
      image: validator:test
      template-files:
        - path: /input/candidate
          template: candidate
      bind-mounts:
        - source: one
          target: /input/
`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadConfig(t, tt.section)
			require.ErrorIs(t, err, ErrInvalidConfigProperty)
		})
	}
}
