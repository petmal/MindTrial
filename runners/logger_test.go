// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package runners

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockEmitter struct {
	mock.Mock
}

func (m *mockEmitter) emitProgressEvent() {
	m.Called()
}

func (m *mockEmitter) emitMessageEvent(message string) {
	m.Called(message)
}

func TestEmittingLogger_Message(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	emitter.On("emitMessageEvent", "test message").Once()

	emittingLogger.Message(context.Background(), logging.LevelInfo, "test message")

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_MessageWithArgs(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	emitter.On("emitMessageEvent", "test message with value: 42").Once()

	emittingLogger.Message(context.Background(), logging.LevelInfo, "test message with value: %d", 42)

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_Error(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	emitter.On("emitMessageEvent", "error occurred").Once()

	emittingLogger.Error(context.Background(), logging.LevelError, errors.ErrUnsupported, "error occurred")

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_ErrorWithNilError(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	emitter.On("emitMessageEvent", "no error").Once()

	emittingLogger.Error(context.Background(), logging.LevelWarn, nil, "no error")

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_ErrorWithArgs(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	emitter.On("emitMessageEvent", "error occurred with code: 500").Once()

	emittingLogger.Error(context.Background(), logging.LevelError, errors.ErrUnsupported, "error occurred with code: %d", 500)

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_WithContext(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	// Test WithContext returns a new logger with context appended.
	contextLogger := emittingLogger.WithContext("test-context: ")
	assert.NotSame(t, emittingLogger, contextLogger, "WithContext should return a new logger instance")
}

func TestEmittingLogger_WithContextMessage(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)
	contextLogger := emittingLogger.WithContext("test-context: ")

	emitter.On("emitMessageEvent", "test-context: test message").Once()

	contextLogger.Message(context.Background(), logging.LevelInfo, "test message")

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_WithContextError(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)
	contextLogger := emittingLogger.WithContext("error-context: ")

	emitter.On("emitMessageEvent", "error-context: error occurred").Once()

	contextLogger.Error(context.Background(), logging.LevelError, errors.ErrUnsupported, "error occurred")

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_ContextChaining(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	// Test chaining multiple contexts.
	contextLogger1 := emittingLogger.WithContext("level1: ")
	contextLogger2 := contextLogger1.WithContext("level2: ")

	emitter.On("emitMessageEvent", "level1: level2: test message").Once()

	contextLogger2.Message(context.Background(), logging.LevelInfo, "test message")

	emitter.AssertExpectations(t)
}

// testStructuredError is a test error that implements logging.StructuredError.
type testStructuredError struct {
	msg    string
	fields map[string]any
}

func (e *testStructuredError) Error() string             { return e.msg }
func (e *testStructuredError) LogFields() map[string]any { return e.fields }

func TestEmittingLogger_ErrorWithStructuredError(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	structuredErr := &testStructuredError{
		msg:    "structured failure",
		fields: map[string]any{"raw_message": "test body", "stop_reason": "end_turn"},
	}

	// The emitted message should be the same formatted message, unaffected by structured fields.
	emitter.On("emitMessageEvent", "structured error occurred").Once()

	emittingLogger.Error(context.Background(), logging.LevelError, structuredErr, "structured error occurred")

	emitter.AssertExpectations(t)
}

func TestEmittingLogger_ErrorWithWrappedStructuredError(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	emitter := &mockEmitter{}

	emittingLogger := NewEmittingLogger(logger, emitter)

	structuredErr := &testStructuredError{
		msg:    "inner failure",
		fields: map[string]any{"response_body": `{"error":"bad request"}`},
	}
	wrappedErr := fmt.Errorf("outer: %w", structuredErr)

	// The emitted message should be the same formatted message, unaffected by wrapping or structured fields.
	emitter.On("emitMessageEvent", "wrapped structured error occurred").Once()

	emittingLogger.Error(context.Background(), logging.LevelError, wrappedErr, "wrapped structured error occurred")

	emitter.AssertExpectations(t)
}
