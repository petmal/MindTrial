// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	providertools "github.com/petmal/mindtrial/providers/tools"
)

var (
	// ErrCustomValidatorNotFound is returned when a selected custom validator is unavailable.
	ErrCustomValidatorNotFound = errors.New("custom validator not found")
	// ErrCustomValidatorTemplate is returned when a custom validator template cannot be compiled or rendered.
	ErrCustomValidatorTemplate = errors.New("invalid custom validator template")
	// ErrCustomValidatorExecution is returned when a custom validator cannot be run to completion.
	ErrCustomValidatorExecution = errors.New("custom validator execution failed")
	// ErrCustomValidatorResponse is returned when a custom validator emits an invalid response.
	ErrCustomValidatorResponse = errors.New("invalid custom validator response")
)

// CompileCustomValidatorTemplates checks that all command, environment, and template-file
// templates of cfg compile, without requiring any template data.
func CompileCustomValidatorTemplates(cfg config.ValidatorConfig) error {
	for i, argument := range cfg.Command {
		if _, err := utils.ParseTemplate("custom-validator-command", argument); err != nil {
			return fmt.Errorf("%w: command argument %d: %w", ErrCustomValidatorTemplate, i, err)
		}
	}
	for _, name := range utils.SortedKeys(cfg.Env) {
		if _, err := utils.ParseTemplate("custom-validator-env", cfg.Env[name]); err != nil {
			return fmt.Errorf("%w: environment variable %q: %w", ErrCustomValidatorTemplate, name, err)
		}
	}
	for _, file := range cfg.TemplateFiles {
		if _, err := utils.ParseTemplate("custom-validator-file", file.Template); err != nil {
			return fmt.Errorf("%w: template file %q: %w", ErrCustomValidatorTemplate, file.Path, err)
		}
	}
	return nil
}

type customValidatorResponse struct {
	Correct     *bool  `json:"correct"`
	Title       string `json:"title"`
	Explanation string `json:"explanation"`
}

type customValidator struct {
	config config.ValidatorConfig
}

func newCustomValidator(cfg config.ValidatorConfig) Validator {
	return &customValidator{config: cfg}
}

// customValidatorTemplateData is the context of custom validator templates; it follows the
// judge prompt vocabulary and adds the execution metadata of the task attempt.
type customValidatorTemplateData struct {
	providertools.ExecutionTemplateData
	OriginalTask customValidatorTemplateOriginalTask
	Candidate    customValidatorTemplateCandidate
	Rules        customValidatorTemplateRules
}

type customValidatorTemplateOriginalTask struct {
	Prompt               string
	ResponseResultFormat string
	ExpectedResults      []interface{}
}

type customValidatorTemplateCandidate struct {
	Response interface{}
}

type customValidatorTemplateRules struct {
	CaseSensitive    bool
	IgnoreWhitespace bool
	TrimLines        bool
}

func (v *customValidator) IsCorrect(ctx context.Context, logger logging.Logger, rules config.ValidationRules, expected utils.ValueSet, actual providers.Result, originalPrompt string, expectedResponseFormat config.ResponseFormat) (ValidationResult, error) {
	return v.IsCorrectWithEnvironment(ctx, logger, rules, expected, actual, originalPrompt, expectedResponseFormat, nil, providertools.ExecutionTemplateData{})
}

func (v *customValidator) IsCorrectWithEnvironment(
	ctx context.Context,
	logger logging.Logger,
	rules config.ValidationRules,
	expected utils.ValueSet,
	actual providers.Result,
	originalPrompt string,
	expectedResponseFormat config.ResponseFormat,
	environment providers.ExecutionEnvironment,
	metadata providertools.ExecutionTemplateData,
) (ValidationResult, error) {
	expectedResults := expected.Values()
	if expectedResults == nil {
		// Dynamic tasks have no expected-result; an empty array keeps the JSON shape stable.
		expectedResults = []interface{}{}
	}
	command, env, files, err := v.renderInputs(customValidatorTemplateData{
		ExecutionTemplateData: metadata,
		OriginalTask: customValidatorTemplateOriginalTask{
			Prompt:               originalPrompt,
			ResponseResultFormat: expectedResponseFormat.String(),
			ExpectedResults:      expectedResults,
		},
		Candidate: customValidatorTemplateCandidate{
			Response: actual.GetFinalAnswerContent(),
		},
		Rules: customValidatorTemplateRules{
			CaseSensitive:    rules.IsCaseSensitive(),
			IgnoreWhitespace: rules.IsIgnoreWhitespace(),
			TrimLines:        rules.IsTrimLines(),
		},
	})
	if err != nil {
		return ValidationResult{}, fmt.Errorf("custom validator %q: %w", v.config.Name, err)
	}

	toolConfig := config.ToolConfig{
		Name:         "custom-validator-" + v.config.Name,
		Image:        v.config.Image,
		Description:  "MindTrial trusted custom validator.",
		Parameters:   map[string]interface{}{"type": "object"},
		Command:      command,
		Env:          env,
		Dependencies: v.config.Dependencies,
	}

	executor, err := providertools.NewToolExecutor(ctx, environment)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("%w: custom validator %q setup: %w", ErrCustomValidatorExecution, v.config.Name, err)
	}
	defer func() {
		if cleanupErr := executor.Close(); cleanupErr != nil {
			logger.Error(ctx, logging.LevelWarn, cleanupErr, "failed to close custom validator executor")
		}
	}()

	tool := providertools.NewDockerTool(&toolConfig, nil, v.config.Timeout, v.config.MaxMemoryMB, v.config.CpuPercent)
	tool.SetReadOnlyFiles(files)
	executor.RegisterTool(tool)

	responseBytes, err := executor.ExecuteTool(ctx, logger, toolConfig.Name, json.RawMessage(`{}`), nil, nil)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("%w: custom validator %q: %w", ErrCustomValidatorExecution, v.config.Name, err)
	}
	response, err := decodeCustomValidatorResponse(responseBytes)
	if err != nil {
		return ValidationResult{}, err
	}

	return ValidationResult{
		IsCorrect:   *response.Correct,
		Title:       response.Title,
		Explanation: response.Explanation,
	}, nil
}

// renderInputs renders the command arguments, environment values, and template files of the validator.
func (v *customValidator) renderInputs(data customValidatorTemplateData) (command []string, env map[string]string, files map[string][]byte, err error) {
	if v.config.Command != nil {
		command = make([]string, len(v.config.Command))
		for i, argument := range v.config.Command {
			if command[i], err = utils.ExpandTemplate("custom-validator-command", argument, data); err != nil {
				return nil, nil, nil, fmt.Errorf("%w: command argument %d: %w", ErrCustomValidatorTemplate, i, err)
			}
		}
	}
	if v.config.Env != nil {
		env = make(map[string]string, len(v.config.Env))
		for name, value := range v.config.Env {
			if env[name], err = utils.ExpandTemplate("custom-validator-env", value, data); err != nil {
				return nil, nil, nil, fmt.Errorf("%w: environment variable %q: %w", ErrCustomValidatorTemplate, name, err)
			}
		}
	}
	files = make(map[string][]byte, len(v.config.TemplateFiles))
	for _, file := range v.config.TemplateFiles {
		content, expandErr := utils.ExpandTemplate("custom-validator-file", file.Template, data)
		if expandErr != nil {
			return nil, nil, nil, fmt.Errorf("%w: template file %q: %w", ErrCustomValidatorTemplate, file.Path, expandErr)
		}
		files[path.Clean(file.Path)] = []byte(content)
	}
	return command, env, files, nil
}

func (v *customValidator) ToCanonical(_ config.ValidationRules, value interface{}) interface{} {
	return value
}

func (v *customValidator) GetName() string {
	return "custom validator " + v.config.Name
}

func (v *customValidator) Close(context.Context) error {
	return nil
}

func decodeCustomValidatorResponse(data []byte) (customValidatorResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var response customValidatorResponse
	if err := decoder.Decode(&response); err != nil {
		return customValidatorResponse{}, fmt.Errorf("%w: %w", ErrCustomValidatorResponse, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return customValidatorResponse{}, fmt.Errorf("%w: trailing validator output", ErrCustomValidatorResponse)
	}
	if response.Correct == nil || strings.TrimSpace(response.Title) == "" || strings.TrimSpace(response.Explanation) == "" {
		return customValidatorResponse{}, fmt.Errorf("%w: correct, title, and explanation are required", ErrCustomValidatorResponse)
	}
	return response, nil
}
