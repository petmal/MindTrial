// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package logging provides a structured logging interface compatible with slog
// levels and common logging utilities for the MindTrial application.
package logging

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
)

// Common logging levels for structured logging.
const (
	LevelTrace = slog.Level(-8) // most verbose
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError // least verbose
)

// UnknownLogValue is the placeholder text used when logging nil or unknown values.
const UnknownLogValue = "<unknown>"

// Logger defines a generic logging interface following slog style with log levels.
// It provides structured logging capabilities for both regular messages and error handling.
type Logger interface {
	// Message logs a message at the specified level with optional format arguments.
	Message(ctx context.Context, level slog.Level, msg string, args ...any)

	// Error logs an error at the specified level with optional format arguments.
	Error(ctx context.Context, level slog.Level, err error, msg string, args ...any)

	// WithContext returns a new Logger that appends the specified context to the existing prefix.
	// This allows for hierarchical logging where components can add their context
	// without affecting the original logger instance. Each call extends the prefix chain.
	WithContext(context string) Logger
}

// FormatLogInt64 formats an int64 pointer value for logging.
// If the pointer is nil, it returns a placeholder value.
func FormatLogInt64(value *int64) string {
	if value != nil {
		return strconv.FormatInt(*value, 10)
	}
	return UnknownLogValue
}

// FormatLogText formats a slice of strings for logging with
// tab indentation and double-newline separation.
// If the slice is empty, it returns a tab-indented placeholder value.
func FormatLogText(lines []string) string {
	if len(lines) > 0 {
		return "\t" + strings.Join(lines, "\n\n\t")
	}
	return "\t" + UnknownLogValue
}
