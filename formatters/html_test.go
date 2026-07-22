// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"bytes"
	"encoding/json"
	"html"
	"regexp"
	"sync"
	"testing"

	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var timestampLock sync.Mutex
var currentVersionDataLock sync.Mutex

func TestUpdateGoldenHTML(t *testing.T) {
	updateGoldenFiles(t, NewHTMLFormatter(), []goldenFileTestCase{
		{"testdata/empty.html", runners.Results{}},
		{"testdata/results.html", mockResults},
	})
}

func TestHTMLFormatterWrite(t *testing.T) {
	tests := []struct {
		name    string
		results runners.Results
		want    string
	}{
		{
			name:    "format no results",
			results: runners.Results{},
			want:    "testdata/empty.html",
		},
		{
			name:    "format some results",
			results: mockResults,
			want:    "testdata/results.html",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFixedMetadata(t, func() {
				formatter := NewHTMLFormatter()
				assertFormatterOutputFromFile(t, formatter, tt.results, tt.want)
			})
		})
	}
}

func TestHTMLFormatterFileExt(t *testing.T) {
	formatter := NewHTMLFormatter()
	assert.Equal(t, "html", formatter.FileExt())
}

// TestHTMLFormatterTagsSurviveDelimiterCharacters guards against a regression of a
// delimiter-collision bug: tags used to be embedded as a comma-joined string and split
// back on ',' by the page's JavaScript, which corrupted any tag that itself contained a
// comma (or otherwise broke matching). Tags are now JSON-encoded, so they must round-trip
// exactly regardless of their content, including tags containing commas or quotes.
func TestHTMLFormatterTagsSurviveDelimiterCharacters(t *testing.T) {
	wantTags := []string{"needs,fix", `quoted"tag`, "plain"}
	results := runners.Results{
		"provider-name": []runners.RunResult{
			{
				Provider: "provider-name",
				Task:     "task-name",
				Run:      "run-name",
				Kind:     runners.Success,
				TaskMetadata: runners.TaskMetadata{
					Tags: wantTags,
				},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, NewHTMLFormatter().Write(results, &buf))

	matches := regexp.MustCompile(`data-tags="([^"]*)"`).FindStringSubmatch(buf.String())
	require.Len(t, matches, 2, "expected exactly one data-tags attribute in output")

	var gotTags []string
	require.NoError(t, json.Unmarshal([]byte(html.UnescapeString(matches[1])), &gotTags))
	assert.Equal(t, wantTags, gotTags)
}
