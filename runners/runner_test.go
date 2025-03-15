// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package runners

import (
	"context"
	"log"
	"testing"

	"github.com/petmal/mindtrial/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerRun(t *testing.T) {
	type args struct {
		ctx   context.Context
		tasks []config.Task
	}
	tests := []struct {
		name    string
		r       Runner
		args    args
		want    Results
		wantErr bool
	}{
		{
			name: "test results states",
			r:    createMockRunner(t),
			args: args{
				context.Background(),
				[]config.Task{
					{
						Name:           "success",
						ExpectedResult: "Provident quas tenetur repellat deserunt ut neque culpa.",
					},
					{
						Name:           "failure",
						ExpectedResult: "Aperiam assumenda id provident ratione eos molestiae.",
					},
					{
						Name:           "error",
						ExpectedResult: "Doloribus quis incidunt velit quia.",
					},
					{
						Name:           "failure",
						ExpectedResult: "Veritatis aliquid accusantium dolore voluptate optio dolor.",
					},
					{
						Name:           "success",
						ExpectedResult: "Omnis omnis ea quia et ut est.",
					},
				},
			},
			want: Results{
				"mock provider 1": []RunResult{
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "Bacon",
						Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
						Want:     "provident quas tenetur repellat deserunt ut neque culpa.",
						Details:  "success\n\nmock success",
					},
					{
						Kind:     Failure,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "Bacon",
						Got:      "facere aperiam recusandae totam magnam nulla corrupti.",
						Want:     "aperiam assumenda id provident ratione eos molestiae.",
						Details:  "failure\n\nmock failure",
					},
					{
						Kind:     Error,
						Task:     "error",
						Provider: "mock provider 1",
						Run:      "Bacon",
						Want:     "doloribus quis incidunt velit quia.",
						Got:      "mock error",
						Details:  "",
					},
					{
						Kind:     Failure,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "Bacon",
						Got:      "facere aperiam recusandae totam magnam nulla corrupti.",
						Want:     "veritatis aliquid accusantium dolore voluptate optio dolor.",
						Details:  "failure\n\nmock failure",
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "Bacon",
						Got:      "omnis omnis ea quia et ut est.",
						Want:     "omnis omnis ea quia et ut est.",
						Details:  "success\n\nmock success",
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
						Want:     "provident quas tenetur repellat deserunt ut neque culpa.",
						Details:  "success\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "aperiam assumenda id provident ratione eos molestiae.",
						Want:     "aperiam assumenda id provident ratione eos molestiae.",
						Details:  "failure\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "error",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "doloribus quis incidunt velit quia.",
						Want:     "doloribus quis incidunt velit quia.",
						Details:  "error\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "veritatis aliquid accusantium dolore voluptate optio dolor.",
						Want:     "veritatis aliquid accusantium dolore voluptate optio dolor.",
						Details:  "failure\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 1",
						Run:      "pass",
						Got:      "omnis omnis ea quia et ut est.",
						Want:     "omnis omnis ea quia et ut est.",
						Details:  "success\n\nmock pass",
					},
				},
				"mock provider 2": []RunResult{
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
						Want:     "provident quas tenetur repellat deserunt ut neque culpa.",
						Details:  "success\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "aperiam assumenda id provident ratione eos molestiae.",
						Want:     "aperiam assumenda id provident ratione eos molestiae.",
						Details:  "failure\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "error",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "doloribus quis incidunt velit quia.",
						Want:     "doloribus quis incidunt velit quia.",
						Details:  "error\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "failure",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "veritatis aliquid accusantium dolore voluptate optio dolor.",
						Want:     "veritatis aliquid accusantium dolore voluptate optio dolor.",
						Details:  "failure\n\nmock pass",
					},
					{
						Kind:     Success,
						Task:     "success",
						Provider: "mock provider 2",
						Run:      "pass",
						Got:      "omnis omnis ea quia et ut est.",
						Want:     "omnis omnis ea quia et ut est.",
						Details:  "success\n\nmock pass",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Run(tt.args.ctx, tt.args.tasks)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, tt.r.GetResults())
			}
		})
	}
}

func createMockRunner(t *testing.T) Runner {
	return createMockRunnerFromConfig(t, []config.ProviderConfig{
		{
			Name: "mock provider 1",
			Runs: []config.RunConfig{
				{
					Name:                 "Bacon",
					Model:                "microchip",
					MaxRequestsPerMinute: 50,
				},
				{
					Name:  "pass",
					Model: "parsing",
				},
			},
		},
		{
			Name: "mock provider 2",
			Runs: []config.RunConfig{
				{
					Name:  "pass",
					Model: "parsing",
				},
			},
		},
	})
}

func createMockRunnerFromConfig(t *testing.T, cfg []config.ProviderConfig) Runner {
	runner, err := NewDefaultRunner(context.Background(), cfg, log.Default())
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}

	return runner
}

func TestProviderResultsByRunAndKind(t *testing.T) {
	mockResults := Results{
		"mock provider 1": []RunResult{
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 1",
				Run:      "p1r1",
				Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
				Details:  "success\n\nmock success",
			},
			{
				Kind:     Failure,
				Task:     "failure",
				Provider: "mock provider 1",
				Run:      "p1r1",
				Got:      "aperiam assumenda id provident ratione eos molestiae.",
				Details:  "failure\n\nmock failure",
			},
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 1",
				Run:      "p1r1",
				Got:      "autem aspernatur pariatur iure accusamus.",
				Details:  "success\n\nmock success",
			},
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 1",
				Run:      "p1r2",
				Got:      "provident aperiam quaerat.",
				Details:  "success\n\nmock success",
			},
		},
		"mock provider 2": []RunResult{
			{
				Kind:     Error,
				Task:     "error",
				Provider: "mock provider 2",
				Run:      "p2r1",
				Got:      "est expedita id sequi provident aut aut.",
				Details:  "error\n\nmock error",
			},
			{
				Kind:     Failure,
				Task:     "failure",
				Provider: "mock provider 2",
				Run:      "p2r1",
				Got:      "saepe aperiam culpa voluptatem est.",
				Details:  "failure\n\nmock failure",
			},
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 2",
				Run:      "p2r1",
				Got:      "aliquam nesciunt et laboriosam.",
				Details:  "success\n\nmock success",
			},
		},
		"mock provider 3": []RunResult{
			{
				Kind:     Success,
				Task:     "success",
				Provider: "mock provider 3",
				Run:      "p3r2",
				Got:      "consectetur doloremque sit quibusdam.",
				Details:  "success\n\nmock success",
			},
		},
		"mock provider 4": []RunResult{},
	}
	type args struct {
		provider string
	}
	tests := []struct {
		name string
		r    Results
		args args
		want map[string]map[ResultKind][]RunResult
	}{
		{
			name: "multiple runs, multiple results",
			r:    mockResults,
			args: args{
				provider: "mock provider 1",
			},
			want: map[string]map[ResultKind][]RunResult{
				"p1r1": {
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 1",
							Run:      "p1r1",
							Got:      "provident quas tenetur repellat deserunt ut neque culpa.",
							Details:  "success\n\nmock success",
						},
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 1",
							Run:      "p1r1",
							Got:      "autem aspernatur pariatur iure accusamus.",
							Details:  "success\n\nmock success",
						},
					},
					Failure: {
						{
							Kind:     Failure,
							Task:     "failure",
							Provider: "mock provider 1",
							Run:      "p1r1",
							Got:      "aperiam assumenda id provident ratione eos molestiae.",
							Details:  "failure\n\nmock failure",
						},
					},
				},
				"p1r2": {
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 1",
							Run:      "p1r2",
							Got:      "provident aperiam quaerat.",
							Details:  "success\n\nmock success",
						},
					},
				},
			},
		},
		{
			name: "single run, multiple results",
			r:    mockResults,
			args: args{
				provider: "mock provider 2",
			},
			want: map[string]map[ResultKind][]RunResult{
				"p2r1": {
					Error: {
						{
							Kind:     Error,
							Task:     "error",
							Provider: "mock provider 2",
							Run:      "p2r1",
							Got:      "est expedita id sequi provident aut aut.",
							Details:  "error\n\nmock error",
						},
					},
					Failure: {
						{
							Kind:     Failure,
							Task:     "failure",
							Provider: "mock provider 2",
							Run:      "p2r1",
							Got:      "saepe aperiam culpa voluptatem est.",
							Details:  "failure\n\nmock failure",
						},
					},
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 2",
							Run:      "p2r1",
							Got:      "aliquam nesciunt et laboriosam.",
							Details:  "success\n\nmock success",
						},
					},
				},
			},
		},
		{
			name: "single run, single result",
			r:    mockResults,
			args: args{
				provider: "mock provider 3",
			},
			want: map[string]map[ResultKind][]RunResult{
				"p3r2": {
					Success: {
						{
							Kind:     Success,
							Task:     "success",
							Provider: "mock provider 3",
							Run:      "p3r2",
							Got:      "consectetur doloremque sit quibusdam.",
							Details:  "success\n\nmock success",
						},
					},
				},
			},
		},
		{
			name: "no runs, no results",
			r:    mockResults,
			args: args{
				provider: "mock provider 4",
			},
			want: map[string]map[ResultKind][]RunResult{},
		},
		{
			name: "unknown provider",
			r:    mockResults,
			args: args{
				provider: "mock provider 5",
			},
			want: map[string]map[ResultKind][]RunResult{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.ProviderResultsByRunAndKind(tt.args.provider))
		})
	}
}
func TestRunResultGetID(t *testing.T) {
	tests := []struct {
		name      string
		runResult RunResult
		want      string
	}{
		{
			name: "simple case",
			runResult: RunResult{
				Task:     "test-task",
				Provider: "test-provider",
				Run:      "test-run",
			},
			want: "run-test-provider-test-run-test-task",
		},
		{
			name: "with spaces",
			runResult: RunResult{
				Task:     "test task",
				Provider: "test provider",
				Run:      "test run",
			},
			want: "run-test-provider-test-run-test-task",
		},
		{
			name: "with special characters",
			runResult: RunResult{
				Task:     "test!@#$%task",
				Provider: "test&*()provider",
				Run:      "test+=[]{};:'run",
			},
			want: "run-test____provider-test_________run-test_____task",
		},
		{
			name: "with Unicode characters",
			runResult: RunResult{
				Task:     "testλ♥task",
				Provider: "testπøprovider",
				Run:      "test★☆run",
			},
			want: "run-test__provider-test__run-test__task",
		},
		{
			name: "with empty fields",
			runResult: RunResult{
				Task:     "",
				Provider: "",
				Run:      "",
			},
			want: "run---",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.runResult.GetID()
			assert.Equal(t, tt.want, got)
		})
	}
}
