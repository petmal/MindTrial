// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package runners

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/validators"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerPreflightBindMountSources(t *testing.T) {
	entryPoints := []struct {
		name string
		run  func(*defaultRunner, context.Context, []config.Task) (ResultSet, error)
	}{
		{
			name: "Run",
			run:  (*defaultRunner).Run,
		},
		{
			name: "Start",
			run: func(runner *defaultRunner, ctx context.Context, tasks []config.Task) (ResultSet, error) {
				return runner.Start(ctx, tasks)
			},
		},
	}
	for _, entry := range entryPoints {
		for _, consumer := range []string{"tool", "service via tool", "custom validator", "service via validator"} {
			for _, sourceKind := range []string{"directory", "missing", "file", "relative", "unused"} {
				if entry.name == "Start" && (sourceKind == "directory" || sourceKind == "unused") {
					continue
				}
				t.Run(entry.name+"/"+consumer+"/"+sourceKind, func(t *testing.T) {
					source := t.TempDir()
					var wantErr error
					switch sourceKind {
					case "missing", "unused":
						source = filepath.Join(source, "missing")
						wantErr = os.ErrNotExist
					case "file":
						source = testutils.CreateMockFile(t, "bind-source-*.txt", []byte("keep"))
						t.Cleanup(func() { assert.NoError(t, os.Remove(source)) })
						wantErr = config.ErrInvalidConfigProperty
					case "relative":
						source = "relative-source"
						wantErr = config.ErrInvalidConfigProperty
					}
					mounts := []config.BindMountConfig{
						{
							Source: source,
							Target: "/exports",
						},
					}
					tools := taskServicesTestTools()
					services := taskServicesTestServices()
					validator := worldStateValidator()
					validator.Dependencies = nil
					var task config.Task
					var wantConsumer string
					switch consumer {
					case "tool":
						tools[1].BindMounts = mounts
						task = newToolTask(t, "python")
						wantConsumer = "tool 'python'"
					case "service via tool":
						services[0].BindMounts = mounts
						task = newToolTask(t, "world-client")
						wantConsumer = "service 'world'"
					case "custom validator":
						validator.BindMounts = mounts
						task = newCustomValidatedTask(t, "task", validator.Name, nil)
						wantConsumer = "custom validator 'world-state'"
					case "service via validator":
						services[0].BindMounts = mounts
						validator.Dependencies = []config.ServiceDependency{
							{
								Service: "world",
							},
						}
						task = newCustomValidatedTask(t, "task", validator.Name, nil)
						wantConsumer = "service 'world'"
					}
					if sourceKind == "unused" {
						task = newTaskServicesTask(t, "unused", config.ToolSelector{
							ServiceInputs: map[string]map[string]interface{}{
								"world": {"seed": "unused"},
							},
						}, &config.ToolSelector{
							Disabled: testutils.Ptr(true),
							Tools:    []config.ToolSelection{{Name: "world-client"}, {Name: "python"}},
						})
						wantErr = nil
					}
					customValidators := []config.ValidatorConfig{validator}
					runner := &defaultRunner{
						validatorFactory: validators.NewFactoryWithCustomValidators(nil, customValidators),
						tools:            tools,
						taskRuntime: taskRuntimeConfig{
							services:         services,
							customValidators: customValidators,
						},
						logger:        zerolog.Nop(),
						toolValidator: &stubToolValidator{},
					}
					// A later invalid task must block the whole batch, not just that task.
					tasks := []config.Task{newToolTask(t), task, task}
					results, err := entry.run(runner, t.Context(), tasks)
					if wantErr != nil {
						require.ErrorIs(t, err, wantErr)
						require.ErrorIs(t, err, config.ErrInvalidConfigProperty)
						assert.Nil(t, results, "preflight failure must not start task execution")
						assert.Contains(t, err.Error(), wantConsumer+" cannot be used")
						assert.Equal(t, 1, strings.Count(err.Error(), strconv.Quote(source)), "each selected definition is checked once")
						return
					}
					require.NoError(t, err)
					require.NotNil(t, results)
					assert.Empty(t, results.GetResults())
				})
			}
		}
	}
}
