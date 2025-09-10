// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package validators provides validation mechanisms for AI model responses.
// It includes support for both value matching and LLM-based
// semantic equivalence validation using judge models.
package validators

import (
	"context"
	"errors"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
)

// ErrUnsupportedResponseFormatValidation is returned when a validator cannot handle the response format.
var ErrUnsupportedResponseFormatValidation = errors.New("unsupported response format validation")

// ValidationResult contains the result of a validation check.
type ValidationResult struct {
	// IsCorrect indicates whether the validation passed.
	IsCorrect bool
	// Title provides a descriptive title for the validation type.
	Title string
	// Explanation provides an optional explanation of the validation result.
	Explanation string
	// Usage contains token usage statistics for the validation step when available.
	Usage providers.Usage
}

// Validator verifies AI model responses.
type Validator interface {
	// IsCorrect checks if result matches expected value using the provided validation rules.
	// The originalPrompt and expectedResponseFormat provide additional context for semantic validation.
	// The logger parameter allows validators to emit structured log messages during validation.
	IsCorrect(ctx context.Context, logger logging.Logger, rules config.ValidationRules, expected utils.ValueSet, actual providers.Result, originalPrompt string, expectedResponseFormat config.ResponseFormat) (ValidationResult, error)
	// ToCanonical normalizes value for validation using the provided validation rules.
	// For string values, applies string normalization rules (case, whitespace, etc.).
	// For object values, recursively normalizes all string fields within the object structure.
	ToCanonical(rules config.ValidationRules, value interface{}) interface{}
	// GetName returns a descriptive user-friendly name for the validator.
	GetName() string
	// Close cleans up any resources used by the validator.
	Close(ctx context.Context) error
}
