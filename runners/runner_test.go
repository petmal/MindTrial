// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package runners

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerRun(t *testing.T) {
	expectedUsage := TokenUsage{
		InputTokens:  testutils.Ptr(int64(8200209999917998)),
		OutputTokens: nil,
	}

	type args struct {
		ctx   context.Context
		tasks []config.Task
	}
	tests := []struct {
		name    string
		r       Runner
		args    args
		want    Results
		wantErr bool
	}{
		{
			name: "test results states",
			r:    createMockRunner(t),
			args: args{
				context.Background(),
				[]config.Task{
					{
						Name:           "success",
						ExpectedResult: utils.NewValueSet("Provident quas tenetur repellat deserunt ut neque culpa."),
					},
					{
						Name:           "failure",
						ExpectedResult: utils.NewValueSet("Aperiam assumenda id provident ratione eos molestiae."),
					},
					{
						Name:           "error",
						ExpectedResult: utils.NewValueSet("Doloribus quis incidunt velit quia."),
					},
					{
						Name:           "failure",
						ExpectedResult: utils.NewValueSet("Veritatis aliquid accusantium dolore voluptate optio dolor."),
					},
					{
						Name:           "success",
						ExpectedResult: utils.NewValueSet("Omnis omnis ea quia et ut est."),
					},
					{
						Name:           "not_supported",
						ExpectedResult: utils.NewValueSet("Unde accusantium sit et enim temporibus qui distinctio assumenda."),
					},
					{
						Name:           "failure",
						ExpectedResult: utils.NewValueSet("rerum nam illo", "dolore praesentium non"),
						ValidationRules: &config.ValidationRules{
							Judge: config.JudgeSelector{
								Enabled: testutils.Ptr(true),
								Name:    testutils.Ptr("test-judge"),
								Variant: testutils.Ptr("judge_evaluation"),
							},
						},
					},
					{
						Name:           "success",
						ExpectedResult: utils.NewValueSet("corporis et ipsa", "nesciunt sed quia"),
						ValidationRules: &config.ValidationRules{
							Judge: config.JudgeSelector{
								Enabled: testutils.Ptr(true),
								Name:    testutils.Ptr("test-judge"),
								Variant: testutils.Ptr("judge_evaluation"),
							},
						},
					},
				},
			},
			want: Results{
				"mock provider 1": []RunResult{
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
						Want:     utils.NewValueSet("provident quas tenetur repellat deserunt ut neque culpa."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock success"},
								ActualAnswer:   []string{"Provident quas tenetur repellat deserunt ut neque culpa."},
								ExpectedAnswer: [][]string{{"Provident quas tenetur repellat deserunt ut neque culpa."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Failure,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "facere aperiam recusandae totam magnam nulla corrupti.",
						Want:     utils.NewValueSet("aperiam assumenda id provident ratione eos molestiae."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock failure"},
								ActualAnswer:   []string{"Facere aperiam recusandae totam magnam nulla corrupti."},
								ExpectedAnswer: [][]string{{"Aperiam assumenda id provident ratione eos molestiae."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response does not match any of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Error,
						Task:     "error",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "mock error",
						Want:     utils.NewValueSet("doloribus quis incidunt velit quia."),
						Details: Details{
							Answer:     AnswerDetails{},
							Validation: ValidationDetails{},
							Error: ErrorDetails{
								Title:     "Execution Error",
								Message:   "mock error",
								Usage:     expectedUsage,
								ToolUsage: map[string]ToolUsage{},
							},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Failure,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "facere aperiam recusandae totam magnam nulla corrupti.",
						Want:     utils.NewValueSet("veritatis aliquid accusantium dolore voluptate optio dolor."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock failure"},
								ActualAnswer:   []string{"Facere aperiam recusandae totam magnam nulla corrupti."},
								ExpectedAnswer: [][]string{{"Veritatis aliquid accusantium dolore voluptate optio dolor."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response does not match any of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "omnis omnis ea quia et ut est.",
						Want:     utils.NewValueSet("omnis omnis ea quia et ut est."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock success"},
								ActualAnswer:   []string{"Omnis omnis ea quia et ut est."},
								ExpectedAnswer: [][]string{{"Omnis omnis ea quia et ut est."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     NotSupported,
						Task:     "not_supported",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "feature not supported by provider: mock not supported",
						Want:     utils.NewValueSet("unde accusantium sit et enim temporibus qui distinctio assumenda."),
						Details: Details{
							Answer:     AnswerDetails{},
							Validation: ValidationDetails{},
							Error: ErrorDetails{
								Title:     "Feature Not Supported",
								Message:   "feature not supported by provider: mock not supported",
								Usage:     expectedUsage,
								ToolUsage: map[string]ToolUsage{},
							},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Failure,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "Facere aperiam recusandae totam magnam nulla corrupti.",
						Want:     utils.NewValueSet("rerum nam illo", "dolore praesentium non"),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock failure"},
								ActualAnswer:   []string{"Facere aperiam recusandae totam magnam nulla corrupti."},
								ExpectedAnswer: [][]string{{"rerum nam illo"}, {"dolore praesentium non"}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Semantic Assessment",
								Explanation: []string{"Response is not semantically equivalent to any of the accepted answers.", "", "Judge reasoning:", "mock success"},
								Usage:       expectedUsage,
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "mock",
						Got:      "corporis et ipsa",
						Want:     utils.NewValueSet("corporis et ipsa", "nesciunt sed quia"),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock success"},
								ActualAnswer:   []string{"corporis et ipsa"},
								ExpectedAnswer: [][]string{{"corporis et ipsa"}, {"nesciunt sed quia"}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Semantic Assessment",
								Explanation: []string{"Response is semantically equivalent to one of the accepted answers.", "", "Judge reasoning:", "mock success"},
								Usage:       expectedUsage,
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
						Want:     utils.NewValueSet("provident quas tenetur repellat deserunt ut neque culpa."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Provident quas tenetur repellat deserunt ut neque culpa."},
								ExpectedAnswer: [][]string{{"Provident quas tenetur repellat deserunt ut neque culpa."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "aperiam assumenda id provident ratione eos molestiae.",
						Want:     utils.NewValueSet("aperiam assumenda id provident ratione eos molestiae."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Aperiam assumenda id provident ratione eos molestiae."},
								ExpectedAnswer: [][]string{{"Aperiam assumenda id provident ratione eos molestiae."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "error",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "doloribus quis incidunt velit quia.",
						Want:     utils.NewValueSet("doloribus quis incidunt velit quia."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "error",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Doloribus quis incidunt velit quia."},
								ExpectedAnswer: [][]string{{"Doloribus quis incidunt velit quia."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "veritatis aliquid accusantium dolore voluptate optio dolor.",
						Want:     utils.NewValueSet("veritatis aliquid accusantium dolore voluptate optio dolor."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Veritatis aliquid accusantium dolore voluptate optio dolor."},
								ExpectedAnswer: [][]string{{"Veritatis aliquid accusantium dolore voluptate optio dolor."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "omnis omnis ea quia et ut est.",
						Want:     utils.NewValueSet("omnis omnis ea quia et ut est."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Omnis omnis ea quia et ut est."},
								ExpectedAnswer: [][]string{{"Omnis omnis ea quia et ut est."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "not_supported",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "unde accusantium sit et enim temporibus qui distinctio assumenda.",
						Want:     utils.NewValueSet("unde accusantium sit et enim temporibus qui distinctio assumenda."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "not_supported",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Unde accusantium sit et enim temporibus qui distinctio assumenda."},
								ExpectedAnswer: [][]string{{"Unde accusantium sit et enim temporibus qui distinctio assumenda."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "rerum nam illo",
						Want:     utils.NewValueSet("rerum nam illo", "dolore praesentium non"),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"rerum nam illo"},
								ExpectedAnswer: [][]string{{"rerum nam illo"}, {"dolore praesentium non"}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Semantic Assessment",
								Explanation: []string{"Response is semantically equivalent to one of the accepted answers.", "", "Judge reasoning:", "mock success"},
								Usage:       expectedUsage,
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "corporis et ipsa",
						Want:     utils.NewValueSet("corporis et ipsa", "nesciunt sed quia"),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"corporis et ipsa"},
								ExpectedAnswer: [][]string{{"corporis et ipsa"}, {"nesciunt sed quia"}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Semantic Assessment",
								Explanation: []string{"Response is semantically equivalent to one of the accepted answers.", "", "Judge reasoning:", "mock success"},
								Usage:       expectedUsage,
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
				},
				"mock provider 2": []RunResult{
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
						Want:     utils.NewValueSet("provident quas tenetur repellat deserunt ut neque culpa."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Provident quas tenetur repellat deserunt ut neque culpa."},
								ExpectedAnswer: [][]string{{"Provident quas tenetur repellat deserunt ut neque culpa."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "aperiam assumenda id provident ratione eos molestiae.",
						Want:     utils.NewValueSet("aperiam assumenda id provident ratione eos molestiae."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Aperiam assumenda id provident ratione eos molestiae."},
								ExpectedAnswer: [][]string{{"Aperiam assumenda id provident ratione eos molestiae."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "error",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "doloribus quis incidunt velit quia.",
						Want:     utils.NewValueSet("doloribus quis incidunt velit quia."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "error",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Doloribus quis incidunt velit quia."},
								ExpectedAnswer: [][]string{{"Doloribus quis incidunt velit quia."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "veritatis aliquid accusantium dolore voluptate optio dolor.",
						Want:     utils.NewValueSet("veritatis aliquid accusantium dolore voluptate optio dolor."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Veritatis aliquid accusantium dolore voluptate optio dolor."},
								ExpectedAnswer: [][]string{{"Veritatis aliquid accusantium dolore voluptate optio dolor."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "omnis omnis ea quia et ut est.",
						Want:     utils.NewValueSet("omnis omnis ea quia et ut est."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Omnis omnis ea quia et ut est."},
								ExpectedAnswer: [][]string{{"Omnis omnis ea quia et ut est."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "not_supported",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "unde accusantium sit et enim temporibus qui distinctio assumenda.",
						Want:     utils.NewValueSet("unde accusantium sit et enim temporibus qui distinctio assumenda."),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "not_supported",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"Unde accusantium sit et enim temporibus qui distinctio assumenda."},
								ExpectedAnswer: [][]string{{"Unde accusantium sit et enim temporibus qui distinctio assumenda."}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Response Assessment",
								Explanation: []string{"Response matches one of the accepted answers."},
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "rerum nam illo",
						Want:     utils.NewValueSet("rerum nam illo", "dolore praesentium non"),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "failure",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"rerum nam illo"},
								ExpectedAnswer: [][]string{{"rerum nam illo"}, {"dolore praesentium non"}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Semantic Assessment",
								Explanation: []string{"Response is semantically equivalent to one of the accepted answers.", "", "Judge reasoning:", "mock success"},
								Usage:       expectedUsage,
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "corporis et ipsa",
						Want:     utils.NewValueSet("corporis et ipsa", "nesciunt sed quia"),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "success",
								Explanation:    []string{"mock pass"},
								ActualAnswer:   []string{"corporis et ipsa"},
								ExpectedAnswer: [][]string{{"corporis et ipsa"}, {"nesciunt sed quia"}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{
								Title:       "Semantic Assessment",
								Explanation: []string{"Response is semantically equivalent to one of the accepted answers.", "", "Judge reasoning:", "mock success"},
								Usage:       expectedUsage,
								ToolUsage:   map[string]ToolUsage{},
							},
							Error: ErrorDetails{},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test judge evaluation error",
			r: createMockRunnerFromConfig(t, []config.ProviderConfig{
				{
					Name: "mock provider 1",
					Runs: []config.RunConfig{
						{
							Name:  "custom", // uses task name as final answer from mock run
							Model: "custom-model",
						},
					},
				},
			}, []config.JudgeConfig{
				{
					Name: "test-judge",
					Provider: config.ProviderConfig{
						Name: "mock",
						Runs: []config.RunConfig{
							{
								Name:  "judge_evaluation",
								Model: "judge-model-default",
							},
						},
					},
				},
			}, nil, zerolog.Nop()),
			args: args{
				context.Background(),
				[]config.Task{
					{
						Name:           "error", // returned as final answer, causing judge to fail
						ExpectedResult: utils.NewValueSet("Expected answer"),
						ValidationRules: &config.ValidationRules{
							Judge: config.JudgeSelector{
								Enabled: testutils.Ptr(true),
								Name:    testutils.Ptr("test-judge"),
								Variant: testutils.Ptr("judge_evaluation"),
							},
						},
					},
				},
			},
			want: Results{
				"mock provider 1": []RunResult{
					{
						Kind:     Error,
						Task:     "error",
						Provider: "mock provider 1",
						Run:      "custom",
						Got:      "error", // provider returns task name ("error") as response
						Want:     utils.NewValueSet("Expected answer"),
						Details: Details{
							Answer: AnswerDetails{
								Title:          "error",
								Explanation:    []string{},
								ActualAnswer:   []string{"error"},
								ExpectedAnswer: [][]string{{"Expected answer"}},
								Usage:          expectedUsage,
								ToolUsage:      map[string]ToolUsage{},
							},
							Validation: ValidationDetails{},
							Error: ErrorDetails{
								Title:     "Validation Error",
								Message:   "judge evaluation failed: mock error",
								Usage:     expectedUsage,
								ToolUsage: map[string]ToolUsage{},
							},
						},
						Duration: 7211609999927884 * time.Nanosecond,
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.args.tasks {
				if err := tt.args.tasks[i].ResolveValidationRules(config.ValidationRules{}); err != nil {
					t.Fatalf("failed to resolve validation rules: %v", err)
				}
			}

			got, err := tt.r.Run(tt.args.ctx, tt.args.tasks)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Clear random TraceID from results before comparison.
				results := got.GetResults()
				for provider := range results {
					for i := range results[provider] {
						assert.NotEmpty(t, results[provider][i].TraceID, "TraceID should not be empty")
						results[provider][i].TraceID = ""
					}
				}

				assert.Equal(t, tt.want, results)
			}
		})
	}
}

func TestRunnerRunWithRetry(t *testing.T) {
	tests := []struct {
		name              string
		maxRetryAttempts  uint
		taskName          string
		expectedKind      ResultKind
		expectedGot       string
		expectedInDetails string
	}{
		{
			name:              "retry succeeds within max attempts",
			maxRetryAttempts:  uint(4),
			taskName:          "retry_2",
			expectedKind:      Success,
			expectedGot:       "provident quas tenetur repellat deserunt ut neque culpa.",
			expectedInDetails: "mock success after 3 attempts",
		},
		{
			name:              "retry exhausted - max attempts reached",
			maxRetryAttempts:  uint(2),
			taskName:          "retry_5",
			expectedKind:      Error,
			expectedGot:       "failed to generate response: retryable error: mock transient error (retry 2)",
			expectedInDetails: "mock transient error (retry 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := createMockRunnerFromConfig(t, []config.ProviderConfig{
				{
					Name: "mock provider",
					Runs: []config.RunConfig{
						{
							Name: "mock",
							RetryPolicy: &config.RetryPolicy{
								MaxRetryAttempts:    tt.maxRetryAttempts,
								InitialDelaySeconds: 1,
							},
						},
					},
				},
			}, []config.JudgeConfig{}, nil, zerolog.New(zerolog.NewTestWriter(t)))

			tasks := []config.Task{
				{
					Name:           tt.taskName,
					ExpectedResult: utils.NewValueSet("Provident quas tenetur repellat deserunt ut neque culpa."),
				},
			}

			got, err := mockRunner.Run(context.Background(), tasks)
			require.NoError(t, err)

			results := got.GetResults()
			require.Len(t, results, 1)
			require.Contains(t, results, "mock provider")
			require.Len(t, results["mock provider"], 1)

			result := results["mock provider"][0]
			assert.Equal(t, "mock provider", result.Provider)
			assert.Equal(t, "mock", result.Run)
			assert.Equal(t, tt.taskName, result.Task)
			assert.Equal(t, tt.expectedKind, result.Kind)
			assert.Equal(t, tt.expectedGot, result.Got)

			switch tt.expectedKind {
			case Success:
				assert.NotZero(t, result.Details.Answer, "Success should have Answer details")
				assert.NotZero(t, result.Details.Validation, "Success should have Validation details")
				assert.Zero(t, result.Details.Error, "Success should not have Error details")
				assert.Contains(t, result.Details.Answer.Explanation, tt.expectedInDetails)
			case Error:
				assert.Zero(t, result.Details.Answer, "Error should not have Answer details")
				assert.Zero(t, result.Details.Validation, "Error should not have Validation details")
				assert.NotZero(t, result.Details.Error, "Error should have Error details")
				assert.Contains(t, result.Details.Error.Message, tt.expectedInDetails)
			}
		})
	}
}

func TestRunnerRunWithTools(t *testing.T) {
	// Define tools for the test.
	tools := []config.ToolConfig{
		{Name: "tool1"},
		{Name: "tool2"},
	}

	// Create runner with tools using createMockRunnerFromConfig.
	runner := createMockRunnerFromConfig(t, []config.ProviderConfig{
		{
			Name: "mock provider 1",
			Runs: []config.RunConfig{
				{Name: "mock", Model: "test-model"},
			},
		},
	}, []config.JudgeConfig{}, tools, zerolog.Nop())

	// Helper function to verify tool usage.
	verifyToolUsage := func(t *testing.T, toolUsage map[string]ToolUsage) {
		require.NotNil(t, toolUsage, "ToolUsage should not be nil")
		assert.Len(t, toolUsage, 2, "Should have usage for 2 tools")

		for _, tool := range tools {
			tu, exists := toolUsage[tool.Name]
			assert.True(t, exists, "Tool %s should have usage", tool.Name)
			assert.Equal(t, int64(2), *tu.CallCount, "CallCount should be 2 for tool %s", tool.Name)
			expectedDuration := 150 * time.Millisecond
			assert.Equal(t, &expectedDuration, tu.TotalDuration, "TotalDuration should be 150ms for tool %s", tool.Name)
		}
	}

	// Helper function to verify token usage.
	verifyTokenUsage := func(t *testing.T, usage TokenUsage) {
		expectedInputTokens := int64(8200209999917998)
		assert.Equal(t, &expectedInputTokens, usage.InputTokens, "InputTokens should match")
		assert.Nil(t, usage.OutputTokens, "OutputTokens should be nil")
	}

	tests := []struct {
		name           string
		task           config.Task
		wantResultKind ResultKind
	}{
		{
			name: "success with tools",
			task: config.Task{
				Name:           "test_task_with_tools",
				ExpectedResult: utils.NewValueSet("test_task_with_tools"), // mock returns task name as answer
			},
			wantResultKind: Success,
		},
		{
			name: "failure with tools",
			task: config.Task{
				Name:           "failure",
				ExpectedResult: utils.NewValueSet("different_expected"), // won't match the mock failure response
			},
			wantResultKind: Failure,
		},
		{
			name: "error with tools",
			task: config.Task{
				Name:           "error",
				ExpectedResult: utils.NewValueSet("some_expected"),
			},
			wantResultKind: Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run the task.
			results, err := runner.Run(context.Background(), []config.Task{tt.task})
			require.NoError(t, err)

			// Verify results.
			allResults := results.GetResults()
			require.Contains(t, allResults, "mock provider 1")

			providerResults := allResults["mock provider 1"]
			require.Len(t, providerResults, 1, "Should have exactly one result")

			result := providerResults[0]
			assert.Equal(t, tt.wantResultKind, result.Kind, "Result kind should match expected")
			assert.Equal(t, tt.task.Name, result.Task, "Task name should match")
			assert.Equal(t, "mock", result.Run, "Should use mock run")

			// Check usage in appropriate details based on result kind.
			switch tt.wantResultKind {
			case Success, Failure:
				verifyTokenUsage(t, result.Details.Answer.Usage)
				verifyToolUsage(t, result.Details.Answer.ToolUsage)
			case Error:
				verifyTokenUsage(t, result.Details.Error.Usage)
				verifyToolUsage(t, result.Details.Error.ToolUsage)
			}
		})
	}
}

func createMockRunner(t *testing.T) Runner {
	return createMockRunnerFromConfig(t, []config.ProviderConfig{
		{
			Name: "mock provider 1",
			Runs: []config.RunConfig{
				{
					Name:                 "mock",
					Model:                "microchip",
					MaxRequestsPerMinute: 50,
				},
				{
					Name:  "pass",
					Model: "parsing",
				},
			},
		},
		{
			Name: "mock provider 2",
			Runs: []config.RunConfig{
				{
					Name:  "pass",
					Model: "parsing",
				},
			},
		},
	}, []config.JudgeConfig{
		{
			Name: "test-judge",
			Provider: config.ProviderConfig{
				Name: "mock",
				Runs: []config.RunConfig{
					{
						Name:  "judge_evaluation",
						Model: "judge-model-default",
					},
				},
			},
		},
	}, nil, zerolog.Nop())
}

func createMockRunnerFromConfig(t *testing.T, cfg []config.ProviderConfig, judges []config.JudgeConfig, tools []config.ToolConfig, logger zerolog.Logger) Runner {
	runner, err := NewDefaultRunner(context.Background(), cfg, judges, tools, logger)
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}

	return runner
}

type stubToolValidator struct {
	validatedTools []string
	validateErr    error
	closed         bool
}

func (s *stubToolValidator) ValidateTool(ctx context.Context, cfg config.ToolConfig) error {
	s.validatedTools = append(s.validatedTools, cfg.Name)
	if s.validateErr != nil {
		return s.validateErr
	}
	return nil
}

func (s *stubToolValidator) Close() error {
	s.closed = true
	return nil
}

func TestDefaultRunnerAssertCanRun(t *testing.T) {
	tests := []struct {
		name          string
		tools         []config.ToolConfig
		taskToolNames []string
		validateErr   error
		expectedTools []string
		expectedError string
		wantErr       bool
	}{
		{
			name: "validates single tool",
			tools: []config.ToolConfig{
				{Name: "echo", Image: "alpine:latest"},
				{Name: "cat", Image: "linux:latest"},
			},
			taskToolNames: []string{"echo"},
			expectedTools: []string{"echo"},
			wantErr:       false,
		},
		{
			name: "validates multiple tools",
			tools: []config.ToolConfig{
				{Name: "echo", Image: "alpine:latest"},
				{Name: "cat", Image: "linux:latest"},
			},
			taskToolNames: []string{"echo", "cat"},
			expectedTools: []string{"echo", "cat"},
			wantErr:       false,
		},
		{
			name: "deduplicates tools - validates each tool once",
			tools: []config.ToolConfig{
				{Name: "echo", Image: "alpine:latest"},
				{Name: "cat", Image: "alpine:latest"},
			},
			taskToolNames: []string{"echo", "cat"},
			expectedTools: []string{"echo", "cat"},
			wantErr:       false,
		},
		{
			name: "reports validation failure",
			tools: []config.ToolConfig{
				{Name: "echo", Image: "alpine:latest"},
			},
			taskToolNames: []string{"echo"},
			validateErr:   errors.New("docker image missing"), //nolint:err113
			expectedTools: []string{"echo"},
			expectedError: "could not start because:\ntool 'echo' cannot be used: docker image missing",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			stub := &stubToolValidator{validateErr: tt.validateErr}

			runner := &defaultRunner{
				validatorFactory: validators.NewFactory(nil),
				tools:            tt.tools,
				toolValidator:    stub,
			}

			task := newToolTask(t, tt.taskToolNames...)

			err := runner.assertCanRun(ctx, []config.Task{task})
			if tt.wantErr {
				require.Error(t, err)
				require.EqualError(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, tt.expectedTools, stub.validatedTools)
			assert.False(t, stub.closed)

			runner.Close(ctx)
			assert.True(t, stub.closed)
		})
	}
}

func newToolTask(t *testing.T, toolNames ...string) config.Task {
	t.Helper()

	toolSelections := make([]config.ToolSelection, len(toolNames))
	for i, name := range toolNames {
		toolSelections[i] = config.ToolSelection{Name: name}
	}

	task := config.Task{
		Name:                 "tool-task",
		Prompt:               "use tool",
		ResponseResultFormat: config.NewResponseFormat("text"),
		ExpectedResult:       utils.NewValueSet("expected"),
		ToolSelector: &config.ToolSelector{
			Tools: toolSelections,
		},
	}

	require.NoError(t, task.ResolveValidationRules(config.ValidationRules{}))
	task.ResolveToolSelector(config.ToolSelector{})

	return task
}

func TestProviderResultsByRunAndKind(t *testing.T) {
	mockResults := Results{
		"mock provider 1": []RunResult{
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 1",
				Run:      "p1r1",
				Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
				Details: Details{
					Answer: AnswerDetails{
						Title:       "success",
						Explanation: []string{"mock success"},
					},
					Validation: ValidationDetails{
						Title:       "validation success",
						Explanation: []string{"mock validation pass"},
						ToolUsage:   map[string]ToolUsage{},
					},
					Error: ErrorDetails{},
				},
			},
			{
				Kind:     Failure,
				Task:     "failure",
				Provider: "mock provider 1",
				Run:      "p1r1",
				Got:      "aperiam assumenda id provident ratione eos molestiae.",
				Details: Details{
					Answer: AnswerDetails{
						Title:       "success",
						Explanation: []string{"mock success"},
					},
					Validation: ValidationDetails{
						Title:       "validation failed",
						Explanation: []string{"mock validation fail"},
						ToolUsage:   map[string]ToolUsage{},
					},
					Error: ErrorDetails{},
				},
			},
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 1",
				Run:      "p1r1",
				Got:      "autem aspernatur pariatur iure accusamus.",
				Details: Details{
					Answer: AnswerDetails{
						Title:       "success",
						Explanation: []string{"mock success"},
					},
					Validation: ValidationDetails{
						Title:       "validation success",
						Explanation: []string{"mock validation pass"},
						ToolUsage:   map[string]ToolUsage{},
					},
					Error: ErrorDetails{},
				},
			},
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 1",
				Run:      "p1r2",
				Got:      "provident aperiam quaerat.",
				Details: Details{
					Answer: AnswerDetails{
						Title:       "success",
						Explanation: []string{"mock success"},
					},
					Validation: ValidationDetails{
						Title:       "validation success",
						Explanation: []string{"mock validation pass"},
						ToolUsage:   map[string]ToolUsage{},
					},
					Error: ErrorDetails{},
				},
			},
		},
		"mock provider 2": []RunResult{
			{
				Kind:     Error,
				Task:     "error",
				Provider: "mock provider 2",
				Run:      "p2r1",
				Got:      "mock error",
				Details: Details{
					Answer:     AnswerDetails{},
					Validation: ValidationDetails{},
					Error: ErrorDetails{
						Title:     "error",
						Message:   "mock error",
						ToolUsage: map[string]ToolUsage{},
					},
				},
			},
			{
				Kind:     Failure,
				Task:     "failure",
				Provider: "mock provider 2",
				Run:      "p2r1",
				Got:      "saepe aperiam culpa voluptatem est.",
				Details: Details{
					Answer: AnswerDetails{
						Title:       "success",
						Explanation: []string{"mock success"},
					},
					Validation: ValidationDetails{
						Title:       "validation failed",
						Explanation: []string{"mock validation fail"},
						ToolUsage:   map[string]ToolUsage{},
					},
					Error: ErrorDetails{},
				},
			},
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 2",
				Run:      "p2r1",
				Got:      "aliquam nesciunt et laboriosam.",
				Details: Details{
					Answer: AnswerDetails{
						Title:       "success",
						Explanation: []string{"mock success"},
					},
					Validation: ValidationDetails{
						Title:       "validation success",
						Explanation: []string{"mock validation pass"},
						ToolUsage:   map[string]ToolUsage{},
					},
					Error: ErrorDetails{},
				},
			},
			{
				Kind:     NotSupported,
				Task:     "not_supported",
				Provider: "mock provider 2",
				Run:      "p2r1",
				Got:      "feature not supported by provider: mock not supported",
				Details: Details{
					Answer:     AnswerDetails{},
					Validation: ValidationDetails{},
					Error: ErrorDetails{
						Title:     "not_supported",
						Message:   "mock not supported",
						ToolUsage: map[string]ToolUsage{},
					},
				},
			},
		},
		"mock provider 3": []RunResult{
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 3",
				Run:      "p3r2",
				Got:      "consectetur doloremque sit quibusdam.",
				Details: Details{
					Answer: AnswerDetails{
						Title:       "success",
						Explanation: []string{"mock success"},
					},
					Validation: ValidationDetails{
						Title:       "validation success",
						Explanation: []string{"mock validation pass"},
						ToolUsage:   map[string]ToolUsage{},
					},
					Error: ErrorDetails{},
				},
			},
		},
		"mock provider 4": []RunResult{},
	}
	type args struct {
		provider string
	}
	tests := []struct {
		name string
		r    Results
		args args
		want map[string]map[ResultKind][]RunResult
	}{
		{
			name: "multiple runs, multiple results",
			r:    mockResults,
			args: args{
				provider: "mock provider 1",
			},
			want: map[string]map[ResultKind][]RunResult{
				"p1r1": {
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 1",
							Run:      "p1r1",
							Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
							Details: Details{
								Answer: AnswerDetails{
									Title:       "success",
									Explanation: []string{"mock success"},
								},
								Validation: ValidationDetails{
									Title:       "validation success",
									Explanation: []string{"mock validation pass"},
									ToolUsage:   map[string]ToolUsage{},
								},
								Error: ErrorDetails{},
							},
						},
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 1",
							Run:      "p1r1",
							Got:      "autem aspernatur pariatur iure accusamus.",
							Details: Details{
								Answer: AnswerDetails{
									Title:       "success",
									Explanation: []string{"mock success"},
								},
								Validation: ValidationDetails{
									Title:       "validation success",
									Explanation: []string{"mock validation pass"},
									ToolUsage:   map[string]ToolUsage{},
								},
								Error: ErrorDetails{},
							},
						},
					},
					Failure: {
						{
							Kind:     Failure,
							Task:     "failure",
							Provider: "mock provider 1",
							Run:      "p1r1",
							Got:      "aperiam assumenda id provident ratione eos molestiae.",
							Details: Details{
								Answer: AnswerDetails{
									Title:       "success",
									Explanation: []string{"mock success"},
								},
								Validation: ValidationDetails{
									Title:       "validation failed",
									Explanation: []string{"mock validation fail"},
									ToolUsage:   map[string]ToolUsage{},
								},
								Error: ErrorDetails{},
							},
						},
					},
				},
				"p1r2": {
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 1",
							Run:      "p1r2",
							Got:      "provident aperiam quaerat.",
							Details: Details{
								Answer: AnswerDetails{
									Title:       "success",
									Explanation: []string{"mock success"},
								},
								Validation: ValidationDetails{
									Title:       "validation success",
									Explanation: []string{"mock validation pass"},
									ToolUsage:   map[string]ToolUsage{},
								},
								Error: ErrorDetails{},
							},
						},
					},
				},
			},
		},
		{
			name: "single run, multiple results",
			r:    mockResults,
			args: args{
				provider: "mock provider 2",
			},
			want: map[string]map[ResultKind][]RunResult{
				"p2r1": {
					Error: {
						{
							Kind:     Error,
							Task:     "error",
							Provider: "mock provider 2",
							Run:      "p2r1",
							Got:      "mock error",
							Details: Details{
								Answer:     AnswerDetails{},
								Validation: ValidationDetails{},
								Error: ErrorDetails{
									Title:     "error",
									Message:   "mock error",
									ToolUsage: map[string]ToolUsage{},
								},
							},
						},
					},
					Failure: {
						{
							Kind:     Failure,
							Task:     "failure",
							Provider: "mock provider 2",
							Run:      "p2r1",
							Got:      "saepe aperiam culpa voluptatem est.",
							Details: Details{
								Answer: AnswerDetails{
									Title:       "success",
									Explanation: []string{"mock success"},
								},
								Validation: ValidationDetails{
									Title:       "validation failed",
									Explanation: []string{"mock validation fail"},
									ToolUsage:   map[string]ToolUsage{},
								},
								Error: ErrorDetails{},
							},
						},
					},
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 2",
							Run:      "p2r1",
							Got:      "aliquam nesciunt et laboriosam.",
							Details: Details{
								Answer: AnswerDetails{
									Title:       "success",
									Explanation: []string{"mock success"},
								},
								Validation: ValidationDetails{
									Title:       "validation success",
									Explanation: []string{"mock validation pass"},
									ToolUsage:   map[string]ToolUsage{},
								},
								Error: ErrorDetails{},
							},
						},
					},
					NotSupported: {
						{
							Kind:     NotSupported,
							Task:     "not_supported",
							Provider: "mock provider 2",
							Run:      "p2r1",
							Got:      "feature not supported by provider: mock not supported",
							Details: Details{
								Answer:     AnswerDetails{},
								Validation: ValidationDetails{},
								Error: ErrorDetails{
									Title:     "not_supported",
									Message:   "mock not supported",
									ToolUsage: map[string]ToolUsage{},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "single run, single result",
			r:    mockResults,
			args: args{
				provider: "mock provider 3",
			},
			want: map[string]map[ResultKind][]RunResult{
				"p3r2": {
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 3",
							Run:      "p3r2",
							Got:      "consectetur doloremque sit quibusdam.",
							Details: Details{
								Answer: AnswerDetails{
									Title:       "success",
									Explanation: []string{"mock success"},
								},
								Validation: ValidationDetails{
									Title:       "validation success",
									Explanation: []string{"mock validation pass"},
									ToolUsage:   map[string]ToolUsage{},
								},
								Error: ErrorDetails{},
							},
						},
					},
				},
			},
		},
		{
			name: "no runs, no results",
			r:    mockResults,
			args: args{
				provider: "mock provider 4",
			},
			want: map[string]map[ResultKind][]RunResult{},
		},
		{
			name: "unknown provider",
			r:    mockResults,
			args: args{
				provider: "mock provider 5",
			},
			want: map[string]map[ResultKind][]RunResult{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.ProviderResultsByRunAndKind(tt.args.provider))
		})
	}
}

func TestRunResultGetID(t *testing.T) {
	tests := []struct {
		name      string
		runResult RunResult
		want      string
	}{
		{
			name: "simple case",
			runResult: RunResult{
				Task:     "test-task",
				Provider: "test-provider",
				Run:      "test-run",
			},
			want: "run-test-provider-test-run-test-task",
		},
		{
			name: "with spaces",
			runResult: RunResult{
				Task:     "test task",
				Provider: "test provider",
				Run:      "test run",
			},
			want: "run-test-provider-test-run-test-task",
		},
		{
			name: "with special characters",
			runResult: RunResult{
				Task:     "test!@#$%task",
				Provider: "test&*()provider",
				Run:      "test+=[]{};:'run",
			},
			want: "run-test____provider-test_________run-test_____task",
		},
		{
			name: "with Unicode characters",
			runResult: RunResult{
				Task:     "testλ♥task",
				Provider: "testπøprovider",
				Run:      "test★☆run",
			},
			want: "run-test__provider-test__run-test__task",
		},
		{
			name: "with empty fields",
			runResult: RunResult{
				Task:     "",
				Provider: "",
				Run:      "",
			},
			want: "run---",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.runResult.GetID()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunnerIntegrationWithValidation(t *testing.T) {
	// Default validation rules.
	defaultRules := config.ValidationRules{
		CaseSensitive:    testutils.Ptr(false),
		IgnoreWhitespace: testutils.Ptr(false),
	}

	// Set up runner.
	runner, err := NewDefaultRunner(context.Background(), []config.ProviderConfig{
		{
			Name: "mock provider 1",
			Runs: []config.RunConfig{
				{Name: "custom", Model: "test-model"}, // uses task name as answer
			},
		},
	}, []config.JudgeConfig{}, nil, zerolog.Nop())
	require.NoError(t, err)

	tests := []struct {
		name           string
		task           config.Task
		wantResultKind ResultKind
	}{
		{
			name: "case insensitive match by default",
			task: config.Task{
				Name:           "Hello_World",
				ExpectedResult: utils.NewValueSet("hello_world"), // should match case insensitively
			},
			wantResultKind: Success,
		},
		{
			name: "task rule override - case sensitive causes failure",
			task: config.Task{
				Name:           "Case_Test",
				ExpectedResult: utils.NewValueSet("case_test"), // won't match due to case difference
				ValidationRules: &config.ValidationRules{
					CaseSensitive: testutils.Ptr(true), // override to case sensitive
				},
			},
			wantResultKind: Failure,
		}, {
			name: "task rule override - ignore whitespace enables match",
			task: config.Task{
				Name:           "white space test",                  // task name contains spaces
				ExpectedResult: utils.NewValueSet("whitespacetest"), // expected without spaces
				ValidationRules: &config.ValidationRules{
					IgnoreWhitespace: testutils.Ptr(true), // override to ignore whitespace
				},
			},
			wantResultKind: Success,
		},
		{
			name: "task rule override - whitespace sensitivity causes failure",
			task: config.Task{
				Name:           "spaced out test",                  // task name contains spaces
				ExpectedResult: utils.NewValueSet("spacedouttest"), // expected without spaces
				ValidationRules: &config.ValidationRules{
					IgnoreWhitespace: testutils.Ptr(false), // override to be whitespace sensitive
				},
			},
			wantResultKind: Failure, // should fail because "spaced out test" != "spacedouttest"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.task.ResolveValidationRules(defaultRules); err != nil {
				t.Fatalf("failed to resolve validation rules: %v", err)
			}

			results, err := runner.Run(context.Background(), []config.Task{tt.task})
			require.NoError(t, err)

			allResults := results.GetResults()
			require.Contains(t, allResults, "mock provider 1")

			providerResults := allResults["mock provider 1"]
			require.Len(t, providerResults, 1, "Should have exactly one result")

			result := providerResults[0]
			assert.Equal(t, tt.wantResultKind, result.Kind, "Result kind should match expected")
			assert.Equal(t, "custom", result.Run, "Should use custom run")
			assert.Equal(t, tt.task.Name, result.Task, "Task name should match")
		})
	}
}

func TestToLines(t *testing.T) {
	tests := []struct {
		name string
		set  utils.ValueSet
		want [][]string
	}{
		{
			name: "empty set",
			set:  utils.NewValueSet(),
			want: [][]string{},
		},
		{
			name: "single string",
			set:  utils.NewValueSet("single line"),
			want: [][]string{{"single line"}},
		},
		{
			name: "multiple lines",
			set:  utils.NewValueSet("first line\r\nsecond line\nthird line"),
			want: [][]string{{"first line", "second line", "third line"}},
		},
		{
			name: "double newlines",
			set:  utils.NewValueSet("alpha\n\nbeta", "gamma\r\n\r\ndelta"),
			want: [][]string{{"alpha", "", "beta"}, {"gamma", "", "delta"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toLines(tt.set)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
