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

func TestMistral_FileUploadNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MistralAI{} // nil client is sufficient to exercise early check

	runCfg := config.RunConfig{Name: "test-run", Model: "mistral-embed"} // non-vision model
	task := config.Task{
		Name:  "with_file",
		Files: []config.TaskFile{mockTaskFile(t, "img.png", "file://img.png", "image/png")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileUploadNotSupported)
}

func TestMistral_FileTypeNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &MistralAI{} // nil client is sufficient to exercise early validation

	// Use a vision-capable model prefix to bypass the isFileUploadSupported() check
	runCfg := config.RunConfig{Name: "test-run", Model: "mistral-large-test"}
	task := config.Task{
		Name:  "bad_file_type",
		Files: []config.TaskFile{mockTaskFile(t, "file.txt", "file://file.txt", "text/plain")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileNotSupported)
}
