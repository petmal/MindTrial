// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"regexp"
	"strings"
	"sync"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
)

// whitespaceRegex is a compiled regular expression for matching whitespace characters.
var whitespaceRegex = regexp.MustCompile(`\s+`)

// valueMatchValidator validates responses by comparing them with expected values.
type valueMatchValidator struct {
}

// valueMatchValidatorInstance is a singleton instance of valueMatchValidator since it has no state.
var valueMatchValidatorInstance = sync.OnceValue(func() Validator {
	return &valueMatchValidator{}
})

// NewValueMatchValidator returns a new Validator that checks results by exact string matching.
// The validator applies validation rules for case sensitivity and whitespace handling.
func NewValueMatchValidator() Validator {
	return valueMatchValidatorInstance()
}

func (v valueMatchValidator) IsCorrect(ctx context.Context, _ logging.Logger, rules config.ValidationRules, expected utils.StringSet, actual providers.Result, _ string, _ string) (ValidationResult, error) {
	isCorrect := expected.Any(func(expectedAnswer string) bool {
		return v.ToCanonical(rules, expectedAnswer) == v.ToCanonical(rules, actual.FinalAnswer)
	})

	var explanation string
	if isCorrect {
		explanation = "Response matches one of the accepted answers."
	} else {
		explanation = "Response does not match any of the accepted answers."
	}

	return ValidationResult{
		IsCorrect:   isCorrect,
		Title:       "Response Assessment",
		Explanation: explanation,
	}, nil
}

func (v valueMatchValidator) ToCanonical(rules config.ValidationRules, value string) string {
	canonical := value

	// Trim each line's leading/trailing whitespace.
	if rules.IsTrimLines() && !rules.IsIgnoreWhitespace() {
		lines := utils.SplitLines(canonical)
		for i := range lines {
			lines[i] = strings.TrimSpace(lines[i])
		}
		canonical = strings.Join(lines, "\n")
	}

	// Handle whitespace.
	if rules.IsIgnoreWhitespace() {
		canonical = whitespaceRegex.ReplaceAllString(canonical, "")
	} else {
		canonical = strings.TrimSpace(canonical)
	}

	// Handle case sensitivity.
	if !rules.IsCaseSensitive() {
		canonical = strings.ToLower(canonical)
	}

	return canonical
}

func (v valueMatchValidator) GetName() string {
	return "value match"
}

func (v valueMatchValidator) Close(ctx context.Context) error {
	return nil
}
