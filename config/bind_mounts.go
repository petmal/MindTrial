// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// BindMountConfig attaches an existing host directory. MindTrial never owns its contents.
type BindMountConfig struct {
	// Source is the host directory, resolved relative to the configuration file when needed.
	Source string `yaml:"source" validate:"required"`
	// Target is the absolute, non-root path inside the container.
	Target string `yaml:"target" validate:"required"`
	// Writable permits writes to the host directory; mounts are read-only by default.
	Writable bool `yaml:"writable,omitempty"`
}

// ValidateSource checks that the resolved host source exists and is a directory.
// Callers should recheck at mount time because the filesystem may change after preflight.
func (c BindMountConfig) ValidateSource() error {
	if !filepath.IsAbs(c.Source) {
		return fmt.Errorf("%w: bind source %q is not an absolute host path", ErrInvalidConfigProperty, c.Source)
	}
	info, err := os.Stat(c.Source)
	if err != nil {
		return fmt.Errorf("%w: bind source %q: %w", ErrInvalidConfigProperty, c.Source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: bind source %q is not a directory", ErrInvalidConfigProperty, c.Source)
	}
	return nil
}

// resolveBindMounts interprets relative sources against config.yaml, as with output-dir.
// Never expand task templates in host paths. Defer filesystem checks until preflight: an
// unrelated configured tool or service need not be available for this evaluation.
func (c *AppConfig) resolveBindMounts(configDir string) error {
	for _, tool := range c.Tools {
		if len(tool.BindMounts) == 0 {
			continue
		}
		reserved := make([]string, 0, 2+len(tool.ParameterFiles))
		reserved = append(reserved, tool.SharedDir, tool.AuxiliaryDir)
		for _, parameterPath := range tool.ParameterFiles {
			reserved = append(reserved, parameterPath)
		}
		if err := resolveMounts(configDir, tool.BindMounts, reserved); err != nil {
			return fmt.Errorf("tool %q: %w", tool.Name, err)
		}
	}
	for _, service := range c.Services {
		if err := resolveMounts(configDir, service.BindMounts, nil); err != nil {
			return fmt.Errorf("service %q: %w", service.Name, err)
		}
	}
	for _, validator := range c.Validators {
		if len(validator.BindMounts) == 0 {
			continue
		}
		var reserved []string
		for _, file := range validator.TemplateFiles {
			reserved = append(reserved, file.Path)
		}
		if err := resolveMounts(configDir, validator.BindMounts, reserved); err != nil {
			return fmt.Errorf("validator %q: %w", validator.Name, err)
		}
	}
	return nil
}

func resolveMounts(configDir string, mounts []BindMountConfig, reserved []string) error {
	for i := range mounts {
		mount := &mounts[i]
		if strings.TrimSpace(mount.Source) == "" || !path.IsAbs(mount.Target) || path.Clean(mount.Target) == "/" {
			return fmt.Errorf("%w: bind-mounts[%d] needs a host source and an absolute non-root container target", ErrInvalidConfigProperty, i)
		}
		mount.Target = path.Clean(mount.Target)
		for _, reservedPath := range reserved {
			if reservedPath == "" {
				continue
			}
			reservedPath = path.Clean(reservedPath)
			if pathContains(reservedPath, mount.Target) || pathContains(mount.Target, reservedPath) {
				return fmt.Errorf("%w: bind target %q overlaps container path %q", ErrInvalidConfigProperty, mount.Target, reservedPath)
			}
		}
		source := mount.Source
		if !filepath.IsAbs(source) {
			source = filepath.Join(configDir, source)
		}
		absoluteSource, err := filepath.Abs(source)
		if err != nil {
			return fmt.Errorf("%w: bind source %q: %w", ErrInvalidConfigProperty, mount.Source, err)
		}
		mount.Source = absoluteSource
		reserved = append(reserved, mount.Target)
	}
	return nil
}

// pathContains compares cleaned POSIX paths at component boundaries.
func pathContains(parent string, child string) bool {
	return parent == child || strings.HasPrefix(child, strings.TrimSuffix(parent, "/")+"/")
}
