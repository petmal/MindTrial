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
	mockFile := TaskFile{}
	assert.Empty(t, mockFile.basePath)
	mockFile.SetBasePath("/base/path")
	assert.Equal(t, "/base/path", mockFile.basePath)
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
