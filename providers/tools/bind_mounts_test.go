// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindDirectoryRejectsMissingSourceOrFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	file := testutils.CreateMockFile(t, "bind-source-*.txt", []byte("keep"))
	t.Cleanup(func() { assert.NoError(t, os.Remove(file)) })

	tests := []struct {
		name    string
		source  string
		wantErr error
	}{
		{
			name:    "missing directory",
			source:  missing,
			wantErr: os.ErrNotExist,
		},
		{
			name:    "file",
			source:  file,
			wantErr: config.ErrInvalidConfigProperty,
		},
		{
			name:    "relative source",
			source:  filepath.Join("relative", "source"),
			wantErr: config.ErrInvalidConfigProperty,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker := newServiceDocker(t, testAPIVersion)
			services := testServices()
			services[0].BindMounts = []config.BindMountConfig{
				{
					Source:   tt.source,
					Target:   "/exports",
					Writable: true,
				},
			}
			taskRuntime, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), TaskRuntimeConfig{
				Services:         services,
				RequiredServices: []string{"world"},
			})

			require.ErrorIs(t, err, ErrTaskRuntimeConfig)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, err.Error(), strconv.Quote(tt.source))
			assert.Nil(t, taskRuntime)
			assert.Empty(t, docker.snapshot().createdNetworkIDs)
			assert.Empty(t, docker.snapshot().createdIDs)
		})
	}
	assert.NoFileExists(t, missing)
	assert.NoDirExists(t, missing)
	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(contents))
}

func TestDockerToolExecutorBindMounts(t *testing.T) {
	tests := []struct {
		name     string
		writable bool
	}{
		{
			name: "read only by default",
		},
		{
			name:     "writable",
			writable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := t.TempDir()
			marker := filepath.Join(source, "keep.txt")
			require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o600))
			docker := testutils.NewDockerFake(t, testutils.DockerFakeResult{Stdout: `{"ok":true}`})
			executor, err := NewDockerToolExecutor(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() {
				assert.NoError(t, executor.Close())
				assert.FileExists(t, marker)
			})

			toolConfig := config.ToolConfig{
				Name:  "export",
				Image: "tool:test",
				BindMounts: []config.BindMountConfig{
					{
						Source:   source,
						Target:   "/exports",
						Writable: tt.writable,
					},
				},
			}
			executor.RegisterTool(NewDockerTool(&toolConfig, nil, nil, nil, nil))
			result, err := executor.ExecuteTool(t.Context(), testutils.NewTestLogger(t), toolConfig.Name, json.RawMessage(`{}`), nil, nil)
			require.NoError(t, err)
			assert.JSONEq(t, `{"ok":true}`, string(result))

			containers := docker.Containers()
			require.Len(t, containers, 1)
			require.Len(t, containers[0].Mounts, 1)
			mounted, ok := containers[0].Mounts["/exports"]
			require.True(t, ok)
			assert.Equal(t, source, mounted.DirectorySource)
			assert.Equal(t, !tt.writable, mounted.ReadOnly)
			contents, err := os.ReadFile(marker)
			require.NoError(t, err)
			assert.Equal(t, "keep", string(contents))
		})
	}
}

// The CI smoke uses a preloaded BusyBox image and a real daemon. No model or
// provider credentials are involved. It can be run locally with the same env var.
func TestDockerServiceFlushesBindMount(t *testing.T) {
	image := os.Getenv("MINDTRIAL_TEST_BUSYBOX_IMAGE")
	if image == "" {
		t.Skip("set MINDTRIAL_TEST_BUSYBOX_IMAGE to a locally available BusyBox image")
	}
	root := t.TempDir()
	service := config.ServiceConfig{
		Name:  "writer",
		Image: image,
		Endpoint: &config.ServiceEndpointConfig{
			Port: 8000,
		},
		BindMounts: []config.BindMountConfig{
			{
				Source:   root,
				Target:   "/exports",
				Writable: true,
			},
		},
		InputEnv:    map[string]string{"output": "OUT"},
		Command:     []string{"sh", "-ec", `trap 'printf closed > "$OUT.closed"; exit 0' TERM; touch "$OUT.ready"; while :; do sleep 0.1; done`},
		Healthcheck: []string{"sh", "-c", `test -f "$OUT.ready"`},
	}
	cfg := TaskRuntimeConfig{
		Services:         []config.ServiceConfig{service},
		RequiredServices: []string{"writer"},
		ServiceInputs: map[string]map[string]interface{}{
			"writer": {"output": "/exports/{{ .Service.InstanceID }}"},
		},
	}
	for range 2 {
		taskRuntime, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), cfg)
		require.NoError(t, err)
		require.NoError(t, taskRuntime.Close(t.Context()))
	}
	files, err := filepath.Glob(filepath.Join(root, "*.closed"))
	require.NoError(t, err)
	require.Len(t, files, 2)
	for _, file := range files {
		contents, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, "closed", string(contents))
	}
}
