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

func TestDeepseek_FileUploadNotSupported(t *testing.T) {
	logger := testutils.NewTestLogger(t)
	p := &Deepseek{} // nil client is sufficient to test early error
	runCfg := config.RunConfig{Name: "test-run", Model: "directional"}
	task := config.Task{
		Name:  "with_file",
		Files: []config.TaskFile{mockTaskFile(t, "img.png", "file://img.png", "image/png")},
	}
	_, err := p.Run(context.Background(), logger, runCfg, task)
	require.ErrorIs(t, err, ErrFileUploadNotSupported)
}
