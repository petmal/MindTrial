// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package formatters provides output formatting functionality for MindTrial results.
// It supports multiple output formats including HTML, CSV, JSON, and text logs.
// The JSON format implements the Codec interface, enabling bidirectional serialization
// for result persistence and merging across separate runs.
package formatters

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
)

var (
	// ErrPrintResults indicates that result formatting failed.
	ErrPrintResults = errors.New("failed to print formatted results")
	// ErrReadResults indicates that result reading failed.
	ErrReadResults = errors.New("failed to read results")
	// ErrUnsupportedInputFormat indicates an unsupported file format.
	ErrUnsupportedInputFormat = errors.New("unsupported input format")
)

// Formatter handles converting results into specific output formats.
type Formatter interface {
	// FileExt returns the formatter's file extension.
	FileExt() string
	// Write outputs formatted results to the writer.
	Write(results runners.Results, out io.Writer) error
}

// Codec extends the Formatter interface with the ability to read results back.
type Codec interface {
	Formatter
	// Read parses results from the reader.
	Read(in io.Reader) (runners.Results, error)
}

// codecs is the registry of all available codecs.
var codecs = []Codec{NewJSONCodec()}

// ReadResultsFromFile reads results from a file, selecting the appropriate codec based on file extension.
func ReadResultsFromFile(path string) (runners.Results, error) {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	for _, codec := range codecs {
		if strings.EqualFold(codec.FileExt(), ext) {
			f, err := os.Open(filepath.Clean(path))
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrReadResults, err)
			}
			defer f.Close()
			return codec.Read(f)
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnsupportedInputFormat, ext)
}

// formatAnswerText returns a single plain text block containing all possible formatted answers for
// CSV and other text-based outputs. If there is only one answer, it returns the answer directly.
// If there are multiple answers, it returns them as a bracket-formatted list.
func formatAnswerText(result runners.RunResult) string {
	answers := FormatAnswer(result, false)
	if len(answers) == 1 {
		return answers[0]
	}

	// Indent all lines in each answer.
	indentedAnswers := make([]string, len(answers))
	for i, answer := range answers {
		lines := utils.SplitLines(answer)
		for j, line := range lines {
			lines[j] = "    " + line
		}
		indentedAnswers[i] = strings.Join(lines, "\n")
	}

	return "[\n" + strings.Join(indentedAnswers, "\n  ,\n") + "\n]"
}
