// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGradeVerdict(t *testing.T) {
	tests := []struct {
		name            string
		rules           config.ValidationRules
		passingVerdicts utils.ValueSet
		verdict         interface{}
		want            bool
		wantErr         bool
	}{
		{
			name:            "boolean const match",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{"correct": true}),
			verdict:         map[string]interface{}{"correct": true},
			want:            true,
		},
		{
			name:            "boolean const mismatch",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{"correct": true}),
			verdict:         map[string]interface{}{"correct": false},
			want:            false,
		},
		{
			name:            "multiple values enum match",
			passingVerdicts: utils.NewValueSet("Yes", "Y"),
			verdict:         "Y",
			want:            true,
		},
		{
			name:            "multiple values enum mismatch",
			passingVerdicts: utils.NewValueSet("Yes", "Y"),
			verdict:         "No",
			want:            false,
		},
		{
			name:            "case-insensitive string criterion",
			rules:           config.ValidationRules{},
			passingVerdicts: utils.NewValueSet("yes"),
			verdict:         "YES",
			want:            true,
		},
		{
			name:            "case-sensitive string criterion",
			rules:           config.ValidationRules{CaseSensitive: testutils.Ptr(true)},
			passingVerdicts: utils.NewValueSet("yes"),
			verdict:         "YES",
			want:            false,
		},
		{
			name:            "whitespace normalization parity",
			rules:           config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)},
			passingVerdicts: utils.NewValueSet("a b c"),
			verdict:         "a  b\tc",
			want:            true,
		},
		{
			name:            "numeric canonicalization parity",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{"score": int64(90)}),
			verdict:         map[string]interface{}{"score": float64(90)},
			want:            true,
		},
		{
			name:            "nested maps and slices",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{"tags": []interface{}{"a", "b"}}),
			verdict:         map[string]interface{}{"tags": []interface{}{"a", "b"}},
			want:            true,
		},
		{
			name: "score threshold pass via explicit schema",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]interface{}{
					"score": map[string]interface{}{"type": "integer", "exclusiveMinimum": 80},
				},
				"required": []interface{}{"score"},
			}),
			verdict: map[string]interface{}{"score": int64(90)},
			want:    true,
		},
		{
			name: "score threshold fail via explicit schema",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]interface{}{
					"score": map[string]interface{}{"type": "integer", "exclusiveMinimum": 80},
				},
				"required": []interface{}{"score"},
			}),
			verdict: map[string]interface{}{"score": int64(50)},
			want:    false,
		},
		{
			name: "grade enum pass via explicit schema",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]interface{}{
					"grade": map[string]interface{}{"enum": []interface{}{"A", "B", "C"}},
				},
				"required": []interface{}{"grade"},
			}),
			verdict: map[string]interface{}{"grade": "B"},
			want:    true,
		},
		{
			name: "explicit schema validates raw data without normalization",
			rules: config.ValidationRules{
				// Even with case-insensitive rules configured, an explicit schema
				// must validate the raw verdict rather than a normalized copy.
				CaseSensitive: testutils.Ptr(false),
			},
			passingVerdicts: utils.NewValueSet(map[string]interface{}{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]interface{}{
					"grade": map[string]interface{}{"const": "A"},
				},
				"required": []interface{}{"grade"},
			}),
			verdict: map[string]interface{}{"grade": "a"},
			want:    false,
		},
		{
			name: "malformed explicit schema errors",
			passingVerdicts: utils.NewValueSet(map[string]interface{}{
				"$schema":    "https://json-schema.org/draft/2020-12/schema",
				"properties": "this_should_be_an_object_not_a_string",
			}),
			verdict: map[string]interface{}{"grade": "A"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GradeVerdict(tt.rules, tt.passingVerdicts, tt.verdict)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSchemaValidatorIsCorrect(t *testing.T) {
	tests := []struct {
		name     string
		rules    config.ValidationRules
		expected utils.ValueSet
		actual   interface{}
		want     bool
		wantErr  bool
	}{
		{
			name:     "literal value match",
			expected: utils.NewValueSet("42"),
			actual:   "42",
			want:     true,
		},
		{
			name:     "literal value mismatch",
			expected: utils.NewValueSet("42"),
			actual:   "43",
			want:     false,
		},
		{
			name: "explicit schema match",
			expected: utils.NewValueSet(map[string]interface{}{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]interface{}{
					"answer": map[string]interface{}{"type": "integer", "exclusiveMinimum": 0},
				},
				"required": []interface{}{"answer"},
			}),
			actual: map[string]interface{}{"answer": int64(4)},
			want:   true,
		},
		{
			name: "explicit schema mismatch",
			expected: utils.NewValueSet(map[string]interface{}{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]interface{}{
					"answer": map[string]interface{}{"type": "integer", "exclusiveMinimum": 0},
				},
				"required": []interface{}{"answer"},
			}),
			actual:  map[string]interface{}{"answer": int64(-1)},
			want:    false,
			wantErr: false,
		},
		{
			name: "malformed explicit schema errors",
			expected: utils.NewValueSet(map[string]interface{}{
				"$schema":    "https://json-schema.org/draft/2020-12/schema",
				"properties": "this_should_be_an_object_not_a_string",
			}),
			actual:  map[string]interface{}{"answer": int64(4)},
			wantErr: true,
		},
	}

	validator := NewSchemaValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), tt.rules, tt.expected, createMockResult(tt.actual), "", config.ResponseFormat{})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.IsCorrect)
		})
	}
}

func TestSchemaValidatorToCanonical(t *testing.T) {
	rules := config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true)}
	validator := NewSchemaValidator()
	assert.Equal(t, "abc", validator.ToCanonical(rules, "a b c"))
}

func TestSchemaValidatorGetName(t *testing.T) {
	assert.Equal(t, "schema match", NewSchemaValidator().GetName())
}

func TestSchemaValidatorClose(t *testing.T) {
	assert.NoError(t, NewSchemaValidator().Close(context.Background()))
}

func TestNewSchemaValidatorIsSingleton(t *testing.T) {
	assert.Same(t, NewSchemaValidator(), NewSchemaValidator())
}
