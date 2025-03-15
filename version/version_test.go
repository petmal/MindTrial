// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package version

import (
	"runtime/debug"
	"sync"
	"testing"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var sourceLock sync.Mutex

func TestName(t *testing.T) {
	assert.Equal(t, "MindTrial", Name)
}

func TestGetVersion(t *testing.T) {
	testutils.SyncCall(&sourceLock, func() {
		originalSource := source
		source = func() debug.Module {
			return debug.Module{
				Version: "driver",
			}
		}
		defer func() { source = originalSource }()
		assert.Equal(t, "driver", GetVersion())
	})
}

func TestGetSource(t *testing.T) {
	testutils.SyncCall(&sourceLock, func() {
		originalSource := source
		source = func() debug.Module {
			return debug.Module{
				Path: "neat-thread.com/necessary-baggy/unaware-polenta",
			}
		}
		defer func() { source = originalSource }()
		assert.Equal(t, "neat-thread.com/necessary-baggy/unaware-polenta", GetSource())
	})
}
