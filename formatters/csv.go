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
	"strconv"
	"strings"

	"github.com/petmal/mindtrial/pkg/utils"
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

	headers := []string{"TraceID", "Provider", "Run", "Task", "Status", "DurationMS", "Answer", "Details", "Suite", "Category", "Difficulty", "Tags"}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}

	return ForEachOrdered(results, func(_ string, runResults []runners.RunResult) error {
		for _, result := range runResults {
			row := []string{result.TraceID, result.Provider, result.Run, result.Task, ToStatus(result.Kind), strconv.FormatInt(RoundToMS(result.Duration).Milliseconds(), 10), formatAnswerText(result), utils.ToString(newDetailsView(result.Details)), result.TaskMetadata.Suite, result.TaskMetadata.Category, result.TaskMetadata.Difficulty, strings.Join(result.TaskMetadata.Tags, ",")}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("%w: %v", ErrPrintResults, err)
			}
		}
		return nil
	})
}
