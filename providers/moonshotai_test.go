// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/require"
)

func TestMoonshotAI_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MoonshotAI{} // nil client is sufficient to exercise parameter mapping and validation

	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "kimi-test",
		DisableStructuredOutput: true,
		// MoonshotAI does not set ResponseFormat when DisableStructuredOutput is true, so no incompatibility
	}
	task := config.Task{
		Name: "t",
		Files: []config.TaskFile{
			mockTaskFile(t, "test.txt", "file://test.txt", "text/plain"), // Unsupported file type to cause early error
		},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.Error(t, err) // Should error due to unsupported file type
	require.NotErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestMoonshotAI_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MoonshotAI{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "kimi-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}