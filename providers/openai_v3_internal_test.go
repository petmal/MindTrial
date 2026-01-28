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

func TestOpenAIV3_Run_IncompatibleResponseFormat(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAIV3Provider{}
	runCfg := config.RunConfig{
		Name:                    "test-run",
		Model:                   "gpt-test",
		DisableStructuredOutput: true,
		ModelParams: openAIV3ModelParams{
			ResponseFormat: ResponseFormatJSONObject.Ptr(),
		},
	}
	_, err := p.Run(context.Background(), logger, runCfg, config.Task{Name: "t"})
	require.ErrorIs(t, err, ErrIncompatibleResponseFormat)
}

func TestOpenAIV3_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &openAIV3Provider{} // nil client is sufficient to exercise early validation

	runCfg := config.RunConfig{Name: "test-run", Model: "gpt-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}
