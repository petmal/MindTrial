// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/petmal/mindtrial/runners"
)

const currentFormatVersion = 1

// resultsSchemaURL is the public URL of the JSON schema describing this format version's
// document structure (see resultsJSONSchema in json_schema.go and the checked-in
// schema/results-v1.schema.json). Every generated document includes it via the "$schema"
// field so tooling/LLM consumers can locate the schema without guessing.
const resultsSchemaURL = "https://raw.githubusercontent.com/petmal/mindtrial/main/schema/results-v1.schema.json"

// NewJSONCodec creates a new codec that reads and writes results in JSON format.
func NewJSONCodec() Codec {
	return &jsonCodec{}
}

type jsonCodec struct{}

type jsonDocument struct {
	Schema        string      `json:"$schema,omitempty" jsonschema:"title=Schema" jsonschema_description:"The URL of the JSON schema describing this document's structure."`
	FormatVersion int         `json:"FormatVersion" jsonschema:"title=Format Version,enum=1" jsonschema_description:"The version of this JSON document's structure. Readers should reject documents with an unrecognized version rather than guessing at compatibility."`
	AppName       string      `json:"AppName,omitempty" jsonschema:"title=Application Name" jsonschema_description:"The name of the application that produced this document."`
	AppVersion    string      `json:"AppVersion,omitempty" jsonschema:"title=Application Version" jsonschema_description:"The version of the application that produced this document."`
	CreatedAt     string      `json:"CreatedAt,omitempty" jsonschema:"title=Created At" jsonschema_description:"The timestamp at which this document was generated, formatted per RFC 1123 with numeric time zone (e.g. \"Mon, 02 Jan 2006 15:04:05 -0700\")."`
	Results       resultsView `json:"Results" jsonschema:"title=Results" jsonschema_description:"Task results, keyed by provider name."`
}

func (c jsonCodec) FileExt() string {
	return "json"
}

func (c jsonCodec) Write(results runners.Results, out io.Writer) error {
	doc := jsonDocument{
		Schema:        resultsSchemaURL,
		FormatVersion: currentFormatVersion,
		AppName:       currentVersionData.Name,
		AppVersion:    currentVersionData.Version,
		CreatedAt:     Timestamp(),
		Results:       toResultsView(results),
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}
	data = append(data, '\n')
	if _, err = out.Write(data); err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}
	return nil
}

func (c jsonCodec) Read(in io.Reader) (runners.Results, error) {
	dec := json.NewDecoder(in)
	dec.UseNumber()

	var doc jsonDocument
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadResults, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: unexpected trailing data after JSON document", ErrReadResults)
	}
	if doc.FormatVersion != currentFormatVersion {
		return nil, fmt.Errorf("%w: unsupported format version %d (expected %d)", ErrReadResults, doc.FormatVersion, currentFormatVersion)
	}
	results, err := fromResultsView(doc.Results)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadResults, err)
	}
	return results, nil
}
