// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/petmal/mindtrial/runners"
)

// NewCSVFormatter creates a new formatter that outputs results in CSV format.
func NewCSVFormatter() Formatter {
	return &csvFormatter{}
}

type csvFormatter struct{}

func (f csvFormatter) FileExt() string {
	return "csv"
}

func (f csvFormatter) Write(results runners.Results, out io.Writer) error {
	writer := csv.NewWriter(out)
	defer writer.Flush()

	headers := []string{"Provider", "Run", "Task", "Status", "Duration", "Answer", "Details"}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}

	return ForEachOrdered(results, func(_ string, runResults []runners.RunResult) error {
		for _, result := range runResults {
			row := []string{result.Provider, result.Run, result.Task, ToStatus(result.Kind), RoundToMS(result.Duration).String(), formatAnswerText(result), result.Details}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("%w: %v", ErrPrintResults, err)
			}
		}
		return nil
	})
}
