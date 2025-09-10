// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package validators

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	"github.com/petmal/mindtrial/providers/execution"
)

const judgeTaskName = "response assessment"

// judgeResponseFormat is a lazily initialized singleton for the judge task's ResponseFormat.
var judgeResponseFormat = sync.OnceValue(func() config.ResponseFormat {
	judgeSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"correct": map[string]interface{}{
				"type":        "boolean",
				"title":       "Semantic Equivalence Verdict",
				"description": "True if the candidate response is semantically equivalent to any expected answer, false otherwise. Follow the provided evaluation criteria and normalization rules.",
			},
		},
		"required":             []string{"correct"},
		"additionalProperties": false,
	}
	return config.NewResponseFormat(judgeSchema)
})

// judgeExpectedResult is a lazily initialized singleton for the judge task's ExpectedResult.
var judgeExpectedResult = sync.OnceValue(func() utils.ValueSet {
	expectedResult := map[string]interface{}{
		"correct": true,
	}
	return utils.NewValueSet(expectedResult)
})

// judgeValidator uses an LLM to evaluate the correctness of responses.
// It provides semantic validation by comparing model responses against expected answers
// using another AI model as a judge, rather than relying on exact value matching.
type judgeValidator struct {
	executor *execution.Executor
	name     string
}

// NewJudgeValidator creates a new semantic Validator with the given judge configuration and run variant.
// The judge provider will be initialized from the configuration and used to evaluate responses
// for semantic equivalence.
func NewJudgeValidator(ctx context.Context, judgeConfig *config.JudgeConfig, judgeRunVariant config.RunConfig) (Validator, error) {
	judgeProvider, err := providers.NewProvider(ctx, judgeConfig.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create judge provider: %w", err)
	}

	executor := execution.NewExecutor(judgeProvider, judgeRunVariant)
	name := fmt.Sprintf("%s %s judge", judgeRunVariant.Name, judgeConfig.Name)

	return &judgeValidator{
		executor: executor,
		name:     name,
	}, nil
}

// IsCorrect evaluates the response using the judge LLM.
// This validator always returns a result with `IsCorrect` set to false for non-string responses.
// If the tasks requires a structured schema-based response format the validator returns an error.
// The originalPrompt and expectedResponseFormat provide additional context to help the judge
// make more informed evaluations by understanding the task requirements.
func (v *judgeValidator) IsCorrect(ctx context.Context, logger logging.Logger, rules config.ValidationRules, expected utils.ValueSet, actual providers.Result, originalPrompt string, expectedResponseFormat config.ResponseFormat) (result ValidationResult, err error) {
	// Get expected results as strings for judge evaluation.
	expectedStrings, isPlainTextExpected := expected.AsStringSet()

	// Semantic validation requires plain text responses.
	if !isPlainTextExpected {
		return ValidationResult{}, fmt.Errorf("%w: semantic validation requires plain text responses", ErrUnsupportedResponseFormatValidation)
	}

	// Check if actual result is a string - if not, return validation failure.
	actualString, ok := actual.GetFinalAnswerContent().(string)
	if !ok {
		return ValidationResult{
			IsCorrect:   false,
			Title:       "Invalid Response Type",
			Explanation: fmt.Sprintf("Semantic validation requires plain text responses but received %T:\n%v", actual.GetFinalAnswerContent(), utils.ToString(actual.GetFinalAnswerContent())),
			Usage:       actual.GetUsage(),
		}, nil
	}
	// Create prefixed logger for judge evaluation, extending the existing prefix.
	judgeLogger := logger.WithContext(fmt.Sprintf("%s: %s: ", judgeTaskName, v.name))

	// Create a task for the judge to evaluate.
	expectedFormatString, _ := expectedResponseFormat.AsString()
	prompt, err := v.createJudgePrompt(rules, expectedStrings, actualString, originalPrompt, expectedFormatString)
	if err != nil {
		return result, fmt.Errorf("failed to create judge prompt: %w", err)
	}

	judgeTask := config.Task{
		Name:                 judgeTaskName,
		Prompt:               prompt,
		ResponseResultFormat: judgeResponseFormat(),
		ExpectedResult:       judgeExpectedResult(),
	}

	// Execute the judge task and evaluate the response.
	judgeTaskResult, err := v.executor.Execute(ctx, judgeLogger, judgeTask)
	usage := judgeTaskResult.GetUsage()
	if err != nil {
		judgeLogger.Error(ctx, logging.LevelError, err, "finished with error")
		return ValidationResult{Usage: usage}, fmt.Errorf("judge evaluation failed: %w", err)
	}

	judgeLogger.Message(ctx, logging.LevelTrace, "verdict: %s", utils.ToString(judgeTaskResult.GetFinalAnswerContent()))

	// Log statistics about the judge task execution.
	judgeLogger.Message(ctx, logging.LevelDebug, "completed in %s", judgeTaskResult.GetDuration())
	judgeLogger.Message(ctx, logging.LevelDebug, "token usage: [in:%s, out:%s]", logging.FormatLogInt64(usage.InputTokens), logging.FormatLogInt64(usage.OutputTokens))
	judgeLogger.Message(ctx, logging.LevelTrace, "prompts:\n%s", logging.FormatLogText(judgeTaskResult.GetPrompts()))

	validationResult, err := NewValueMatchValidator().IsCorrect(ctx, judgeLogger, config.ValidationRules{}, judgeTask.ExpectedResult, judgeTaskResult, judgeTask.Prompt, judgeTask.ResponseResultFormat)
	if err != nil {
		return ValidationResult{Usage: usage}, fmt.Errorf("failed to evaluate judge response: %w", err)
	}

	var explanation string
	if validationResult.IsCorrect {
		explanation = fmt.Sprintf("Response is semantically equivalent to one of the accepted answers.\n\nJudge reasoning:\n%s", judgeTaskResult.Explanation)
	} else {
		explanation = fmt.Sprintf("Response is not semantically equivalent to any of the accepted answers.\n\nJudge reasoning:\n%s", judgeTaskResult.Explanation)
	}

	return ValidationResult{
		IsCorrect:   validationResult.IsCorrect,
		Title:       "Semantic Assessment",
		Explanation: explanation,
		Usage:       usage,
	}, nil
}

func (v *judgeValidator) ToCanonical(_ config.ValidationRules, value interface{}) interface{} {
	// Judge validation only works with strings.
	// Only trim whitespace to preserve the original model output.
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return value
}

func (v *judgeValidator) GetName() string {
	return v.name
}

func (v *judgeValidator) Close(ctx context.Context) error {
	return v.executor.Provider.Close(ctx)
}

// judgePromptTemplate defines the template for judge semantic evaluation prompts.
var judgePromptTemplate = template.Must(template.New("judgePrompt").Parse(`You are an automatic grader. Decide if the candidate response is semantically equivalent to ANY ONE of the expected answers.

Definitions
- Semantic equivalence: the candidate conveys the same meaning and required facts as an expected answer; wording may differ.
- Extra content: ignore unless it contradicts or changes the meaning.
- Normalization: apply the flags below BEFORE comparing (case/whitespace).

Inputs
Original task prompt:
{{.OriginalPrompt}}

Original answer format instruction:
{{.ExpectedResponseFormat}}

Expected answer(s) (match any one):
{{- range .ExpectedAnswers}}
- {{.}}
{{- end}}

Candidate response:
{{.ActualResponse}}

Validation flags:
- Case sensitive: {{if .Rules.IsCaseSensitive}}yes{{else}}no{{end}}
- Ignore whitespace: {{if .Rules.IsIgnoreWhitespace}}yes{{else}}no{{end}}

Procedure
1. Normalize candidate and each expected answer per the flags.  
2. Compare the candidate to each expected answer independently for semantic equivalence.  
3. Set "correct" to true if ANY match, false otherwise.`))

// createJudgePrompt creates a prompt for the judge to evaluate semantic equivalence.
// The originalPrompt and expectedResponseFormat provide additional context to help the judge
// understand the task requirements and make more informed evaluations.
func (v *judgeValidator) createJudgePrompt(rules config.ValidationRules, expected utils.StringSet, actualResponse, originalPrompt, expectedResponseFormat string) (string, error) {
	data := struct {
		ExpectedAnswers        []string
		ActualResponse         string
		Rules                  config.ValidationRules
		OriginalPrompt         string
		ExpectedResponseFormat string
	}{
		ExpectedAnswers:        expected.Values(),
		ActualResponse:         actualResponse,
		Rules:                  rules,
		OriginalPrompt:         originalPrompt,
		ExpectedResponseFormat: expectedResponseFormat,
	}

	var result strings.Builder
	if err := judgePromptTemplate.Execute(&result, data); err != nil {
		return "", err
	}

	return result.String(), nil
}
