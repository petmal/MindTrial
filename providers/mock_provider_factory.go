//go:build test

// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
)

type MockProvider struct {
	name string
}

func (m MockProvider) Name() string {
	return m.name
}

func (m MockProvider) Validator(expected utils.StringSet) Validator {
	return NewDefaultValidator(expected)
}

func (m *MockProvider) Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error) {
	result = Result{
		Title: task.Name,
		prompts: []string{
			"Porro laudantium quam voluptas.",
			"Et magnam velit unde.",
			"Dolore odio esse et esse.",
		},
		usage: Usage{
			InputTokens:  testutils.Ptr(int64(8200209999917998)),
			OutputTokens: nil,
		},
		duration: 7211609999927884 * time.Nanosecond,
	}

	expectedValidAnswers := task.ExpectedResult.Values()
	if cfg.Name == "pass" {
		result.Explanation = "mock pass"
		result.FinalAnswer = expectedValidAnswers[0]
	} else {
		switch task.Name {
		case "error":
			return result, fmt.Errorf("mock error")
		case "not_supported":
			return result, fmt.Errorf("%w: %s", ErrFeatureNotSupported, "mock not supported")
		case "failure":
			result.Explanation = "mock failure"
			result.FinalAnswer = "Facere aperiam recusandae totam magnam nulla corrupti."
		default:
			result.Explanation = "mock success"
			result.FinalAnswer = expectedValidAnswers[0]
		}
	}

	return result, nil
}

func (m *MockProvider) Close(ctx context.Context) error {
	return nil
}

func NewProvider(ctx context.Context, cfg config.ProviderConfig) (Provider, error) {
	return &MockProvider{
		name: cfg.Name,
	}, nil
}
