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
	// MISTRALAI identifies the Mistral AI provider.
	MISTRALAI string = "mistralai"
	// XAI identifies the xAI provider.
	XAI string = "xai"
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

	// Judges lists LLM configurations for semantic evaluation of open-ended task responses.
	Judges []JudgeConfig `yaml:"judges" validate:"omitempty,unique=Name,dive"`
}

// GetProvidersWithEnabledRuns returns providers with their enabled run configurations.
// Run configurations are resolved using GetRunsResolved before filtering.
// Any disabled run configurations are excluded from the results.
// Providers with no enabled run configurations are excluded from the returned list.
func (ac AppConfig) GetProvidersWithEnabledRuns() []ProviderConfig {
	providers := make([]ProviderConfig, 0, len(ac.Providers))
	for _, provider := range ac.Providers {
		resolved := provider.Resolve(true)
		if len(resolved.Runs) > 0 {
			providers = append(providers, resolved)
		}
	}
	return providers
}

// GetJudgesWithEnabledRuns returns judges with their enabled run variant configurations.
// Run variant configurations are resolved using GetRunsResolved before filtering.
// Any disabled run variant configurations are excluded from the results.
// Judges with no enabled run variant configurations are excluded from the returned list.
func (ac AppConfig) GetJudgesWithEnabledRuns() []JudgeConfig {
	judges := make([]JudgeConfig, 0, len(ac.Judges))
	for _, judge := range ac.Judges {
		resolved := judge.Resolve(true)
		if len(resolved.Provider.Runs) > 0 {
			judges = append(judges, resolved)
		}
	}
	return judges
}

// ProviderConfig defines settings for an AI provider.
type ProviderConfig struct {
	// Name specifies unique identifier of the provider.
	Name string `yaml:"name" validate:"required,oneof=openai google anthropic deepseek mistralai xai"`

	// ClientConfig holds provider-specific client settings.
	ClientConfig ClientConfig `yaml:"client-config" validate:"required"`

	// Runs lists run configurations for this provider.
	Runs []RunConfig `yaml:"runs" validate:"required,unique=Name,dive"`

	// Disabled indicates if all runs should be disabled by default.
	Disabled bool `yaml:"disabled" validate:"omitempty"`

	// RetryPolicy specifies default retry behavior for all runs in this provider.
	RetryPolicy RetryPolicy `yaml:"retry-policy" validate:"omitempty"`
}

// GetRunsResolved returns runs with retry policies and disabled flags resolved.
// If RunConfig.RetryPolicy is nil, the parent ProviderConfig.RetryPolicy value is used instead.
// If RunConfig.Disabled is nil, the parent ProviderConfig.Disabled value is used instead.
func (pc ProviderConfig) GetRunsResolved() []RunConfig {
	resolved := make([]RunConfig, 0, len(pc.Runs))
	for _, run := range pc.Runs {
		if run.RetryPolicy == nil {
			run.RetryPolicy = &pc.RetryPolicy
		}
		if run.Disabled == nil {
			run.Disabled = &pc.Disabled
		}
		resolved = append(resolved, run)
	}
	return resolved
}

// Resolve returns a copy of the provider configuration with runs resolved.
// If excludeDisabledRuns is true, only enabled runs are included.
func (pc ProviderConfig) Resolve(excludeDisabledRuns bool) ProviderConfig {
	resolved := pc
	resolved.Runs = pc.GetRunsResolved()

	if excludeDisabledRuns {
		enabledRuns := make([]RunConfig, 0, len(resolved.Runs))
		for _, run := range resolved.Runs {
			if !*run.Disabled {
				enabledRuns = append(enabledRuns, run)
			}
		}
		resolved.Runs = enabledRuns
	}

	return resolved
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

// MistralAIClientConfig represents Mistral AI provider settings.
type MistralAIClientConfig struct {
	// APIKey is the API key for the Mistral AI generative models provider.
	APIKey string `yaml:"api-key" validate:"required"`
}

// XAIClientConfig represents xAI provider settings.
type XAIClientConfig struct {
	// APIKey is the API key for the xAI provider.
	APIKey string `yaml:"api-key" validate:"required"`
}

// RunConfig defines settings for a single run configuration.
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

	// RetryPolicy specifies retry behavior on transient errors.
	// If set, overrides the parent ProviderConfig.RetryPolicy value.
	RetryPolicy *RetryPolicy `yaml:"retry-policy" validate:"omitempty"`
}

// RetryPolicy defines retry behavior on transient errors.
type RetryPolicy struct {
	// MaxRetryAttempts specifies the maximum number of retry attempts.
	// Value of 0 means no retry attempts will be made.
	MaxRetryAttempts uint `yaml:"max-retry-attempts" validate:"omitempty,min=0"`

	// InitialDelaySeconds specifies the initial delay in seconds before the first retry attempt.
	InitialDelaySeconds int `yaml:"initial-delay-seconds" validate:"omitempty,gt=0"`
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

	// Temperature controls the randomness or "creativity" of the model's outputs.
	// Values range from 0.0 to 2.0, with lower values making the output more focused and deterministic.
	// The default value is 1.0.
	// It is generally recommended to alter this or `TopP` but not both.
	Temperature *float32 `yaml:"temperature" validate:"omitempty,min=0,max=2"`

	// TopP controls diversity via nucleus sampling.
	// Values range from 0.0 to 1.0, with lower values making the output more focused.
	// The default value is 1.0.
	// It is generally recommended to alter this or `Temperature` but not both.
	TopP *float32 `yaml:"top-p" validate:"omitempty,min=0,max=1"`

	// PresencePenalty penalizes new tokens based on whether they appear in the text so far.
	// Values range from -2.0 to 2.0, with positive values encouraging the model to use new tokens,
	// increasing the model's likelihood to talk about new topics.
	// The default value is 0.0.
	PresencePenalty *float32 `yaml:"presence-penalty" validate:"omitempty,min=-2,max=2"`

	// FrequencyPenalty penalizes new tokens based on their frequency in the text so far.
	// Values range from -2.0 to 2.0, with positive values encouraging the model to use less frequent tokens,
	// decreasing the model's likelihood to repeat the same line verbatim.
	// The default value is 0.0.
	FrequencyPenalty *float32 `yaml:"frequency-penalty" validate:"omitempty,min=-2,max=2"`
}

// GoogleAIModelParams represents Google AI model-specific settings.
type GoogleAIModelParams struct {
	// TextResponseFormat indicates whether to use plain-text response format
	// for compatibility with models that do not support JSON.
	TextResponseFormat bool `yaml:"text-response-format" validate:"omitempty"`

	// Temperature controls the randomness or "creativity" of the model's outputs.
	// Values range from 0.0 to 2.0, with lower values making the output more focused and deterministic.
	// The default value is typically around 1.0.
	Temperature *float32 `yaml:"temperature" validate:"omitempty,min=0,max=2"`

	// TopP controls diversity via nucleus sampling.
	// Values range from 0.0 to 1.0, with lower values making the output more focused.
	// The default value is typically around 1.0.
	TopP *float32 `yaml:"top-p" validate:"omitempty,min=0,max=1"`

	// TopK limits response tokens to top K options for each token position.
	// Higher values allow more diverse outputs by considering more token options.
	TopK *int32 `yaml:"top-k" validate:"omitempty,min=0"`

	// PresencePenalty penalizes new tokens based on whether they appear in the text so far.
	// Positive values discourage the use of tokens that have already been used in the response,
	// increasing the vocabulary. Negative values encourage the use of tokens that have already been used.
	// This penalty is binary on/off and not dependent on the number of times the token is used.
	PresencePenalty *float32 `yaml:"presence-penalty" validate:"omitempty"`

	// FrequencyPenalty penalizes new tokens based on their frequency in the text so far.
	// Positive values discourage the use of tokens that have already been used, proportional to
	// the number of times the token has been used. Negative values encourage the model to reuse tokens.
	// This differs from PresencePenalty as it scales with frequency.
	FrequencyPenalty *float32 `yaml:"frequency-penalty" validate:"omitempty"`

	// Seed is used for deterministic generation. When set to a specific value, the model
	// makes a best effort to provide the same response for repeated requests.
	// If not set, a randomly generated seed is used.
	Seed *int32 `yaml:"seed" validate:"omitempty"`
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

	// Temperature controls the randomness or "creativity" of responses.
	// Values range from 0.0 to 1.0, with lower values making the output more focused.
	// The default value is 1.0.
	// It is generally recommended to alter this or `TopP` but not both.
	Temperature *float64 `yaml:"temperature" validate:"omitempty,min=0,max=1"`

	// TopP controls diversity via nucleus sampling.
	// Values range from 0.0 to 1.0, with lower values making the output more focused.
	// You usually only need to use `Temperature`.
	TopP *float64 `yaml:"top-p" validate:"omitempty,min=0,max=1"`

	// TopK limits response tokens to top K options for each token position.
	// Higher values allow more diverse outputs by considering more token options.
	// You usually only need to use `Temperature`.
	TopK *int64 `yaml:"top-k" validate:"omitempty,min=0"`
}

// DeepseekModelParams represents Deepseek model-specific settings.
type DeepseekModelParams struct {
	// Temperature controls the randomness or "creativity" of the model's outputs.
	// Values range from 0.0 to 2.0, with lower values making the output more focused.
	// The default value is 1.0.
	// Recommended values by use case:
	// - 0.0: Coding / Math (best for precise, deterministic outputs)
	// - 1.0: Data Cleaning / Data Analysis
	// - 1.3: General Conversation / Translation
	// - 1.5: Creative Writing / Poetry (more varied and creative outputs)
	Temperature *float32 `yaml:"temperature" validate:"omitempty,min=0,max=2"`

	// TopP controls diversity via nucleus sampling.
	// Values range from 0.0 to 1.0, with lower values making the output more focused.
	// You usually only need to use `Temperature`.
	TopP *float32 `yaml:"top-p" validate:"omitempty,min=0,max=1"`

	// PresencePenalty penalizes new tokens based on whether they appear in the text so far.
	// Values range from -2.0 to 2.0, with positive values encouraging the model to use new tokens,
	// increasing the model's likelihood to talk about new topics.
	// The default value is 0.0.
	PresencePenalty *float32 `yaml:"presence-penalty" validate:"omitempty,min=-2,max=2"`

	// FrequencyPenalty penalizes new tokens based on their frequency in the text so far.
	// Values range from -2.0 to 2.0, with positive values encouraging the model to use less frequent tokens,
	// decreasing the model's likelihood to repeat the same line verbatim.
	// The default value is 0.0.
	FrequencyPenalty *float32 `yaml:"frequency-penalty" validate:"omitempty,min=-2,max=2"`
}

// MistralAIModelParams represents Mistral AI model-specific settings.
type MistralAIModelParams struct {
	// Temperature controls the randomness or "creativity" of the model's outputs.
	// Values range from 0.0 to 1.5, with lower values making the output more focused and deterministic.
	// The default value varies depending on the model.
	// It is generally recommended to alter this or `TopP` but not both.
	Temperature *float32 `yaml:"temperature" validate:"omitempty,min=0,max=1.5"`

	// TopP controls diversity via nucleus sampling.
	// Values range from 0.0 to 1.0, with lower values making the output more focused.
	// The default value is 1.0.
	// It is generally recommended to alter this or `Temperature` but not both.
	TopP *float32 `yaml:"top-p" validate:"omitempty,min=0,max=1"`

	// MaxTokens controls the maximum number of tokens to generate in the completion.
	// The token count of the prompt plus max_tokens cannot exceed the model's context length.
	MaxTokens *int32 `yaml:"max-tokens" validate:"omitempty,min=0"`

	// PresencePenalty penalizes new tokens based on whether they appear in the text so far.
	// Values range from -2.0 to 2.0, with positive values encouraging the model to use new tokens,
	// increasing the model's likelihood to talk about new topics.
	// The default value is 0.0.
	PresencePenalty *float32 `yaml:"presence-penalty" validate:"omitempty,min=-2,max=2"`

	// FrequencyPenalty penalizes new tokens based on their frequency in the text so far.
	// Values range from -2.0 to 2.0, with positive values encouraging the model to use less frequent tokens,
	// decreasing the model's likelihood to repeat the same line verbatim.
	// The default value is 0.0.
	FrequencyPenalty *float32 `yaml:"frequency-penalty" validate:"omitempty,min=-2,max=2"`

	// RandomSeed provides the seed to use for random sampling.
	// If set, requests will generate deterministic results.
	RandomSeed *int32 `yaml:"random-seed" validate:"omitempty"`

	// PromptMode sets the prompt mode for the request.
	// When set to "reasoning", a system prompt will be used to instruct the model to reason if supported.
	PromptMode *string `yaml:"prompt-mode" validate:"omitempty,oneof=reasoning"`

	// SafePrompt controls whether to inject a safety prompt before all conversations.
	SafePrompt *bool `yaml:"safe-prompt" validate:"omitempty"`
}

// XAIModelParams represents xAI model-specific settings.
type XAIModelParams struct {
	// Temperature controls the randomness or "creativity" of the model's outputs.
	// Notes: Higher values (e.g. 0.8) make outputs more random; lower values
	// (e.g. 0.2) make outputs more focused and deterministic.
	// Valid range: 0.0 — 2.0. Default: 1.0.
	Temperature *float32 `yaml:"temperature" validate:"omitempty,min=0,max=2"`

	// TopP controls diversity via nucleus sampling (probability mass cutoff).
	// Notes: Use either Temperature or TopP, not both, for sampling control.
	// Valid range: (0.0, 1.0]. Default: 1.0.
	TopP *float32 `yaml:"top-p" validate:"omitempty,min=0,max=1"`

	// MaxCompletionTokens controls the maximum number of tokens to generate in the completion.
	MaxCompletionTokens *int32 `yaml:"max-completion-tokens" validate:"omitempty,min=0"`

	// PresencePenalty penalizes new tokens based on whether they appear in the text so far.
	// Notes: Positive values encourage the model to introduce new topics.
	// Valid range: -2.0 — 2.0. Default: 0.0.
	PresencePenalty *float32 `yaml:"presence-penalty" validate:"omitempty,min=-2,max=2"`

	// FrequencyPenalty penalizes new tokens based on their frequency in the text so far.
	// Notes: Positive values discourage repetition.
	// Valid range: -2.0 — 2.0. Default: 0.0.
	FrequencyPenalty *float32 `yaml:"frequency-penalty" validate:"omitempty,min=-2,max=2"`

	// ReasoningEffort constrains how much "reasoning" budget to spend for reasoning-capable models.
	// Notes: Not all reasoning models support this option.
	// Valid values: "low", "high".
	ReasoningEffort *string `yaml:"reasoning-effort" validate:"omitempty,oneof=low high"`

	// Seed requests deterministic sampling when possible.
	// No guaranteed determinism — xAI makes a best-effort to return
	// repeatable outputs for identical inputs when `seed` and other parameters are the same.
	Seed *int32 `yaml:"seed" validate:"omitempty"`
}

// JudgeConfig defines configuration for an LLM judge used for semantic evaluation of complex open-ended task responses.
// Judges analyze the meaning and quality of answers rather than performing exact text matching,
// enabling evaluation of subjective or creative tasks where multiple valid interpretations exist.
type JudgeConfig struct {
	// Name is the unique identifier for this judge configuration.
	Name string `yaml:"name" validate:"required"`

	// Provider encapsulates the provider configuration for the judge.
	Provider ProviderConfig `yaml:"provider" validate:"required"`
}

// Resolve returns a copy of the judge configuration with run variants resolved.
// If excludeDisabledRuns is true, only enabled run variants are included.
func (jc JudgeConfig) Resolve(excludeDisabledRuns bool) JudgeConfig {
	resolved := jc
	resolved.Provider = jc.Provider.Resolve(excludeDisabledRuns)
	return resolved
}

// UnmarshalYAML implements custom YAML unmarshaling for ProviderConfig.
// It handles provider-specific client configuration based on provider name.
func (pc *ProviderConfig) UnmarshalYAML(value *yaml.Node) error {
	var temp struct {
		Name         string      `yaml:"name"`
		ClientConfig yaml.Node   `yaml:"client-config"`
		Runs         yaml.Node   `yaml:"runs"`
		Disabled     bool        `yaml:"disabled"`
		RetryPolicy  RetryPolicy `yaml:"retry-policy"`
	}

	if err := value.Decode(&temp); err != nil {
		return err
	}

	pc.Name = temp.Name
	pc.Disabled = temp.Disabled
	pc.RetryPolicy = temp.RetryPolicy

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
	case MISTRALAI:
		cfg := MistralAIClientConfig{}
		if err := temp.ClientConfig.Decode(&cfg); err != nil {
			return err
		}
		pc.ClientConfig = cfg
	case XAI:
		cfg := XAIClientConfig{}
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
		Name                 string       `yaml:"name"`
		Model                string       `yaml:"model"`
		MaxRequestsPerMinute int          `yaml:"max-requests-per-minute"`
		Disabled             *bool        `yaml:"disabled"`
		ModelParams          yaml.Node    `yaml:"model-parameters"`
		RetryPolicy          *RetryPolicy `yaml:"retry-policy"`
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
		(*out)[i].RetryPolicy = temp[i].RetryPolicy

		if !temp[i].ModelParams.IsZero() {
			switch provider {
			case OPENAI:
				params := OpenAIModelParams{}
				if err := temp[i].ModelParams.Decode(&params); err != nil {
					return err
				}
				(*out)[i].ModelParams = params
			case GOOGLE:
				params := GoogleAIModelParams{}
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
			case DEEPSEEK:
				params := DeepseekModelParams{}
				if err := temp[i].ModelParams.Decode(&params); err != nil {
					return err
				}
				(*out)[i].ModelParams = params
			case MISTRALAI:
				params := MistralAIModelParams{}
				if err := temp[i].ModelParams.Decode(&params); err != nil {
					return err
				}
				(*out)[i].ModelParams = params
			case XAI:
				params := XAIModelParams{}
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
