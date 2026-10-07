// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tools

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/oklog/ulid/v2"
	"github.com/petmal/mindtrial/pkg/utils"
)

const dockerCleanupTimeout = 15 * time.Second

// dockerResourceName returns a unique Docker object name that never embeds user-configured names.
func dockerResourceName(kind string) string {
	return "mindtrial-" + kind + "-" + strings.ToLower(ulid.Make().String())
}

func removeDockerContainer(ctx context.Context, cli *client.Client, containerID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dockerCleanupTimeout)
	defer cancel()

	err := cli.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{Force: true, RemoveVolumes: true})
	switch {
	case err == nil, errdefs.IsConflict(err), errdefs.IsNotFound(err):
		return nil
	default:
		return err
	}
}

func removeDockerNetwork(ctx context.Context, cli *client.Client, networkID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dockerCleanupTimeout)
	defer cancel()

	if err := cli.NetworkRemove(cleanupCtx, networkID); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

// readDockerContainerLogs returns the demultiplexed container output; a non-empty tail limits it
// to that many trailing lines.
func readDockerContainerLogs(ctx context.Context, cli *client.Client, containerID string, containerKind string, tail string) (stdout string, stderr string, err error) {
	logs, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: tail})
	if err != nil {
		return "", "", fmt.Errorf("failed to get %s container logs: %w", containerKind, err)
	}
	defer logs.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, logs); err != nil {
		return "", "", fmt.Errorf("failed to read %s container output: %w", containerKind, err)
	}
	return stdoutBuf.String(), stderrBuf.String(), nil
}

func formatEnv(values map[string]string) []string {
	keys := utils.SortedKeys(values)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return result
}

func applyDockerResourceLimits(hostConfig *container.HostConfig, maxMemoryMB *int, cpuPercent *int) {
	if maxMemoryMB != nil {
		hostConfig.Memory = int64(*maxMemoryMB) * 1024 * 1024
	}
	if cpuPercent != nil {
		hostConfig.NanoCPUs = int64(runtime.NumCPU()) * int64(*cpuPercent) * 10000000
	}
}
