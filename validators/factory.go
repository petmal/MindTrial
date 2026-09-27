// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/petmal/mindtrial/config"
)

const (
	valueMatchValidatorCacheKey = "value_match_validator"
	schemaValidatorCacheKey     = "schema_validator"
)

var (
	// ErrJudgeNotFound is returned when a judge configuration is not found.
	ErrJudgeNotFound = errors.New("judge not found")
	// ErrJudgeVariantNotFound is returned when a judge run variant is not found.
	ErrJudgeVariantNotFound = errors.New("judge run variant not found")
)

// judgeConfigVariant represents a judge configuration and its associated run variant configuration.
type judgeConfigVariant struct {
	judgeConfig *config.JudgeConfig
	runConfig   *config.RunConfig
}

// Factory creates and manages validator instances.
// It provides caching to improve performance.
type Factory struct {
	cache                    sync.Map
	judgeConfigs             []config.JudgeConfig
	judgeConfigVariantLookup map[string]map[string]*judgeConfigVariant
	customValidatorConfigs   map[string]config.ValidatorConfig
}

// NewFactory creates a validator factory without custom validators.
func NewFactory(availableJudges []config.JudgeConfig) *Factory {
	return NewFactoryWithCustomValidators(availableJudges, nil)
}

// NewFactoryWithCustomValidators creates a validator factory with Docker custom validators.
func NewFactoryWithCustomValidators(availableJudges []config.JudgeConfig, availableCustomValidators []config.ValidatorConfig) *Factory {
	// Build lookup table for fast judge and run variant config lookup.
	lookup := make(map[string]map[string]*judgeConfigVariant, len(availableJudges))

	for i, judgeConfig := range availableJudges {
		// Build run variant lookup for this judge.
		if lookup[judgeConfig.Name] == nil {
			lookup[judgeConfig.Name] = make(map[string]*judgeConfigVariant, len(judgeConfig.Provider.Runs))
		}

		for j, runConfig := range judgeConfig.Provider.Runs {
			lookup[judgeConfig.Name][runConfig.Name] = &judgeConfigVariant{
				judgeConfig: &availableJudges[i],
				runConfig:   &availableJudges[i].Provider.Runs[j],
			}
		}
	}

	customValidatorConfigs := make(map[string]config.ValidatorConfig, len(availableCustomValidators))
	for _, validatorConfig := range availableCustomValidators {
		customValidatorConfigs[validatorConfig.Name] = validatorConfig
	}

	return &Factory{
		judgeConfigs:             availableJudges,
		judgeConfigVariantLookup: lookup,
		customValidatorConfigs:   customValidatorConfigs,
	}
}

func (f *Factory) createJudgeCacheKey(judge config.JudgeSelector) string {
	return fmt.Sprintf("judge_%s_%s", judge.GetName(), judge.GetVariant())
}

func (f *Factory) createCustomValidatorCacheKey(name string) string {
	return "custom_" + name
}

// GetValidator returns a validator for the given validation rules.
// Selection: custom-validator -> CustomValidator, judge -> JudgeValidator,
// schema-validation -> SchemaValidator, otherwise ValueMatchValidator.
func (f *Factory) GetValidator(ctx context.Context, rules config.ValidationRules) (Validator, error) {
	if rules.UseCustomValidator() {
		return f.getCustomValidator(ctx, rules.GetCustomValidatorName())
	}
	if rules.UseJudge() {
		return f.getJudgeValidator(ctx, rules.Judge)
	}
	if rules.UseSchemaValidation() {
		return f.getSchemaValidator(), nil
	}
	return f.getValueMatchValidator(), nil
}

// AssertExists checks if a judge configuration exists for the given judge selector.
// Returns an error if the judge configuration does not exist.
func (f *Factory) AssertExists(judge config.JudgeSelector) error {
	_, _, err := f.lookupJudgeConfig(judge)
	return err
}

// AssertCustomValidatorExists checks whether a named custom validator is configured.
func (f *Factory) AssertCustomValidatorExists(name string) error {
	if _, exists := f.customValidatorConfigs[name]; !exists {
		return fmt.Errorf("%w: %s", ErrCustomValidatorNotFound, name)
	}
	return nil
}

func (f *Factory) getCustomValidator(ctx context.Context, name string) (Validator, error) {
	key := f.createCustomValidatorCacheKey(name)

	if validator, exists := f.cache.Load(key); exists {
		return validator.(Validator), nil
	}

	validatorConfig, exists := f.customValidatorConfigs[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCustomValidatorNotFound, name)
	}

	validator := newCustomValidator(validatorConfig)
	actual, loaded := f.cache.LoadOrStore(key, validator)
	if loaded {
		// Another goroutine won the cache race; close this redundant instance.
		_ = validator.Close(ctx) // best-effort cleanup; caller gets the cached validator
	}
	return actual.(Validator), nil
}

func (f *Factory) getValueMatchValidator() Validator {
	if validator, exists := f.cache.Load(valueMatchValidatorCacheKey); exists {
		return validator.(Validator)
	}

	validator := NewValueMatchValidator()
	actual, _ := f.cache.LoadOrStore(valueMatchValidatorCacheKey, validator)
	return actual.(Validator)
}

func (f *Factory) getSchemaValidator() Validator {
	if validator, exists := f.cache.Load(schemaValidatorCacheKey); exists {
		return validator.(Validator)
	}

	validator := NewSchemaValidator()
	actual, _ := f.cache.LoadOrStore(schemaValidatorCacheKey, validator)
	return actual.(Validator)
}

// lookupJudgeConfig looks up judge configuration and run variant configuration for the given judge selector.
func (f *Factory) lookupJudgeConfig(judge config.JudgeSelector) (*config.JudgeConfig, *config.RunConfig, error) {
	judgeName := judge.GetName()
	judgeVariant := judge.GetVariant()

	runLookup, exists := f.judgeConfigVariantLookup[judgeName]
	if !exists {
		return nil, nil, fmt.Errorf("%w: %s", ErrJudgeNotFound, judgeName)
	}

	entry, exists := runLookup[judgeVariant]
	if !exists {
		return nil, nil, fmt.Errorf("%w: %s for judge %s", ErrJudgeVariantNotFound, judgeVariant, judgeName)
	}

	return entry.judgeConfig, entry.runConfig, nil
}

func (f *Factory) getJudgeValidator(ctx context.Context, judge config.JudgeSelector) (Validator, error) {
	key := f.createJudgeCacheKey(judge)

	if validator, exists := f.cache.Load(key); exists {
		return validator.(Validator), nil
	}

	// Look up judge configuration and run variant configuration.
	judgeConfig, judgeRunVariant, err := f.lookupJudgeConfig(judge)
	if err != nil {
		return nil, err
	}

	// Create judge validator.
	// Judge validators currently do not use any tools, so we pass nil for availableTools.
	validator, err := NewJudgeValidator(ctx, judgeConfig, *judgeRunVariant, nil)
	if err != nil {
		return nil, err
	}

	actual, loaded := f.cache.LoadOrStore(key, validator)
	if loaded {
		// Another goroutine won the cache race; close this redundant instance.
		_ = validator.Close(ctx) // best-effort cleanup; caller gets the cached validator
	}
	return actual.(Validator), nil
}

// Close closes all cached validators and returns any errors that occurred.
func (f *Factory) Close(ctx context.Context) error {
	var errs []error

	f.cache.Range(func(_, value interface{}) bool {
		if validator, ok := value.(Validator); ok {
			errs = append(errs, validator.Close(ctx))
		}
		return true
	})

	return errors.Join(errs...)
}
