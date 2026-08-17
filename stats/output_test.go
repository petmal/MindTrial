// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/pkg/testutils"
)

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    OutputFormat
		wantErr bool
	}{
		{name: "blank defaults to text", input: "", want: OutputFormatText},
		{name: "text", input: "TEXT", want: OutputFormatText},
		{name: "csv", input: "csv", want: OutputFormatCSV},
		{name: "json", input: "json", want: OutputFormatJSON},
		{name: "jsonl", input: "jsonl", want: OutputFormatJSONL},
		{name: "invalid", input: "xml", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOutputFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidOutputFormat)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func sampleRecords() []Record {
	return []Record{
		{
			Dimensions:        map[string]string{"provider": "openai", "run": "gpt-4"},
			Count:             2,
			Passed:            1,
			Failed:            1,
			PassRate:          50,
			Accuracy:          50,
			TotalDuration:     testutils.Ptr(time.Duration(1000)),
			TotalInputTokens:  testutils.Ptr(int64(100)),
			TotalOutputTokens: testutils.Ptr(int64(50)),
		},
		{
			Dimensions: map[string]string{"provider": "anthropic", "run": "claude"},
			Count:      1,
			Passed:     1,
			PassRate:   100,
			Accuracy:   100,
		},
	}
}

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	groupBy := []Dimension{DimensionProvider, DimensionRun}
	require.NoError(t, Write(OutputFormatCSV, groupBy, sampleRecords(), &buf))

	r := csv.NewReader(&buf)
	rows, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3) // header + 2 records

	assert.Equal(t, "provider", rows[0][0])
	assert.Equal(t, "run", rows[0][1])
	assert.Equal(t, "Count", rows[0][2])

	assert.Equal(t, "openai", rows[1][0])
	assert.Equal(t, "gpt-4", rows[1][1])
	assert.Equal(t, "2", rows[1][2])

	assert.Equal(t, "anthropic", rows[2][0])
	assert.Equal(t, "claude", rows[2][1])
}

func TestWriteText(t *testing.T) {
	var buf bytes.Buffer
	groupBy := []Dimension{DimensionProvider, DimensionRun}
	require.NoError(t, Write(OutputFormatText, groupBy, sampleRecords(), &buf))

	out := buf.String()
	assert.Contains(t, out, "provider")
	assert.Contains(t, out, "openai")
	assert.Contains(t, out, "anthropic")

	scanner := bufio.NewScanner(strings.NewReader(out))
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}
	assert.Equal(t, 3, lineCount) // header + 2 records
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Write(OutputFormatJSON, nil, sampleRecords(), &buf))

	var got []Record
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "openai", got[0].Dimensions["provider"])
	assert.Equal(t, 2, got[0].Count)
}

func TestWriteJSONL(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Write(OutputFormatJSONL, nil, sampleRecords(), &buf))

	scanner := bufio.NewScanner(strings.NewReader(buf.String()))
	var lines []string
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	require.Len(t, lines, 2)

	for _, line := range lines {
		var rec Record
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
	}
}
