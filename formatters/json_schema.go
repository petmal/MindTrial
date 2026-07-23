// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"github.com/invopop/jsonschema"
)

// resultsJSONSchema generates the JSON schema describing the structure of MindTrial's
// JSON results document (see jsonDocument and the view types in json_view.go). It is
// intended for external tooling/LLM consumers of the JSON results output, not for the
// codec's own read/write path, which relies on Go's encoding/json instead. The generated
// schema is checked into schema/results-v1.schema.json; regenerate it with
// `go test -tags=test ./formatters/... -run TestUpdateGoldenResultsSchema -update-golden`
// after a deliberate change to jsonDocument or its view types.
func resultsJSONSchema() *jsonschema.Schema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect(jsonDocument{})
	schema.ID = resultsSchemaURL
	schema.Title = "MindTrial Results"
	schema.Description = "The structure of a MindTrial JSON results output document."
	return schema
}
