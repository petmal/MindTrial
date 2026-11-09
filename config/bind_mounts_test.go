// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindMountConfigValidateSource(t *testing.T) {
	directory := t.TempDir()
	file := testutils.CreateMockFile(t, "bind-source-*.txt", []byte("keep"))
	t.Cleanup(func() { assert.NoError(t, os.Remove(file)) })
	tests := []struct {
		name    string
		source  string
		wantErr error
	}{
		{
			name:   "directory",
			source: directory,
		},
		{
			name:    "missing directory",
			source:  filepath.Join(directory, "missing"),
			wantErr: os.ErrNotExist,
		},
		{
			name:    "file",
			source:  file,
			wantErr: ErrInvalidConfigProperty,
		},
		{
			name:    "relative source",
			source:  "relative",
			wantErr: ErrInvalidConfigProperty,
		},
		{
			name:    "empty source",
			wantErr: ErrInvalidConfigProperty,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (BindMountConfig{Source: tt.source}).ValidateSource()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, ErrInvalidConfigProperty)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
	assert.NoDirExists(t, filepath.Join(directory, "missing"))
	assert.FileExists(t, file)
}

func TestBindMountConfigValidateSourceRechecksDirectory(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.Mkdir(source, 0o700))
	mount := BindMountConfig{Source: source}
	require.NoError(t, mount.ValidateSource())
	require.NoError(t, os.Remove(source))
	require.ErrorIs(t, mount.ValidateSource(), os.ErrNotExist)
}
