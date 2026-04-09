// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateGoldenJSON(t *testing.T) {
	updateGoldenFiles(t, NewJSONCodec(), []goldenFileTestCase{
		{"testdata/empty.json", runners.Results{}},
		{"testdata/results.json", mockResults},
	})
}

func TestJSONCodecWrite(t *testing.T) {
	tests := []struct {
		name    string
		results runners.Results
		want    string
	}{
		{
			name:    "format no results",
			results: runners.Results{},
			want:    "testdata/empty.json",
		},
		{
			name:    "format some results",
			results: mockResults,
			want:    "testdata/results.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFixedMetadata(t, func() {
				codec := NewJSONCodec()
				assertFormatterOutputFromFile(t, codec, tt.results, tt.want)
			})
		})
	}
}

func TestJSONCodecRead(t *testing.T) {
	codec := NewJSONCodec()

	t.Run("round-trip", func(t *testing.T) {
		withFixedMetadata(t, func() {
			var buf bytes.Buffer
			require.NoError(t, codec.Write(mockResults, &buf))

			got, err := codec.Read(&buf)
			require.NoError(t, err)

			expected := testutils.ReadFile(t, "testdata/results.json")

			var actual bytes.Buffer
			require.NoError(t, codec.Write(got, &actual))

			assert.Equal(t, string(expected), actual.String())
		})
	})

	t.Run("empty results round-trip", func(t *testing.T) {
		withFixedMetadata(t, func() {
			var buf bytes.Buffer
			require.NoError(t, codec.Write(runners.Results{}, &buf))

			got, err := codec.Read(&buf)
			require.NoError(t, err)

			var expected bytes.Buffer
			require.NoError(t, codec.Write(runners.Results{}, &expected))

			var actual bytes.Buffer
			require.NoError(t, codec.Write(got, &actual))

			assert.Equal(t, expected.String(), actual.String())
		})
	})

	t.Run("invalid format version", func(t *testing.T) {
		data := []byte(`{"FormatVersion": 999, "Results": {}}`)
		_, err := codec.Read(bytes.NewReader(data))
		require.Error(t, err)
		require.ErrorIs(t, err, ErrReadResults)
		assert.Contains(t, err.Error(), "unsupported format version 999")
	})

	t.Run("malformed JSON", func(t *testing.T) {
		data := []byte(`{invalid json}`)
		_, err := codec.Read(bytes.NewReader(data))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrReadResults)
	})

	t.Run("trailing JSON document", func(t *testing.T) {
		data := []byte(`{"FormatVersion": 1, "Results": {}}{"extra": true}`)
		_, err := codec.Read(bytes.NewReader(data))
		require.Error(t, err)
		require.ErrorIs(t, err, ErrReadResults)
		assert.Contains(t, err.Error(), "unexpected trailing data")
	})

	t.Run("trailing garbage", func(t *testing.T) {
		data := []byte(`{"FormatVersion": 1, "Results": {}} garbage`)
		_, err := codec.Read(bytes.NewReader(data))
		require.Error(t, err)
		require.ErrorIs(t, err, ErrReadResults)
		assert.Contains(t, err.Error(), "unexpected trailing data")
	})

	t.Run("provider key mismatch", func(t *testing.T) {
		data := []byte(`{
  "FormatVersion": 1,
  "Results": {
    "ProviderX": [
      {
        "TraceID": "t1",
        "Kind": "Passed",
        "Task": "task1",
        "Provider": "ProviderY",
        "Run": "run1",
        "Got": "a",
        "Want": "a",
        "Details": {},
        "DurationNS": 1000000000
      }
    ]
  }
}`)
		_, err := codec.Read(bytes.NewReader(data))
		require.Error(t, err)
		require.ErrorIs(t, err, ErrReadResults)
		assert.Contains(t, err.Error(), "does not match")
	})
}

func TestJSONCodecReadFromFile(t *testing.T) {
	codec := NewJSONCodec()

	t.Run("read from golden file", func(t *testing.T) {
		withFixedMetadata(t, func() {
			f, err := os.Open("testdata/results.json")
			require.NoError(t, err)
			defer f.Close()

			got, err := codec.Read(f)
			require.NoError(t, err)

			expected := testutils.ReadFile(t, "testdata/results.json")

			var actual bytes.Buffer
			require.NoError(t, codec.Write(got, &actual))

			assert.Equal(t, string(expected), actual.String())
		})
	})
}

func TestJSONCodecFileExt(t *testing.T) {
	codec := NewJSONCodec()
	assert.Equal(t, "json", codec.FileExt())
}

func TestReadResultsFromFile(t *testing.T) {
	t.Run("unsupported extension", func(t *testing.T) {
		_, err := ReadResultsFromFile("file.xyz")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedInputFormat)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ReadResultsFromFile("nonexistent.json")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrReadResults)
	})

	t.Run("read JSON file", func(t *testing.T) {
		withFixedMetadata(t, func() {
			got, err := ReadResultsFromFile("testdata/results.json")
			require.NoError(t, err)

			codec := NewJSONCodec()

			expected := testutils.ReadFile(t, "testdata/results.json")

			var actual bytes.Buffer
			require.NoError(t, codec.Write(got, &actual))

			assert.Equal(t, string(expected), actual.String())
		})
	})
}

func TestJSONCodecCrossFormatConsistency(t *testing.T) {
	results, err := ReadResultsFromFile("testdata/results.json")
	require.NoError(t, err)

	tests := []struct {
		name      string
		formatter Formatter
		want      string
	}{
		{
			name:      "JSON to HTML",
			formatter: NewHTMLFormatter(),
			want:      "testdata/results.html",
		},
		{
			name:      "JSON to CSV",
			formatter: NewCSVFormatter(),
			want:      "testdata/results.csv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFixedMetadata(t, func() {
				assertFormatterOutputFromFile(t, tt.formatter, results, tt.want)
			})
		})
	}
}

// withFixedMetadata sets fixed timestamp and version metadata to produce consistent results,
// then runs the given function.
func withFixedMetadata(t *testing.T, fn func()) {
	t.Helper()
	testutils.SyncCall(&timestampLock, func() {
		originalTimestamp := timestamp
		timestamp = func(_ time.Time) string {
			return "1985-03-04T22:10:00"
		}
		defer func() { timestamp = originalTimestamp }()

		testutils.SyncCall(&currentVersionDataLock, func() {
			originalCurrentVersionData := currentVersionData
			currentVersionData = VersionData{
				Name:    "MindTrial",
				Version: "(testing)",
				Source:  "github.com/petmal/mindtrial",
			}
			defer func() { currentVersionData = originalCurrentVersionData }()
			fn()
		})
	})
}
