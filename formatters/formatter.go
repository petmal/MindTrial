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
	"strings"

	"github.com/petmal/mindtrial/runners"
)

const textAnswerSeparator = "---\n"

// ErrPrintResults indicates that result formatting failed.
var ErrPrintResults = errors.New("failed to print formatted results")

// Formatter handles converting results into specific output formats.
type Formatter interface {
	// FileExt returns the formatter's file extension.
	FileExt() string
	// Write outputs formatted results to the writer.
	Write(results runners.Results, out io.Writer) error
}

// formatAnswerText returns a single plain text block containing all possible formatted answers separated by a separator
// for CSV and other text-based outputs.
func formatAnswerText(result runners.RunResult) string {
	return strings.TrimSpace(strings.Join(FormatAnswer(result, false), textAnswerSeparator))
}
