// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/petmal/mindtrial/pkg/utils"
	"gopkg.in/yaml.v3"
)

// downloadTimeout defines the maximum time allowed for downloading remote files.
const downloadTimeout = time.Minute

var (
	// ErrInvalidTaskProperty indicates invalid task definition.
	ErrInvalidTaskProperty = errors.New("invalid task property")
	// ErrInvalidURI indicates that the specified URI is invalid or not supported.
	ErrInvalidURI = errors.New("invalid URI")
	// ErrDownloadFile indicates that a remote file could not be downloaded.
	ErrDownloadFile = errors.New("failed to download remote file")
	// ErrAccessFile indicates that a local file could not be accessed.
	ErrAccessFile = errors.New("file is not accessible")
)

// URI represents a parsed URI/URL that can be used to reference a file.
type URI struct {
	raw    string
	parsed *url.URL
}

// UnmarshalYAML implements custom YAML unmarshaling for URI.
func (u *URI) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTaskProperty, err)
	}

	if err := u.Parse(raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTaskProperty, err)
	}

	return nil
}

// Parse parses a raw URI string into a structured URI object.
// It validates that the URI scheme is supported.
func (u *URI) Parse(raw string) (err error) {
	if raw == "" {
		return fmt.Errorf("%w: empty URI value", ErrInvalidURI)
	}

	u.raw = raw
	normalized := filepath.ToSlash(raw)

	// Special handling for Windows absolute paths with drive letters.
	if filepath.IsAbs(raw) && len(raw) >= 2 && raw[1] == ':' {
		u.parsed = &url.URL{
			Scheme: "",
			Path:   normalized,
		}
	} else {
		u.parsed, err = url.Parse(normalized)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidURI, err)
		} else if !isSupportedScheme(u.parsed.Scheme) {
			return fmt.Errorf("%w: unsupported scheme: %s", ErrInvalidURI, u.parsed.Scheme)
		}
	}

	return nil
}

// isSupportedScheme checks if the given URI scheme is supported by this application.
func isSupportedScheme(scheme string) bool {
	return isLocalFile(scheme) || isRemoteFile(scheme)
}

// isLocalFile checks if the given URI scheme represents a local file.
// A scheme that is either empty or "file" represents a local file.
func isLocalFile(scheme string) bool {
	return scheme == "" || scheme == "file"
}

// isRemoteFile checks if the given URI scheme represents a remote file.
func isRemoteFile(scheme string) bool {
	return scheme == "http" || scheme == "https"
}

// MarshalYAML implements custom YAML marshaling for URI.
func (u URI) MarshalYAML() (interface{}, error) {
	return u.raw, nil
}

// URL returns the parsed URL.
func (u URI) URL() *url.URL {
	return u.parsed
}

// IsLocalFile checks if the URI references a local file.
func (u URI) IsLocalFile() bool {
	return isLocalFile(u.parsed.Scheme)
}

// IsRemoteFile checks if the URI references a remote file.
func (u URI) IsRemoteFile() bool {
	return isRemoteFile(u.parsed.Scheme)
}

// String returns the original raw URI string.
func (u URI) String() string {
	return u.raw
}

// Path returns the filesystem path for local URIs.
// For relative local paths, it uses the provided basePath to create an absolute path.
func (u URI) Path(basePath string) string {
	switch u.parsed.Scheme {
	case "file":
		return u.parsed.Path
	case "":
		return MakeAbs(basePath, u.raw)
	default:
		return u.raw
	}
}

// Tasks represents the top-level task configuration structure.
type Tasks struct {
	// TaskConfig contains all task definitions and settings.
	TaskConfig TaskConfig `yaml:"task-config" validate:"required"`
}

// SystemPrompt represents a system prompt configuration.
type SystemPrompt struct {
	// Template is the template string for the system prompt.
	// It can reference `{{.ResponseResultFormat}}` to include the task's response format.
	Template *string `yaml:"template" validate:"omitempty"`
}

// GetTemplate returns the template string and true if it is set and not blank.
func (s SystemPrompt) GetTemplate() (template string, ok bool) {
	if ok = s.Template != nil && IsNotBlank(*s.Template); ok {
		template = *s.Template
	}
	return
}

// MergeWith merges this system prompt with another and returns the result.
// The provided other values override these values if set.
func (these SystemPrompt) MergeWith(other *SystemPrompt) SystemPrompt {
	resolved := these

	if other != nil {
		setIfNotNil(&resolved.Template, other.Template)
	}

	return resolved
}

// TaskConfig represents task definitions and global settings.
type TaskConfig struct {
	// Tasks is a list of tasks to be executed.
	Tasks []Task `yaml:"tasks" validate:"required,unique=Name,dive"`

	// Disabled indicates whether all tasks should be disabled by default.
	// Individual tasks can override this setting.
	Disabled bool `yaml:"disabled" validate:"omitempty"`

	// ValidationRules are default validation settings for all tasks.
	// Individual tasks can override these settings.
	ValidationRules ValidationRules `yaml:"validation-rules" validate:"omitempty"`

	// SystemPrompt is the default system prompt configuration for all tasks.
	// Individual tasks can override this configuration.
	SystemPrompt SystemPrompt `yaml:"system-prompt" validate:"omitempty"`
}

// GetEnabledTasks returns a filtered list of tasks that are not disabled.
// If Task.Disabled is nil, the global TaskConfig.Disabled value is used instead.
func (o TaskConfig) GetEnabledTasks() []Task {
	enabledTasks := make([]Task, 0, len(o.Tasks))
	for _, task := range o.Tasks {
		if !ResolveFlagOverride(task.Disabled, o.Disabled) {
			enabledTasks = append(enabledTasks, task)
		}
	}
	return enabledTasks
}

// TaskFile represents a file to be included with a task.
type TaskFile struct {
	// Name is a unique identifier for the file, used to reference it in prompts.
	Name string `yaml:"name" validate:"required"`

	// URI is the path or URL to the file.
	URI URI `yaml:"uri" validate:"required"`

	// Type is the MIME type of the file.
	// If not provided, it will be inferred from the file extension or content.
	Type string `yaml:"type" validate:"omitempty"`

	// basePath is used to resolve relative local paths.
	basePath string

	content   func(context.Context, *TaskFile) ([]byte, error)
	base64    func(context.Context, *TaskFile) (string, error)
	typeValue func(context.Context, *TaskFile) (string, error)
}

// UnmarshalYAML implements custom YAML unmarshaling for TaskFile.
func (f *TaskFile) UnmarshalYAML(value *yaml.Node) error {
	// Define an alias to the TaskFile structure to avoid recursive unmarshaling.
	type taskFileAlias TaskFile
	aliasValue := taskFileAlias{}

	if err := value.Decode(&aliasValue); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTaskProperty, err)
	}

	// Copy values from alias to the actual TaskFile.
	*f = TaskFile(aliasValue)

	// Set functions to load content and type on demand.
	f.content = OnceWithContext(
		func(ctx context.Context, state *TaskFile) (data []byte, err error) {
			if state.URI.IsRemoteFile() {
				if data, err = downloadFile(ctx, state.URI.URL()); err != nil {
					return nil, err
				}
			} else {
				if data, err = os.ReadFile(state.URI.Path(state.basePath)); err != nil {
					return nil, fmt.Errorf("%w: %v", ErrAccessFile, err)
				}
			}

			return data, nil
		},
	)

	f.base64 = OnceWithContext(
		func(ctx context.Context, state *TaskFile) (string, error) {
			content, err := state.Content(ctx)
			if err != nil {
				return "", err
			}
			return base64.StdEncoding.EncodeToString(content), nil
		},
	)

	f.typeValue = OnceWithContext(
		func(ctx context.Context, state *TaskFile) (string, error) {
			if state.Type != "" {
				return state.Type, nil
			}

			// Try to infer from file extension first.
			if ext := filepath.Ext(state.URI.String()); ext != "" {
				if mimeType := mime.TypeByExtension(ext); mimeType != "" {
					return mimeType, nil
				}
			}

			// Fall back to detecting from content.
			content, err := state.Content(ctx)
			if err != nil {
				return "", err
			}

			return http.DetectContentType(content), nil
		},
	)

	return nil
}

// SetBasePath sets the base path used to resolve relative local paths.
func (f *TaskFile) SetBasePath(basePath string) {
	f.basePath = basePath
}

// downloadFile downloads a file from a URL and returns its content.
func downloadFile(ctx context.Context, url *url.URL) ([]byte, error) {
	// Create a child context with timeout.
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %v", ErrDownloadFile, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: network request failed for '%s': %v", ErrDownloadFile, url.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: server returned status %d for '%s'", ErrDownloadFile, resp.StatusCode, url.String())
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read file data: %v", ErrDownloadFile, err)
	}
	return data, nil
}

// Validate checks if a local file exists, is accessible, and is not a directory.
// Remote files are not validated as they will be checked when accessed.
func (f *TaskFile) Validate() error {
	if !f.URI.IsLocalFile() {
		return nil // Only validate local files.
	}

	path := f.URI.Path(f.basePath)
	if fileInfo, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: file does not exist: %s", ErrAccessFile, path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("%w: permission denied: %s", ErrAccessFile, path)
		}
		return fmt.Errorf("%w: %v", ErrAccessFile, err)
	} else if fileInfo.IsDir() {
		return fmt.Errorf("%w: path is a directory, not a file: %s", ErrAccessFile, path)
	}

	return nil
}

// Content returns the raw file content, loading it on demand.
func (f *TaskFile) Content(ctx context.Context) ([]byte, error) {
	return f.content(ctx, f)
}

// Base64 returns the base64-encoded file content, loading it on demand.
func (f *TaskFile) Base64(ctx context.Context) (string, error) {
	return f.base64(ctx, f)
}

// TypeValue returns the MIME type, inferring it if not set, loading content if needed.
func (f *TaskFile) TypeValue(ctx context.Context) (string, error) {
	return f.typeValue(ctx, f)
}

// GetDataURL returns a complete data URL for the file (e.g., "data:image/png;base64,...").
func (f *TaskFile) GetDataURL(ctx context.Context) (string, error) {
	mimeType, err := f.TypeValue(ctx)
	if err != nil {
		return "", err
	}

	base64Content, err := f.Base64(ctx)
	if err != nil {
		return "", err
	}

	return "data:" + mimeType + ";base64," + base64Content, nil
}

// Task defines a single test case to be executed by AI models.
type Task struct {
	// Name is a display-friendly identifier shown in results.
	Name string `yaml:"name" validate:"required"`

	// Prompt that will be sent to the AI model.
	Prompt string `yaml:"prompt" validate:"required"`

	// ResponseResultFormat specifies how the AI should format the final answer to the prompt.
	ResponseResultFormat string `yaml:"response-result-format" validate:"required"`

	// ExpectedResult is the set of accepted valid answers for the prompt.
	// All values must follow the ResponseResultFormat precisely.
	// Only one needs to match for the response to be considered correct.
	ExpectedResult utils.StringSet `yaml:"expected-result" validate:"required"`

	// Disabled indicates whether this specific task should be skipped.
	// If set, overrides the global TaskConfig.Disabled value.
	Disabled *bool `yaml:"disabled" validate:"omitempty"`

	// ValidationRules are validation settings for this specific task.
	// If set, overrides the global TaskConfig.ValidationRules values.
	ValidationRules *ValidationRules `yaml:"validation-rules" validate:"omitempty"`

	// SystemPrompt is the system prompt configuration for this specific task.
	// If set, overrides the global TaskConfig.SystemPrompt values.
	SystemPrompt *SystemPrompt `yaml:"system-prompt" validate:"omitempty"`

	// Files is a list of files to be included with the prompt.
	// This is primarily used for images but can support other file types
	// depending on the provider's capabilities.
	Files []TaskFile `yaml:"files" validate:"omitempty,unique=Name,dive"`

	// resolvedSystemPrompt is the resolved system prompt template for this task.
	resolvedSystemPrompt string
}

// GetResolvedSystemPrompt returns the resolved system prompt template for this task and true if it is not blank.
func (t Task) GetResolvedSystemPrompt() (prompt string, ok bool) {
	return t.resolvedSystemPrompt, IsNotBlank(t.resolvedSystemPrompt)
}

// ResolveSystemPrompt resolves the system prompt template for this task using the provided default.
// The resolved template can be retrieved using GetResolvedSystemPrompt().
func (t *Task) ResolveSystemPrompt(defaultConfig SystemPrompt) error {
	resolvedTemplate := defaultConfig.MergeWith(t.SystemPrompt)

	if templateValue, ok := resolvedTemplate.GetTemplate(); ok {
		tmpl, err := template.New("system-prompt").Option("missingkey=error").Parse(templateValue)
		if err != nil {
			return fmt.Errorf("failed to parse system prompt template: %w", err)
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, map[string]interface{}{
			"ResponseResultFormat": t.ResponseResultFormat,
		}); err != nil {
			return fmt.Errorf("failed to execute system prompt template: %w", err)
		}
		t.resolvedSystemPrompt = buf.String()
	}

	return nil
}

// JudgeSelector defines settings for using a judge in validation.
type JudgeSelector struct {
	// Enabled determines whether judge evaluation is enabled.
	Enabled *bool `yaml:"enabled" validate:"omitempty"`

	// Name specifies the name of the judge configuration to use.
	Name *string `yaml:"name" validate:"omitempty"`

	// Variant specifies the run variant name from the judge's provider configuration.
	Variant *string `yaml:"variant" validate:"omitempty"`
}

// IsEnabled returns whether judge evaluation is enabled.
func (js JudgeSelector) IsEnabled() bool {
	return js.Enabled != nil && *js.Enabled
}

// GetName returns the judge name, or empty string if not set.
func (js JudgeSelector) GetName() (name string) {
	if js.Name != nil {
		name = *js.Name
	}
	return
}

// GetVariant returns the judge run variant, or empty string if not set.
func (js JudgeSelector) GetVariant() (variant string) {
	if js.Variant != nil {
		variant = *js.Variant
	}
	return
}

// MergeWith merges this judge configuration with another and returns the result.
// The provided other values override these values if set.
func (these JudgeSelector) MergeWith(other JudgeSelector) JudgeSelector {
	resolved := these

	setIfNotNil(&resolved.Enabled, other.Enabled)
	setIfNotNil(&resolved.Name, other.Name)
	setIfNotNil(&resolved.Variant, other.Variant)

	return resolved
}

// ValidationRules represents task validation rules.
// It controls how model responses should be validated against expected results.
type ValidationRules struct {
	// CaseSensitive determines whether string comparison should be case-sensitive.
	CaseSensitive *bool `yaml:"case-sensitive" validate:"omitempty"`

	// IgnoreWhitespace determines whether all whitespace should be ignored during comparison.
	// When true, all whitespace characters (spaces, tabs, newlines) are removed before comparison.
	IgnoreWhitespace *bool `yaml:"ignore-whitespace" validate:"omitempty"`

	// TrimLines determines whether to trim leading and trailing whitespace of each line
	// before comparison.
	TrimLines *bool `yaml:"trim-lines" validate:"omitempty"`

	// Judge specifies the judge configuration to use for evaluation.
	// When enabled, an LLM will be used to evaluate the correctness of the response
	// instead of simple string matching.
	Judge JudgeSelector `yaml:"judge" validate:"omitempty"`
}

// IsCaseSensitive returns whether validation should be case sensitive.
func (vr ValidationRules) IsCaseSensitive() bool {
	return vr.CaseSensitive != nil && *vr.CaseSensitive
}

// IsIgnoreWhitespace returns whether whitespace should be ignored during validation.
func (vr ValidationRules) IsIgnoreWhitespace() bool {
	return vr.IgnoreWhitespace != nil && *vr.IgnoreWhitespace
}

// IsTrimLines returns whether each line should be trimmed before validation.
func (vr ValidationRules) IsTrimLines() bool {
	return vr.TrimLines != nil && *vr.TrimLines
}

// UseJudge returns whether judge evaluation is enabled.
func (vr ValidationRules) UseJudge() bool {
	return vr.Judge.IsEnabled()
}

// MergeWith merges these validation rules with other rules and returns the result.
// The provided other values override these values if set.
func (these ValidationRules) MergeWith(other *ValidationRules) ValidationRules {
	resolved := these

	if other != nil {
		setIfNotNil(&resolved.CaseSensitive, other.CaseSensitive)
		setIfNotNil(&resolved.IgnoreWhitespace, other.IgnoreWhitespace)
		setIfNotNil(&resolved.TrimLines, other.TrimLines)

		resolved.Judge = resolved.Judge.MergeWith(other.Judge)
	}

	return resolved
}

func setIfNotNil[T any](dst **T, src *T) {
	if src != nil {
		*dst = src
	}
}

// SetBaseFilePath sets the base path for all local files in the task.
// The resolved paths are validated to ensure they are accessible.
func (t *Task) SetBaseFilePath(basePath string) error {
	for i := range t.Files {
		t.Files[i].SetBasePath(basePath)
		if err := t.Files[i].Validate(); err != nil {
			return fmt.Errorf("file '%s' in task '%s' failed validation with base directory '%s': %w", t.Files[i].Name, t.Name, basePath, err)
		}
	}
	return nil
}
