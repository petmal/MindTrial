// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serviceDocker is a stateful Docker daemon fake built on dockerAPIMock for task runtime tests.
type serviceDocker struct {
	mu             sync.Mutex
	state          serviceDockerState
	stopped        map[string]bool
	stopAll        bool
	healthSequence []string
	failImage      string
	images         map[string]any // image inspect responses by image ID; missing images are not found
}

// serviceDockerState records the Docker objects and requests seen by serviceDocker.
type serviceDockerState struct {
	networks          map[string]serviceDockerNetwork   // by network ID
	containers        map[string]containerCreatePayload // by container ID
	containerNames    map[string]string                 // by container ID
	createdNetworkIDs []string
	createdIDs        []string
	startedIDs        []string
	removedContainers []string
	removedNetworks   []string
	inspectedImages   []string
	inspectCount      int
	logRequests       int
}

type serviceDockerNetwork struct {
	Name     string `json:"Name"`
	Internal bool   `json:"Internal"`
}

// newServiceDocker starts a fake daemon and points the Docker client environment at it.
func newServiceDocker(t *testing.T, apiVersion string) *serviceDocker {
	mock := newDockerAPIMock(t)
	mock.apiVersion = apiVersion
	t.Setenv("DOCKER_HOST", mock.host())
	t.Setenv("DOCKER_API_VERSION", "")
	t.Setenv("DOCKER_CERT_PATH", "")
	t.Setenv("DOCKER_TLS_VERIFY", "")

	docker := &serviceDocker{
		state: serviceDockerState{
			networks:       make(map[string]serviceDockerNetwork),
			containers:     make(map[string]containerCreatePayload),
			containerNames: make(map[string]string),
		},
		stopped: make(map[string]bool),
	}
	mock.onNetworkCreate = func(w http.ResponseWriter, r *http.Request) {
		var req serviceDockerNetwork
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		docker.mu.Lock()
		id := fmt.Sprintf("net-%d", len(docker.state.createdNetworkIDs)+1)
		docker.state.networks[id] = req
		docker.state.createdNetworkIDs = append(docker.state.createdNetworkIDs, id)
		docker.mu.Unlock()
		writeJSON(t, w, http.StatusCreated, map[string]string{"Id": id})
	}
	mock.onNetworkRemove = func(w http.ResponseWriter, r *http.Request) {
		docker.mu.Lock()
		docker.state.removedNetworks = append(docker.state.removedNetworks, lastPathSegment(r.URL.Path))
		docker.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}
	mock.onCreate = func(w http.ResponseWriter, r *http.Request) {
		var req containerCreatePayload
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		docker.mu.Lock()
		defer docker.mu.Unlock()
		if req.Image == docker.failImage {
			writeJSON(t, w, http.StatusInternalServerError, map[string]string{"message": "create failed"})
			return
		}
		id := fmt.Sprintf("ctr-%d", len(docker.state.createdIDs)+1)
		docker.state.containers[id] = req
		docker.state.containerNames[id] = r.URL.Query().Get("name")
		docker.state.createdIDs = append(docker.state.createdIDs, id)
		writeJSON(t, w, http.StatusCreated, map[string]string{"Id": id})
	}
	mock.onStart = func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, mock.basePath()+"/containers/"), "/start")
		docker.mu.Lock()
		docker.state.startedIDs = append(docker.state.startedIDs, id)
		docker.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}
	mock.onInspect = func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, mock.basePath()+"/containers/"), "/json")
		docker.mu.Lock()
		docker.state.inspectCount++
		state := map[string]any{"Running": !docker.stopAll && !docker.stopped[id]}
		if docker.state.containers[id].Healthcheck != nil {
			status := "starting"
			if slices.Contains(docker.state.startedIDs, id) {
				status = "healthy"
				if len(docker.healthSequence) > 0 {
					status, docker.healthSequence = docker.healthSequence[0], docker.healthSequence[1:]
				}
			}
			state["Health"] = map[string]string{"Status": status}
		}
		imageID := testImageID(docker.state.containers[id].Image)
		docker.mu.Unlock()
		writeJSON(t, w, http.StatusOK, map[string]any{"Id": id, "Image": imageID, "State": state})
	}
	mock.onImageInspect = func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, mock.basePath()+"/images/"), "/json")
		docker.mu.Lock()
		docker.state.inspectedImages = append(docker.state.inspectedImages, id)
		inspected, ok := docker.images[id]
		docker.mu.Unlock()
		if !ok {
			writeJSON(t, w, http.StatusNotFound, map[string]string{"message": "no such image"})
			return
		}
		writeJSON(t, w, http.StatusOK, inspected)
	}
	mock.onWait = func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]int{"StatusCode": 0})
	}
	mock.onLogs = func(w http.ResponseWriter, _ *http.Request) {
		docker.mu.Lock()
		docker.state.logRequests++
		docker.mu.Unlock()
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		_, _ = w.Write(encodeDockerFrames(dockerLogFrame{Stream: 1, Data: `{"status":"ok"}`}))
	}
	mock.onRemove = func(w http.ResponseWriter, r *http.Request) {
		docker.mu.Lock()
		docker.state.removedContainers = append(docker.state.removedContainers, lastPathSegment(r.URL.Path))
		docker.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}
	return docker
}

// testImageID returns the fake content-addressable image ID of an image reference.
func testImageID(imageReference string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(imageReference)))
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	assert.NoError(t, json.NewEncoder(w).Encode(value))
}

func lastPathSegment(urlPath string) string {
	return urlPath[strings.LastIndex(urlPath, "/")+1:]
}

// containerByImage returns the ID and create payload of the only container created from image.
func (d *serviceDocker) containerByImage(t *testing.T, image string) (string, containerCreatePayload) {
	t.Helper()
	state := d.snapshot()
	var ids []string
	for _, id := range state.createdIDs {
		if state.containers[id].Image == image {
			ids = append(ids, id)
		}
	}
	require.Len(t, ids, 1, "containers created from image %q", image)
	return ids[0], state.containers[ids[0]]
}

func (d *serviceDocker) stop(containerID string) {
	d.configure(func() { d.stopped[containerID] = true })
}

// configure changes fake daemon behavior while no request handler is running.
func (d *serviceDocker) configure(change func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	change()
}

func (d *serviceDocker) snapshot() serviceDockerState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return serviceDockerState{
		networks:          maps.Clone(d.state.networks),
		containers:        maps.Clone(d.state.containers),
		containerNames:    maps.Clone(d.state.containerNames),
		createdNetworkIDs: slices.Clone(d.state.createdNetworkIDs),
		createdIDs:        slices.Clone(d.state.createdIDs),
		startedIDs:        slices.Clone(d.state.startedIDs),
		removedContainers: slices.Clone(d.state.removedContainers),
		removedNetworks:   slices.Clone(d.state.removedNetworks),
		inspectedImages:   slices.Clone(d.state.inspectedImages),
		inspectCount:      d.state.inspectCount,
		logRequests:       d.state.logRequests,
	}
}

func testServices() []config.ServiceConfig {
	return []config.ServiceConfig{
		{
			Name:    "world",
			Image:   "world:latest",
			Command: []string{"world", "serve"},
			Env:     map[string]string{"WORLD_MODE": "benchmark"},
			Endpoint: &config.ServiceEndpointConfig{
				Port: 8000,
			},
			InputEnv: map[string]string{
				"seed":  "WORLD_SEED",
				"rooms": "WORLD_ROOMS",
			},
			Healthcheck:    []string{"world", "health"},
			StartupTimeout: testutils.Ptr(90 * time.Second),
			MaxMemoryMB:    testutils.Ptr(512),
			CpuPercent:     testutils.Ptr(25),
		},
		{
			Name:  "shop",
			Image: "shop:latest",
			Endpoint: &config.ServiceEndpointConfig{
				Scheme: "https",
				Port:   8443,
			},
		},
	}
}

func testTemplateData() ExecutionTemplateData {
	return ExecutionTemplateData{
		Evaluation: EvaluationTemplateData{Seed: "eval-seed"},
		Task:       NameTemplateData{Name: "task-a"},
		Provider:   NameTemplateData{Name: "provider-a"},
		Run:        NameTemplateData{Name: "run-a"},
	}
}

func newTestTaskRuntime(t *testing.T, requiredServices ...string) *TaskRuntime {
	t.Helper()
	taskRuntime, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), TaskRuntimeConfig{
		Services:         testServices(),
		RequiredServices: requiredServices,
		ServiceInputs: map[string]map[string]interface{}{
			"world": {"seed": "{{ hash .Evaluation.Seed .Task.Name }}", "rooms": 6},
		},
		TemplateData: testTemplateData(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, taskRuntime.Close(context.WithoutCancel(t.Context())))
	})
	return taskRuntime
}

func TestNewTaskRuntimeValidatesConfigurationBeforeDockerSetup(t *testing.T) {
	tests := []struct {
		name             string
		services         []config.ServiceConfig
		requiredServices []string
		serviceInputs    map[string]map[string]interface{}
		wantErrText      string
	}{
		{
			name:             "unknown required service",
			services:         testServices(),
			requiredServices: []string{"missing"},
			wantErrText:      `unknown service "missing"`,
		},
		{
			name:          "unknown input service",
			services:      testServices(),
			serviceInputs: map[string]map[string]interface{}{"missing": {"seed": 1}},
			wantErrText:   `service-inputs references unknown service "missing"`,
		},
		{
			name:          "undeclared input",
			services:      testServices(),
			serviceInputs: map[string]map[string]interface{}{"world": {"unknown": 1}},
			wantErrText:   `service "world" does not declare input "unknown"`,
		},
		{
			name:             "service without endpoint",
			services:         []config.ServiceConfig{{Name: "world", Image: "world:latest"}},
			requiredServices: []string{"world"},
			wantErrText:      `service "world" has no endpoint configured`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker := newServiceDocker(t, testAPIVersion)

			_, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), TaskRuntimeConfig{
				Services:         tt.services,
				RequiredServices: tt.requiredServices,
				ServiceInputs:    tt.serviceInputs,
			})

			require.ErrorIs(t, err, ErrTaskRuntimeConfig)
			assert.Contains(t, err.Error(), tt.wantErrText)
			assert.Empty(t, docker.snapshot().createdNetworkIDs)
		})
	}
}

func TestNewTaskRuntimeRejectsUnsupportedDockerAPI(t *testing.T) {
	docker := newServiceDocker(t, "1.43")

	_, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), TaskRuntimeConfig{
		Services:         testServices(),
		RequiredServices: []string{"shop"},
	})

	require.ErrorIs(t, err, ErrTaskRuntimeUnsupported)
	assert.Contains(t, err.Error(), "Docker Engine 25.0+ (Engine API 1.44+)")
	assert.Empty(t, docker.snapshot().createdNetworkIDs)
}

func TestDockerToolExecutorValidateTaskServiceSupport(t *testing.T) {
	tests := []struct {
		name              string
		daemonAPIVersion  string
		clientAPIOverride string
		wantErr           bool
	}{
		{
			name:             "negotiated API 1.44",
			daemonAPIVersion: "1.44",
		},
		{
			name:             "negotiated API newer than 1.44",
			daemonAPIVersion: "1.47",
		},
		{
			name:             "negotiated API older than 1.44",
			daemonAPIVersion: "1.43",
			wantErr:          true,
		},
		{
			name:              "client pinned to an older API",
			daemonAPIVersion:  "1.47",
			clientAPIOverride: "1.43",
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newServiceDocker(t, tt.daemonAPIVersion)
			t.Setenv("DOCKER_API_VERSION", tt.clientAPIOverride)
			executor, err := NewDockerToolExecutor(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = executor.Close() })

			err = executor.ValidateTaskServiceSupport(t.Context())
			if tt.wantErr {
				require.ErrorIs(t, err, ErrTaskRuntimeUnsupported)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewTaskRuntimeStartsEachServiceOnItsOwnInternalNetwork(t *testing.T) {
	docker := newServiceDocker(t, testAPIVersion)
	taskRuntime := newTestTaskRuntime(t, "world", "shop")

	worldID, world := docker.containerByImage(t, "world:latest")
	shopID, shop := docker.containerByImage(t, "shop:latest")
	state := docker.snapshot()
	require.Len(t, state.createdNetworkIDs, 2)
	networkNames := make([]string, 0, len(state.createdNetworkIDs))
	for _, id := range state.createdNetworkIDs {
		network := state.networks[id]
		assert.True(t, network.Internal, "service networks must not route outside the host")
		assert.True(t, strings.HasPrefix(network.Name, "mindtrial-service-network-"))
		networkNames = append(networkNames, network.Name)
	}
	assert.NotEqual(t, networkNames[0], networkNames[1], "each service needs its own network")

	expectedSeed, err := utils.ExpandTemplate("seed", `{{ hash "eval-seed" "task-a" }}`, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"world", "serve"}, world.Cmd)
	assert.ElementsMatch(t, []string{"WORLD_MODE=benchmark", "WORLD_ROOMS=6", "WORLD_SEED=" + expectedSeed}, world.Env)
	assert.Equal(t, networkNames[0], world.HostConfig.NetworkMode)
	assert.Equal(t, networkNames[1], shop.HostConfig.NetworkMode)
	require.Len(t, world.NetworkingConfig.EndpointsConfig, 1)
	worldHost := serviceAlias(t, world, networkNames[0])
	assert.Regexp(t, `^[a-z0-9-]+$`, worldHost, "the service host must be a valid DNS label")
	require.Len(t, shop.NetworkingConfig.EndpointsConfig, 1)
	assert.Contains(t, shop.NetworkingConfig.EndpointsConfig, networkNames[1])
	assert.JSONEq(t, "null", string(world.HostConfig.PortBindings), "service ports must not be published to the host")
	assert.Equal(t, int64(512)*1024*1024, world.HostConfig.Memory)
	assert.Equal(t, int64(runtime.NumCPU())*25*10000000, world.HostConfig.NanoCPUs)

	require.NotNil(t, world.Healthcheck)
	assert.Equal(t, []string{"CMD", "world", "health"}, world.Healthcheck.Test)
	assert.Equal(t, 90*time.Second, world.Healthcheck.StartPeriod)
	assert.Equal(t, serviceHealthcheckStartInterval, world.Healthcheck.StartInterval)
	assert.Equal(t, 90*time.Second, world.Healthcheck.Timeout, "a slow probe may use the whole startup timeout")
	assert.Zero(t, world.Healthcheck.Retries)
	assert.Nil(t, shop.Healthcheck)

	for _, id := range []string{worldID, shopID} {
		assert.True(t, strings.HasPrefix(state.containerNames[id], "mindtrial-service-"))
		assert.NotContains(t, state.containerNames[id], "world")
		assert.NotContains(t, state.containerNames[id], "shop")
	}

	require.NoError(t, taskRuntime.Close(t.Context()))
	closed := docker.snapshot()
	assert.ElementsMatch(t, []string{worldID, shopID}, closed.removedContainers)
	assert.ElementsMatch(t, state.createdNetworkIDs, closed.removedNetworks)

	require.NoError(t, taskRuntime.Close(t.Context()), "Close must be idempotent")
	assert.Equal(t, closed, docker.snapshot())
}

func TestTaskRuntimeConsumersJoinOnlyDeclaredServiceNetworks(t *testing.T) {
	docker := newServiceDocker(t, testAPIVersion)
	taskRuntime := newTestTaskRuntime(t, "world", "shop")
	_, world := docker.containerByImage(t, "world:latest")
	_, shop := docker.containerByImage(t, "shop:latest")
	worldNetwork, shopNetwork := world.HostConfig.NetworkMode, shop.HostConfig.NetworkMode
	worldHost := serviceAlias(t, world, worldNetwork)
	shopHost := serviceAlias(t, shop, shopNetwork)

	executor, err := taskRuntime.NewToolExecutor(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = executor.Close() })

	tools := []config.ToolConfig{
		{
			Name:  "world-client",
			Image: "world-client:latest",
			Dependencies: []config.ServiceDependency{{
				Service: "world",
				Env: map[string]string{
					"WORLD_URL":  "{{ .Endpoint }}",
					"WORLD_ADDR": "{{ .Name }}@{{ .Host }}:{{ .Port }}",
				},
			}},
		},
		{
			Name:  "both-client",
			Image: "both-client:latest",
			Dependencies: []config.ServiceDependency{
				{Service: "world", Env: map[string]string{"WORLD_URL": "{{ .Endpoint }}"}},
				{Service: "shop", Env: map[string]string{"SHOP_URL": "{{ .Endpoint }}"}},
			},
		},
		{
			Name:  "python",
			Image: "python:latest",
		},
	}
	for i := range tools {
		executor.RegisterTool(NewDockerTool(&tools[i], nil, nil, nil, nil))
		_, err := executor.ExecuteTool(t.Context(), testutils.NewTestLogger(t), tools[i].Name, json.RawMessage(`{}`), nil, nil)
		require.NoError(t, err)
	}

	worldClientID, worldClient := docker.containerByImage(t, "world-client:latest")
	assert.Equal(t, worldNetwork, worldClient.HostConfig.NetworkMode)
	assert.Equal(t, []string{worldNetwork}, utils.SortedKeys(worldClient.NetworkingConfig.EndpointsConfig))
	assert.ElementsMatch(t, []string{
		"WORLD_URL=http://" + worldHost + ":8000",
		"WORLD_ADDR=world@" + worldHost + ":8000",
	}, worldClient.Env)
	assert.True(t, strings.HasPrefix(docker.snapshot().containerNames[worldClientID], "mindtrial-tool-"))
	assert.NotContains(t, docker.snapshot().containerNames[worldClientID], "world-client")

	_, bothClient := docker.containerByImage(t, "both-client:latest")
	assert.ElementsMatch(t, []string{worldNetwork, shopNetwork}, utils.SortedKeys(bothClient.NetworkingConfig.EndpointsConfig))
	assert.Contains(t, []string{worldNetwork, shopNetwork}, bothClient.HostConfig.NetworkMode)
	assert.ElementsMatch(t, []string{
		"WORLD_URL=http://" + worldHost + ":8000",
		"SHOP_URL=https://" + shopHost + ":8443",
	}, bothClient.Env)

	_, python := docker.containerByImage(t, "python:latest")
	assert.Equal(t, "none", python.HostConfig.NetworkMode)
	assert.Empty(t, python.NetworkingConfig.EndpointsConfig)
}

// serviceAlias returns the only network alias of a service container on its service network.
func serviceAlias(t *testing.T, service containerCreatePayload, networkName string) string {
	t.Helper()
	endpoint, ok := service.NetworkingConfig.EndpointsConfig[networkName]
	require.True(t, ok, "service must join network %q", networkName)
	require.Len(t, endpoint.Aliases, 1)
	return endpoint.Aliases[0]
}

func TestStandaloneToolExecutorRejectsServiceDependencies(t *testing.T) {
	docker := newServiceDocker(t, testAPIVersion)
	executor, err := NewToolExecutor(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = executor.Close() })

	tool := config.ToolConfig{
		Name:         "world-client",
		Image:        "world-client:latest",
		Dependencies: []config.ServiceDependency{{Service: "world"}},
	}
	executor.RegisterTool(NewDockerTool(&tool, nil, nil, nil, nil))
	_, err = executor.ExecuteTool(t.Context(), testutils.NewTestLogger(t), tool.Name, json.RawMessage(`{}`), nil, nil)

	require.ErrorIs(t, err, ErrToolInternal)
	require.ErrorIs(t, err, ErrTaskRuntimeConfig)
	assert.Contains(t, err.Error(), "no task runtime is active")
	assert.Empty(t, docker.snapshot().createdIDs)
}

func TestNewTaskRuntimeCleansUpAfterPartialStartupFailure(t *testing.T) {
	docker := newServiceDocker(t, testAPIVersion)
	docker.configure(func() { docker.failImage = "world:latest" })

	_, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), TaskRuntimeConfig{
		Services:         testServices(),
		RequiredServices: []string{"shop", "world"},
	})

	require.ErrorIs(t, err, ErrTaskRuntimeInternal)
	state := docker.snapshot()
	require.Len(t, state.createdIDs, 1, "only the first service container was created")
	assert.Equal(t, state.createdIDs, state.removedContainers)
	require.Len(t, state.createdNetworkIDs, 2)
	assert.ElementsMatch(t, state.createdNetworkIDs, state.removedNetworks)
}

func TestNewTaskRuntimeServiceReadiness(t *testing.T) {
	tests := []struct {
		name           string
		service        string
		startupTimeout time.Duration
		healthSequence []string
		stopped        bool
		wantErr        error
		wantErrMessage string
		minInspects    int
	}{
		{
			name:    "service without healthcheck must be running",
			service: "shop",
			stopped: true,
			wantErr: ErrTaskRuntimeExecution,
		},
		{
			name:    "running service without healthcheck is ready",
			service: "shop",
		},
		{
			name:           "service with healthcheck waits until healthy",
			service:        "world",
			healthSequence: []string{"starting", "starting", "healthy"},
			minInspects:    3,
		},
		{
			name:    "service with healthcheck stopping before healthy",
			service: "world",
			stopped: true,
			wantErr: ErrTaskRuntimeExecution,
		},
		{
			name:           "service with healthcheck not healthy within its startup timeout",
			service:        "world",
			startupTimeout: 300 * time.Millisecond,
			healthSequence: slices.Repeat([]string{"starting"}, 1000),
			wantErr:        ErrTaskRuntimeExecution,
			wantErrMessage: "did not become healthy within 300ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker := newServiceDocker(t, testAPIVersion)
			docker.configure(func() {
				docker.healthSequence = tt.healthSequence
				docker.stopAll = tt.stopped
			})
			services := testServices()
			if tt.startupTimeout != 0 {
				services[0].StartupTimeout = &tt.startupTimeout
			}

			taskRuntime, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), TaskRuntimeConfig{
				Services:         services,
				RequiredServices: []string{tt.service},
			})

			state := docker.snapshot()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.ErrorContains(t, err, tt.wantErrMessage)
				assert.Positive(t, state.logRequests, "service output is logged for diagnostics")
				assert.Equal(t, state.createdIDs, state.removedContainers)
				assert.ElementsMatch(t, state.createdNetworkIDs, state.removedNetworks)
				return
			}
			require.NoError(t, err)
			assert.GreaterOrEqual(t, state.inspectCount, tt.minInspects)
			require.NoError(t, taskRuntime.Close(t.Context()))
		})
	}
}

func TestTaskRuntimeRejectsStoppedServiceForLaterConsumers(t *testing.T) {
	docker := newServiceDocker(t, testAPIVersion)
	taskRuntime := newTestTaskRuntime(t, "world")
	worldID, _ := docker.containerByImage(t, "world:latest")
	docker.stop(worldID)

	executor, err := taskRuntime.NewToolExecutor(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = executor.Close() })
	tool := config.ToolConfig{
		Name:         "world-client",
		Image:        "world-client:latest",
		Dependencies: []config.ServiceDependency{{Service: "world"}},
	}
	executor.RegisterTool(NewDockerTool(&tool, nil, nil, nil, nil))
	_, err = executor.ExecuteTool(t.Context(), testutils.NewTestLogger(t), tool.Name, json.RawMessage(`{}`), nil, nil)

	require.ErrorIs(t, err, ErrToolInternal)
	require.ErrorIs(t, err, ErrTaskRuntimeExecution)
	state := docker.snapshot()
	assert.Len(t, state.createdIDs, 1, "no consumer container is created for a stopped service")
	assert.Positive(t, state.logRequests, "service output is logged for diagnostics")
}

func TestTaskRuntimeDependencyErrorsPreserveErrorChain(t *testing.T) {
	tests := []struct {
		name       string
		toolEnv    map[string]string
		dependency config.ServiceDependency
	}{
		{
			name:       "malformed dependency template",
			dependency: config.ServiceDependency{Service: "world", Env: map[string]string{"WORLD_URL": "{{ .Endpoint"}},
		},
		{
			name:       "unknown dependency template field",
			dependency: config.ServiceDependency{Service: "world", Env: map[string]string{"WORLD_URL": "{{ .Missing }}"}},
		},
		{
			name:       "service not started for the attempt",
			dependency: config.ServiceDependency{Service: "shop"},
		},
		{
			name:       "tool environment conflicts with a dependency value",
			toolEnv:    map[string]string{"WORLD_URL": "http://elsewhere:8000"},
			dependency: config.ServiceDependency{Service: "world", Env: map[string]string{"WORLD_URL": "{{ .Endpoint }}"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newServiceDocker(t, testAPIVersion)
			taskRuntime := newTestTaskRuntime(t, "world")
			executor, err := taskRuntime.NewToolExecutor(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = executor.Close() })

			tool := config.ToolConfig{
				Name:         "client",
				Image:        "client:latest",
				Env:          tt.toolEnv,
				Dependencies: []config.ServiceDependency{tt.dependency},
			}
			executor.RegisterTool(NewDockerTool(&tool, nil, nil, nil, nil))
			_, err = executor.ExecuteTool(t.Context(), testutils.NewTestLogger(t), tool.Name, json.RawMessage(`{}`), nil, nil)

			require.ErrorIs(t, err, ErrToolInternal)
			require.ErrorIs(t, err, ErrTaskRuntimeConfig)
			assert.NotErrorIs(t, err, ErrTaskRuntimeExecution)
		})
	}
}

func TestIsTaskRuntimeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error"},
		{name: "ordinary tool failure", err: fmt.Errorf("%w: tool exited with code 1", ErrToolInternal)},
		{name: "invalid runtime configuration", err: fmt.Errorf("%w: %w", ErrToolInternal, ErrTaskRuntimeConfig), want: true},
		{name: "unsupported Docker daemon", err: ErrTaskRuntimeUnsupported, want: true},
		{name: "stopped service", err: fmt.Errorf("service %q is unavailable: %w", "world", ErrTaskRuntimeExecution), want: true},
		{name: "runtime lifecycle failure", err: ErrTaskRuntimeInternal, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsTaskRuntimeError(tt.err))
		})
	}
}

func TestTaskRuntimeServiceInputTemplateErrors(t *testing.T) {
	newServiceDocker(t, testAPIVersion)

	_, err := NewTaskRuntime(t.Context(), testutils.NewTestLogger(t), TaskRuntimeConfig{
		Services:         testServices(),
		RequiredServices: []string{"world"},
		ServiceInputs: map[string]map[string]interface{}{
			"world": {"seed": "{{ .Evaluation.Missing }}"},
		},
		TemplateData: testTemplateData(),
	})

	require.ErrorIs(t, err, ErrTaskRuntimeConfig)
	assert.Contains(t, err.Error(), `service "world" input "seed"`)
}

func TestNewTaskRuntimeLogsServiceImageIdentity(t *testing.T) {
	tests := []struct {
		name         string
		images       map[string]any
		wantLevel    slog.Level
		wantContains []string
		wantExcludes []string
	}{
		{
			name: "image with registry digest, tags, and labels",
			images: map[string]any{
				testImageID("shop:latest"): map[string]any{
					"Id":          testImageID("shop:latest"),
					"RepoTags":    []string{"shop:latest"},
					"RepoDigests": []string{"registry.example/shop@sha256:feed"},
					"Config": map[string]any{
						"Labels": map[string]string{
							"org.opencontainers.image.revision": "abc123",
							"org.opencontainers.image.version":  "1.2.3",
							"io.buildah.version":                "1.39.0",
						},
					},
				},
			},
			wantLevel: logging.LevelInfo,
			wantContains: []string{
				`service shop: using image "shop:latest" (ID: ` + testImageID("shop:latest"),
				"digests: [registry.example/shop@sha256:feed]",
				"tags: [shop:latest]",
				"OCI labels: [org.opencontainers.image.revision=abc123 org.opencontainers.image.version=1.2.3]",
			},
			wantExcludes: []string{"io.buildah.version"},
		},
		{
			name:         "image inspect failure does not prevent startup",
			wantLevel:    logging.LevelWarn,
			wantContains: []string{`service shop: failed to inspect service image "shop:latest" (ID: ` + testImageID("shop:latest") + ")"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker := newServiceDocker(t, testAPIVersion)
			docker.configure(func() { docker.images = tt.images })
			logger := newRecordingLogger()

			taskRuntime, err := NewTaskRuntime(t.Context(), logger, TaskRuntimeConfig{
				Services:         testServices(),
				RequiredServices: []string{"shop"},
			})
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, taskRuntime.Close(context.WithoutCancel(t.Context()))) })

			assert.Equal(t, []string{testImageID("shop:latest")}, docker.snapshot().inspectedImages, "the image of the created container is inspected")
			message, ok := logger.find(tt.wantContains[0])
			require.True(t, ok, "image identity must be logged")
			assert.Equal(t, tt.wantLevel, message.level)
			for _, want := range tt.wantContains {
				assert.Contains(t, message.text, want)
			}
			for _, unwanted := range tt.wantExcludes {
				assert.NotContains(t, message.text, unwanted)
			}
		})
	}
}

func TestNewTaskRuntimeLogsServiceStartupOutput(t *testing.T) {
	docker := newServiceDocker(t, testAPIVersion)
	logger := newRecordingLogger()
	taskRuntime, err := NewTaskRuntime(t.Context(), logger, TaskRuntimeConfig{
		Services:         testServices(),
		RequiredServices: []string{"shop"},
	})
	require.NoError(t, err)
	shopID, _ := docker.containerByImage(t, "shop:latest")

	assert.Equal(t, 1, docker.snapshot().logRequests, "startup output is read once the service is ready")
	message, ok := logger.find(fmt.Sprintf("service shop: service container %q output (last %d bytes per stream)", shopID, maxServiceDiagnosticLogBytes))
	require.True(t, ok, "startup output must be logged")
	assert.Equal(t, logging.LevelDebug, message.level)
	assert.Contains(t, message.text, `{"status":"ok"}`)

	require.NoError(t, taskRuntime.Close(t.Context()))
	assert.Equal(t, 1, docker.snapshot().logRequests, "service output is not read again at shutdown")
}

// recordedLogMessage is one formatted message captured by recordingLogger.
type recordedLogMessage struct {
	level slog.Level
	text  string
}

// recordingLogger captures formatted log messages, including those of derived loggers.
type recordingLogger struct {
	mu       *sync.Mutex
	messages *[]recordedLogMessage
	prefix   string
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{
		mu:       &sync.Mutex{},
		messages: &[]recordedLogMessage{},
	}
}

func (l *recordingLogger) Message(_ context.Context, level slog.Level, msg string, args ...any) {
	l.record(level, fmt.Sprintf(msg, args...))
}

func (l *recordingLogger) Error(_ context.Context, level slog.Level, err error, msg string, args ...any) {
	l.record(level, fmt.Sprintf(msg, args...)+": "+err.Error())
}

func (l *recordingLogger) WithContext(context string) logging.Logger {
	return &recordingLogger{
		mu:       l.mu,
		messages: l.messages,
		prefix:   l.prefix + context,
	}
}

func (l *recordingLogger) record(level slog.Level, text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.messages = append(*l.messages, recordedLogMessage{level: level, text: l.prefix + text})
}

// find returns the first captured message containing substring.
func (l *recordingLogger) find(substring string) (recordedLogMessage, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, message := range *l.messages {
		if strings.Contains(message.text, substring) {
			return message, true
		}
	}
	return recordedLogMessage{}, false
}
