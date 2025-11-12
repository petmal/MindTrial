//go:build !test

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

// NewProvider creates a new AI model provider based on the given configuration.
// It returns an error if the provider name is unknown or initialization fails.
func NewProvider(ctx context.Context, cfg config.ProviderConfig, availableTools []config.ToolConfig) (Provider, error) {
	switch cfg.Name {
	case config.OPENAI:
		return NewOpenAI(cfg.ClientConfig.(config.OpenAIClientConfig), availableTools), nil
	case config.GOOGLE:
		return NewGoogleAI(ctx, cfg.ClientConfig.(config.GoogleAIClientConfig), availableTools)
	case config.ANTHROPIC:
		return NewAnthropic(cfg.ClientConfig.(config.AnthropicClientConfig), availableTools), nil
	case config.DEEPSEEK:
		return NewDeepseek(cfg.ClientConfig.(config.DeepseekClientConfig), availableTools)
	case config.MISTRALAI:
		return NewMistralAI(cfg.ClientConfig.(config.MistralAIClientConfig), availableTools)
	case config.XAI:
		return NewXAI(cfg.ClientConfig.(config.XAIClientConfig), availableTools)
	case config.ALIBABA:
		return NewAlibaba(cfg.ClientConfig.(config.AlibabaClientConfig), availableTools), nil
	case config.MOONSHOTAI:
		return NewMoonshotAI(cfg.ClientConfig.(config.MoonshotAIClientConfig), availableTools), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownProviderName, cfg.Name)
}
