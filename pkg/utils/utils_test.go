// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package utils

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNoPanic(t *testing.T) {
	tests := []struct {
		name    string
		fn      func() error
		wantErr bool
	}{
		{
			name: "no panic",
			fn: func() error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "panic occurs",
			fn: func() error {
				panic("something went wrong")
			},
			wantErr: true,
		},
		{
			name: "function returns error",
			fn: func() error {
				return errors.ErrUnsupported
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NoPanic(tt.fn)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestJSONFromMarkdown(t *testing.T) {
	type args struct {
		content string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "valid JSON block",
			args: args{
				content: "Here is some JSON: ```json {\"key\": \"value\"} ```",
			},
			want: "{\"key\": \"value\"}",
		},
		{
			name: "no JSON block",
			args: args{
				content: "Here is some text without JSON.",
			},
			want: "Here is some text without JSON.",
		},
		{
			name: "malformed JSON block",
			args: args{
				content: "Here is some malformed JSON: ```json {key: value} ```",
			},
			want: "{key: value}",
		},
		{
			name: "multiple JSON blocks",
			args: args{
				content: "First block: ```json {\"key1\": \"value1\"} ``` Second block: ```json {\"key2\": \"value2\"} ```",
			},
			want: "{\"key1\": \"value1\"}",
		},
		{
			name: "JSON block with extra spaces",
			args: args{
				content: "Here is some JSON with spaces: ```json   {\"key\": \"value\"}   ```",
			},
			want: "{\"key\": \"value\"}",
		},
		{
			name: "valid JSON block with newlines",
			args: args{
				content: "Here is some JSON: ```json\n{\n \"key\": \"value\"\n}\n```",
			},
			want: "{\n \"key\": \"value\"\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, JSONFromMarkdown(tt.args.content))
		})
	}
}

func TestRepairTextJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "simple valid JSON",
			content: `{"key": "value"}`,
			want:    `{"key": "value"}`,
			wantErr: false,
		},
		{
			name:    "simple invalid JSON",
			content: `{"key": "value"`,
			want:    `{"key": "value"}`,
			wantErr: false,
		},
		{
			name:    "empty content",
			content: ``,
			wantErr: true,
		},
		{
			name: "invalid JSON with multiline strings",
			content: `{
  "title": "Tempore sed veritatis autem accusantium qui voluptatem nulla numquam ea.",
  "explanation": "Placeat officia quidem labore odio velit ipsa.:

1. **sunt 1**: Est autem ducimus hic non ipsam quo dolore. 
   - 8803409999911966 - 2065609999979344 = 9163509999908364
   - 63959 - 47682 = 70893
   - 32458.26 - 20117.49 = 22241.36
   - commodi non consequatur 819609999991804 - 0x4 = **92232.95**. accusantium, voluptatem vel 51645 voluptas deleniti aliquid **necessitatibus) 822**.",
  "final_answer": "1. Product\n2. Soap\n3. Devolved\n4. connecting\n5. system-worthy"
}`,
			want: `{
  "title": "Tempore sed veritatis autem accusantium qui voluptatem nulla numquam ea.",
  "explanation": "Placeat officia quidem labore odio velit ipsa.:\n\n1. **sunt 1**: Est autem ducimus hic non ipsam quo dolore. \n   - 8803409999911966 - 2065609999979344 = 9163509999908364\n   - 63959 - 47682 = 70893\n   - 32458.26 - 20117.49 = 22241.36\n   - commodi non consequatur 819609999991804 - 0x4 = **92232.95**. accusantium, voluptatem vel 51645 voluptas deleniti aliquid **necessitatibus) 822**.",
  "final_answer": "1. Product\n2. Soap\n3. Devolved\n4. connecting\n5. system-worthy"
}`,
			wantErr: false,
		},
		{
			name: "invalid JSON with markdown",
			content: "```json" + `{
  "title": "Tempore sed veritatis autem accusantium qui voluptatem nulla numquam ea.",
  "explanation": "Placeat officia quidem labore odio velit ipsa.:

1. **sunt 1**: Est autem ducimus hic non ipsam quo dolore. 
   - 8803409999911966 - 2065609999979344 = 9163509999908364
   - 63959 - 47682 = 70893
   - 32458.26 - 20117.49 = 22241.36
   - commodi non consequatur 819609999991804 - 0x4 = **92232.95**. accusantium, voluptatem vel 51645 voluptas deleniti aliquid **necessitatibus) 822**.",
  "final_answer": "1. Product\n2. Soap\n3. Devolved\n4. connecting\n5. system-worthy"
}` + "```",
			want: `{
  "title": "Tempore sed veritatis autem accusantium qui voluptatem nulla numquam ea.",
  "explanation": "Placeat officia quidem labore odio velit ipsa.:\n\n1. **sunt 1**: Est autem ducimus hic non ipsam quo dolore. \n   - 8803409999911966 - 2065609999979344 = 9163509999908364\n   - 63959 - 47682 = 70893\n   - 32458.26 - 20117.49 = 22241.36\n   - commodi non consequatur 819609999991804 - 0x4 = **92232.95**. accusantium, voluptatem vel 51645 voluptas deleniti aliquid **necessitatibus) 822**.",
  "final_answer": "1. Product\n2. Soap\n3. Devolved\n4. connecting\n5. system-worthy"
}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RepairTextJSON(tt.content)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestStringSet_NewStringSet(t *testing.T) {
	s := NewStringSet("a", "b", "a", "c")
	assert.ElementsMatch(t, []string{"a", "b", "c"}, s.Values())
}

func TestStringSet_Any(t *testing.T) {
	s := NewStringSet("a", "b", "c")
	assert.True(t, s.Any(func(v string) bool { return v == "b" }))
	assert.False(t, s.Any(func(v string) bool { return v == "z" }))
}

func TestStringSet_Map(t *testing.T) {
	s := NewStringSet("a", "A", "b", "c")
	require.ElementsMatch(t, []string{"a", "A", "b", "c"}, s.Values())
	mapped := s.Map(strings.ToUpper) // "a" and "A" will both map to "A"
	assert.ElementsMatch(t, []string{"A", "B", "C"}, mapped.Values())
}

func TestStringSet_YAMLUnmarshal(t *testing.T) {
	var unmarshaled StringSet
	err := yaml.Unmarshal([]byte("- a\n- b\n- a\n- c\n"), &unmarshaled)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, unmarshaled.Values())

	err = yaml.Unmarshal([]byte("foo"), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, []string{"foo"}, unmarshaled.Values())
}

func TestStringSet_YAMLMarshal(t *testing.T) {
	s := NewStringSet("a", "b", "c")
	marshaled, err := yaml.Marshal(s)
	require.NoError(t, err)
	assert.Contains(t, string(marshaled), "a")
	assert.Contains(t, string(marshaled), "b")
	assert.Contains(t, string(marshaled), "c")
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "single line",
			input: "single line",
			want:  []string{"single line"},
		},
		{
			name:  "multiple lines",
			input: "first line\r\nsecond line\nthird line",
			want:  []string{"first line", "second line", "third line"},
		},
		{
			name:  "double newlines",
			input: "first line\n\nsecond line\r\n\r\nthird line",
			want:  []string{"first line", "", "second line", "", "third line"},
		},
		{
			name:  "multiple newlines",
			input: "first line\n\r\n\nsecond line\n\r\n\r\n\r\nthird line",
			want:  []string{"first line", "", "", "second line", "", "", "", "third line"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitLines(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateAgainstSchema(t *testing.T) {
	tests := []struct {
		name    string
		schema  map[string]interface{}
		values  []interface{}
		wantErr bool
		errType error
	}{
		{
			name: "valid schema with valid value",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type": "string",
					},
					"age": map[string]interface{}{
						"type": "number",
					},
				},
				"required": []interface{}{"name"},
			},
			values: []interface{}{
				map[string]interface{}{
					"name": "John",
					"age":  30,
				},
			},
			wantErr: false,
		},
		{
			name: "valid schema with multiple valid values",
			schema: map[string]interface{}{
				"type": "string",
			},
			values: []interface{}{
				"hello",
				"world",
				"test",
			},
			wantErr: false,
		},
		{
			name: "valid schema with no values",
			schema: map[string]interface{}{
				"type": "string",
			},
			values:  []interface{}{},
			wantErr: false,
		},
		{
			name: "invalid schema",
			schema: map[string]interface{}{
				"type": "invalid_type",
			},
			values: []interface{}{
				"test",
			},
			wantErr: true,
			errType: ErrInvalidJSONSchema,
		},
		{
			name: "valid schema with invalid value",
			schema: map[string]interface{}{
				"type": "string",
			},
			values: []interface{}{
				123, // number instead of string
			},
			wantErr: true,
			errType: ErrJSONSchemaValidation,
		},
		{
			name: "valid schema with mixed valid and invalid values",
			schema: map[string]interface{}{
				"type": "string",
			},
			values: []interface{}{
				"valid",
				123, // invalid
			},
			wantErr: true,
			errType: ErrJSONSchemaValidation,
		},
		{
			name: "object schema with missing required field",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type": "string",
					},
					"age": map[string]interface{}{
						"type": "number",
					},
				},
				"required": []interface{}{"name"},
			},
			values: []interface{}{
				map[string]interface{}{
					"age": 30, // missing required "name"
				},
			},
			wantErr: true,
			errType: ErrJSONSchemaValidation,
		},
		{
			name: "array schema validation",
			schema: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "number",
				},
			},
			values: []interface{}{
				[]interface{}{1, 2, 3},
				[]interface{}{4.5, 6.7},
			},
			wantErr: false,
		},
		{
			name: "array schema with invalid items",
			schema: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "number",
				},
			},
			values: []interface{}{
				[]interface{}{1, "string", 3}, // string in number array
			},
			wantErr: true,
			errType: ErrJSONSchemaValidation,
		},
		{
			name: "malformed schema structure",
			schema: map[string]interface{}{
				"properties": "invalid", // should be object
			},
			values: []interface{}{
				"test",
			},
			wantErr: true,
			errType: ErrInvalidJSONSchema,
		},
		{
			name: "simple number validation",
			schema: map[string]interface{}{
				"type": "number",
			},
			values: []interface{}{
				42,
				3.14,
			},
			wantErr: false,
		},
		{
			name: "boolean validation",
			schema: map[string]interface{}{
				"type": "boolean",
			},
			values: []interface{}{
				true,
				false,
			},
			wantErr: false,
		},
		{
			name: "null validation",
			schema: map[string]interface{}{
				"type": "null",
			},
			values: []interface{}{
				nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgainstSchema(tt.schema, tt.values...)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					require.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSortedKeys(t *testing.T) {
	tests := []struct {
		name string
		m    map[int]interface{}
		want []int
	}{
		{
			name: "empty map",
			m:    map[int]interface{}{},
			want: []int{},
		},
		{
			name: "single element",
			m:    map[int]interface{}{1: nil},
			want: []int{1},
		},
		{
			name: "multiple elements",
			m:    map[int]interface{}{3: nil, 1: nil, 2: nil},
			want: []int{1, 2, 3},
		},
		{
			name: "negative and positive keys",
			m:    map[int]interface{}{-1: nil, 2: nil, -3: nil, 0: nil},
			want: []int{-3, -1, 0, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SortedKeys(tt.m))
		})
	}
}
