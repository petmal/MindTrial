// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package testutils

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/rs/zerolog"
)

// TestLogger is a logger implementation for testing that wraps zerolog and integrates
// with the Go testing framework. It outputs log messages through the test writer
// so they appear properly in test output and are captured when tests fail.
type TestLogger struct {
	logger zerolog.Logger
	prefix string
}

// NewTestLogger creates a new TestLogger that outputs to the test framework.
// Log messages will be properly associated with the test and displayed in test output.
func NewTestLogger(t *testing.T) *TestLogger {
	return &TestLogger{
		logger: zerolog.New(zerolog.NewTestWriter(t)),
	}
}

// getEvent maps slog levels to zerolog events.
func (tl *TestLogger) getEvent(level slog.Level) *zerolog.Event {
	switch {
	case level < slog.LevelDebug:
		return tl.logger.Trace()
	case level < slog.LevelInfo:
		return tl.logger.Debug()
	case level < slog.LevelWarn:
		return tl.logger.Info()
	case level < slog.LevelError:
		return tl.logger.Warn()
	default:
		return tl.logger.Error()
	}
}

// Message logs a message at the specified level with optional formatting arguments.
func (tl *TestLogger) Message(ctx context.Context, level slog.Level, msg string, args ...any) {
	formattedMsg := fmt.Sprintf(msg, args...)
	formattedMsg = tl.prefix + formattedMsg
	tl.getEvent(level).Msg(formattedMsg)
}

// Error logs an error message at the specified level with optional formatting arguments.
func (tl *TestLogger) Error(ctx context.Context, level slog.Level, err error, msg string, args ...any) {
	formattedMsg := fmt.Sprintf(msg, args...)
	formattedMsg = tl.prefix + formattedMsg
	tl.getEvent(level).Err(err).Msg(formattedMsg)
}

// WithContext returns a new logger with additional context.
// The context string will be prepended to all log messages from the returned logger.
func (tl *TestLogger) WithContext(context string) logging.Logger {
	newPrefix := tl.prefix + context
	return &TestLogger{
		logger: tl.logger,
		prefix: newPrefix,
	}
}
