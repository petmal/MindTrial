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

// NewSummaryLogFormatter creates a new formatter that outputs results as an ASCII table summary.
func NewSummaryLogFormatter() Formatter {
	return &summaryLogFormatter{}
}

type summaryLogFormatter struct{}

func (f summaryLogFormatter) FileExt() string {
	return "summary.log"
}

func (f summaryLogFormatter) Write(results runners.Results, out io.Writer) error {
	tab := tabwriter.NewWriter(out, 0, 0, 1, ' ', tabwriter.Debug)
	defer tab.Flush()
	if _, err := fmt.Fprintf(tab, "Provider\tRun\t%s\t%s\t%s\t%s\tPass Rate (%%)\tAccuracy (%%)\tError Rate (%%)\tTotal Duration\t\n", Passed, Failed, Error, Skipped); err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}
	return ForEachOrdered(results, func(provider string, _ []runners.RunResult) error {
		resultsByRunAndKind := results.ProviderResultsByRunAndKind(provider)
		return ForEachOrdered(resultsByRunAndKind, func(run string, resultsByKind map[runners.ResultKind][]runners.RunResult) error {
			if _, err := fmt.Fprintf(tab, "%s\t%s\t%d\t%d\t%d\t%d\t%.2f\t%.2f\t%.2f\t%s\t\n",
				provider, run,
				CountByKind(resultsByKind, runners.Success),
				CountByKind(resultsByKind, runners.Failure),
				CountByKind(resultsByKind, runners.Error),
				CountByKind(resultsByKind, runners.NotSupported),
				Percent(PassRate(resultsByKind)),
				Percent(AccuracyRate(resultsByKind)),
				Percent(ErrorRate(resultsByKind)),
				RoundToMS(TotalDuration(resultsByKind, runners.Success, runners.Failure, runners.Error, runners.NotSupported))); err != nil {
				return fmt.Errorf("%w: %v", ErrPrintResults, err)
			}
			return nil
		})
	})
}
