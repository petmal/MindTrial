// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tools

import (
	"github.com/docker/docker/api/types/mount"
	"github.com/petmal/mindtrial/config"
)

// directoryBindMounts only borrows the source. It must not create, chmod or remove it.
func directoryBindMounts(configs []config.BindMountConfig) ([]mount.Mount, error) {
	var mounts []mount.Mount
	for _, mountConfig := range configs {
		if err := mountConfig.ValidateSource(); err != nil {
			return nil, err
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   mountConfig.Source,
			Target:   mountConfig.Target,
			ReadOnly: !mountConfig.Writable,
		})
	}
	return mounts, nil
}
