// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package runners

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/rs/zerolog"
)

// EmittingLogger implements the logging.Logger interface and additionally
// emits log messages as events through the provided event emitter.
// This allows log messages to be broadcasted to UI components or other consumers.
type EmittingLogger struct {
	logger  zerolog.Logger
	emitter eventEmitter
	prefix  string
}

// NewEmittingLogger creates a new EmittingLogger that wraps the provided zerolog.Logger
// and emits log messages through the provided event emitter.
func NewEmittingLogger(logger zerolog.Logger, emitter eventEmitter) logging.Logger {
	return &EmittingLogger{
		logger:  logger,
		emitter: emitter,
	}
}

// Message logs a message at the specified level with optional format arguments.
// The message is logged by the logger and emitted as an event.
func (l *EmittingLogger) Message(ctx context.Context, level slog.Level, msg string, args ...any) {
	formattedMsg := fmt.Sprintf(msg, args...)
	formattedMsg = l.prefix + formattedMsg
	l.getEvent(level).Msg(formattedMsg)
	l.emitter.emitMessageEvent(formattedMsg)
}

// Error logs an error at the specified level with optional format arguments.
// The error and message are logged by the logger and emitted as an event.
func (l *EmittingLogger) Error(ctx context.Context, level slog.Level, err error, msg string, args ...any) {
	formattedMsg := fmt.Sprintf(msg, args...)
	formattedMsg = l.prefix + formattedMsg
	l.getEvent(level).Err(err).Msg(formattedMsg)
	l.emitter.emitMessageEvent(formattedMsg)
}

// WithContext returns a new Logger that appends the specified context to the existing prefix.
func (l *EmittingLogger) WithContext(context string) logging.Logger {
	newPrefix := l.prefix + context
	return &EmittingLogger{
		logger:  l.logger,
		emitter: l.emitter,
		prefix:  newPrefix,
	}
}

// getEvent returns a zerolog event for the given slog level.
func (l *EmittingLogger) getEvent(level slog.Level) *zerolog.Event {
	switch {
	case level < logging.LevelDebug:
		return l.logger.Trace()
	case level < logging.LevelInfo:
		return l.logger.Debug()
	case level < logging.LevelWarn:
		return l.logger.Info()
	case level < logging.LevelError:
		return l.logger.Warn()
	default:
		return l.logger.Error()
	}
}
