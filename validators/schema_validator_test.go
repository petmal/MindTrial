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
	validator := NewSchemaValidator()

	t.Run("numeric range pass", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"type":    "number",
			"minimum": 9.9,
			"maximum": 10.1,
		})
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult(10.0), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.True(t, result.IsCorrect)
		assert.Equal(t, "Schema Assessment", result.Title)
	})

	t.Run("numeric range fail", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"type":    "number",
			"minimum": 9.9,
			"maximum": 10.1,
		})
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult(11.0), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.False(t, result.IsCorrect)
		assert.Equal(t, "Schema Assessment", result.Title)
		// Explanation must contain the exact validator error.
		expectedErr := ""
		if e := utils.ValidateAgainstSchema(map[string]interface{}{"type": "number", "minimum": 9.9, "maximum": 10.1}, 11.0); e != nil {
			expectedErr = e.Error()
		}
		assert.Contains(t, result.Explanation, expectedErr)
	})

	t.Run("object schema pass", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{"type": "integer", "exclusiveMinimum": 0},
			},
			"required": []interface{}{"answer"},
		})
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult(map[string]interface{}{"answer": int64(4)}), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.True(t, result.IsCorrect)
	})

	t.Run("object schema fail", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{"type": "integer", "exclusiveMinimum": 0},
			},
			"required": []interface{}{"answer"},
		})
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult(map[string]interface{}{"answer": int64(-1)}), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.False(t, result.IsCorrect)
	})

	t.Run("schema without $schema", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"type": "string",
		})
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult("hello"), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.True(t, result.IsCorrect)
	})

	t.Run("multiple expected values returns error", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{"type": "string"}, map[string]interface{}{"type": "number"})
		_, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult("hello"), "", config.ResponseFormat{})
		require.Error(t, err)
	})

	t.Run("non-object expected value returns error", func(t *testing.T) {
		schema := utils.NewValueSet("not-an-object")
		_, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult("hello"), "", config.ResponseFormat{})
		require.Error(t, err)
	})

	t.Run("malformed schema returns error", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"$schema":    "https://json-schema.org/draft/2020-12/schema",
			"properties": "this_should_be_an_object_not_a_string",
		})
		_, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult(map[string]interface{}{"answer": int64(4)}), "", config.ResponseFormat{})
		require.Error(t, err)
	})

	t.Run("mismatch returns IsCorrect false with explanation", func(t *testing.T) {
		schemaMap := map[string]interface{}{"type": "string", "minLength": 5}
		schema := utils.NewValueSet(schemaMap)
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult("hi"), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.False(t, result.IsCorrect)
		expectedErr := ""
		if e := utils.ValidateAgainstSchema(schemaMap, "hi"); e != nil {
			expectedErr = e.Error()
		}
		assert.Contains(t, result.Explanation, expectedErr)
	})

	t.Run("case and whitespace rules do not affect schema validation", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{"type": "string", "const": "Hello"})
		rules := config.ValidationRules{CaseSensitive: testutils.Ptr(false), IgnoreWhitespace: testutils.Ptr(true)}
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), rules, schema, createMockResult("hello"), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.False(t, result.IsCorrect, "schema validation must not be case-insensitive")
	})

	t.Run("explicit schema with $schema still works", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{"type": "integer", "exclusiveMinimum": 0},
			},
			"required": []interface{}{"answer"},
		})
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult(map[string]interface{}{"answer": int64(4)}), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.True(t, result.IsCorrect)
	})

	t.Run("plain text string does not coerce to number", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"type":    "number",
			"minimum": 9.9,
			"maximum": 10.1,
		})
		result, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult("10"), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.False(t, result.IsCorrect, "schema validation must not coerce string \"10\" to number 10")
	})

	t.Run("plain text string pattern pass and fail", func(t *testing.T) {
		schema := utils.NewValueSet(map[string]interface{}{
			"type":    "string",
			"pattern": "^#[0-9A-Fa-f]{6}$",
		})
		pass, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult("#1a2B3c"), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.True(t, pass.IsCorrect)

		fail, err := validator.IsCorrect(context.Background(), testutils.NewTestLogger(t), config.ValidationRules{}, schema, createMockResult("not-a-colour"), "", config.ResponseFormat{})
		require.NoError(t, err)
		assert.False(t, fail.IsCorrect)
	})
}

func TestSchemaValidatorToCanonical(t *testing.T) {
	validator := NewSchemaValidator()
	rules := config.ValidationRules{IgnoreWhitespace: testutils.Ptr(true), CaseSensitive: testutils.Ptr(false)}
	assert.Equal(t, "a b c", validator.ToCanonical(rules, "a b c"))
	assert.Equal(t, map[string]interface{}{"key": "Value"}, validator.ToCanonical(rules, map[string]interface{}{"key": "Value"}))
	assert.Equal(t, 42, validator.ToCanonical(rules, 42))
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
