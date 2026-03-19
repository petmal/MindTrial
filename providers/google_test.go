// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"testing"

	"github.com/CircleCI-Research/evalbench/config"
	"github.com/CircleCI-Research/evalbench/pkg/testutils"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestGoogle_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &GoogleAI{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "gemini-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}

func TestRemoveFunctionCallFromRequestConfig(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "removes matching function declaration",
			run: func(t *testing.T) {
				config := &genai.GenerateContentConfig{
					ToolConfig: &genai.ToolConfig{
						FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto},
					},
					Tools: []*genai.Tool{
						{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "tool_a"}}},
						{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "tool_b"}}},
					},
				}

				removeFunctionCallFromRequestConfig(config, "tool_a")

				require.Len(t, config.Tools, 2)
				require.NotNil(t, config.ToolConfig)
				require.Empty(t, config.Tools[0].FunctionDeclarations)
				require.Len(t, config.Tools[1].FunctionDeclarations, 1)
				require.Equal(t, "tool_b", config.Tools[1].FunctionDeclarations[0].Name)
			},
		},
		{
			name: "skips nil values and removes only matching names",
			run: func(t *testing.T) {
				config := &genai.GenerateContentConfig{
					ToolConfig: &genai.ToolConfig{
						FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto},
					},
					Tools: []*genai.Tool{
						nil,
						{FunctionDeclarations: []*genai.FunctionDeclaration{nil, {Name: "tool_a"}, {Name: "tool_b"}}},
					},
				}

				removeFunctionCallFromRequestConfig(config, "tool_a")

				require.Len(t, config.Tools, 2)
				require.Nil(t, config.Tools[0])
				require.Len(t, config.Tools[1].FunctionDeclarations, 2)
				require.Nil(t, config.Tools[1].FunctionDeclarations[0])
				require.Equal(t, "tool_b", config.Tools[1].FunctionDeclarations[1].Name)
			},
		},
		{
			name: "unknown function removal is noop",
			run: func(t *testing.T) {
				config := &genai.GenerateContentConfig{
					ToolConfig: &genai.ToolConfig{
						FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto},
					},
					Tools: []*genai.Tool{
						{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "tool_a"}}},
						{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "tool_b"}}},
					},
				}

				removeFunctionCallFromRequestConfig(config, "tool_missing")

				require.Len(t, config.Tools, 2)
				require.NotNil(t, config.ToolConfig)
				require.Len(t, config.Tools[0].FunctionDeclarations, 1)
				require.Equal(t, "tool_a", config.Tools[0].FunctionDeclarations[0].Name)
				require.Len(t, config.Tools[1].FunctionDeclarations, 1)
				require.Equal(t, "tool_b", config.Tools[1].FunctionDeclarations[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
