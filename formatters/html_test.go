// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"sync"
	"testing"

	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
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
