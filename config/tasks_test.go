// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package config

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestURI_Parse(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantErr   bool
		requireOs string
	}{
		{
			name:    "empty string",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "local file path",
			raw:     "path/to/file.txt",
			wantErr: false,
		},
		{
			name:      "absolute windows path",
			raw:       "D:\\projects\\mindtrial\\data.txt",
			wantErr:   false,
			requireOs: "windows", // NOTE: This test should fail on non-Windows systems.
		},
		{
			name:    "relative windows path",
			raw:     "..\\config\\file.txt",
			wantErr: false,
		},
		{
			name:    "windows path UNC",
			raw:     `\\server\share\file.txt`,
			wantErr: false,
		},
		{
			name:    "file scheme",
			raw:     "file:///path/to/file.txt",
			wantErr: false,
		},
		{
			name:    "http scheme",
			raw:     "http://example.com/file.txt",
			wantErr: false,
		},
		{
			name:    "https scheme",
			raw:     "https://example.com/file.txt",
			wantErr: false,
		},
		{
			name:    "unsupported scheme",
			raw:     "ftp://example.com/file.txt",
			wantErr: true,
		},
		{
			name:    "invalid URI",
			raw:     "://invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u URI
			err := u.Parse(tt.raw)

			if tt.wantErr || (tt.requireOs != "" && tt.requireOs != runtime.GOOS) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidURI)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.raw, u.String())
			}
		})
	}
}

func TestURI_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    string
		wantErr bool
	}{
		{
			name:    "valid local path",
			yaml:    "path/to/file.txt",
			want:    "path/to/file.txt",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			yaml:    "http://example.com/file.txt",
			want:    "http://example.com/file.txt",
			wantErr: false,
		},
		{
			name:    "unsupported scheme",
			yaml:    "ftp://example.com/file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got URI
			err := yaml.Unmarshal([]byte(tt.yaml), &got)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidTaskProperty)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.String())
			}
		})
	}
}

func TestURI_MarshalYAML(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "local file path",
			raw:  "path/to/file.txt",
			want: "path/to/file.txt",
		},
		{
			name: "http URL",
			raw:  "http://example.com/file.txt",
			want: "http://example.com/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u URI
			err := u.Parse(tt.raw)
			require.NoError(t, err)

			result, err := u.MarshalYAML()
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestURI_IsLocalFile(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected bool
	}{
		{
			name:     "empty scheme",
			raw:      "path/to/file.txt",
			expected: true,
		},
		{
			name:     "file scheme",
			raw:      "file:///path/to/file.txt",
			expected: true,
		},
		{
			name:     "http scheme",
			raw:      "http://example.com/file.txt",
			expected: false,
		},
		{
			name:     "https scheme",
			raw:      "https://example.com/file.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u URI
			err := u.Parse(tt.raw)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, u.IsLocalFile())
		})
	}
}

func TestURI_IsRemoteFile(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected bool
	}{
		{
			name:     "empty scheme",
			raw:      "path/to/file.txt",
			expected: false,
		},
		{
			name:     "file scheme",
			raw:      "file:///path/to/file.txt",
			expected: false,
		},
		{
			name:     "http scheme",
			raw:      "http://example.com/file.txt",
			expected: true,
		},
		{
			name:     "https scheme",
			raw:      "https://example.com/file.txt",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u URI
			err := u.Parse(tt.raw)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, u.IsRemoteFile())
		})
	}
}

func TestURI_Path(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		basePath string
		expected string
	}{
		{
			name:     "empty scheme",
			raw:      filepath.Join("path", "to", "file.txt"),
			basePath: "",
			expected: filepath.Join("path", "to", "file.txt"),
		},
		{
			name:     "empty scheme with basePath",
			raw:      filepath.Join("path", "to", "file.txt"),
			basePath: os.TempDir(),
			expected: filepath.Join(os.TempDir(), "path", "to", "file.txt"),
		},
		{
			name:     "file scheme",
			raw:      "file:///path/to/file.txt",
			basePath: "",
			expected: "/path/to/file.txt",
		},
		{
			name:     "http scheme",
			raw:      "http://example.com/file.txt",
			basePath: "",
			expected: "http://example.com/file.txt",
		},
		{
			name:     "absolute path with basePath",
			raw:      filepath.Join(os.TempDir(), "path", "to", "file.txt"),
			basePath: filepath.Join(os.TempDir(), "base"),
			expected: filepath.Join(os.TempDir(), "path", "to", "file.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u URI
			err := u.Parse(tt.raw)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, u.Path(tt.basePath))
		})
	}
}

func TestTaskFile_Validate(t *testing.T) {
	tests := []struct {
		name    string
		file    TaskFile
		errType error
	}{
		{
			name:    "valid local file",
			file:    createMockTaskFile(t, testutils.CreateMockFile(t, "valid-*.txt", []byte("test content")), ""),
			errType: nil,
		},
		{
			name:    "non-existent file",
			file:    createMockTaskFile(t, filepath.Join(os.TempDir(), "nonexistent.txt"), ""),
			errType: ErrAccessFile,
		},
		{
			name:    "directory instead of file",
			file:    createMockTaskFile(t, os.TempDir(), ""),
			errType: ErrAccessFile,
		},
		{
			name:    "remote file (no validation)",
			file:    createMockTaskFile(t, "http://example.com/file.txt", ""),
			errType: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.file.Validate()

			if tt.errType != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTaskFile_Content(t *testing.T) {
	// Create a temporary file
	localFileContent := []byte("local file content")
	localFilePath := testutils.CreateMockFile(t, "local-*.txt", localFileContent)

	remoteContent := []byte("remote file content")
	responses := map[string]testutils.MockHTTPResponse{
		"/success": {
			StatusCode: http.StatusOK,
			Content:    remoteContent,
		},
		"/error": {
			StatusCode: http.StatusNotFound,
		},
		"/timeout": {
			StatusCode: http.StatusOK,
			Content:    remoteContent,
			Delay:      100 * time.Millisecond, // shorter than test timeout but long enough to detect
		},
	}
	server := testutils.CreateMockServer(t, responses)
	defer server.Close()

	successURL := server.URL + "/success"
	errorURL := server.URL + "/error"

	tests := []struct {
		name        string
		file        TaskFile
		wantContent []byte
		wantErr     bool
	}{
		{
			name:        "local file content",
			file:        createMockTaskFile(t, localFilePath, ""),
			wantContent: localFileContent,
			wantErr:     false,
		},
		{
			name:        "remote file content",
			file:        createMockTaskFile(t, successURL, ""),
			wantContent: remoteContent,
			wantErr:     false,
		},
		{
			name:    "remote file error",
			file:    createMockTaskFile(t, errorURL, ""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := tt.file.Content(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantContent, content)
			}
		})
	}
}

func createMockTaskFile(t *testing.T, uri string, mimeType string) (taskFile TaskFile) {
	require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf("name: %s\nuri: %s\ntype: %s", uri, uri, mimeType)), &taskFile))
	return taskFile
}

func TestTaskFile_Base64(t *testing.T) {
	localFileContent := []byte("local file content")
	localFilePath := testutils.CreateMockFile(t, "local-*.txt", localFileContent)

	expectedBase64 := base64.StdEncoding.EncodeToString(localFileContent)

	tests := []struct {
		name        string
		file        TaskFile
		wantContent string
		wantErr     bool
	}{
		{
			name:        "local file to base64",
			file:        createMockTaskFile(t, localFilePath, ""),
			wantContent: expectedBase64,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := tt.file.Base64(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantContent, content)
			}
		})
	}
}

func TestTaskFile_TypeValue(t *testing.T) {
	textContent := []byte("text content")
	textFilePath := testutils.CreateMockFile(t, "test-*.txt", textContent)

	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}
	pngFilePath := testutils.CreateMockFile(t, "test-*.png", pngHeader)

	tests := []struct {
		name         string
		file         TaskFile
		expectedType string
		wantErr      bool
	}{
		{
			name:         "explicit type",
			file:         createMockTaskFile(t, textFilePath, "text/custom"),
			expectedType: "text/custom",
			wantErr:      false,
		},
		{
			name:         "infer from extension",
			file:         createMockTaskFile(t, textFilePath, "text/plain; charset=utf-8"),
			expectedType: "text/plain; charset=utf-8",
			wantErr:      false,
		},
		{
			name:         "infer from content",
			file:         createMockTaskFile(t, pngFilePath, ""),
			expectedType: "image/png",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mimeType, err := tt.file.TypeValue(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedType, mimeType)
			}
		})
	}
}

func TestTaskFile_GetDataURL(t *testing.T) {
	localFileContent := []byte("local file content")
	localFilePath := testutils.CreateMockFile(t, "local-*.txt", localFileContent)

	base64Content := base64.StdEncoding.EncodeToString(localFileContent)
	expectedDataURL := "data:text/plain; charset=utf-8;base64," + base64Content

	tests := []struct {
		name            string
		file            TaskFile
		expectedDataURL string
		wantErr         bool
	}{
		{
			name:            "create data URL from file",
			file:            createMockTaskFile(t, localFilePath, ""),
			expectedDataURL: expectedDataURL,
			wantErr:         false,
		},
		{
			name:            "create data URL with explicit type",
			file:            createMockTaskFile(t, localFilePath, "application/custom"),
			expectedDataURL: "data:application/custom;base64," + base64Content,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataURL, err := tt.file.GetDataURL(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedDataURL, dataURL)
			}
		})
	}
}

func TestTaskFile_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantURI string
		wantErr bool
	}{
		{
			name: "valid task file",
			yaml: `name: image-file
uri: http://example.com/file.txt
type: text/plain`,
			wantURI: "http://example.com/file.txt",
			wantErr: false,
		},
		{
			name: "invalid URI scheme",
			yaml: `name: file
uri: ftp://example.com/file.txt
type: text/plain`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var taskFile TaskFile
			err := yaml.Unmarshal([]byte(tt.yaml), &taskFile)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantURI, taskFile.URI.String())
				// Check that content functions were initialized.
				assert.NotNil(t, taskFile.content)
				assert.NotNil(t, taskFile.base64)
				assert.NotNil(t, taskFile.typeValue)
			}
		})
	}
}

func TestDownloadFile(t *testing.T) {
	expectedContent := []byte("test content")
	responses := map[string]testutils.MockHTTPResponse{
		"/success": {
			StatusCode: http.StatusOK,
			Content:    expectedContent,
		},
		"/error": {
			StatusCode: http.StatusNotFound,
		},
		"/timeout": {
			StatusCode: http.StatusOK,
			Content:    expectedContent,
			Delay:      100 * time.Millisecond, // shorter than test timeout but long enough to detect
		},
	}
	server := testutils.CreateMockServer(t, responses)
	defer server.Close()

	tests := []struct {
		name    string
		url     string
		timeout time.Duration
		want    []byte
		wantErr bool
	}{
		{
			name:    "successful download",
			url:     server.URL + "/success",
			timeout: downloadTimeout,
			want:    expectedContent,
			wantErr: false,
		},
		{
			name:    "error response",
			url:     server.URL + "/error",
			timeout: downloadTimeout,
			wantErr: true,
		},
		{
			name:    "context timeout",
			url:     server.URL + "/timeout",
			timeout: 1 * time.Millisecond, // Force timeout
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse(tt.url)
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			content, err := downloadFile(ctx, parsedURL)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrDownloadFile)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, content)
			}
		})
	}
}

func TestTaskConfig_GetEnabledTasks(t *testing.T) {
	tests := []struct {
		name       string
		taskConfig TaskConfig
		want       []Task
	}{
		{
			name: "all tasks enabled",
			taskConfig: TaskConfig{
				Tasks: []Task{
					{
						Name:   "Task 1",
						Prompt: "Prompt 1",
					},
					{
						Name:   "Task 2",
						Prompt: "Prompt 2",
					},
				},
				Disabled: false,
			},
			want: []Task{
				{
					Name:   "Task 1",
					Prompt: "Prompt 1",
				},
				{
					Name:   "Task 2",
					Prompt: "Prompt 2",
				},
			},
		},
		{
			name: "all tasks disabled globally",
			taskConfig: TaskConfig{
				Tasks: []Task{
					{
						Name:   "Task 1",
						Prompt: "Prompt 1",
					},
					{
						Name:   "Task 2",
						Prompt: "Prompt 2",
					},
				},
				Disabled: true,
			},
			want: []Task{},
		},
		{
			name: "specific tasks disabled",
			taskConfig: TaskConfig{
				Tasks: []Task{
					{
						Name:     "Task 1",
						Prompt:   "Prompt 1",
						Disabled: testutils.Ptr(true),
					},
					{
						Name:   "Task 2",
						Prompt: "Prompt 2",
					},
					{
						Name:     "Task 3",
						Prompt:   "Prompt 3",
						Disabled: testutils.Ptr(true),
					},
				},
				Disabled: false,
			},
			want: []Task{
				{
					Name:   "Task 2",
					Prompt: "Prompt 2",
				},
			},
		},
		{
			name: "some tasks override global disabled",
			taskConfig: TaskConfig{
				Tasks: []Task{
					{
						Name:   "Task 1",
						Prompt: "Prompt 1",
					},
					{
						Name:     "Task 2",
						Prompt:   "Prompt 2",
						Disabled: testutils.Ptr(false),
					},
					{
						Name:   "Task 3",
						Prompt: "Prompt 3",
					},
				},
				Disabled: true,
			},
			want: []Task{
				{
					Name:     "Task 2",
					Prompt:   "Prompt 2",
					Disabled: testutils.Ptr(false),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.taskConfig.GetEnabledTasks()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTaskFile_SetBasePath(t *testing.T) {
	mockData := []byte("test content")
	filePath := testutils.CreateMockFile(t, "test-*.txt", mockData)
	fileDir := filepath.Dir(filePath)

	mockFile := createMockTaskFile(t, filepath.Base(filePath), "text/plain")
	assert.Empty(t, mockFile.basePath)

	mockFile.SetBasePath(fileDir)
	assert.Equal(t, fileDir, mockFile.basePath)

	data, err := mockFile.Content(context.Background())
	require.NoError(t, err)
	assert.Equal(t, mockData, data)
}

func TestTask_SetBaseFilePath(t *testing.T) {
	tests := []struct {
		name    string
		task    Task
		errType error
	}{
		{
			name: "valid local file",
			task: Task{
				Files: []TaskFile{
					createMockTaskFile(t, testutils.CreateMockFile(t, "valid-*.txt", []byte("test content")), ""),
					createMockTaskFile(t, testutils.CreateMockFile(t, "valid-*.txt", []byte("test content")), ""),
				},
			},
			errType: nil,
		},
		{
			name: "non-existent file",
			task: Task{
				Files: []TaskFile{
					createMockTaskFile(t, testutils.CreateMockFile(t, "valid-*.txt", []byte("test content")), ""),
					createMockTaskFile(t, filepath.Join(os.TempDir(), "nonexistent.txt"), ""),
				},
			},
			errType: ErrAccessFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.SetBaseFilePath(os.TempDir())

			if tt.errType != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationRulesResolution(t *testing.T) {
	tests := []struct {
		name        string
		globalRules ValidationRules
		taskRules   *ValidationRules
		expected    ValidationRules
	}{
		{
			name: "nil task rules uses global",
			globalRules: ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(false),
			},
			taskRules: nil,
			expected: ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(false),
			},
		},
		{
			name: "empty task rules uses global",
			globalRules: ValidationRules{
				CaseSensitive:    testutils.Ptr(true),
				IgnoreWhitespace: testutils.Ptr(true),
			},
			taskRules: &ValidationRules{},
			expected: ValidationRules{
				CaseSensitive:    testutils.Ptr(true),
				IgnoreWhitespace: testutils.Ptr(true),
			},
		},
		{
			name: "partial task override",
			globalRules: ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(false),
			},
			taskRules: &ValidationRules{
				CaseSensitive: testutils.Ptr(true), // override only this
			},
			expected: ValidationRules{
				CaseSensitive:    testutils.Ptr(true),  // overridden
				IgnoreWhitespace: testutils.Ptr(false), // from global
			},
		},
		{
			name: "complete task override",
			globalRules: ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(false),
			},
			taskRules: &ValidationRules{
				CaseSensitive:    testutils.Ptr(true),
				IgnoreWhitespace: testutils.Ptr(true),
			},
			expected: ValidationRules{
				CaseSensitive:    testutils.Ptr(true),
				IgnoreWhitespace: testutils.Ptr(true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.globalRules.MergeWith(tt.taskRules)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestValidationRules_IsCaseSensitive(t *testing.T) {
	tests := []struct {
		name  string
		rules ValidationRules
		want  bool
	}{
		{
			name:  "nil case sensitive - default false",
			rules: ValidationRules{},
			want:  false,
		},
		{
			name: "explicitly case sensitive true",
			rules: ValidationRules{
				CaseSensitive: testutils.Ptr(true),
			},
			want: true,
		},
		{
			name: "explicitly case sensitive false",
			rules: ValidationRules{
				CaseSensitive: testutils.Ptr(false),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rules.IsCaseSensitive())
		})
	}
}

func TestValidationRules_IsIgnoreWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		rules ValidationRules
		want  bool
	}{
		{
			name:  "nil ignore whitespace - default false",
			rules: ValidationRules{},
			want:  false,
		},
		{
			name: "explicitly ignore whitespace true",
			rules: ValidationRules{
				IgnoreWhitespace: testutils.Ptr(true),
			},
			want: true,
		},
		{
			name: "explicitly ignore whitespace false",
			rules: ValidationRules{
				IgnoreWhitespace: testutils.Ptr(false),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rules.IsIgnoreWhitespace())
		})
	}
}

func TestValidationRules_UseJudge(t *testing.T) {
	tests := []struct {
		name  string
		rules ValidationRules
		want  bool
	}{
		{
			name:  "no judge configuration - default false",
			rules: ValidationRules{},
			want:  false,
		},
		{
			name: "judge enabled false",
			rules: ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(false),
				},
			},
			want: false,
		},
		{
			name: "judge enabled true",
			rules: ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
				},
			},
			want: true,
		},
		{
			name: "judge enabled nil - default false",
			rules: ValidationRules{
				Judge: JudgeSelector{
					Name:    testutils.Ptr("test-judge"),
					Variant: testutils.Ptr("default"),
					// Enabled is nil
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rules.UseJudge())
		})
	}
}

func TestValidationRules_MergeWith_JudgeField(t *testing.T) {
	tests := []struct {
		name     string
		base     ValidationRules
		other    *ValidationRules
		expected ValidationRules
	}{
		{
			name: "merge judge configurations",
			base: ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
					Name:    testutils.Ptr("base-judge"),
					Variant: testutils.Ptr("base-variant"),
				},
			},
			other: &ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(false),
					Name:    testutils.Ptr("other-judge"),
					// Variant not set, should use base
				},
			},
			expected: ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(false),
					Name:    testutils.Ptr("other-judge"),
					Variant: testutils.Ptr("base-variant"),
				},
			},
		},
		{
			name: "nil other preserves base judge",
			base: ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
					Name:    testutils.Ptr("base-judge"),
					Variant: testutils.Ptr("base-variant"),
				},
			},
			other: nil,
			expected: ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
					Name:    testutils.Ptr("base-judge"),
					Variant: testutils.Ptr("base-variant"),
				},
			},
		},
		{
			name: "empty base gets other judge",
			base: ValidationRules{},
			other: &ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
					Name:    testutils.Ptr("other-judge"),
					Variant: testutils.Ptr("other-variant"),
				},
			},
			expected: ValidationRules{
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
					Name:    testutils.Ptr("other-judge"),
					Variant: testutils.Ptr("other-variant"),
				},
			},
		},
		{
			name: "combine all validation rule fields including judge",
			base: ValidationRules{
				CaseSensitive:    testutils.Ptr(false),
				IgnoreWhitespace: testutils.Ptr(false),
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
					Name:    testutils.Ptr("base-judge"),
				},
			},
			other: &ValidationRules{
				CaseSensitive: testutils.Ptr(true),
				Judge: JudgeSelector{
					Variant: testutils.Ptr("other-variant"),
				},
			},
			expected: ValidationRules{
				CaseSensitive:    testutils.Ptr(true),
				IgnoreWhitespace: testutils.Ptr(false),
				Judge: JudgeSelector{
					Enabled: testutils.Ptr(true),
					Name:    testutils.Ptr("base-judge"),
					Variant: testutils.Ptr("other-variant"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.base.MergeWith(tt.other)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJudgeSelector_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		selector JudgeSelector
		want     bool
	}{
		{
			name:     "nil enabled - default false",
			selector: JudgeSelector{},
			want:     false,
		},
		{
			name: "explicitly enabled true",
			selector: JudgeSelector{
				Enabled: testutils.Ptr(true),
			},
			want: true,
		},
		{
			name: "explicitly enabled false",
			selector: JudgeSelector{
				Enabled: testutils.Ptr(false),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.selector.IsEnabled())
		})
	}
}

func TestJudgeSelector_GetName(t *testing.T) {
	tests := []struct {
		name     string
		selector JudgeSelector
		want     string
	}{
		{
			name:     "nil name - returns empty string",
			selector: JudgeSelector{},
			want:     "",
		},
		{
			name: "has name",
			selector: JudgeSelector{
				Name: testutils.Ptr("test-judge"),
			},
			want: "test-judge",
		},
		{
			name: "empty name",
			selector: JudgeSelector{
				Name: testutils.Ptr(""),
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.selector.GetName())
		})
	}
}

func TestJudgeSelector_GetVariant(t *testing.T) {
	tests := []struct {
		name     string
		selector JudgeSelector
		want     string
	}{
		{
			name:     "nil variant - returns empty string",
			selector: JudgeSelector{},
			want:     "",
		},
		{
			name: "has variant",
			selector: JudgeSelector{
				Variant: testutils.Ptr("fast"),
			},
			want: "fast",
		},
		{
			name: "empty variant",
			selector: JudgeSelector{
				Variant: testutils.Ptr(""),
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.selector.GetVariant())
		})
	}
}

func TestJudgeSelector_MergeWith(t *testing.T) {
	tests := []struct {
		name     string
		base     JudgeSelector
		other    JudgeSelector
		expected JudgeSelector
	}{
		{
			name:     "empty base and other",
			base:     JudgeSelector{},
			other:    JudgeSelector{},
			expected: JudgeSelector{},
		},
		{
			name: "base has all fields, other empty",
			base: JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("base-judge"),
				Variant: testutils.Ptr("base-variant"),
			},
			other: JudgeSelector{},
			expected: JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("base-judge"),
				Variant: testutils.Ptr("base-variant"),
			},
		},
		{
			name: "base empty, other has all fields",
			base: JudgeSelector{},
			other: JudgeSelector{
				Enabled: testutils.Ptr(false),
				Name:    testutils.Ptr("other-judge"),
				Variant: testutils.Ptr("other-variant"),
			},
			expected: JudgeSelector{
				Enabled: testutils.Ptr(false),
				Name:    testutils.Ptr("other-judge"),
				Variant: testutils.Ptr("other-variant"),
			},
		},
		{
			name: "other overrides base fields",
			base: JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("base-judge"),
				Variant: testutils.Ptr("base-variant"),
			},
			other: JudgeSelector{
				Enabled: testutils.Ptr(false),
				Name:    testutils.Ptr("other-judge"),
				Variant: testutils.Ptr("other-variant"),
			},
			expected: JudgeSelector{
				Enabled: testutils.Ptr(false),
				Name:    testutils.Ptr("other-judge"),
				Variant: testutils.Ptr("other-variant"),
			},
		},
		{
			name: "partial override - only some fields in other",
			base: JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("base-judge"),
				Variant: testutils.Ptr("base-variant"),
			},
			other: JudgeSelector{
				Enabled: testutils.Ptr(false),
				// Name and Variant not set, should preserve base values.
			},
			expected: JudgeSelector{
				Enabled: testutils.Ptr(false),
				Name:    testutils.Ptr("base-judge"),
				Variant: testutils.Ptr("base-variant"),
			},
		},
		{
			name: "partial override - different combination",
			base: JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("base-judge"),
				Variant: testutils.Ptr("base-variant"),
			},
			other: JudgeSelector{
				Name:    testutils.Ptr("other-judge"),
				Variant: testutils.Ptr("other-variant"),
				// Enabled not set, should preserve base value.
			},
			expected: JudgeSelector{
				Enabled: testutils.Ptr(true),
				Name:    testutils.Ptr("other-judge"),
				Variant: testutils.Ptr("other-variant"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.base.MergeWith(tt.other)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSystemPrompt_MergeWith(t *testing.T) {
	tests := []struct {
		name  string
		base  SystemPrompt
		other *SystemPrompt
		want  SystemPrompt
	}{
		{
			name:  "other nil, base value set",
			base:  SystemPrompt{Template: testutils.Ptr("base")},
			other: nil,
			want:  SystemPrompt{Template: testutils.Ptr("base")},
		},
		{
			name:  "other nil template, base value set",
			base:  SystemPrompt{Template: testutils.Ptr("base")},
			other: &SystemPrompt{},
			want:  SystemPrompt{Template: testutils.Ptr("base")},
		},
		{
			name:  "other empty template, base value set",
			base:  SystemPrompt{Template: testutils.Ptr("base")},
			other: &SystemPrompt{Template: testutils.Ptr("")},
			want:  SystemPrompt{Template: testutils.Ptr("")},
		},
		{
			name:  "base nil template, other set",
			base:  SystemPrompt{},
			other: &SystemPrompt{Template: testutils.Ptr("other")},
			want:  SystemPrompt{Template: testutils.Ptr("other")},
		},
		{
			name:  "base empty template, other nil",
			base:  SystemPrompt{Template: testutils.Ptr("")},
			other: nil,
			want:  SystemPrompt{Template: testutils.Ptr("")},
		},
		{
			name:  "base empty template, other set",
			base:  SystemPrompt{Template: testutils.Ptr("")},
			other: &SystemPrompt{Template: testutils.Ptr("other")},
			want:  SystemPrompt{Template: testutils.Ptr("other")},
		},
		{
			name:  "other set, base value set",
			base:  SystemPrompt{Template: testutils.Ptr("base")},
			other: &SystemPrompt{Template: testutils.Ptr("other")},
			want:  SystemPrompt{Template: testutils.Ptr("other")},
		},
		{
			name:  "EnableFor: other nil, base value set",
			base:  SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
			other: nil,
			want:  SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
		},
		{
			name:  "EnableFor: other nil field, base value set",
			base:  SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
			other: &SystemPrompt{},
			want:  SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
		},
		{
			name:  "EnableFor: base nil field, other set",
			base:  SystemPrompt{},
			other: &SystemPrompt{EnableFor: testutils.Ptr(EnableForNone)},
			want:  SystemPrompt{EnableFor: testutils.Ptr(EnableForNone)},
		},
		{
			name:  "EnableFor: other overrides base",
			base:  SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
			other: &SystemPrompt{EnableFor: testutils.Ptr(EnableForNone)},
			want:  SystemPrompt{EnableFor: testutils.Ptr(EnableForNone)},
		},
		{
			name: "both Template and EnableFor merge",
			base: SystemPrompt{
				Template:  testutils.Ptr("base template"),
				EnableFor: testutils.Ptr(EnableForAll),
			},
			other: &SystemPrompt{
				Template: testutils.Ptr("other template"),
			},
			want: SystemPrompt{
				Template:  testutils.Ptr("other template"),
				EnableFor: testutils.Ptr(EnableForAll),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.base.MergeWith(tt.other))
		})
	}
}

func TestTask_ResolveSystemPrompt(t *testing.T) {
	tests := []struct {
		name          string
		task          Task
		defaultConfig SystemPrompt
		wantPrompt    string
		wantErr       bool
	}{
		{
			name: "task has no system prompt, uses default",
			task: Task{
				ResponseResultFormat: NewResponseFormat("yaml"),
			},
			defaultConfig: SystemPrompt{Template: testutils.Ptr("Default: {{.ResponseResultFormat}}")},
			wantPrompt:    "Default: yaml",
			wantErr:       false,
		},
		{
			name: "task has template, default has template",
			task: Task{
				SystemPrompt:         &SystemPrompt{Template: testutils.Ptr("Task: {{.ResponseResultFormat}}")},
				ResponseResultFormat: NewResponseFormat("json"),
			},
			defaultConfig: SystemPrompt{Template: testutils.Ptr("Default template")},
			wantPrompt:    "Task: json",
			wantErr:       false,
		},
		{
			name: "invalid template syntax",
			task: Task{
				SystemPrompt:         &SystemPrompt{Template: testutils.Ptr("Invalid {{.MissingBrace")},
				ResponseResultFormat: NewResponseFormat("yaml"),
			},
			defaultConfig: SystemPrompt{Template: testutils.Ptr("Default")},
			wantPrompt:    "",
			wantErr:       true,
		},
		{
			name: "unknown template variable",
			task: Task{
				SystemPrompt:         &SystemPrompt{Template: testutils.Ptr("Variable {{.UnknownVariable}}")},
				ResponseResultFormat: NewResponseFormat("xml"),
			},
			defaultConfig: SystemPrompt{Template: testutils.Ptr("Default")},
			wantPrompt:    "",
			wantErr:       true,
		},
		{
			name: "EnableForText with schema format disables template resolution",
			task: Task{
				SystemPrompt: &SystemPrompt{Template: testutils.Ptr("Template: {{.ResponseResultFormat}}")},
				ResponseResultFormat: NewResponseFormat(map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"answer": map[string]interface{}{"type": "string"},
					},
				}),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForText)},
			wantPrompt:    "",
			wantErr:       false,
		},
		{
			name: "EnableForAll with schema format enables template resolution",
			task: Task{
				SystemPrompt: &SystemPrompt{Template: testutils.Ptr("Template: {{.ResponseResultFormat}}")},
				ResponseResultFormat: NewResponseFormat(map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"answer": map[string]interface{}{"type": "string"},
					},
				}),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
			wantPrompt:    "Template: {\n  \"properties\": {\n    \"answer\": {\n      \"type\": \"string\"\n    }\n  },\n  \"type\": \"object\"\n}",
			wantErr:       false,
		},
		{
			name: "EnableForText with string format and no template creates default instruction",
			task: Task{
				ResponseResultFormat: NewResponseFormat("yes/no"),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForText)},
			wantPrompt:    "Provide the final answer in exactly this format: yes/no",
			wantErr:       false,
		},
		{
			name: "EnableForAll with string format and no template creates default instruction",
			task: Task{
				ResponseResultFormat: NewResponseFormat("yes/no"),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
			wantPrompt:    "Provide the final answer in exactly this format: yes/no",
			wantErr:       false,
		},
		{
			name: "EnableForAll with schema format and no template produces empty prompt",
			task: Task{
				ResponseResultFormat: NewResponseFormat(map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"answer": map[string]interface{}{"type": "string"},
					},
				}),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
			wantPrompt:    "",
			wantErr:       false,
		},
		{
			name: "EnableForNone with string format and no template produces empty prompt",
			task: Task{
				ResponseResultFormat: NewResponseFormat("yes/no"),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForNone)},
			wantPrompt:    "",
			wantErr:       false,
		},
		{
			name: "EnableForNone with schema format and no template produces empty prompt",
			task: Task{
				ResponseResultFormat: NewResponseFormat(map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"answer": map[string]interface{}{"type": "string"},
					},
				}),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForNone)},
			wantPrompt:    "",
			wantErr:       false,
		},
		{
			name: "default EnableFor (text) with string format and no template creates default instruction",
			task: Task{
				ResponseResultFormat: NewResponseFormat("yes/no"),
			},
			defaultConfig: SystemPrompt{}, // No EnableFor set, defaults to EnableForText
			wantPrompt:    "Provide the final answer in exactly this format: yes/no",
			wantErr:       false,
		},
		{
			name: "default EnableFor (text) with schema format and no template produces empty prompt",
			task: Task{
				ResponseResultFormat: NewResponseFormat(map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"answer": map[string]interface{}{"type": "string"},
					},
				}),
			},
			defaultConfig: SystemPrompt{}, // No EnableFor set, defaults to EnableForText
			wantPrompt:    "",
			wantErr:       false,
		},
		{
			name: "EnableForText with string format and custom template resolves template",
			task: Task{
				SystemPrompt:         &SystemPrompt{Template: testutils.Ptr("Custom: {{.ResponseResultFormat}}")},
				ResponseResultFormat: NewResponseFormat("answer"),
			},
			defaultConfig: SystemPrompt{EnableFor: testutils.Ptr(EnableForText)},
			wantPrompt:    "Custom: answer",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.task.GetResolvedSystemPrompt()
			require.False(t, ok)

			err := tt.task.ResolveSystemPrompt(tt.defaultConfig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				resolvedPrompt, ok := tt.task.GetResolvedSystemPrompt()
				if tt.wantPrompt == "" {
					assert.False(t, ok)
				} else {
					assert.True(t, ok)
					assert.Equal(t, tt.wantPrompt, resolvedPrompt)
				}
			}
		})
	}
}

func TestSystemPrompt_GetEnableFor(t *testing.T) {
	tests := []struct {
		name         string
		systemPrompt SystemPrompt
		want         SystemPromptEnabledFor
	}{
		{
			name:         "nil EnableFor defaults to text",
			systemPrompt: SystemPrompt{},
			want:         EnableForText,
		},
		{
			name:         "explicit EnableForAll",
			systemPrompt: SystemPrompt{EnableFor: testutils.Ptr(EnableForAll)},
			want:         EnableForAll,
		},
		{
			name:         "explicit EnableForText",
			systemPrompt: SystemPrompt{EnableFor: testutils.Ptr(EnableForText)},
			want:         EnableForText,
		},
		{
			name:         "explicit EnableForNone",
			systemPrompt: SystemPrompt{EnableFor: testutils.Ptr(EnableForNone)},
			want:         EnableForNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.systemPrompt.GetEnableFor()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSystemPrompt_GetTemplate(t *testing.T) {
	tests := []struct {
		name         string
		systemPrompt SystemPrompt
		want         string
		wantOk       bool
	}{
		{
			name:         "nil template",
			systemPrompt: SystemPrompt{},
			want:         "",
			wantOk:       false,
		},
		{
			name:         "empty template",
			systemPrompt: SystemPrompt{Template: testutils.Ptr("")},
			want:         "",
			wantOk:       false,
		},
		{
			name:         "whitespace only template",
			systemPrompt: SystemPrompt{Template: testutils.Ptr("   ")},
			want:         "",
			wantOk:       false,
		},
		{
			name:         "valid template",
			systemPrompt: SystemPrompt{Template: testutils.Ptr("You are a helpful assistant")},
			want:         "You are a helpful assistant",
			wantOk:       true,
		},
		{
			name:         "template with whitespace",
			systemPrompt: SystemPrompt{Template: testutils.Ptr("  You are a helpful assistant  ")},
			want:         "  You are a helpful assistant  ",
			wantOk:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.systemPrompt.GetTemplate()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestResponseFormat(t *testing.T) {
	t.Run("string format", func(t *testing.T) {
		var format ResponseFormat
		format.raw = "Provide answer as: YES or NO"

		stringValue, isString := format.AsString()
		_, isSchema := format.AsSchema()
		assert.True(t, isString)
		assert.False(t, isSchema)
		assert.Equal(t, "Provide answer as: YES or NO", stringValue)
	})

	t.Run("schema format", func(t *testing.T) {
		var format ResponseFormat
		format.raw = map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{
					"type": "string",
					"enum": []interface{}{"YES", "NO"},
				},
			},
			"required": []interface{}{"answer"},
		}

		_, isString := format.AsString()
		schemaValue, isSchema := format.AsSchema()
		assert.False(t, isString)
		assert.True(t, isSchema)
		assert.NotNil(t, schemaValue)
	})
}

func TestValueSet(t *testing.T) {
	t.Run("string values", func(t *testing.T) {
		expected := utils.NewValueSet("YES", "NO")

		stringSet, ok := expected.AsStringSet()
		assert.True(t, ok)
		assert.Len(t, stringSet.Values(), 2)
		assert.Contains(t, stringSet.Values(), "YES")
		assert.Contains(t, stringSet.Values(), "NO")
		assert.Equal(t, []interface{}{"YES", "NO"}, expected.Values())
	})

	t.Run("object values", func(t *testing.T) {
		expected := utils.NewValueSet(
			map[string]interface{}{"answer": "YES"},
			map[string]interface{}{"answer": "NO"},
		)

		stringSet, ok := expected.AsStringSet()
		assert.False(t, ok)
		assert.Empty(t, stringSet.Values()) // Should be empty since values are not strings
		assert.Len(t, expected.Values(), 2)
	})
}

func TestValidateTaskConfiguration(t *testing.T) {
	t.Run("valid string task", func(t *testing.T) {
		task := Task{
			Name:                 "test",
			Prompt:               "What is 2+2?",
			ResponseResultFormat: NewResponseFormat("Number"),
			ExpectedResult:       utils.NewValueSet("4", "four"),
		}

		taskConfig := TaskConfig{
			Tasks:           []Task{task},
			ValidationRules: ValidationRules{},
		}
		err := taskConfig.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid schema task", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{
					"type": "number",
				},
			},
			"required": []interface{}{"answer"},
		}

		task := Task{
			Name:                 "test",
			Prompt:               "What is 2+2?",
			ResponseResultFormat: NewResponseFormat(schema),
			ExpectedResult: utils.NewValueSet(
				map[string]interface{}{"answer": 4},
				map[string]interface{}{"answer": 4.0},
			),
		}

		taskConfig := TaskConfig{
			Tasks:           []Task{task},
			ValidationRules: ValidationRules{},
		}
		err := taskConfig.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid - string format with object expected results", func(t *testing.T) {
		task := Task{
			Name:                 "test",
			Prompt:               "What is 2+2?",
			ResponseResultFormat: NewResponseFormat("Number"),
			ExpectedResult: utils.NewValueSet(
				map[string]interface{}{"answer": 4},
			),
		}

		taskConfig := TaskConfig{
			Tasks:           []Task{task},
			ValidationRules: ValidationRules{},
		}
		err := taskConfig.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "when response-result-format is plain text, all expected-result values must be plain text")
	})

	t.Run("invalid - schema with judge validation", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{
					"type": "number",
				},
			},
		}
		enabled := true
		task := Task{
			Name:                 "test",
			Prompt:               "What is 2+2?",
			ResponseResultFormat: NewResponseFormat(schema),
			ExpectedResult: utils.NewValueSet(
				map[string]interface{}{"answer": 4},
			),
			ValidationRules: &ValidationRules{
				Judge: JudgeSelector{
					Enabled: &enabled,
				},
			},
		}

		taskConfig := TaskConfig{
			Tasks:           []Task{task},
			ValidationRules: ValidationRules{},
		}
		err := taskConfig.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "semantic validation cannot be used with structured schema-based response-result-format")
	})

	t.Run("invalid - expected result does not conform to schema", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{
					"type": "number",
				},
			},
			"required": []interface{}{"answer"},
		}
		task := Task{
			Name:                 "test",
			Prompt:               "What is 2+2?",
			ResponseResultFormat: NewResponseFormat(schema),
			ExpectedResult: utils.NewValueSet(
				map[string]interface{}{"answer": "four"}, // string instead of number
			),
		}

		taskConfig := TaskConfig{
			Tasks:           []Task{task},
			ValidationRules: ValidationRules{},
		}
		err := taskConfig.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not conform to response-result-format schema")
	})

	t.Run("invalid - malformed JSON schema", func(t *testing.T) {
		invalidSchema := map[string]interface{}{
			"type":       "object",
			"properties": "this_should_be_an_object_not_a_string", // Invalid: properties must be an object
			"required":   []interface{}{"answer"},
		}
		task := Task{
			Name:                 "test",
			Prompt:               "What is 2+2?",
			ResponseResultFormat: NewResponseFormat(invalidSchema),
			ExpectedResult: utils.NewValueSet(
				map[string]interface{}{"answer": 4},
			),
		}

		taskConfig := TaskConfig{
			Tasks:           []Task{task},
			ValidationRules: ValidationRules{},
		}
		err := taskConfig.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response-result-format contains an invalid JSON schema")
	})

	t.Run("invalid - response format neither string nor schema", func(t *testing.T) {
		task := Task{
			Name:   "test",
			Prompt: "What is 2+2?",
			ResponseResultFormat: ResponseFormat{
				raw: 123, // invalid type
			},
			ExpectedResult: utils.NewValueSet("4"),
		}

		taskConfig := TaskConfig{
			Tasks:           []Task{task},
			ValidationRules: ValidationRules{},
		}
		err := taskConfig.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response-result-format must be either plain text or a JSON schema object")
	})
}
