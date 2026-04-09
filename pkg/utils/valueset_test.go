// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestValueSet_NewValueSet(t *testing.T) {
	v := NewValueSet("a", "b", 123, "c")
	assert.ElementsMatch(t, []interface{}{"a", "b", 123, "c"}, v.Values())

	// Test duplicate removal.
	vWithDuplicates := NewValueSet("a", "b", "a", 123, "b", "c")
	assert.ElementsMatch(t, []interface{}{"a", "b", 123, "c"}, vWithDuplicates.Values())
}

func TestValueSet_Any(t *testing.T) {
	v := NewValueSet("a", "b", 123, "c")
	assert.True(t, v.Any(func(val interface{}) bool { return val == "b" }))
	assert.True(t, v.Any(func(val interface{}) bool { return val == 123 }))
	assert.False(t, v.Any(func(val interface{}) bool { return val == "z" }))
	assert.False(t, v.Any(func(val interface{}) bool { return val == 999 }))
}

func TestValueSet_Map(t *testing.T) {
	v := NewValueSet("a", "A", "b", "c")
	require.ElementsMatch(t, []interface{}{"a", "A", "b", "c"}, v.Values())

	// Map strings to uppercase.
	mapped := v.Map(func(val interface{}) interface{} {
		if str, ok := val.(string); ok {
			return strings.ToUpper(str)
		}
		return val
	})
	assert.ElementsMatch(t, []interface{}{"A", "B", "C"}, mapped.Values())
}

func TestValueSet_AsStringSet(t *testing.T) {
	// Test with all strings.
	v1 := NewValueSet("a", "b", "c")
	stringSet, ok := v1.AsStringSet()
	assert.True(t, ok)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, stringSet.Values())

	// Test with mixed types.
	v2 := NewValueSet("a", 123, "c")
	_, ok = v2.AsStringSet()
	assert.False(t, ok)

	// Test with no values.
	v3 := NewValueSet()
	stringSet, ok = v3.AsStringSet()
	assert.True(t, ok)
	assert.Empty(t, stringSet.Values())
}

func TestValueSet_YAMLUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected []interface{}
	}{
		{
			name:     "single string value",
			yaml:     "foo",
			expected: []interface{}{"foo"},
		},
		{
			name:     "list of strings",
			yaml:     "- a\n- b\n- c",
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name:     "mixed types",
			yaml:     "- hello\n- 123\n- true",
			expected: []interface{}{"hello", 123, true},
		},
		{
			name:     "single number",
			yaml:     "42",
			expected: []interface{}{42},
		},
		{
			name: "list with map objects",
			yaml: `- answer: "YES"
  confidence: 0.95
- answer: "NO"
  confidence: 0.90`,
			expected: []interface{}{
				map[string]interface{}{"answer": "YES", "confidence": 0.95},
				map[string]interface{}{"answer": "NO", "confidence": 0.90},
			},
		},
		{
			name: "list with nested objects",
			yaml: `- name: "test"
  values: [1, 2, 3]
- name: "other"
  values: [4, 5, 6]`,
			expected: []interface{}{
				map[string]interface{}{"name": "test", "values": []interface{}{1, 2, 3}},
				map[string]interface{}{"name": "other", "values": []interface{}{4, 5, 6}},
			},
		},
		{
			name: "single map object",
			yaml: `answer: "YES"
confidence: 0.95`,
			expected: []interface{}{
				map[string]interface{}{"answer": "YES", "confidence": 0.95},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var unmarshaled ValueSet
			err := yaml.Unmarshal([]byte(tt.yaml), &unmarshaled)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, unmarshaled.Values())
		})
	}
}

func TestValueSet_YAMLUnmarshal_Error(t *testing.T) {
	var unmarshaled ValueSet

	// Test invalid YAML syntax.
	err := yaml.Unmarshal([]byte("invalid: - :"), &unmarshaled)
	require.Error(t, err)
}

func TestValueSet_YAMLMarshal(t *testing.T) {
	tests := []struct {
		name     string
		values   []interface{}
		contains []string // strings that should be present in the marshaled YAML.
	}{
		{
			name:     "single value",
			values:   []interface{}{"single"},
			contains: []string{"single"},
		},
		{
			name:     "multiple strings",
			values:   []interface{}{"a", "b", "c"},
			contains: []string{"a", "b", "c"},
		},
		{
			name:     "mixed types",
			values:   []interface{}{"hello", 123, true},
			contains: []string{"hello", "123", "true"},
		},
		{
			name: "map objects",
			values: []interface{}{
				map[string]interface{}{"answer": "YES", "confidence": 0.95},
				map[string]interface{}{"answer": "NO", "confidence": 0.90},
			},
			contains: []string{"answer", "YES", "NO", "confidence", "0.95", "0.9"},
		},
		{
			name: "nested objects",
			values: []interface{}{
				map[string]interface{}{"name": "test", "values": []interface{}{1, 2, 3}},
			},
			contains: []string{"name", "test", "values", "1", "2", "3"},
		},
		{
			name: "single map",
			values: []interface{}{
				map[string]interface{}{"key": "value"},
			},
			contains: []string{"key", "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValueSet(tt.values...)
			marshaled, err := yaml.Marshal(v)
			require.NoError(t, err)

			marshaledStr := string(marshaled)
			for _, expected := range tt.contains {
				assert.Contains(t, marshaledStr, expected)
			}
		})
	}
}

func TestValueSet_Values(t *testing.T) {
	// Test that Values returns a copy, not the original slice.
	v := NewValueSet("a", "b", "c")
	values1 := v.Values()
	values2 := v.Values()

	assert.NotSame(t, &values1[0], &values2[0], "Each call to Values() should return a different slice reference")
	assert.ElementsMatch(t, []interface{}{"a", "b", "c"}, v.Values())
}

func TestValueSet_EmptySet(t *testing.T) {
	v := NewValueSet()
	assert.Empty(t, v.Values())
	assert.False(t, v.Any(func(interface{}) bool { return true }))

	mapped := v.Map(func(val interface{}) interface{} { return val })
	assert.Empty(t, mapped.Values())

	stringSet, ok := v.AsStringSet()
	assert.True(t, ok)
	assert.Empty(t, stringSet.Values())
}

func TestValueSet_JSONMarshal(t *testing.T) {
	tests := []struct {
		name     string
		values   []interface{}
		expected string
	}{
		{
			name:     "single string",
			values:   []interface{}{"hello"},
			expected: `"hello"`,
		},
		{
			name:     "single number",
			values:   []interface{}{42},
			expected: `42`,
		},
		{
			name:     "multiple strings",
			values:   []interface{}{"a", "b", "c"},
			expected: `["a","b","c"]`,
		},
		{
			name:     "mixed types",
			values:   []interface{}{"hello", 123, true},
			expected: `["hello",123,true]`,
		},
		{
			name: "single map object",
			values: []interface{}{
				map[string]interface{}{"key": "value"},
			},
			expected: `{"key":"value"}`,
		},
		{
			name: "list of map objects",
			values: []interface{}{
				map[string]interface{}{"answer": "YES", "confidence": 0.95},
				map[string]interface{}{"answer": "NO", "confidence": 0.90},
			},
			expected: `[{"answer":"YES","confidence":0.95},{"answer":"NO","confidence":0.9}]`,
		},
		{
			name: "nested objects",
			values: []interface{}{
				map[string]interface{}{"name": "test", "values": []interface{}{1, 2, 3}},
			},
			expected: `{"name":"test","values":[1,2,3]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValueSet(tt.values...)
			data, err := json.Marshal(v)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestValueSet_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected []interface{}
	}{
		{
			name:     "single string",
			json:     `"hello"`,
			expected: []interface{}{"hello"},
		},
		{
			name:     "single number preserves precision",
			json:     `9876543210`,
			expected: []interface{}{json.Number("9876543210")},
		},
		{
			name:     "array of strings",
			json:     `["a","b","c"]`,
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name:     "boolean",
			json:     `true`,
			expected: []interface{}{true},
		},
		{
			name: "array of map objects",
			json: `[{"answer":"YES","confidence":0.95},{"answer":"NO","confidence":0.90}]`,
			expected: []interface{}{
				map[string]interface{}{"answer": "YES", "confidence": json.Number("0.95")},
				map[string]interface{}{"answer": "NO", "confidence": json.Number("0.90")},
			},
		},
		{
			name: "single map object",
			json: `{"key":"value"}`,
			expected: []interface{}{
				map[string]interface{}{"key": "value"},
			},
		},
		{
			name:     "mixed types",
			json:     `["hello",123,true]`,
			expected: []interface{}{"hello", json.Number("123"), true},
		},
		{
			name: "nested objects",
			json: `[{"name":"test","values":[1,2,3]},{"name":"other","values":[4,5,6]}]`,
			expected: []interface{}{
				map[string]interface{}{"name": "test", "values": []interface{}{json.Number("1"), json.Number("2"), json.Number("3")}},
				map[string]interface{}{"name": "other", "values": []interface{}{json.Number("4"), json.Number("5"), json.Number("6")}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v ValueSet
			err := json.Unmarshal([]byte(tt.json), &v)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, v.Values())
		})
	}
}

func TestValueSet_JSONUnmarshal_Error(t *testing.T) {
	var v ValueSet
	err := json.Unmarshal([]byte(`{invalid}`), &v)
	require.Error(t, err)
}

func TestValueSet_JSONRoundTrip(t *testing.T) {
	original := NewValueSet("hello", "world", 3.14, 9999999999999)
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored ValueSet
	require.NoError(t, json.Unmarshal(data, &restored))

	// Numeric types become json.Number after round-trip (UseNumber preserves precision).
	expected := []interface{}{"hello", "world", json.Number("3.14"), json.Number("9999999999999")}
	assert.Equal(t, expected, restored.Values())

	// Re-marshaling produces identical JSON regardless of the underlying numeric type.
	restoredData, err := json.Marshal(restored)
	require.NoError(t, err)
	assert.JSONEq(t, string(data), string(restoredData))
}
