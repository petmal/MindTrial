// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/petmal/mindtrial/runners"
)

// NewLogFormatter creates a new formatter that outputs detailed results as an ASCII table.
func NewLogFormatter() Formatter {
	return &logFormatter{}
}

type logFormatter struct{}

func (f logFormatter) FileExt() string {
	return "log"
}

func (f logFormatter) Write(results runners.Results, out io.Writer) error {
	tab := tabwriter.NewWriter(out, 0, 0, 1, ' ', tabwriter.Debug)
	defer tab.Flush()
	if _, err := fmt.Fprintln(tab, "TraceID\tProvider\tRun\tTask\tStatus\tDuration\tAnswer\t"); err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}

	return ForEachOrdered(results, func(_ string, runResults []runners.RunResult) error {
		for _, result := range runResults {
			if _, err := fmt.Fprintf(tab, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n", result.TraceID, result.Provider, result.Run, result.Task, ToStatus(result.Kind), RoundToMS(result.Duration), formatAnswerText(result)); err != nil {
				return fmt.Errorf("%w: %v", ErrPrintResults, err)
			}
		}
		return nil
	})
}
