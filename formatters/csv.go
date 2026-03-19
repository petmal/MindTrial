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

	"github.com/CircleCI-Research/evalbench/pkg/utils"
	"github.com/CircleCI-Research/evalbench/pricing"
	"github.com/CircleCI-Research/evalbench/runners"
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

	headers := []string{"TraceID", "Provider", "Run", "Model", "Task", "Status", "Duration", "InputTokens", "OutputTokens", "EstCostUSD", "Answer", "Details"}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}

	return ForEachOrdered(results, func(_ string, runResults []runners.RunResult) error {
		for _, result := range runResults {
			inputTokens, outputTokens := taskTokens(result)
			cost := pricing.EstimateCost(result.Provider, result.Model, inputTokens, outputTokens)
			row := []string{
				result.TraceID, result.Provider, result.Run, result.Model, result.Task,
				ToStatus(result.Kind), RoundToMS(result.Duration).String(),
				fmt.Sprintf("%d", inputTokens), fmt.Sprintf("%d", outputTokens),
				fmt.Sprintf("%.6f", cost),
				formatAnswerText(result), utils.ToString(result.Details),
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("%w: %v", ErrPrintResults, err)
			}
		}
		return nil
	})
}

func taskTokens(result runners.RunResult) (input, output int64) {
	for _, usage := range []runners.TokenUsage{
		result.Details.Answer.Usage,
		result.Details.Validation.Usage,
		result.Details.Error.Usage,
	} {
		if usage.InputTokens != nil {
			input += *usage.InputTokens
		}
		if usage.OutputTokens != nil {
			output += *usage.OutputTokens
		}
	}
	return
}
