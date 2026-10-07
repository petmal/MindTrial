// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tools

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
)

const (
	// minTaskServiceAPIVersion is the Engine API of Docker Engine 25.0, the first to support
	// healthcheck start intervals and attaching multiple networks at container creation.
	minTaskServiceAPIVersion = "1.44"

	serviceHealthcheckStartInterval = 250 * time.Millisecond
	serviceReadinessPollInterval    = 100 * time.Millisecond
	maxServiceDiagnosticLogBytes    = 8192
	// ociImageLabelPrefix is the namespace of the standard OCI image annotations, such as
	// org.opencontainers.image.revision and org.opencontainers.image.version.
	ociImageLabelPrefix = "org.opencontainers.image."
)

var (
	// ErrTaskRuntimeConfig is returned when task-scoped service configuration is invalid.
	ErrTaskRuntimeConfig = errors.New("invalid task runtime configuration")
	// ErrTaskRuntimeUnsupported is returned when the Docker daemon cannot run task-scoped services.
	ErrTaskRuntimeUnsupported = errors.New("task services are not supported by the Docker daemon")
	// ErrTaskRuntimeExecution is returned when a service does not become ready or stops unexpectedly.
	ErrTaskRuntimeExecution = errors.New("task runtime execution failed")
	// ErrTaskRuntimeInternal is returned for Docker and runtime lifecycle failures.
	ErrTaskRuntimeInternal = errors.New("task runtime internal error")
)

// IsTaskRuntimeError reports whether err is a task runtime failure, which a model cannot resolve
// by changing its tool calls.
func IsTaskRuntimeError(err error) bool {
	return errors.Is(err, ErrTaskRuntimeConfig) ||
		errors.Is(err, ErrTaskRuntimeUnsupported) ||
		errors.Is(err, ErrTaskRuntimeExecution) ||
		errors.Is(err, ErrTaskRuntimeInternal)
}

// ExecutionTemplateData identifies the evaluation, task, provider, and run of a task attempt in templates.
type ExecutionTemplateData struct {
	// Evaluation describes the evaluation invocation.
	Evaluation EvaluationTemplateData
	// Task identifies the executed task.
	Task NameTemplateData
	// Provider identifies the provider executing the task.
	Provider NameTemplateData
	// Run identifies the provider run configuration executing the task.
	Run NameTemplateData
}

// EvaluationTemplateData describes one evaluation invocation.
type EvaluationTemplateData struct {
	// Seed is shared by every task attempt of the evaluation invocation.
	Seed string
}

// NameTemplateData exposes the configured name of a task, provider, or run.
type NameTemplateData struct {
	// Name is the configured name.
	Name string
}

// TaskRuntimeConfig describes the services of one provider task attempt.
type TaskRuntimeConfig struct {
	// Services lists the available service definitions.
	Services []config.ServiceConfig
	// RequiredServices names the services started for the attempt.
	RequiredServices []string
	// ServiceInputs supplies task inputs keyed by service name and input name.
	ServiceInputs map[string]map[string]interface{}
	// TemplateData is exposed to string service-input templates.
	TemplateData ExecutionTemplateData
}

// TaskRuntime owns the Docker services and networks of one provider task attempt.
type TaskRuntime struct {
	client        *client.Client
	serviceInputs map[string]map[string]interface{}
	templateData  ExecutionTemplateData

	mu        sync.Mutex
	instances map[string]serviceInstance
	closed    bool
}

// serviceInstance is a running service; its exported fields form the dependency template context.
type serviceInstance struct {
	Name     string
	Host     string
	Port     int
	Endpoint string

	containerID string
	networkID   string
	networkName string
}

// NewTaskRuntime starts the required services of one provider task attempt, each on its own
// internal network. If any service fails to start, all resources created so far are removed.
func NewTaskRuntime(ctx context.Context, logger logging.Logger, cfg TaskRuntimeConfig) (*TaskRuntime, error) {
	services := make(map[string]config.ServiceConfig, len(cfg.Services))
	for _, service := range cfg.Services {
		services[service.Name] = service
	}
	for serviceName, inputs := range cfg.ServiceInputs {
		service, ok := services[serviceName]
		if !ok {
			return nil, fmt.Errorf("%w: service-inputs references unknown service %q", ErrTaskRuntimeConfig, serviceName)
		}
		for inputName := range inputs {
			if _, ok := service.InputEnv[inputName]; !ok {
				return nil, fmt.Errorf("%w: service %q does not declare input %q", ErrTaskRuntimeConfig, serviceName, inputName)
			}
		}
	}
	for _, name := range cfg.RequiredServices {
		service, ok := services[name]
		if !ok {
			return nil, fmt.Errorf("%w: unknown service %q", ErrTaskRuntimeConfig, name)
		}
		if service.Endpoint == nil {
			return nil, fmt.Errorf("%w: service %q has no endpoint configured", ErrTaskRuntimeConfig, name)
		}
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create Docker client: %w", ErrTaskRuntimeInternal, err)
	}
	taskRuntime := &TaskRuntime{
		client:        cli,
		serviceInputs: cfg.ServiceInputs,
		templateData:  cfg.TemplateData,
		instances:     make(map[string]serviceInstance, len(cfg.RequiredServices)),
	}

	ready := false
	defer func() {
		if !ready {
			if closeErr := taskRuntime.Close(ctx); closeErr != nil {
				logger.Error(ctx, logging.LevelWarn, closeErr, "failed to clean up task runtime after setup failure")
			}
		}
	}()

	if err := checkTaskServiceSupport(ctx, cli); err != nil {
		return nil, err
	}
	for _, name := range cfg.RequiredServices {
		if err := taskRuntime.startService(ctx, logger, services[name]); err != nil {
			return nil, err
		}
	}

	ready = true
	return taskRuntime, nil
}

// checkTaskServiceSupport ensures the negotiated Docker Engine API supports task services.
func checkTaskServiceSupport(ctx context.Context, cli *client.Client) error {
	ping, err := cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("%w: failed to reach the Docker daemon: %w", ErrTaskRuntimeInternal, err)
	}
	cli.NegotiateAPIVersionPing(ping)
	if apiVersion := cli.ClientVersion(); versions.LessThan(apiVersion, minTaskServiceAPIVersion) {
		return fmt.Errorf("%w: task services require Docker Engine 25.0+ (Engine API %s+), but Engine API %s is in use", ErrTaskRuntimeUnsupported, minTaskServiceAPIVersion, apiVersion)
	}
	return nil
}

// NewToolExecutor creates an ephemeral tool executor attached to this task runtime.
func (r *TaskRuntime) NewToolExecutor(ctx context.Context) (*DockerToolExecutor, error) {
	if r == nil {
		return NewDockerToolExecutor(ctx)
	}
	return newDockerToolExecutor(r.client, r), nil
}

func (r *TaskRuntime) startService(ctx context.Context, logger logging.Logger, service config.ServiceConfig) error {
	if _, started := r.instances[service.Name]; started {
		return nil
	}
	serviceLogger := logger.WithContext(fmt.Sprintf("service %s: ", service.Name))
	serviceLogger.Message(ctx, logging.LevelInfo, "starting setup")

	env, err := r.resolveServiceEnv(service)
	if err != nil {
		return err
	}

	instance := serviceInstance{
		Name:        service.Name,
		Host:        serviceNetworkHost(service.Name),
		Port:        service.Endpoint.Port,
		networkName: dockerResourceName("service-network"),
	}
	instance.Endpoint = fmt.Sprintf("%s://%s:%d", service.Endpoint.GetScheme(), instance.Host, instance.Port)

	started := false
	defer func() {
		if !started {
			r.removeServiceResources(ctx, serviceLogger, instance)
		}
	}()

	createdNetwork, err := r.client.NetworkCreate(ctx, instance.networkName, network.CreateOptions{Driver: "bridge", Internal: true})
	if err != nil {
		return fmt.Errorf("%w: failed to create service %q network: %w", ErrTaskRuntimeInternal, service.Name, err)
	}
	instance.networkID = createdNetwork.ID
	serviceLogger.Message(ctx, logging.LevelDebug, "created service network %q (ID: %s)", instance.networkName, instance.networkID)

	serviceLogger.Message(ctx, logging.LevelTrace, "setting environment variable names: %v", utils.SortedKeys(env))
	serviceLogger.Message(ctx, logging.LevelTrace, "setting command: %v", service.Command)
	containerConfig := &container.Config{
		Image: service.Image,
		Cmd:   service.Command,
		Env:   formatEnv(env),
	}
	if len(service.Healthcheck) > 0 {
		containerConfig.Healthcheck = &container.HealthConfig{
			Test: append([]string{"CMD"}, service.Healthcheck...),
			// The startup timeout is the governing deadline: startup probe failures are ignored and a probe may be slow.
			StartPeriod:   service.GetStartupTimeout(),
			StartInterval: serviceHealthcheckStartInterval,
			Timeout:       service.GetStartupTimeout(),
		}
	}
	hostConfig := &container.HostConfig{
		NetworkMode:   container.NetworkMode(instance.networkName),
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		LogConfig:     container.LogConfig{Type: "json-file"},
	}
	applyDockerResourceLimits(hostConfig, service.MaxMemoryMB, service.CpuPercent)
	if service.MaxMemoryMB != nil {
		serviceLogger.Message(ctx, logging.LevelTrace, "setting memory limit to %d MB (%d bytes)", *service.MaxMemoryMB, hostConfig.Memory)
	}
	if service.CpuPercent != nil {
		serviceLogger.Message(ctx, logging.LevelTrace, "setting CPU limit to %d%% (%d NanoCPUs)", *service.CpuPercent, hostConfig.NanoCPUs)
	}
	networkingConfig := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
		instance.networkName: {Aliases: []string{instance.Host}},
	}}

	containerName := dockerResourceName("service")
	created, err := r.client.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("%w: failed to create service %q container: %w", ErrTaskRuntimeInternal, service.Name, err)
	}
	instance.containerID = created.ID
	serviceLogger.Message(ctx, logging.LevelDebug, "created service container %q (ID: %s)", containerName, created.ID)
	r.logServiceImage(ctx, serviceLogger, created.ID, service.Image)

	serviceLogger.Message(ctx, logging.LevelInfo, "starting execution")
	if err := r.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("%w: failed to start service %q: %w", ErrTaskRuntimeInternal, service.Name, err)
	}
	if len(service.Healthcheck) > 0 {
		serviceLogger.Message(ctx, logging.LevelDebug, "waiting up to %s for service healthcheck", service.GetStartupTimeout())
	}
	if err := r.waitForService(ctx, created.ID, service); err != nil {
		r.logServiceDiagnostics(ctx, serviceLogger, logging.LevelWarn, created.ID)
		return fmt.Errorf("service %q failed readiness: %w", service.Name, err)
	}

	r.instances[service.Name] = instance
	started = true
	serviceLogger.Message(ctx, logging.LevelInfo, "service container %q is ready", created.ID)
	r.logServiceDiagnostics(ctx, serviceLogger, logging.LevelDebug, created.ID)
	return nil
}

// logServiceImage logs the identity of the exact image a service container was created from:
// its content-addressable ID, registry digests, tags, and standard OCI labels. Failures are
// logged but do not prevent the service from starting.
func (r *TaskRuntime) logServiceImage(ctx context.Context, logger logging.Logger, containerID string, imageReference string) {
	inspectedContainer, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		logger.Error(ctx, logging.LevelWarn, err, "failed to inspect service container for image identity")
		return
	}
	inspectedImage, err := r.client.ImageInspect(ctx, inspectedContainer.Image)
	if err != nil {
		logger.Error(ctx, logging.LevelWarn, err, "failed to inspect service image %q (ID: %s)", imageReference, inspectedContainer.Image)
		return
	}
	var labels map[string]string
	if inspectedImage.Config != nil {
		labels = inspectedImage.Config.Labels
	}
	logger.Message(ctx, logging.LevelInfo, "using image %q (ID: %s, digests: %v, tags: %v, OCI labels: %v)",
		imageReference, inspectedImage.ID, inspectedImage.RepoDigests, inspectedImage.RepoTags, ociImageLabels(labels))
}

// ociImageLabels returns the standard OCI image labels formatted as sorted key=value pairs.
func ociImageLabels(labels map[string]string) []string {
	var result []string
	for _, key := range utils.SortedKeys(labels) {
		if strings.HasPrefix(key, ociImageLabelPrefix) {
			result = append(result, fmt.Sprintf("%s=%s", key, labels[key]))
		}
	}
	return result
}

func (r *TaskRuntime) resolveServiceEnv(service config.ServiceConfig) (map[string]string, error) {
	env := make(map[string]string, len(service.Env)+len(r.serviceInputs[service.Name]))
	for name, value := range service.Env {
		env[name] = value
	}
	for inputName, rawValue := range r.serviceInputs[service.Name] {
		envName := service.InputEnv[inputName]
		value, err := resolveServiceInput(rawValue, r.templateData)
		if err != nil {
			return nil, fmt.Errorf("%w: service %q input %q: %w", ErrTaskRuntimeConfig, service.Name, inputName, err)
		}
		if existing, ok := env[envName]; ok && existing != value {
			return nil, fmt.Errorf("%w: service %q input %q conflicts with configured environment variable %q", ErrTaskRuntimeConfig, service.Name, inputName, envName)
		}
		env[envName] = value
	}
	return env, nil
}

func resolveServiceInput(value interface{}, data ExecutionTemplateData) (string, error) {
	raw, ok := value.(string)
	if !ok {
		return fmt.Sprint(value), nil
	}
	return utils.ExpandTemplate("service-input", raw, data)
}

func (r *TaskRuntime) waitForService(ctx context.Context, containerID string, service config.ServiceConfig) error {
	if len(service.Healthcheck) == 0 {
		return r.checkServiceRunning(ctx, containerID)
	}

	startupTimeout := service.GetStartupTimeout()
	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	ticker := time.NewTicker(serviceReadinessPollInterval)
	defer ticker.Stop()

	for {
		inspected, err := r.client.ContainerInspect(startupCtx, containerID)
		switch {
		case startupCtx.Err() != nil:
			return serviceReadinessError(startupCtx.Err(), startupTimeout)
		case err != nil:
			return fmt.Errorf("%w: failed to inspect service container: %w", ErrTaskRuntimeInternal, err)
		case inspected.State == nil || !inspected.State.Running:
			return fmt.Errorf("%w: service container stopped before becoming ready", ErrTaskRuntimeExecution)
		case inspected.State.Health != nil && inspected.State.Health.Status == container.Healthy:
			return nil
		}

		select {
		case <-startupCtx.Done():
			return serviceReadinessError(startupCtx.Err(), startupTimeout)
		case <-ticker.C:
		}
	}
}

func serviceReadinessError(err error, startupTimeout time.Duration) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: service readiness interrupted: %w", ErrTaskRuntimeInternal, err)
	}
	return fmt.Errorf("%w: service did not become healthy within %s: %w", ErrTaskRuntimeExecution, startupTimeout, err)
}

func (r *TaskRuntime) checkServiceRunning(ctx context.Context, containerID string) error {
	inspected, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Errorf("%w: failed to inspect service container: %w", ErrTaskRuntimeInternal, err)
	}
	if inspected.State == nil || !inspected.State.Running {
		return fmt.Errorf("%w: service container is not running", ErrTaskRuntimeExecution)
	}
	return nil
}

// logServiceDiagnostics logs bounded service output at the given level; it is never returned in
// errors, which may reach the model through tool results.
func (r *TaskRuntime) logServiceDiagnostics(ctx context.Context, logger logging.Logger, level slog.Level, containerID string) {
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dockerCleanupTimeout)
	defer cancel()

	stdout, stderr, err := readDockerContainerLogs(logCtx, r.client, containerID, "service", "")
	if err != nil {
		logger.Error(ctx, logging.LevelWarn, err, "failed to retrieve service container logs")
		return
	}
	logger.Message(ctx, level, "service container %q output (last %d bytes per stream):\nstdout:\n%s\nstderr:\n%s",
		containerID, maxServiceDiagnosticLogBytes, lastBytes(stdout, maxServiceDiagnosticLogBytes), lastBytes(stderr, maxServiceDiagnosticLogBytes))
}

func lastBytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func (r *TaskRuntime) removeServiceResources(ctx context.Context, logger logging.Logger, instance serviceInstance) {
	if instance.containerID != "" {
		if err := removeDockerContainer(ctx, r.client, instance.containerID); err != nil {
			logger.Error(ctx, logging.LevelWarn, err, "failed to remove service container after setup failure")
		}
	}
	if instance.networkID != "" {
		if err := removeDockerNetwork(ctx, r.client, instance.networkID); err != nil {
			logger.Error(ctx, logging.LevelWarn, err, "failed to remove service network after setup failure")
		}
	}
}

// dependencyBinding is the environment and the service networks a consumer container needs.
type dependencyBinding struct {
	env      map[string]string
	networks []string
}

// bindDependencies resolves consumer environment templates against running services and returns
// the networks of exactly the declared services.
func (r *TaskRuntime) bindDependencies(ctx context.Context, logger logging.Logger, dependencies []config.ServiceDependency) (dependencyBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return dependencyBinding{}, fmt.Errorf("%w: runtime is closed", ErrTaskRuntimeInternal)
	}

	binding := dependencyBinding{env: make(map[string]string)}
	for _, dependency := range dependencies {
		instance, ok := r.instances[dependency.Service]
		if !ok {
			return dependencyBinding{}, fmt.Errorf("%w: service %q was not started for this task attempt", ErrTaskRuntimeConfig, dependency.Service)
		}
		if err := r.checkServiceRunning(ctx, instance.containerID); err != nil {
			r.logServiceDiagnostics(ctx, logger.WithContext(fmt.Sprintf("service %s: ", instance.Name)), logging.LevelWarn, instance.containerID)
			return dependencyBinding{}, fmt.Errorf("service %q is unavailable: %w", instance.Name, err)
		}
		for name, raw := range dependency.Env {
			value, err := utils.ExpandTemplate("service-dependency-env", raw, instance)
			if err != nil {
				return dependencyBinding{}, fmt.Errorf("%w: service %q dependency environment variable %q: %w", ErrTaskRuntimeConfig, instance.Name, name, err)
			}
			if existing, ok := binding.env[name]; ok && existing != value {
				return dependencyBinding{}, fmt.Errorf("%w: dependency environment variable %q resolves to conflicting values", ErrTaskRuntimeConfig, name)
			}
			binding.env[name] = value
		}
		binding.networks = append(binding.networks, instance.networkName)
	}
	return binding, nil
}

// containerNetworking attaches a consumer to its dependency networks, or to no network at all.
func (b dependencyBinding) containerNetworking() (container.NetworkMode, *network.NetworkingConfig) {
	if len(b.networks) == 0 {
		return container.NetworkMode(network.NetworkNone), nil
	}
	endpoints := make(map[string]*network.EndpointSettings, len(b.networks))
	for _, networkName := range b.networks {
		endpoints[networkName] = &network.EndpointSettings{}
	}
	return container.NetworkMode(b.networks[0]), &network.NetworkingConfig{EndpointsConfig: endpoints}
}

// Close removes all service containers and networks. It is safe to call more than once.
func (r *TaskRuntime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true

	var errs []error
	serviceNames := utils.SortedKeys(r.instances)
	for _, name := range serviceNames {
		if err := removeDockerContainer(ctx, r.client, r.instances[name].containerID); err != nil {
			errs = append(errs, fmt.Errorf("%w: remove service %q container: %w", ErrTaskRuntimeInternal, name, err))
		}
	}
	for _, name := range serviceNames {
		if err := removeDockerNetwork(ctx, r.client, r.instances[name].networkID); err != nil {
			errs = append(errs, fmt.Errorf("%w: remove service %q network: %w", ErrTaskRuntimeInternal, name, err))
		}
	}
	if err := r.client.Close(); err != nil {
		errs = append(errs, fmt.Errorf("%w: close Docker client: %w", ErrTaskRuntimeInternal, err))
	}
	return errors.Join(errs...)
}

func serviceNetworkHost(name string) string {
	digest := sha256.Sum256([]byte(name))
	return fmt.Sprintf("svc-%x", digest[:6])
}
