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

// NewJSONCodec creates a new codec that reads and writes results in JSON format.
func NewJSONCodec() Codec {
	return &jsonCodec{}
}

type jsonCodec struct{}

type jsonDocument struct {
	FormatVersion int         `json:"FormatVersion"`
	AppName       string      `json:"AppName,omitempty"`
	AppVersion    string      `json:"AppVersion,omitempty"`
	CreatedAt     string      `json:"CreatedAt,omitempty"`
	Results       resultsView `json:"Results"`
}

func (c jsonCodec) FileExt() string {
	return "json"
}

func (c jsonCodec) Write(results runners.Results, out io.Writer) error {
	doc := jsonDocument{
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
