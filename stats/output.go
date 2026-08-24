// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// OutputFormat selects how Write renders computed stats records.
type OutputFormat string

const (
	// OutputFormatText renders a human-readable, column-aligned table.
	OutputFormatText OutputFormat = "text"
	// OutputFormatCSV renders comma-separated values with a header row.
	OutputFormatCSV OutputFormat = "csv"
	// OutputFormatJSON renders a single indented JSON array of records.
	OutputFormatJSON OutputFormat = "json"
	// OutputFormatJSONL renders one JSON object per line, one line per record.
	OutputFormatJSONL OutputFormat = "jsonl"
)

// ErrInvalidOutputFormat is returned for an unrecognized stats output format.
var ErrInvalidOutputFormat = errors.New("invalid stats output format")

// ParseOutputFormat validates and normalizes a --stats-format value. A blank value defaults
// to OutputFormatText.
func ParseOutputFormat(value string) (OutputFormat, error) {
	switch normalized := OutputFormat(strings.ToLower(strings.TrimSpace(value))); normalized {
	case "":
		return OutputFormatText, nil
	case OutputFormatText, OutputFormatCSV, OutputFormatJSON, OutputFormatJSONL:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidOutputFormat, value)
	}
}

// metricColumns lists the fixed (non-dimension) columns written by the text/CSV formats,
// in output order.
var metricColumns = []string{
	"Count", "Passed", "Failed", "Error", "Skipped",
	"PassRate", "Accuracy", "ErrorRate",
	"TotalDuration", "MedianDuration", "StddevDuration",
	"TotalInputTokens", "MedianInputTokens", "StddevInputTokens",
	"TotalOutputTokens", "MedianOutputTokens", "StddevOutputTokens",
	"TotalReasoningTokens", "MedianReasoningTokens", "StddevReasoningTokens",
	"TotalCacheReadTokens", "MedianCacheReadTokens", "StddevCacheReadTokens",
	"TotalCacheWriteTokens", "MedianCacheWriteTokens", "StddevCacheWriteTokens",
	"TotalToolCalls", "MedianToolCalls", "StddevToolCalls",
	"TransientErrors", "ResponseParsingErrors",
	"EstimatedCandidateCost", "CandidateCostCurrency",
}

// Write renders records in the given format to out. groupBy determines the dimension column
// order for the text/CSV formats; it is ignored for json/jsonl, which serialize
// Record.Dimensions as a map instead.
func Write(format OutputFormat, groupBy []Dimension, records []Record, out io.Writer) error {
	switch format {
	case OutputFormatCSV:
		return writeCSV(groupBy, records, out)
	case OutputFormatJSON:
		return writeJSON(records, out, false)
	case OutputFormatJSONL:
		return writeJSON(records, out, true)
	default:
		return writeText(groupBy, records, out)
	}
}

func header(groupBy []Dimension) []string {
	columns := make([]string, 0, len(groupBy)+len(metricColumns))
	for _, dim := range groupBy {
		columns = append(columns, string(dim))
	}
	return append(columns, metricColumns...)
}

func row(groupBy []Dimension, r Record) []string {
	values := make([]string, 0, len(groupBy)+len(metricColumns))
	for _, dim := range groupBy {
		values = append(values, r.Dimensions[string(dim)])
	}
	return append(values, metricValues(r)...)
}

func metricValues(r Record) []string {
	return []string{
		strconv.Itoa(r.Count), strconv.Itoa(r.Passed), strconv.Itoa(r.Failed), strconv.Itoa(r.Error), strconv.Itoa(r.Skipped),
		formatFloat(r.PassRate), formatFloat(r.Accuracy), formatFloat(r.ErrorRate),
		formatDurationPtr(r.TotalDuration), formatDurationPtr(r.MedianDuration), formatDurationPtr(r.StddevDuration),
		formatInt64Ptr(r.TotalInputTokens), formatFloatPtr(r.MedianInputTokens), formatFloatPtr(r.StddevInputTokens),
		formatInt64Ptr(r.TotalOutputTokens), formatFloatPtr(r.MedianOutputTokens), formatFloatPtr(r.StddevOutputTokens),
		formatInt64Ptr(r.TotalReasoningTokens), formatFloatPtr(r.MedianReasoningTokens), formatFloatPtr(r.StddevReasoningTokens),
		formatInt64Ptr(r.TotalCacheReadTokens), formatFloatPtr(r.MedianCacheReadTokens), formatFloatPtr(r.StddevCacheReadTokens),
		formatInt64Ptr(r.TotalCacheWriteTokens), formatFloatPtr(r.MedianCacheWriteTokens), formatFloatPtr(r.StddevCacheWriteTokens),
		formatInt64Ptr(r.TotalToolCalls), formatFloatPtr(r.MedianToolCalls), formatFloatPtr(r.StddevToolCalls),
		strconv.Itoa(r.TransientErrors), strconv.Itoa(r.ResponseParsingErrors),
		formatCostPtr(r.EstimatedCandidateCost), r.CandidateCostCurrency,
	}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func formatFloatPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return formatFloat(*v)
}

// formatCostPtr renders an estimated cost with enough precision to stay meaningful for
// per-million-token rates, which routinely produce fractions of a cent.
func formatCostPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 6, 64)
}

func formatInt64Ptr(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func formatDurationPtr(v *time.Duration) string {
	if v == nil {
		return ""
	}
	return v.String()
}

func writeCSV(groupBy []Dimension, records []Record, out io.Writer) error {
	w := csv.NewWriter(out)
	if err := w.Write(header(groupBy)); err != nil {
		return err
	}
	for _, r := range records {
		if err := w.Write(row(groupBy, r)); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeText(groupBy []Dimension, records []Record, out io.Writer) error {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(header(groupBy), "\t")); err != nil {
		return err
	}
	for _, r := range records {
		if _, err := fmt.Fprintln(tw, strings.Join(row(groupBy, r), "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeJSON(records []Record, out io.Writer, lines bool) error {
	if lines {
		enc := json.NewEncoder(out)
		for _, r := range records {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return nil
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}
