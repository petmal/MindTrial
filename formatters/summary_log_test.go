// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"testing"

	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
)

func TestSummaryLogFormatterWrite(t *testing.T) {
	tests := []struct {
		name    string
		results runners.Results
		want    string
	}{
		{
			name:    "format no results",
			results: runners.Results{},
			want:    "testdata/empty.summary.log",
		},
		{
			name:    "format some results",
			results: mockResults,
			want:    "testdata/results.summary.log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewSummaryLogFormatter()
			assertFormatterOutputFromFile(t, formatter, tt.results, tt.want)
		})
	}
}

func TestSummaryLogFormatterFileExt(t *testing.T) {
	formatter := NewSummaryLogFormatter()
	assert.Equal(t, "summary.log", formatter.FileExt())
}
