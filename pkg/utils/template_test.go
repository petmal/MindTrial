// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandTemplate(t *testing.T) {
	type candidate struct {
		Response interface{}
	}
	tests := []struct {
		name    string
		text    string
		data    any
		want    string
		wantErr bool
	}{
		{
			name: "map and struct fields",
			text: "{{ .Task.Name }}={{ .Candidate.Response }}",
			data: map[string]any{
				"Task":      map[string]any{"Name": "cart"},
				"Candidate": candidate{Response: "done"},
			},
			want: "cart=done",
		},
		{
			name: "stable hash",
			text: `{{ hash "eval-seed" "task-a" }}`,
			want: "9526074796148898333",
		},
		{
			name: "hash distinguishes argument boundaries",
			text: `{{ if eq (hash "ab" "c") (hash "a" "bc") }}same{{ else }}different{{ end }}`,
			want: "different",
		},
		{
			name: "json encodes structured values without HTML escaping",
			text: "{{ json .Value }}",
			data: map[string]any{"Value": map[string]any{"items": []any{"a<b", 2}}},
			want: `{"items":["a<b",2]}`,
		},
		{
			name: "json encodes an empty collection",
			text: "{{ json .Value }}",
			data: map[string]any{"Value": []any{}},
			want: "[]",
		},
		{
			name:    "missing map key",
			text:    "{{ .Evaluation.Seed }}",
			data:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "malformed template",
			text:    "{{ .Endpoint",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandTemplate("test", tt.text, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTemplate(t *testing.T) {
	_, err := ParseTemplate("helpers", `{{ hash .Evaluation.Seed .Task.Name }} {{ json .Candidate.Response }}`)
	require.NoError(t, err, "helpers must be available at parse time without data")

	_, err = ParseTemplate("unknown-function", "{{ unknown .Value }}")
	require.Error(t, err)

	_, err = ParseTemplate("malformed", "{{ .Value")
	require.Error(t, err)
}
