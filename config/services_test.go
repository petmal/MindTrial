// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
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
