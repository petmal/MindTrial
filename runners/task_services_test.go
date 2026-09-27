// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package runners

import (
	"cmp"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/providers/execution"
	providertools "github.com/petmal/mindtrial/providers/tools"
	"github.com/petmal/mindtrial/validators"
)

func taskServicesTestServices() []config.ServiceConfig {
	return []config.ServiceConfig{
		{
			Name:     "world",
			Image:    "world:latest",
			Endpoint: &config.ServiceEndpointConfig{Port: 8000},
			InputEnv: map[string]string{"seed": "WORLD_SEED"},
		},
		{
			Name:     "shop",
			Image:    "shop:latest",
			Endpoint: &config.ServiceEndpointConfig{Port: 8080},
			InputEnv: map[string]string{"currency": "SHOP_CURRENCY"},
		},
	}
}

func taskServicesTestTools() []config.ToolConfig {
	return []config.ToolConfig{
		{
			Name:  "world-client",
			Image: "world-client:latest",
			Dependencies: []config.ServiceDependency{{
				Service: "world",
				Env:     map[string]string{"WORLD_URL": "{{ .Endpoint }}"},
			}},
		},
		{
			Name:  "python",
			Image: "python:latest",
		},
	}
}

// newTaskServicesTask resolves a task against an inherited tool selector the way task loading does.
func newTaskServicesTask(t *testing.T, name string, inherited config.ToolSelector, selector *config.ToolSelector) config.Task {
	t.Helper()
	task := config.Task{
		Name:                 name,
		Prompt:               "use the world",
		ResponseResultFormat: config.NewResponseFormat("text"),
		ExpectedResult:       utils.NewValueSet("expected"),
		ToolSelector:         selector,
	}
	require.NoError(t, task.ResolveValidationRules(config.ValidationRules{}))
	task.ResolveToolSelector(inherited)
	return task
}

func TestDefaultRunnerAssertCanRunTaskServices(t *testing.T) {
	inheritedShopInputs := config.ToolSelector{ServiceInputs: map[string]map[string]interface{}{
		"shop": {"currency": "CAD"},
	}}
	worldTools := &config.ToolSelector{Tools: []config.ToolSelection{{Name: "world-client"}}}

	tests := []struct {
		name                  string
		tools                 []config.ToolConfig
		inherited             config.ToolSelector
		selector              *config.ToolSelector
		stub                  *stubToolValidator
		wantErr               error
		wantErrText           string
		wantTaskServiceChecks int
	}{
		{
			name:                  "valid service-backed task",
			tools:                 taskServicesTestTools(),
			selector:              &config.ToolSelector{Tools: worldTools.Tools, ServiceInputs: map[string]map[string]interface{}{"world": {"seed": "{{ hash .Evaluation.Seed .Task.Name }}"}}},
			stub:                  &stubToolValidator{},
			wantTaskServiceChecks: 1,
		},
		{
			name: "missing dependency service",
			tools: []config.ToolConfig{{
				Name:         "world-client",
				Image:        "world-client:latest",
				Dependencies: []config.ServiceDependency{{Service: "missing"}},
			}},
			selector:    worldTools,
			stub:        &stubToolValidator{},
			wantErr:     ErrServiceNotFound,
			wantErrText: "task 'task' tool 'world-client' requires service 'missing'",
		},
		{
			name:        "service inputs for an unknown service",
			tools:       taskServicesTestTools(),
			selector:    &config.ToolSelector{ServiceInputs: map[string]map[string]interface{}{"missing": {"seed": 1}}},
			stub:        &stubToolValidator{},
			wantErr:     ErrServiceNotFound,
			wantErrText: "task 'task' configures service-inputs for 'missing'",
		},
		{
			name:        "undeclared service input",
			tools:       taskServicesTestTools(),
			selector:    &config.ToolSelector{ServiceInputs: map[string]map[string]interface{}{"world": {"rooms": 6}}},
			stub:        &stubToolValidator{},
			wantErr:     ErrInvalidTaskRuntimeConfig,
			wantErrText: "task 'task' configures undeclared input 'rooms' for service 'world'",
		},
		{
			name:      "inherited inputs for a service the task does not use",
			tools:     taskServicesTestTools(),
			inherited: inheritedShopInputs,
			selector:  &config.ToolSelector{Tools: []config.ToolSelection{{Name: "python"}}},
			stub:      &stubToolValidator{},
		},
		{
			name:      "inherited inputs with all tools disabled",
			tools:     taskServicesTestTools(),
			inherited: inheritedShopInputs,
			selector:  &config.ToolSelector{Disabled: testutils.Ptr(true), Tools: worldTools.Tools},
			stub:      &stubToolValidator{},
		},
		{
			name:        "malformed service input template",
			tools:       taskServicesTestTools(),
			selector:    &config.ToolSelector{Tools: worldTools.Tools, ServiceInputs: map[string]map[string]interface{}{"world": {"seed": "{{ hash .Evaluation.Seed"}}},
			stub:        &stubToolValidator{},
			wantErr:     ErrInvalidTaskRuntimeConfig,
			wantErrText: "task 'task' service-inputs world.seed",
		},
		{
			name: "malformed tool dependency template",
			tools: []config.ToolConfig{{
				Name:         "world-client",
				Image:        "world-client:latest",
				Dependencies: []config.ServiceDependency{{Service: "world", Env: map[string]string{"WORLD_URL": "{{ .Endpoint"}}},
			}},
			selector:    worldTools,
			stub:        &stubToolValidator{},
			wantErr:     ErrInvalidTaskRuntimeConfig,
			wantErrText: "task 'task' tool 'world-client' dependency on service 'world' has an invalid environment template for WORLD_URL",
		},
		{
			name:        "missing service image",
			tools:       taskServicesTestTools(),
			selector:    worldTools,
			stub:        &stubToolValidator{imageErr: errors.ErrUnsupported},
			wantErr:     errors.ErrUnsupported,
			wantErrText: "service 'world' cannot be used",
		},
		{
			name:                  "Docker without task service support",
			tools:                 taskServicesTestTools(),
			selector:              worldTools,
			stub:                  &stubToolValidator{taskServiceSupportErr: errors.ErrUnsupported},
			wantErr:               errors.ErrUnsupported,
			wantErrText:           "task services cannot be used",
			wantTaskServiceChecks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &defaultRunner{
				validatorFactory: validators.NewFactory(nil),
				tools:            tt.tools,
				taskRuntime:      taskRuntimeConfig{services: taskServicesTestServices()},
				logger:           zerolog.Nop(),
				toolValidator:    tt.stub,
			}
			task := newTaskServicesTask(t, "task", tt.inherited, tt.selector)

			_, err := runner.Run(t.Context(), []config.Task{task})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), tt.wantErrText)
			} else {
				require.NoError(t, err)
			}
			if tt.wantTaskServiceChecks > 0 || tt.wantErr == nil {
				assert.Equal(t, tt.wantTaskServiceChecks, tt.stub.taskServiceSupportCalls)
			}
		})
	}
}

// newCustomValidatedTask resolves a dynamic task, without expected-result, validated by the named custom validator.
func newCustomValidatedTask(t *testing.T, name string, validatorName string, selector *config.ToolSelector) config.Task {
	t.Helper()
	task := config.Task{
		Name:                 name,
		Prompt:               "win the world",
		ResponseResultFormat: config.NewResponseFormat("winning code"),
		ValidationRules:      &config.ValidationRules{CustomValidator: testutils.Ptr(validatorName)},
		ToolSelector:         selector,
	}
	require.NoError(t, task.ResolveValidationRules(config.ValidationRules{}))
	task.ResolveToolSelector(config.ToolSelector{})
	return task
}

func worldStateValidator() config.ValidatorConfig {
	return config.ValidatorConfig{
		Name:          "world-state",
		Image:         "validator:latest",
		Command:       []string{"validate", "--candidate-file", "/input/candidate"},
		TemplateFiles: []config.ValidatorTemplateFile{{Path: "/input/candidate", Template: "{{ .Candidate.Response }}"}},
		Dependencies: []config.ServiceDependency{{
			Service: "world",
			Env:     map[string]string{"WORLD_URL": "{{ .Endpoint }}"},
		}},
	}
}

func TestDefaultRunnerAssertCanRunCustomValidators(t *testing.T) {
	withValidator := func(update func(*config.ValidatorConfig)) config.ValidatorConfig {
		validator := worldStateValidator()
		update(&validator)
		return validator
	}

	tests := []struct {
		name                  string
		validator             config.ValidatorConfig
		selected              string
		stub                  *stubToolValidator
		wantErr               error
		wantErrText           string
		wantImages            []string
		wantTaskServiceChecks int
	}{
		{
			name:                  "validator backed by a task service",
			validator:             worldStateValidator(),
			stub:                  &stubToolValidator{},
			wantImages:            []string{"validator:latest", "world:latest"},
			wantTaskServiceChecks: 1,
		},
		{
			name:       "validator without services",
			validator:  withValidator(func(v *config.ValidatorConfig) { v.Dependencies = nil }),
			stub:       &stubToolValidator{},
			wantImages: []string{"validator:latest"},
		},
		{
			name:        "unknown validator",
			validator:   worldStateValidator(),
			selected:    "missing",
			stub:        &stubToolValidator{},
			wantErr:     validators.ErrCustomValidatorNotFound,
			wantErrText: "task 'first' requires custom validator 'missing'",
		},
		{
			name:        "missing validator image",
			validator:   worldStateValidator(),
			stub:        &stubToolValidator{imageErr: errors.ErrUnsupported},
			wantErr:     errors.ErrUnsupported,
			wantErrText: "custom validator 'world-state' cannot be used",
		},
		{
			name:        "malformed validator template",
			validator:   withValidator(func(v *config.ValidatorConfig) { v.TemplateFiles[0].Template = "{{ .Candidate.Response" }),
			stub:        &stubToolValidator{},
			wantErr:     validators.ErrCustomValidatorTemplate,
			wantErrText: `custom validator 'world-state': invalid custom validator template: template file "/input/candidate"`,
		},
		{
			name:        "validator dependency on a missing service",
			validator:   withValidator(func(v *config.ValidatorConfig) { v.Dependencies[0].Service = "missing" }),
			stub:        &stubToolValidator{},
			wantErr:     ErrServiceNotFound,
			wantErrText: "task 'first' custom validator 'world-state' requires service 'missing'",
		},
		{
			name:        "malformed validator dependency template",
			validator:   withValidator(func(v *config.ValidatorConfig) { v.Dependencies[0].Env["WORLD_URL"] = "{{ .Endpoint" }),
			stub:        &stubToolValidator{},
			wantErr:     ErrInvalidTaskRuntimeConfig,
			wantErrText: "task 'first' custom validator 'world-state' dependency on service 'world' has an invalid environment template for WORLD_URL",
		},
		{
			name:                  "Docker without task service support",
			validator:             worldStateValidator(),
			stub:                  &stubToolValidator{taskServiceSupportErr: errors.ErrUnsupported},
			wantErr:               errors.ErrUnsupported,
			wantErrText:           "task services cannot be used",
			wantTaskServiceChecks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customValidators := []config.ValidatorConfig{tt.validator}
			runner := &defaultRunner{
				validatorFactory: validators.NewFactoryWithCustomValidators(nil, customValidators),
				tools:            taskServicesTestTools(),
				taskRuntime:      taskRuntimeConfig{services: taskServicesTestServices(), customValidators: customValidators},
				logger:           zerolog.Nop(),
				toolValidator:    tt.stub,
			}
			selected := cmp.Or(tt.selected, tt.validator.Name)
			tasks := []config.Task{
				newCustomValidatedTask(t, "first", selected, nil),
				newCustomValidatedTask(t, "second", selected, nil),
			}

			_, err := runner.Run(t.Context(), tasks)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), tt.wantErrText)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantImages, tt.stub.validatedImages, "every image is checked once")
			}
			if tt.wantTaskServiceChecks > 0 || tt.wantErr == nil {
				assert.Equal(t, tt.wantTaskServiceChecks, tt.stub.taskServiceSupportCalls)
			}
		})
	}
}

func TestRunnerCustomValidation(t *testing.T) {
	tests := []struct {
		name      string
		result    testutils.DockerFakeResult
		wantKind  ResultKind
		wantTitle string
	}{
		{
			name:      "accepted answer",
			result:    testutils.DockerFakeResult{Stdout: `{"correct":true,"title":"Won","explanation":"The world is in a winning state."}`},
			wantKind:  Success,
			wantTitle: "Won",
		},
		{
			name:      "rejected answer",
			result:    testutils.DockerFakeResult{Stdout: `{"correct":false,"title":"Not won","explanation":"The world is not in a winning state."}`},
			wantKind:  Failure,
			wantTitle: "Not won",
		},
		{
			name:      "protocol violation",
			result:    testutils.DockerFakeResult{Stdout: "won"},
			wantKind:  Error,
			wantTitle: "Validation Error",
		},
		{
			name:      "Docker infrastructure error",
			result:    testutils.DockerFakeResult{CreateError: "No such image: validator:latest"},
			wantKind:  Error,
			wantTitle: "Validation Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker := testutils.NewDockerFake(t, tt.result)
			validator := config.ValidatorConfig{
				Name:    "world-state",
				Image:   "validator:latest",
				Command: []string{"validate", "{{ .Candidate.Response }}", "{{ .Provider.Name }}/{{ .Run.Name }}"},
			}
			runner, factory := newTaskServicesRunner(t, []config.ProviderConfig{{
				Name: "mock provider 1",
				Runs: []config.RunConfig{{Name: "custom", Model: "test-model"}},
			}}, "fixed-seed", validator)
			task := newCustomValidatedTask(t, "NX-1", "world-state", &config.ToolSelector{Tools: []config.ToolSelection{{Name: "world-client"}}})

			results, err := runner.Run(t.Context(), []config.Task{task})
			require.NoError(t, err)

			providerResults := results.GetResults()["mock provider 1"]
			require.Len(t, providerResults, 1)
			result := providerResults[0]
			assert.Equal(t, tt.wantKind, result.Kind)
			assert.Equal(t, "NX-1", result.Got)
			assert.Empty(t, result.Want.Values())
			assert.Equal(t, ValidationMethodCustom, result.Details.Validation.Method)
			assert.Empty(t, result.Details.Validation.ToolCalls, "validator runs are not model tool calls")
			assert.Empty(t, result.Details.Answer.ToolCalls, "validator runs are not model tool calls")
			if tt.wantKind == Error {
				assert.Equal(t, tt.wantTitle, result.Details.Error.Title)
				assert.True(t, result.Details.Error.FromValidation)
			} else {
				assert.Equal(t, tt.wantTitle, result.Details.Validation.Title)
			}

			_, environments := factory.snapshot()
			require.Len(t, environments, 1)
			uses, usedAfterClose := environments[0].usage()
			assert.Equal(t, 1, uses, "the validator runs in the task runtime of the successful attempt")
			assert.False(t, usedAfterClose, "the task runtime stays open until validation completes")
			assert.True(t, environments[0].isClosed(), "the task runtime is closed after validation")
			if tt.result.CreateError == "" {
				containers := docker.Containers()
				require.Len(t, containers, 1)
				assert.Equal(t, []string{"validate", "NX-1", "mock provider 1/custom"}, containers[0].Cmd)
			}
		})
	}
}

// recordingTaskRuntimeFactory replaces Docker task runtimes with recording environments.
type recordingTaskRuntimeFactory struct {
	mu           sync.Mutex
	configs      []providertools.TaskRuntimeConfig
	environments []*recordingTaskRuntime
}

func (f *recordingTaskRuntimeFactory) create(_ context.Context, _ logging.Logger, cfg providertools.TaskRuntimeConfig) (execution.ScopedEnvironment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	environment := &recordingTaskRuntime{}
	f.configs = append(f.configs, cfg)
	f.environments = append(f.environments, environment)
	return environment, nil
}

func (f *recordingTaskRuntimeFactory) snapshot() ([]providertools.TaskRuntimeConfig, []*recordingTaskRuntime) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]providertools.TaskRuntimeConfig(nil), f.configs...), append([]*recordingTaskRuntime(nil), f.environments...)
}

type recordingTaskRuntime struct {
	mu             sync.Mutex
	closed         bool
	uses           int
	usedAfterClose bool
}

func (r *recordingTaskRuntime) NewToolExecutor(ctx context.Context) (*providertools.DockerToolExecutor, error) {
	r.mu.Lock()
	r.uses++
	r.usedAfterClose = r.usedAfterClose || r.closed
	r.mu.Unlock()
	return providertools.NewDockerToolExecutor(ctx)
}

func (r *recordingTaskRuntime) Close(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

func (r *recordingTaskRuntime) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

func (r *recordingTaskRuntime) usage() (uses int, usedAfterClose bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.uses, r.usedAfterClose
}

func newTaskServicesRunner(t *testing.T, providerConfigs []config.ProviderConfig, evaluationSeed string, customValidators ...config.ValidatorConfig) (Runner, *recordingTaskRuntimeFactory) {
	t.Helper()
	runner, err := NewDefaultRunnerWithRuntime(t.Context(), providerConfigs, nil, taskServicesTestTools(), TaskRuntimeSettings{
		Services:         taskServicesTestServices(),
		CustomValidators: customValidators,
		EvaluationSeed:   evaluationSeed,
	}, zerolog.New(zerolog.NewTestWriter(t)))
	require.NoError(t, err)
	t.Cleanup(func() { runner.Close(context.WithoutCancel(t.Context())) })

	factory := &recordingTaskRuntimeFactory{}
	concreteRunner := runner.(*defaultRunner)
	concreteRunner.newTaskRuntime = factory.create
	concreteRunner.toolValidator = &stubToolValidator{}
	return runner, factory
}

func TestRunnerTaskRuntimeLifecycle(t *testing.T) {
	runner, factory := newTaskServicesRunner(t, []config.ProviderConfig{{
		Name: "mock provider 1",
		Runs: []config.RunConfig{{
			Name:        "mock",
			Model:       "test-model",
			RetryPolicy: &config.RetryPolicy{MaxRetryAttempts: 1, InitialDelaySeconds: 1},
		}},
	}}, "")
	inherited := config.ToolSelector{ServiceInputs: map[string]map[string]interface{}{
		"shop":  {"currency": "CAD"},
		"world": {"seed": "{{ hash .Evaluation.Seed .Task.Name }}"},
	}}
	retriedTask := newTaskServicesTask(t, "retry_1", inherited, &config.ToolSelector{Tools: []config.ToolSelection{{Name: "world-client"}}})
	plainTask := newTaskServicesTask(t, "plain", inherited, &config.ToolSelector{Tools: []config.ToolSelection{{Name: "python"}}})

	results, err := runner.Run(t.Context(), []config.Task{retriedTask, plainTask})
	require.NoError(t, err)

	providerResults := results.GetResults()["mock provider 1"]
	require.Len(t, providerResults, 2)
	for _, result := range providerResults {
		assert.Equal(t, Success, result.Kind, "task %s", result.Task)
	}

	configs, environments := factory.snapshot()
	require.Len(t, environments, 2, "only the service-backed task creates runtimes, one per attempt")
	for i, environment := range environments {
		assert.True(t, environment.isClosed(), "runtime of attempt %d must be closed", i+1)
		assert.Equal(t, []string{"world"}, configs[i].RequiredServices)
		assert.Equal(t, map[string]map[string]interface{}{
			"world": {"seed": "{{ hash .Evaluation.Seed .Task.Name }}"},
		}, configs[i].ServiceInputs, "inputs of services the task does not use are not passed to the runtime")
		assert.Equal(t, "retry_1", configs[i].TemplateData.Task.Name)
		assert.Equal(t, "mock provider 1", configs[i].TemplateData.Provider.Name)
		assert.Equal(t, "mock", configs[i].TemplateData.Run.Name)
		assert.NotEmpty(t, configs[i].TemplateData.Evaluation.Seed)
	}
	assert.Equal(t, configs[0].TemplateData, configs[1].TemplateData, "a retry recreates the runtime from the same inputs")
}

func TestRunnerEvaluationSeed(t *testing.T) {
	providerConfigs := []config.ProviderConfig{
		{Name: "mock provider 1", Runs: []config.RunConfig{{Name: "custom", Model: "test-model"}}},
		{Name: "mock provider 2", Runs: []config.RunConfig{{Name: "custom", Model: "test-model"}, {Name: "second", Model: "test-model"}}},
	}
	selector := &config.ToolSelector{Tools: []config.ToolSelection{{Name: "world-client"}}}

	evaluationSeeds := func(t *testing.T, runner Runner, factory *recordingTaskRuntimeFactory) []string {
		t.Helper()
		before, _ := factory.snapshot()
		tasks := []config.Task{
			newTaskServicesTask(t, "first", config.ToolSelector{}, selector),
			newTaskServicesTask(t, "second", config.ToolSelector{}, selector),
		}
		results, err := runner.Run(t.Context(), tasks)
		require.NoError(t, err)

		after, _ := factory.snapshot()
		require.Len(t, after, len(before)+6, "every task attempt of every provider run creates a runtime")
		seeds := make(map[string]bool)
		for _, cfg := range after[len(before):] {
			seeds[cfg.TemplateData.Evaluation.Seed] = true
		}
		for _, providerResults := range results.GetResults() {
			for _, result := range providerResults {
				assert.True(t, seeds[result.Evaluation.Seed], "result %s/%s/%s records the evaluation seed", result.Provider, result.Run, result.Task)
			}
		}
		return utils.SortedKeys(seeds)
	}

	t.Run("generated for each evaluation", func(t *testing.T) {
		runner, factory := newTaskServicesRunner(t, providerConfigs, "")

		first := evaluationSeeds(t, runner, factory)
		second := evaluationSeeds(t, runner, factory)

		require.Len(t, first, 1, "all task attempts of one evaluation share the seed")
		require.Len(t, second, 1, "all task attempts of one evaluation share the seed")
		assert.NotEmpty(t, first[0])
		assert.NotEqual(t, first[0], second[0], "a reused runner generates a fresh seed per evaluation")
	})

	t.Run("explicit seed reused", func(t *testing.T) {
		runner, factory := newTaskServicesRunner(t, providerConfigs, "fixed-seed")

		assert.Equal(t, []string{"fixed-seed"}, evaluationSeeds(t, runner, factory))
		assert.Equal(t, []string{"fixed-seed"}, evaluationSeeds(t, runner, factory))
	})
}
