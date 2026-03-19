// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/CircleCI-Research/evalbench/runners"
)

// NewJSONLFormatter creates a new formatter that outputs one JSON object per line
// for each provider/run combination. Designed for append-only historical tracking
// so results can be compared across runs over time.
func NewJSONLFormatter() Formatter {
	return &jsonlFormatter{}
}

type jsonlFormatter struct{}

func (f jsonlFormatter) FileExt() string {
	return "jsonl"
}

// runSummary is the JSON structure written for each provider+run combination.
type runSummary struct {
	Timestamp     string  `json:"timestamp"`
	Provider      string  `json:"provider"`
	Run           string  `json:"run"`
	Model         string  `json:"model"`
	Passed        int     `json:"passed"`
	Failed        int     `json:"failed"`
	Error         int     `json:"error"`
	Skipped       int     `json:"skipped"`
	PassRate      float64 `json:"pass_rate_pct"`
	Accuracy      float64 `json:"accuracy_pct"`
	ErrorRate     float64 `json:"error_rate_pct"`
	DurationMs    int64   `json:"total_duration_ms"`
	InputTokens   int64   `json:"total_input_tokens"`
	OutputTokens  int64   `json:"total_output_tokens"`
	EstCostUSD    float64 `json:"estimated_cost_usd"`
}

func (f jsonlFormatter) Write(results runners.Results, out io.Writer) error {
	now := time.Now().UTC().Format(time.RFC3339)
	allKinds := []runners.ResultKind{runners.Success, runners.Failure, runners.Error, runners.NotSupported}

	return ForEachOrdered(results, func(provider string, _ []runners.RunResult) error {
		resultsByRunAndKind := results.ProviderResultsByRunAndKind(provider)
		return ForEachOrdered(resultsByRunAndKind, func(run string, resultsByKind map[runners.ResultKind][]runners.RunResult) error {
			var model string
			for _, kind := range allKinds {
				for _, r := range resultsByKind[kind] {
					model = r.Model
					break
				}
				if model != "" {
					break
				}
			}

			summary := runSummary{
				Timestamp:    now,
				Provider:     provider,
				Run:          run,
				Model:        model,
				Passed:       CountByKind(resultsByKind, runners.Success),
				Failed:       CountByKind(resultsByKind, runners.Failure),
				Error:        CountByKind(resultsByKind, runners.Error),
				Skipped:      CountByKind(resultsByKind, runners.NotSupported),
				PassRate:     Percent(PassRate(resultsByKind)),
				Accuracy:     Percent(AccuracyRate(resultsByKind)),
				ErrorRate:    Percent(ErrorRate(resultsByKind)),
				DurationMs:   TotalDuration(resultsByKind, allKinds...).Milliseconds(),
				InputTokens:  TotalInputTokens(resultsByKind, allKinds...),
				OutputTokens: TotalOutputTokens(resultsByKind, allKinds...),
				EstCostUSD:   EstimatedCost(resultsByKind, allKinds...),
			}

			line, err := json.Marshal(summary)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPrintResults, err)
			}
			if _, err := fmt.Fprintf(out, "%s\n", line); err != nil {
				return fmt.Errorf("%w: %v", ErrPrintResults, err)
			}
			return nil
		})
	})
}
