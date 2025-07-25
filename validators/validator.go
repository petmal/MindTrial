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

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
)

// ValidationResult contains the result of a validation check.
type ValidationResult struct {
	// IsCorrect indicates whether the validation passed.
	IsCorrect bool
	// Title provides a descriptive title for the validation type.
	Title string
	// Explanation provides an optional explanation of the validation result.
	Explanation string
	// assessment contains the original assessment result.
	// This field may not be populated by all validators.
	assessment *providers.Result
}

// Explain returns a formatted explanation of the validation result.
func (vr ValidationResult) Explain() string {
	return vr.Title + "\n\n" + vr.Explanation
}

// GetAssessmentResult returns the original assessment provider result if available.
func (vr ValidationResult) GetAssessmentResult() *providers.Result {
	return vr.assessment
}

// Validator verifies AI model responses.
type Validator interface {
	// IsCorrect checks if result matches expected value using the provided validation rules.
	// The originalPrompt and expectedResponseFormat provide additional context for semantic validation.
	IsCorrect(ctx context.Context, rules config.ValidationRules, expected utils.StringSet, actual providers.Result, originalPrompt string, expectedResponseFormat string) (ValidationResult, error)
	// ToCanonical normalizes string for validation using the provided validation rules.
	ToCanonical(rules config.ValidationRules, value string) string
	// GetName returns a descriptive user-friendly name for the validator.
	GetName() string
	// Close cleans up any resources used by the validator.
	Close(ctx context.Context) error
}
