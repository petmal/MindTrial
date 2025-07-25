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
	"strings"
	"text/template"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers"
	"github.com/sethvargo/go-retry"
	"golang.org/x/time/rate"
)

const judgeTaskName = "judge_evaluation"

// judgeValidator uses an LLM to evaluate the correctness of responses.
// It provides semantic validation by comparing model responses against expected answers
// using another AI model as a judge, rather than relying on exact value matching.
type judgeValidator struct {
	judge           providers.Provider
	judgeRunVariant config.RunConfig
	limiter         *rate.Limiter
}

// NewJudgeValidator creates a new semantic Validator with the given provider and run variant configuration.
// The judge provider will be used to evaluate responses for semantic equivalence.
// Rate limiting is applied based on the run variant configuration's MaxRequestsPerMinute setting.
func NewJudgeValidator(judge providers.Provider, judgeRunVariant config.RunConfig) Validator {
	var limiter *rate.Limiter
	if judgeRunVariant.MaxRequestsPerMinute > 0 {
		// Allow a burst up to the per-minute limit.
		limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(judgeRunVariant.MaxRequestsPerMinute)), judgeRunVariant.MaxRequestsPerMinute)
	}

	return &judgeValidator{
		judge:           judge,
		judgeRunVariant: judgeRunVariant,
		limiter:         limiter,
	}
}

// IsCorrect evaluates the response using the judge LLM.
// The originalPrompt and expectedResponseFormat provide additional context to help the judge
// make more informed evaluations by understanding the task requirements.
func (v *judgeValidator) IsCorrect(ctx context.Context, rules config.ValidationRules, expected utils.StringSet, actual providers.Result, originalPrompt string, expectedResponseFormat string) (result ValidationResult, err error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}

	if v.limiter != nil {
		if err := v.limiter.Wait(ctx); err != nil {
			return result, err
		}
	}

	// Create a task for the judge to evaluate.
	prompt, err := v.createJudgePrompt(rules, expected, actual.FinalAnswer, originalPrompt, expectedResponseFormat)
	if err != nil {
		return result, fmt.Errorf("failed to create judge prompt: %w", err)
	}

	judgeTask := config.Task{
		Name:                 judgeTaskName,
		Prompt:               prompt,
		ResponseResultFormat: "0 (incorrect) or 1 (correct)",
		ExpectedResult:       utils.NewStringSet("1"),
	}

	// Execute the judge task and evaluate the response.
	judgeTaskResult, err := v.executeJudgeTask(ctx, judgeTask)
	if err != nil {
		return result, fmt.Errorf("judge evaluation failed: %w", err)
	}

	validationResult, err := NewValueMatchValidator().IsCorrect(ctx, config.ValidationRules{}, judgeTask.ExpectedResult, judgeTaskResult, judgeTask.Prompt, judgeTask.ResponseResultFormat)
	if err != nil {
		return result, fmt.Errorf("failed to evaluate judge response: %w", err)
	}

	var explanation string
	if validationResult.IsCorrect {
		explanation = fmt.Sprintf("Response is semantically equivalent to one of the accepted answers.\n\nJudge reasoning:\n%s", judgeTaskResult.Explanation)
	} else {
		explanation = fmt.Sprintf("Response is not semantically equivalent to any of the accepted answers.\n\nActual response:\n%s\n\nJudge reasoning:\n%s", actual.FinalAnswer, judgeTaskResult.Explanation)
	}

	return ValidationResult{
		IsCorrect:   validationResult.IsCorrect,
		Title:       "Semantic Assessment",
		Explanation: explanation,
		assessment:  &judgeTaskResult,
	}, nil
}

func (v *judgeValidator) ToCanonical(_ config.ValidationRules, value string) string {
	// For judge validator, we only trim whitespace to preserve the original model output.
	return strings.TrimSpace(value)
}

func (v *judgeValidator) GetName() string {
	return fmt.Sprintf("%s (%s) judge", v.judge.Name(), v.judgeRunVariant.Name)
}

func (v *judgeValidator) Close(ctx context.Context) error {
	if v.judge != nil {
		return v.judge.Close(ctx)
	}
	return nil
}

func (v *judgeValidator) executeJudgeTask(ctx context.Context, task config.Task) (providers.Result, error) {
	if v.judgeRunVariant.RetryPolicy != nil && v.judgeRunVariant.RetryPolicy.MaxRetryAttempts > 0 { // check if retry is enabled
		backoff := retry.NewExponential(time.Duration(v.judgeRunVariant.RetryPolicy.InitialDelaySeconds) * time.Second)
		backoff = retry.WithMaxRetries(uint64(v.judgeRunVariant.RetryPolicy.MaxRetryAttempts), backoff)

		return retry.DoValue(ctx, backoff, func(ctx context.Context) (result providers.Result, err error) {
			if err := ctx.Err(); err != nil { // canceled or timed out
				return result, err
			}

			if v.limiter != nil {
				if err := v.limiter.Wait(ctx); err != nil {
					return result, err
				}
			}

			result, err = v.judge.Run(ctx, v.judgeRunVariant, task)
			if errors.Is(err, providers.ErrRetryable) {
				return result, retry.RetryableError(err)
			}
			return result, err
		})
	} else {
		return v.judge.Run(ctx, v.judgeRunVariant, task) // no retries enabled, run only once
	}
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
3. If ANY match, the response is correct; else incorrect.`))

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
