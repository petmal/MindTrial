// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

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

	if err := validate.Struct(cfg); err != nil {
		return cfg, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
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
func OnceWithContext[T any](f func(context.Context) (T, error)) func(context.Context) (T, error) {
	var (
		once  sync.Once
		valid bool
		p     any
		r     T
		err   error
	)

	g := func(ctx context.Context) {
		defer func() {
			p = recover()
			if !valid {
				panic(p)
			}
		}()
		r, err = f(ctx)
		f = nil // allow function to be garbage collected
		valid = true
	}

	return func(ctx context.Context) (T, error) {
		once.Do(func() { g(ctx) })
		if !valid {
			panic(p)
		}
		return r, err
	}
}
