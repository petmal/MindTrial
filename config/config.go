// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package config contains the data models representing the structure of configuration
// and task definition files for the MindTrial application. It provides configuration management
// and handles loading and validation of application settings, provider configurations,
// and task definitions from YAML files.
package config

import (
	"errors"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// OPENAI identifies the OpenAI provider.
	OPENAI string = "openai"
	// GOOGLE identifies the Google AI provider.
	GOOGLE string = "google"
	// ANTHROPIC identifies the Anthropic provider.
	ANTHROPIC string = "anthropic"
	// DEEPSEEK identifies the Deepseek provider.
	DEEPSEEK string = "deepseek"
)

// ErrInvalidConfigProperty indicates invalid configuration.
var ErrInvalidConfigProperty = errors.New("invalid configuration property")

// Config represents the top-level configuration structure.
type Config struct {
	// Config contains application-wide settings.
	Config AppConfig `yaml:"config" validate:"required"`
}

// AppConfig defines application-wide settings.
type AppConfig struct {
	// LogFile specifies path to the log file.
	LogFile string `yaml:"log-file" validate:"omitempty,filepath"`

	// OutputDir specifies directory where results will be saved.
	OutputDir string `yaml:"output-dir" validate:"required"`

	// OutputBaseName specifies base filename for result files.
	OutputBaseName string `yaml:"output-basename" validate:"omitempty,filepath"`

	// TaskSource specifies path to the task definitions file.
	TaskSource string `yaml:"task-source" validate:"required,filepath"`

	// Providers lists configurations for AI providers whose models will be used
	// to execute tasks during the trial run.
	Providers []ProviderConfig `yaml:"providers" validate:"required,dive"`
}

// GetProvidersWithEnabledRuns returns providers with their enabled run configurations.
// If RunConfig.Disabled is nil, the parent ProviderConfig.Disabled value is used instead.
// Any disabled run configurations are excluded from the results.
// Providers with no enabled run configurations are excluded from the returned list.
func (ac AppConfig) GetProvidersWithEnabledRuns() []ProviderConfig {
	providers := make([]ProviderConfig, 0, len(ac.Providers))
	for _, provider := range ac.Providers {
		enabledRuns := make([]RunConfig, 0, len(provider.Runs))
		for _, run := range provider.Runs {
			if !ResolveFlagOverride(run.Disabled, provider.Disabled) {
				enabledRuns = append(enabledRuns, run)
			}
		}
		if len(enabledRuns) > 0 {
			providers = append(providers, ProviderConfig{
				Name:         provider.Name,
				ClientConfig: provider.ClientConfig,
				Runs:         enabledRuns,
				Disabled:     provider.Disabled,
			})
		}
	}
	return providers
}

// ProviderConfig defines settings for an AI provider.
type ProviderConfig struct {
	// Name specifies unique identifier of the provider.
	Name string `yaml:"name" validate:"required,oneof=openai google anthropic deepseek"`

	// ClientConfig holds provider-specific client settings.
	ClientConfig ClientConfig `yaml:"client-config" validate:"required"`

	// Runs lists test run configurations for this provider.
	Runs []RunConfig `yaml:"runs" validate:"required,dive"`

	// Disabled indicates if all runs should be disabled by default.
	Disabled bool `yaml:"disabled" validate:"omitempty"`
}

// ClientConfig is a marker interface for provider-specific configurations.
type ClientConfig interface{}

// OpenAIClientConfig represents OpenAI provider settings.
type OpenAIClientConfig struct {
	// APIKey is the API key for the OpenAI provider.
	APIKey string `yaml:"api-key" validate:"required"`
}

// GoogleAIClientConfig represents Google AI provider settings.
type GoogleAIClientConfig struct {
	// APIKey is the API key for the Google AI generative models provider.
	APIKey string `yaml:"api-key" validate:"required"`
}

// AnthropicClientConfig represents Anthropic provider settings.
type AnthropicClientConfig struct {
	// APIKey is the API key for the Anthropic generative models provider.
	APIKey string `yaml:"api-key" validate:"required"`
	// RequestTimeout specifies the timeout for API requests.
	RequestTimeout *time.Duration `yaml:"request-timeout" validate:"omitempty"`
}

// DeepseekClientConfig represents Deepseek provider settings.
type DeepseekClientConfig struct {
	// APIKey is the API key for the Deepseek generative models provider.
	APIKey string `yaml:"api-key" validate:"required"`
	// RequestTimeout specifies the timeout for API requests.
	RequestTimeout *time.Duration `yaml:"request-timeout" validate:"omitempty"`
}

// RunConfig defines settings for a single test configuration.
type RunConfig struct {
	// Name is a display-friendly identifier shown in results.
	Name string `yaml:"name" validate:"required"`

	// Model specifies target model's identifier.
	Model string `yaml:"model" validate:"required"`

	// MaxRequestsPerMinute limits the number of API requests per minute sent to this specific model.
	// Value of 0 means no rate limiting will be applied.
	MaxRequestsPerMinute int `yaml:"max-requests-per-minute" validate:"omitempty,numeric,min=0"`

	// Disabled indicates if this run configuration should be skipped.
	// If set, overrides the parent ProviderConfig.Disabled value.
	Disabled *bool `yaml:"disabled" validate:"omitempty"`

	// ModelParams holds any model-specific configuration parameters.
	ModelParams ModelParams `yaml:"model-parameters" validate:"omitempty"`
}

// ModelParams is a marker interface for model-specific parameters.
type ModelParams interface{}

// OpenAIModelParams represents OpenAI model-specific settings.
type OpenAIModelParams struct {
	// ReasoningEffort controls effort level on reasoning for reasoning models.
	// Valid values are: "low", "medium", "high".
	ReasoningEffort *string `yaml:"reasoning-effort" validate:"omitempty,oneof=low medium high"`

	// TextResponseFormat indicates whether to use plain-text response format
	// for compatibility with models that do not support JSON.
	TextResponseFormat bool `yaml:"text-response-format" validate:"omitempty"`
}

// AnthropicModelParams represents Anthropic model-specific settings.
type AnthropicModelParams struct {
	// MaxTokens controls the maximum number of tokens available to the model for generating a response.
	// This includes the thinking budget for reasoning models.
	MaxTokens *int64 `yaml:"max-tokens" validate:"omitempty,min=0"`

	// ThinkingBudgetTokens specifies the number of tokens the model can use for its internal reasoning process.
	// It must be at least 1024 and less than `MaxTokens`.
	// If set, this enables enhanced reasoning capabilities for the model.
	ThinkingBudgetTokens *int64 `yaml:"thinking-budget-tokens" validate:"omitempty,min=1024,ltfield=MaxTokens"`
}

// UnmarshalYAML implements custom YAML unmarshaling for ProviderConfig.
// It handles provider-specific client configuration based on provider name.
func (pc *ProviderConfig) UnmarshalYAML(value *yaml.Node) error {
	var temp struct {
		Name         string    `yaml:"name"`
		ClientConfig yaml.Node `yaml:"client-config"`
		Runs         yaml.Node `yaml:"runs"`
		Disabled     bool      `yaml:"disabled"`
	}

	if err := value.Decode(&temp); err != nil {
		return err
	}

	pc.Name = temp.Name
	pc.Disabled = temp.Disabled

	if err := decodeRuns(temp.Name, &temp.Runs, &pc.Runs); err != nil {
		return err
	}

	switch temp.Name {
	case OPENAI:
		cfg := OpenAIClientConfig{}
		if err := temp.ClientConfig.Decode(&cfg); err != nil {
			return err
		}
		pc.ClientConfig = cfg
	case GOOGLE:
		cfg := GoogleAIClientConfig{}
		if err := temp.ClientConfig.Decode(&cfg); err != nil {
			return err
		}
		pc.ClientConfig = cfg
	case ANTHROPIC:
		cfg := AnthropicClientConfig{}
		if err := temp.ClientConfig.Decode(&cfg); err != nil {
			return err
		}
		pc.ClientConfig = cfg
	case DEEPSEEK:
		cfg := DeepseekClientConfig{}
		if err := temp.ClientConfig.Decode(&cfg); err != nil {
			return err
		}
		pc.ClientConfig = cfg
	default:
		return fmt.Errorf("%w: unknown client-config for provider: %s", ErrInvalidConfigProperty, temp.Name)
	}

	return nil
}

func decodeRuns(provider string, value *yaml.Node, out *[]RunConfig) error {
	var temp []struct {
		Name                 string    `yaml:"name"`
		Model                string    `yaml:"model"`
		MaxRequestsPerMinute int       `yaml:"max-requests-per-minute"`
		Disabled             *bool     `yaml:"disabled"`
		ModelParams          yaml.Node `yaml:"model-parameters"`
	}

	if err := value.Decode(&temp); err != nil {
		return err
	}

	*out = make([]RunConfig, len(temp))
	for i := range temp {
		(*out)[i].Name = temp[i].Name
		(*out)[i].Model = temp[i].Model
		(*out)[i].MaxRequestsPerMinute = temp[i].MaxRequestsPerMinute
		(*out)[i].Disabled = temp[i].Disabled

		if !temp[i].ModelParams.IsZero() {
			switch provider {
			case OPENAI:
				params := OpenAIModelParams{}
				if err := temp[i].ModelParams.Decode(&params); err != nil {
					return err
				}
				(*out)[i].ModelParams = params
			case ANTHROPIC:
				params := AnthropicModelParams{}
				if err := temp[i].ModelParams.Decode(&params); err != nil {
					return err
				}
				(*out)[i].ModelParams = params
			default:
				return fmt.Errorf("%w: provider '%s' does not support model parameters", ErrInvalidConfigProperty, provider)
			}
		}
	}

	return nil
}
