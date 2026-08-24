// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/petmal/mindtrial/pkg/utils"
	"gopkg.in/yaml.v3"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// envPlaceholderMatcher matches an exact {{.Env.NAME}} template placeholder (tolerating
// extra whitespace around the field, e.g. {{ .Env.NAME }}) referencing any single
// environment variable name, rejecting any other template expression (e.g. a nested
// field or an additional pipeline).
var envPlaceholderMatcher = regexp.MustCompile(`^\{\{\s*\.Env\.[A-Za-z_][A-Za-z0-9_]*\s*\}\}$`)

var (
	// ErrResolveConfigValue indicates that a configuration value could not be resolved.
	ErrResolveConfigValue = errors.New("failed to resolve configuration value")
	// ErrMissingEnvironmentVariable indicates an explicit environment-backed config value was unresolved or empty.
	ErrMissingEnvironmentVariable = errors.New("missing environment variable")
)

// LoadConfigFromFile reads and validates application configuration from the specified file path.
// Returns error if the file cannot be read or contains invalid configuration.
func LoadConfigFromFile(ctx context.Context, path string) (*Config, error) {
	fp, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open configuration file: %w", err)
	}
	defer fp.Close()

	fileContents, err := io.ReadAll(fp)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	cfg := &Config{}
	if err := yamlUnmarshalStrict(fileContents, cfg); err != nil {
		return nil, fmt.Errorf("malformed configuration file: %w", err)
	}

	if err := resolveEnvironmentConfigValues(cfg, os.Environ()); err != nil {
		return cfg, fmt.Errorf("invalid configuration definition: %w", err)
	}

	if err := validate.Struct(cfg); err != nil {
		return cfg, fmt.Errorf("invalid configuration definition: %w", err)
	}

	// Validate tool parameters schemas.
	for _, tool := range cfg.Config.Tools {
		if err := validateToolParameters(tool.Parameters); err != nil {
			return cfg, fmt.Errorf("invalid tool configuration: invalid parameters for tool '%s': %w", tool.Name, err)
		}
	}

	// Validate judge configurations.
	for _, judge := range cfg.Config.Judges {
		if err := judge.Validate(); err != nil {
			return cfg, fmt.Errorf("invalid judge configuration: invalid parameters for judge '%s': %w", judge.Name, err)
		}
	}

	if err := validatePricingConfig(cfg.Config); err != nil {
		return cfg, fmt.Errorf("invalid pricing configuration: %w", err)
	}

	return cfg, nil
}

// validatePricingConfig validates every Pricing block configured at the application,
// provider, run, and judge variant levels (see Pricing.Validate).
func validatePricingConfig(cfg AppConfig) error {
	if err := cfg.Pricing.Validate(); err != nil {
		return fmt.Errorf("application: %w", err)
	}
	for _, provider := range cfg.Providers {
		if err := validateProviderPricingConfig(provider); err != nil {
			return fmt.Errorf("provider '%s': %w", provider.Name, err)
		}
	}
	for _, judge := range cfg.Judges {
		if err := validateProviderPricingConfig(judge.Provider); err != nil {
			return fmt.Errorf("judge '%s': %w", judge.Name, err)
		}
	}
	return nil
}

// validateProviderPricingConfig validates the Pricing blocks configured on a provider and
// each of its runs.
func validateProviderPricingConfig(pc ProviderConfig) error {
	if err := pc.Pricing.Validate(); err != nil {
		return err
	}
	for _, run := range pc.Runs {
		if err := run.Pricing.Validate(); err != nil {
			return fmt.Errorf("run '%s': %w", run.Name, err)
		}
	}
	return nil
}

// resolveEnvironmentConfigValues resolves environment-backed configuration values.
func resolveEnvironmentConfigValues(cfg *Config, env []string) error {
	envMap := make(map[string]string, len(env))
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		envMap[parts[0]] = parts[1]
	}

	for i := range cfg.Config.Providers {
		if err := resolveProviderEnvironmentConfig(&cfg.Config.Providers[i], envMap); err != nil {
			return err
		}
	}
	for i := range cfg.Config.Judges {
		if err := resolveProviderEnvironmentConfig(&cfg.Config.Judges[i].Provider, envMap); err != nil {
			return err
		}
	}
	return nil
}

func resolveProviderEnvironmentConfig(cfg *ProviderConfig, envMap map[string]string) error {
	switch clientConfig := cfg.ClientConfig.(type) {
	case OpenAIClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "OPENAI_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case OpenRouterClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "OPENROUTER_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case GoogleAIClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "GOOGLE_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case AnthropicClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "ANTHROPIC_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case DeepseekClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "DEEPSEEK_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case MistralAIClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "MISTRAL_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case XAIClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "XAI_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case AlibabaClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "DASHSCOPE_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	case MoonshotAIClientConfig:
		resolvedKey, err := resolveAPIKeyValue(clientConfig.APIKey, "MOONSHOT_API_KEY", envMap)
		if err != nil {
			return fmt.Errorf("%w: provider %q: %w", ErrResolveConfigValue, cfg.Name, err)
		}
		clientConfig.APIKey = resolvedKey
		cfg.ClientConfig = clientConfig
	}
	return nil
}

func resolveAPIKeyValue(raw string, fallbackEnvName string, envMap map[string]string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if value, ok := envMap[fallbackEnvName]; ok && strings.TrimSpace(value) != "" {
			return value, nil
		}
		return "", nil
	}

	if !envPlaceholderMatcher.MatchString(trimmed) {
		return raw, nil
	}

	var resolved strings.Builder
	tmpl, err := template.New("api-key").Option("missingkey=error").Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: invalid template for environment-backed API key: %v", ErrResolveConfigValue, err)
	}
	if err := tmpl.Execute(&resolved, struct{ Env map[string]string }{Env: envMap}); err != nil {
		return "", fmt.Errorf("%w: %w", ErrMissingEnvironmentVariable, err)
	}
	if strings.TrimSpace(resolved.String()) == "" {
		return "", fmt.Errorf("%w: environment-backed API key resolved to an empty value", ErrMissingEnvironmentVariable)
	}
	return resolved.String(), nil
}

// validateToolParameters validates that the tool parameters map is a valid JSON schema.
func validateToolParameters(parameters map[string]interface{}) error {
	if len(parameters) == 0 {
		return nil // empty parameters are allowed
	}

	if err := utils.ValidateAgainstSchema(parameters); err != nil {
		return fmt.Errorf("parameters must be a valid JSON schema: %w", err)
	}
	return nil
}

// LoadTasksFromFile reads and validates task definitions from the specified file path.
// Returns error if the file cannot be read or contains invalid task definitions.
func LoadTasksFromFile(ctx context.Context, path string) (*Tasks, error) {
	fp, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open tasks file: %w", err)
	}
	defer fp.Close()

	fileContents, err := io.ReadAll(fp)
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks file: %w", err)
	}

	cfg := &Tasks{}
	if err := yamlUnmarshalStrict(fileContents, cfg); err != nil {
		return nil, fmt.Errorf("malformed tasks file: %w", err)
	}

	if err := validate.Struct(cfg); err != nil {
		return cfg, fmt.Errorf("invalid task definition: %w", err)
	}

	// Resolve system prompt templates, validation rules and tool selections for all tasks.
	for i, task := range cfg.TaskConfig.Tasks {
		if err := cfg.TaskConfig.Tasks[i].ResolveSystemPrompt(cfg.TaskConfig.SystemPrompt); err != nil {
			return cfg, fmt.Errorf("invalid system prompt configuration for task '%s': %w", task.Name, err)
		}
		if err := cfg.TaskConfig.Tasks[i].ResolveValidationRules(cfg.TaskConfig.ValidationRules); err != nil {
			return cfg, fmt.Errorf("invalid validation rules configuration for task '%s': %w", task.Name, err)
		}
		cfg.TaskConfig.Tasks[i].ResolveToolSelector(cfg.TaskConfig.ToolSelector)
		cfg.TaskConfig.Tasks[i].ResolveMaxTurns(cfg.TaskConfig.MaxTurns)
		for j := range cfg.TaskConfig.Tasks[i].Files {
			cfg.TaskConfig.Tasks[i].Files[j].ResolveFileOptions(cfg.TaskConfig.FileOptions)
		}
	}

	// Validate task configuration consistency.
	if err := cfg.TaskConfig.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid task configuration: %w", err)
	}

	return cfg, nil
}

// yamlUnmarshalStrict is a helper function for strict YAML unmarshaling that fails on unknown fields.
func yamlUnmarshalStrict(in []byte, out interface{}) error {
	// NOTE: currently does not propagate to custom unmarshalers:
	// https://github.com/go-yaml/yaml/issues/460
	decoder := yaml.NewDecoder(bytes.NewReader(in))
	decoder.KnownFields(true) // fail on unknown fields
	return decoder.Decode(out)
}

// IsNotBlank returns true if the given string contains non-whitespace characters.
func IsNotBlank(value string) bool {
	return len(strings.TrimSpace(value)) > 0
}

// ResolveFileNamePattern takes a filename pattern containing time placeholders and returns
// a string with the placeholders replaced by values from the given time reference.
// Supported placeholders: {{.Year}}, {{.Month}}, {{.Day}}, {{.Hour}}, {{.Minute}}, {{.Second}}.
// Returns the original pattern if it cannot be resolved.
func ResolveFileNamePattern(pattern string, timeRef time.Time) string {
	tmpl, err := template.New("filename").Parse(pattern)
	if err != nil {
		return pattern
	}
	resolved := strings.Builder{}
	if err := tmpl.Execute(&resolved, struct {
		Year   string
		Month  string
		Day    string
		Hour   string
		Minute string
		Second string
	}{
		Year:   strconv.Itoa(timeRef.Year()),
		Month:  formatWithLeadingZero(int(timeRef.Month())),
		Day:    formatWithLeadingZero(timeRef.Day()),
		Hour:   formatWithLeadingZero(timeRef.Hour()),
		Minute: formatWithLeadingZero(timeRef.Minute()),
		Second: formatWithLeadingZero(timeRef.Second()),
	}); err != nil {
		return pattern
	}
	return resolved.String()
}

func formatWithLeadingZero(value int) string {
	return fmt.Sprintf("%02d", value)
}

// ResolveFlagOverride returns override value if not nil, otherwise returns parent value.
func ResolveFlagOverride(override *bool, parentValue bool) bool {
	if override != nil {
		return *override
	}
	return parentValue
}

// MakeAbs converts relative file path to absolute using the given base directory.
// Returns original path if it's already absolute or blank.
func MakeAbs(baseDirPath string, filePath string) string {
	if IsNotBlank(filePath) {
		if filepath.IsAbs(filePath) {
			return filePath
		}
		return filepath.Join(baseDirPath, filePath)
	}
	return filePath
}

// CleanIfNotBlank cleans the given file path if it's not blank.
// Returns original path if it's blank.
func CleanIfNotBlank(filePath string) string {
	if IsNotBlank(filePath) {
		return filepath.Clean(filePath)
	}
	return filePath
}

// OnceWithContext returns a function that invokes f only once regardless of the supplied context.
// The first call's context is used for execution, and subsequent calls simply return the cached result.
// This is similar to sync.OnceValues but specifically for functions that need a context.
func OnceWithContext[S any, T any](f func(context.Context, *S) (T, error)) func(context.Context, *S) (T, error) {
	var (
		once  sync.Once
		valid bool
		p     any
		r     T
		err   error
	)

	g := func(ctx context.Context, state *S) {
		defer func() {
			p = recover()
			if !valid {
				panic(p)
			}
		}()
		r, err = f(ctx, state)
		f = nil // allow function to be garbage collected
		valid = true
	}

	return func(ctx context.Context, state *S) (T, error) {
		once.Do(func() { g(ctx, state) })
		if !valid {
			panic(p)
		}
		return r, err
	}
}
