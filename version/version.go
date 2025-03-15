// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package version provides information about MindTrial including application name, version, and source code repository.
package version

import (
	"runtime/debug"
	"sync"
)

// Name of the application.
const Name string = "MindTrial"

var source = sync.OnceValue(func() debug.Module {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main
	}
	panic("failed to retrieve the main package metadata")
})

// GetVersion returns the version of the application.
func GetVersion() string {
	return source().Version
}

// GetSource returns the source path of the main package.
func GetSource() string {
	return source().Path
}
