// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"fmt"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockResults = runners.Results{
	"provider-name": []runners.RunResult{
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-success",
			Kind:     runners.Success,
			Duration: 95 * time.Second,
			Want:     utils.NewStringSet("Quos aut rerum quaerat qui ad culpa."),
			Got:      "Quos aut rerum quaerat qui ad culpa.",
			Details:  "Quis ea voluptatem non.\r\nAperiam dolor est alias odit enim fugiat vitae aliquam dolor.",
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-failure",
			Kind:     runners.Failure,
			Duration: 10 * time.Second,
			Want:     utils.NewStringSet("Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."),
			Got:      "Ipsam ea et optio explicabo eius et.",
			Details:  "Ut eos eius modi nihil voluptatem error.\n\nVeniam omnis at possimus aliquid.\r\nUt voluptatem ullam et ea non beatae eos adipisci incidunt. Saepe atque occaecati. Tempore animi magni sequi modi omnis.\nConsequatur hic sint laboriosam maiores unde vero ipsum.\n",
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-success-multiple-answers",
			Kind:     runners.Success,
			Duration: 17 * time.Second,
			Want:     utils.NewStringSet("Deserunt quo sint minus eos officiis et.", "Quos aut rerum quaerat qui ad culpa."),
			Got:      "Quos aut rerum quaerat qui ad culpa.",
			Details:  "Quis ea voluptatem non.\r\nAperiam dolor est alias odit enim fugiat vitae aliquam dolor.",
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-failure-multiple-answers",
			Kind:     runners.Failure,
			Duration: 3*time.Minute + 800*time.Millisecond,
			Want:     utils.NewStringSet("Dolores saepe ad sed rerum autem iure minima et.", "Nihil reprehenderit enim voluptatum dolore nisi neque quia aut qui."),
			Got:      "Ipsam ea et optio explicabo eius et.",
			Details:  "Ut eos eius modi nihil voluptatem error.\n\nVeniam omnis at possimus aliquid.\r\nUt voluptatem ullam et ea non beatae eos adipisci incidunt. Saepe atque occaecati. Tempore animi magni sequi modi omnis.\nConsequatur hic sint laboriosam maiores unde vero ipsum.\n",
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-error",
			Kind:     runners.Error,
			Duration: 0 * time.Second,
			Want:     utils.NewStringSet("Cum et rem."),
			Got:      "error message",
			Details:  "Pariatur rem dolores corporis voluptas aut eum repellat pariatur.",
		},
		{
			Provider: "provider-name",
			Task:     "task-name",
			Run:      "run-not-supported",
			Kind:     runners.NotSupported,
			Duration: 500 * time.Millisecond,
			Want:     utils.NewStringSet("Animi aut eligendi repellendus debitis harum aut."),
			Got:      "Sequi molestiae iusto sit sit dolorum aut.",
			Details:  "Placeat itaque voluptatem. Impedit aut quia velit. Libero ducimus tenetur vel et quibusdam. Et fugiat culpa. Tenetur iste aut mollitia corrupti et suscipit quia. Voluptatem incidunt aut et aliquam unde autem deleniti ea ea.",
		},
	},
}

func TestCSVFormatterWrite(t *testing.T) {
	tests := []struct {
		name    string
		results runners.Results
		want    string
	}{
		{
			name:    "format no results",
			results: runners.Results{},
			want:    "testdata/empty.csv",
		},
		{
			name:    "format some results",
			results: mockResults,
			want:    "testdata/results.csv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewCSVFormatter()
			assertFormatterOutputFromFile(t, formatter, tt.results, tt.want)
		})
	}
}

func assertFormatterOutputFromFile(t *testing.T, formatter Formatter, results runners.Results, expectedContentsFilePath string) {
	outputFileNamePattern := fmt.Sprintf("*.%s", formatter.FileExt())
	got := testutils.CreateOpenNewTestFile(t, outputFileNamePattern)
	gotFilePath := got.Name()
	require.NoError(t, formatter.Write(results, got))
	require.NoError(t, got.Close())
	t.Logf("Generated formatted file: %s\n", gotFilePath)
	testutils.AssertFileContentsSameAs(t, expectedContentsFilePath, gotFilePath)
}

func TestCSVFormatterFileExt(t *testing.T) {
	formatter := NewCSVFormatter()
	assert.Equal(t, "csv", formatter.FileExt())
}
