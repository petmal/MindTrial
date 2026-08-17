// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/formatters"
	"github.com/petmal/mindtrial/pkg/testutils"
)

var updateGolden = flag.Bool("update-golden", false, "update golden test files")

const (
	realWorldResultsInput  = "testdata/real-world-results.json"
	realWorldResultsGolden = "testdata/real-world-results.csv"
)

var realWorldGroupBy = []Dimension{DimensionProvider, DimensionRun}

func TestUpdateGoldenRealWorldResultsStats(t *testing.T) {
	if !*updateGolden {
		t.Skip("use -update-golden to regenerate golden files")
	}

	f, err := os.Create(realWorldResultsGolden)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, Write(OutputFormatCSV, realWorldGroupBy, computeRealWorldStats(t), f))
	t.Logf("Updated %s", realWorldResultsGolden)
}

// TestComputeStatsRealWorldResultsGolden computes stats (grouped by provider,run, the CLI
// default) over a real MindTrial JSON result artifact predating reasoning-token/pricing
// support and even the Model/RunConfig/ResponseParsing fields (AppVersion v0.21.0), and
// asserts the output matches a golden CSV byte-for-byte. Every value in the golden file was
// independently derived from the raw JSON by hand (pass/fail/error/skipped counts and
// rates; input-token sums honoring each result's cache-token accounting, including the
// cache_tokens_included cases where a nonzero cache-read counter must NOT be added on top
// of InputTokens; output-token sums; candidate tool-call sums/median/stddev, including
// tool calls recorded under Details.Error - not just Details.Answer - for the mistralai
// run, whose every task ended in a parsing error after invoking tools; and duration sums/
// medians) before being committed as golden, so this test also guards against silent
// regressions in real-world token/duration/tool-call aggregation, not just synthetic
// fixtures.
func TestComputeStatsRealWorldResultsGolden(t *testing.T) {
	got := testutils.CreateOpenNewTestFile(t, "*.csv")
	defer got.Close()
	require.NoError(t, Write(OutputFormatCSV, realWorldGroupBy, computeRealWorldStats(t), got))

	testutils.AssertFileContentsSameAs(t, realWorldResultsGolden, got.Name())
}

func computeRealWorldStats(t *testing.T) []Record {
	t.Helper()
	results, err := formatters.ReadResultsFromFile(realWorldResultsInput)
	require.NoError(t, err)

	records, err := ComputeStats(results, realWorldGroupBy, Filters{})
	require.NoError(t, err)
	return records
}
