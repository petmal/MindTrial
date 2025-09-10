// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"math"
	"reflect"
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

func (v valueMatchValidator) IsCorrect(ctx context.Context, _ logging.Logger, rules config.ValidationRules, expected utils.ValueSet, actual providers.Result, _ string, _ config.ResponseFormat) (ValidationResult, error) {
	canonicalActual := v.ToCanonical(rules, actual.GetFinalAnswerContent())
	isCorrect := expected.Any(func(expectedValue interface{}) bool {
		canonicalExpected := v.ToCanonical(rules, expectedValue)
		return reflect.DeepEqual(canonicalExpected, canonicalActual)
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

func (v valueMatchValidator) ToCanonical(rules config.ValidationRules, value interface{}) interface{} {
	// For string values, apply string normalization.
	if str, ok := value.(string); ok {
		return v.toCanonicalString(rules, str)
	}
	// For objects, apply recursive canonicalization.
	return v.toCanonicalObject(rules, value)
}

// toCanonicalObject recursively converts an object to canonical form by normalizing string values.
func (v valueMatchValidator) toCanonicalObject(rules config.ValidationRules, value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch val := value.(type) {
	case string:
		return v.toCanonicalString(rules, val)

	case map[string]interface{}:
		result := make(map[string]interface{})
		// Sort keys for deterministic comparison.
		keys := utils.SortedKeys(val)

		for _, k := range keys {
			result[k] = v.toCanonicalObject(rules, val[k])
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(val))
		for i, elem := range val {
			result[i] = v.toCanonicalObject(rules, elem)
		}
		return result

	case float64:
		// Handle number comparison edge cases - convert to int if it's a whole number.
		if val == float64(int64(val)) {
			return int64(val)
		}
		return val

	default:
		// For other types, normalize numbers if applicable, otherwise return as-is.
		// Numeric normalization ensures consistent comparison of data
		// from various sources such as JSON and YAML that may represent numbers differently.
		return normalizeNumbers(val)
	}
}

func (v valueMatchValidator) toCanonicalString(rules config.ValidationRules, value string) string {
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

// normalizeNumbers normalizes numeric types in the given value:
// - Any signed int type (int, int8, int16, int32) -> int64
// - Any unsigned int type except uint64 (uint, uint8, uint16, uint32) -> int64
// - uint64 -> uint64 (unchanged)
// - float32 -> float64
// - Other types remain unchanged.
func normalizeNumbers(value interface{}) interface{} {
	switch val := value.(type) {
	case int:
		return int64(val)
	case int8:
		return int64(val)
	case int16:
		return int64(val)
	case int32:
		return int64(val)
	case uint:
		if val > math.MaxInt64 {
			return uint64(val) // convert large uint to uint64 to avoid overflow
		}
		return int64(val)
	case uint8:
		return int64(val)
	case uint16:
		return int64(val)
	case uint32:
		return int64(val)
	case float32:
		return float64(val)
	default:
		return value
	}
}

func (v valueMatchValidator) GetName() string {
	return "value match"
}

func (v valueMatchValidator) Close(ctx context.Context) error {
	return nil
}
