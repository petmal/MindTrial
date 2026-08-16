// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"errors"
	"sync"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
)

// GradeVerdict grades a judge's raw verdict against the configured passing-verdicts
// criterion, returning whether the verdict counts as a pass. This is the single grading
// path used to evaluate judge verdicts of any shape (boolean, score, grade, etc.), so
// custom verdict formats and passing criteria go through the same logic as the built-in
// default. The underlying logic is generic (rules, an expected utils.ValueSet, and an
// actual value), so it also backs schemaValidator's exact-or-explicit-schema matching.
//
// If passingVerdicts specifies an explicit JSON Schema (config.ExplicitSchema),
// verdict is validated against it directly, without normalization: an explicit schema
// describes the exact shape of a passing verdict, so applying rules-based normalization
// (e.g. case-insensitive comparison) would silently change its meaning.
//
// Otherwise, passingVerdicts is treated as a set of literal value(s) that verdict must
// equal after canonicalization: both the expected value(s) and verdict are canonicalized
// with rules (the same canonicalization valueMatchValidator uses for ordinary task
// answers), then compared via a generated "const" (single value) or "enum" (multiple
// values) schema.
//
// Returns (false, nil) when verdict does not conform to the schema (a legitimate
// grading failure, not an error), and a non-nil error only for a malformed schema.
func GradeVerdict(rules config.ValidationRules, passingVerdicts utils.ValueSet, verdict interface{}) (bool, error) {
	if schema, ok := config.ExplicitSchema(passingVerdicts); ok {
		return validateAgainstSchema(schema, verdict)
	}

	canonicalValues := utils.NewValueSet(canonicalizeValues(rules, passingVerdicts.Values())...).Values()
	canonicalVerdict := canonicalizeValue(rules, verdict)

	var schema map[string]interface{}
	if len(canonicalValues) == 1 {
		schema = map[string]interface{}{"const": canonicalValues[0]}
	} else {
		schema = map[string]interface{}{"enum": canonicalValues}
	}
	return validateAgainstSchema(schema, canonicalVerdict)
}

// validateAgainstSchema validates value against schema, translating a schema
// validation failure into (false, nil) since that represents a legitimate grading
// failure rather than an error. Any other error (e.g. an invalid schema) is returned as-is.
func validateAgainstSchema(schema map[string]interface{}, value interface{}) (bool, error) {
	if err := utils.ValidateAgainstSchema(schema, value); err != nil {
		if errors.Is(err, utils.ErrJSONSchemaValidation) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// schemaValidator validates responses against expected value(s), supporting an explicit
// JSON Schema (config.ExplicitSchema) as an alternative to exact matching, so a task's
// expected-result can describe valid structured output declaratively instead of requiring
// an exact field-by-field match.
//
// Not currently registered in Factory/exposed via task configuration; reserved for a
// future commit that lets tasks opt into schema-based expected-result matching.
type schemaValidator struct{}

// schemaValidatorInstance is a singleton instance of schemaValidator since it has no state.
var schemaValidatorInstance = sync.OnceValue(func() Validator {
	return &schemaValidator{}
})

// NewSchemaValidator returns a new Validator that checks results by exact matching or,
// when the expected value is an explicit JSON Schema, by schema conformance.
func NewSchemaValidator() Validator {
	return schemaValidatorInstance()
}

func (v schemaValidator) IsCorrect(_ context.Context, _ logging.Logger, rules config.ValidationRules, expected utils.ValueSet, actual providers.Result, _ string, _ config.ResponseFormat) (ValidationResult, error) {
	isCorrect, err := GradeVerdict(rules, expected, actual.GetFinalAnswerContent())
	if err != nil {
		return ValidationResult{}, err
	}

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

func (v schemaValidator) ToCanonical(rules config.ValidationRules, value interface{}) interface{} {
	return canonicalizeValue(rules, value)
}

func (v schemaValidator) GetName() string {
	return "schema match"
}

func (v schemaValidator) Close(_ context.Context) error {
	return nil
}

// canonicalizeValues canonicalizes each value in values using rules.
func canonicalizeValues(rules config.ValidationRules, values []interface{}) []interface{} {
	canonical := make([]interface{}, len(values))
	for i, v := range values {
		canonical[i] = canonicalizeValue(rules, v)
	}
	return canonical
}
