// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package tools provides implementations for executing tools
// as part of MindTrial's function calling capabilities.
package tools

import (
	"errors"
)

var (
	// ErrToolNotAvailable is returned when a requested tool is not available.
	ErrToolNotAvailable = errors.New("tool not available")
	// ErrToolExecutionFailed is returned when a tool executes but fails with an error.
	ErrToolExecutionFailed = errors.New("tool execution failed")
	// ErrInvalidToolArguments is returned when tool arguments are invalid or don't match the expected schema.
	ErrInvalidToolArguments = errors.New("invalid tool arguments")
	// ErrToolInternal is returned for low-level internal errors during tool execution.
	ErrToolInternal = errors.New("tool internal error")
	// ErrUnsupportedToolType is returned when an unsupported tool type is encountered.
	ErrUnsupportedToolType = errors.New("unsupported tool type")
	// ErrToolMaxCallsExceeded is returned when a tool has exceeded its maximum call limit.
	ErrToolMaxCallsExceeded = errors.New("tool max calls exceeded")
	// ErrToolTimeout is returned when a tool execution times out.
	ErrToolTimeout = errors.New("tool execution timeout")
)
