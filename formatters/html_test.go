// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"sync"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
)

var timestampLock sync.Mutex
var currentVersionDataLock sync.Mutex

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
			testutils.SyncCall(&timestampLock, func() {
				// Set fixed timestamp to produce consistent results.
				originalTimestamp := timestamp
				timestamp = func(_ time.Time) string {
					return "1985-03-04T22:10:00"
				}
				defer func() { timestamp = originalTimestamp }()

				testutils.SyncCall(&currentVersionDataLock, func() {
					// Set fixed version metadata to produce consistent results.
					originalCurrentVersionData := currentVersionData
					currentVersionData = VersionData{
						Name:    "MindTrial",
						Version: "(testing)",
						Source:  "github.com/petmal/mindtrial",
					}
					defer func() { currentVersionData = originalCurrentVersionData }()
					formatter := NewHTMLFormatter()
					assertFormatterOutputFromFile(t, formatter, tt.results, tt.want)
				})
			})
		})
	}
}

func TestHTMLFormatterFileExt(t *testing.T) {
	formatter := NewHTMLFormatter()
	assert.Equal(t, "html", formatter.FileExt())
}
