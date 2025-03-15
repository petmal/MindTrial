// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

// Tasks represents the top-level task configuration structure.
type Tasks struct {
	// TaskConfig contains all task definitions and settings.
	TaskConfig TaskConfig `yaml:"task-config" validate:"required"`
}

// TaskConfig represents task definitions and global settings.
type TaskConfig struct {
	// Tasks is a list of tasks to be executed.
	Tasks []Task `yaml:"tasks" validate:"required,dive"`

	// Disabled indicates whether all tasks should be disabled by default.
	// Individual tasks can override this setting.
	Disabled bool `yaml:"disabled" validate:"omitempty"`
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

// Task defines a single test case to be executed by AI models.
type Task struct {
	// Name is a display-friendly identifier shown in results.
	Name string `yaml:"name" validate:"required"`

	// Prompt that will be sent to the AI model.
	Prompt string `yaml:"prompt" validate:"required"`

	// ResponseResultFormat specifies how the AI should format the final answer to the prompt.
	ResponseResultFormat string `yaml:"response-result-format" validate:"required"`

	// ExpectedResult is the correct final answer to the prompt that the model should provide.
	// It must follow the ResponseResultFormat precisely.
	ExpectedResult string `yaml:"expected-result" validate:"required"`

	// Disabled indicates whether this specific task should be skipped.
	// If set, overrides the global TaskConfig.Disabled value.
	Disabled *bool `yaml:"disabled" validate:"omitempty"`
}
