// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// ParseTemplate parses text as a strict text/template that fails on missing map keys.
// Templates can use the shared helper functions:
//   - hash: a stable uint64 derived from all of its arguments;
//   - json: the JSON encoding of its argument.
func ParseTemplate(name string, text string) (*template.Template, error) {
	return template.New(name).Option("missingkey=error").Funcs(templateFuncs).Parse(text)
}

// ExpandTemplate parses text with ParseTemplate and executes it with data.
func ExpandTemplate(name string, text string, data any) (string, error) {
	tmpl, err := ParseTemplate(name, text)
	if err != nil {
		return "", err
	}

	var expanded strings.Builder
	if err := tmpl.Execute(&expanded, data); err != nil {
		return "", err
	}
	return expanded.String(), nil
}

var templateFuncs = template.FuncMap{
	"hash": stableHash,
	"json": encodeJSON,
}

// stableHash hashes the length-prefixed text form of every part with SHA-256, so the result
// does not depend on the Go runtime and part boundaries are unambiguous.
func stableHash(parts ...any) uint64 {
	digest := sha256.New()
	var length [8]byte
	for _, part := range parts {
		value := fmt.Append(nil, part)
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(value)
	}
	return binary.BigEndian.Uint64(digest.Sum(nil)[:8])
}

func encodeJSON(value any) (string, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}
