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

	"github.com/petmal/mindtrial/config"
)

type MockProvider struct {
	name string
}

func (m MockProvider) Name() string {
	return m.name
}

func (m MockProvider) Validator(expected string) Validator {
	return NewDefaultValidator(expected)
}

func (m *MockProvider) Run(ctx context.Context, cfg config.RunConfig, task config.Task) (result Result, err error) {
	result = Result{
		Title: task.Name,
	}

	if cfg.Name == "pass" {
		result.Explanation = "mock pass"
		result.FinalAnswer = task.ExpectedResult
	} else {
		switch task.Name {
		case "error":
			return result, fmt.Errorf("mock error")
		case "failure":
			result.Explanation = "mock failure"
			result.FinalAnswer = "Facere aperiam recusandae totam magnam nulla corrupti."
		default:
			result.Explanation = "mock success"
			result.FinalAnswer = task.ExpectedResult
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
