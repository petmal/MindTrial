// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// resultsSchemaPath is the checked-in JSON schema artifact for the JSON results
// document. Run TestUpdateGoldenResultsSchema with -update-golden to regenerate it.
const resultsSchemaPath = "../schema/results-v1.schema.json"

func marshalResultsJSONSchema(t *testing.T) []byte {
	t.Helper()
	data, err := json.MarshalIndent(resultsJSONSchema(), "", "  ")
	require.NoError(t, err)
	return append(data, '\n')
}

func TestUpdateGoldenResultsSchema(t *testing.T) {
	if !*updateGolden {
		t.Skip("use -update-golden to regenerate golden files")
	}
	require.NoError(t, os.WriteFile(resultsSchemaPath, marshalResultsJSONSchema(t), 0644)) //nolint:gosec // schema file is meant to be publicly readable
	t.Logf("Updated %s", resultsSchemaPath)
}

// TestResultsJSONSchemaUpToDate ensures the checked-in schema/results-v1.schema.json
// artifact stays in sync with the jsonschema tags declared on jsonDocument and its view
// types; run with -update-golden to regenerate it after a deliberate change.
func TestResultsJSONSchemaUpToDate(t *testing.T) {
	got := testutils.CreateOpenNewTestFile(t, "*.schema.json")
	_, err := got.Write(marshalResultsJSONSchema(t))
	require.NoError(t, err)
	require.NoError(t, got.Close())

	testutils.AssertFileContentsSameAs(t, resultsSchemaPath, got.Name())
}
