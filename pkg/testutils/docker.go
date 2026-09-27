// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package testutils

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// DockerFakeResult configures how a DockerFake runs every container.
type DockerFakeResult struct {
	// Stdout is written to the standard output of every container.
	Stdout string
	// ExitCode is the exit code of every container.
	ExitCode int
	// Hang keeps every container running until the waiting request is canceled.
	Hang bool
	// CreateError, when set, fails every container creation with this Docker API error message.
	CreateError string
}

// DockerFakeContainer describes a container created through a DockerFake.
type DockerFakeContainer struct {
	// Cmd is the container command.
	Cmd []string
	// Env is the container environment in NAME=value form.
	Env []string
	// NetworkMode is the container network mode.
	NetworkMode string
	// Mounts maps the container path of every bind mount to the mounted file captured at creation.
	Mounts map[string]DockerFakeMount
}

// DockerFakeMount is a file bind mounted into a DockerFake container.
type DockerFakeMount struct {
	// Content is the mounted file content at container creation.
	Content []byte
	// ReadOnly reports whether the mount is read-only.
	ReadOnly bool
}

// DockerFake is a minimal Docker Engine API server for tests of code that runs one-shot
// containers through the Docker client configured from the environment.
type DockerFake struct {
	result DockerFakeResult

	mu         sync.Mutex
	containers []DockerFakeContainer
}

var dockerAPIPathVersion = regexp.MustCompile(`^/v[0-9.]+`)

// NewDockerFake starts a DockerFake and points the Docker client environment of the test at it.
func NewDockerFake(t *testing.T, result DockerFakeResult) *DockerFake {
	t.Helper()
	fake := &DockerFake{result: result}
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)

	t.Setenv("DOCKER_HOST", "tcp://"+server.Listener.Addr().String())
	t.Setenv("DOCKER_API_VERSION", "")
	t.Setenv("DOCKER_CERT_PATH", "")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	return fake
}

// Containers returns the containers created so far, in creation order.
func (f *DockerFake) Containers() []DockerFakeContainer {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]DockerFakeContainer(nil), f.containers...)
}

func (f *DockerFake) handle(w http.ResponseWriter, r *http.Request) {
	requestPath := dockerAPIPathVersion.ReplaceAllString(r.URL.Path, "")
	switch {
	case requestPath == "/_ping":
		w.Header().Set("API-Version", "1.44")
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && requestPath == "/containers/create":
		f.create(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(requestPath, "/start"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && strings.HasSuffix(requestPath, "/wait"):
		if f.result.Hang {
			<-r.Context().Done()
			return
		}
		writeDockerJSON(w, http.StatusOK, map[string]int{"StatusCode": f.result.ExitCode})
	case r.Method == http.MethodGet && strings.HasSuffix(requestPath, "/logs"):
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		header := make([]byte, 8)
		// Multiplexed stream frame: stdout stream type followed by the big-endian payload size.
		header[0] = 1
		binary.BigEndian.PutUint32(header[4:], uint32(len(f.result.Stdout))) //nolint:gosec
		_, _ = w.Write(append(header, f.result.Stdout...))
	case r.Method == http.MethodDelete && strings.HasPrefix(requestPath, "/containers/"):
		w.WriteHeader(http.StatusNoContent)
	default:
		writeDockerJSON(w, http.StatusNotFound, map[string]string{"message": fmt.Sprintf("unsupported fake Docker request: %s %s", r.Method, r.URL.Path)})
	}
}

func (f *DockerFake) create(w http.ResponseWriter, r *http.Request) {
	if f.result.CreateError != "" {
		writeDockerJSON(w, http.StatusNotFound, map[string]string{"message": f.result.CreateError})
		return
	}
	var request struct {
		Cmd        []string `json:"Cmd"`
		Env        []string `json:"Env"`
		HostConfig struct {
			NetworkMode string `json:"NetworkMode"`
			Mounts      []struct {
				Source   string `json:"Source"`
				Target   string `json:"Target"`
				ReadOnly bool   `json:"ReadOnly"`
			} `json:"Mounts"`
		} `json:"HostConfig"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeDockerJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	created := DockerFakeContainer{
		Cmd:         request.Cmd,
		Env:         request.Env,
		NetworkMode: request.HostConfig.NetworkMode,
		Mounts:      make(map[string]DockerFakeMount, len(request.HostConfig.Mounts)),
	}
	for _, mount := range request.HostConfig.Mounts {
		content, err := os.ReadFile(mount.Source)
		if err != nil {
			writeDockerJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		created.Mounts[mount.Target] = DockerFakeMount{Content: content, ReadOnly: mount.ReadOnly}
	}

	f.mu.Lock()
	f.containers = append(f.containers, created)
	id := fmt.Sprintf("fake-%d", len(f.containers))
	f.mu.Unlock()
	writeDockerJSON(w, http.StatusCreated, map[string]string{"Id": id})
}

func writeDockerJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
