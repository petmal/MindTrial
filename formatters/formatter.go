// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package formatters provides output formatting functionality for MindTrial results.
// It supports multiple output formats including HTML, CSV, and text logs.
package formatters

import (
	"errors"
	"io"

	"github.com/petmal/mindtrial/runners"
)

// ErrPrintResults indicates that result formatting failed.
var ErrPrintResults = errors.New("failed to print formatted results")

// Formatter handles converting results into specific output formats.
type Formatter interface {
	// FileExt returns the formatter's file extension.
	FileExt() string
	// Write outputs formatted results to the writer.
	Write(results runners.Results, out io.Writer) error
}
