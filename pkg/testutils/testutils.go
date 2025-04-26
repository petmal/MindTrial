// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package testutils provides utilities for capturing output, managing test files, and making assertions in tests.
package testutils

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	stdoutLock sync.Mutex
	osArgsLock sync.Mutex
)

// CaptureStdout captures standard output during the execution of the provided function
// and returns it as a string. This function is synchronized to prevent concurrent stdout capture.
func CaptureStdout(t *testing.T, fn func()) (stdout string) {
	SyncCall(&stdoutLock, func() {
		// Create a temporary file to capture os.Stdout.
		fp, err := os.CreateTemp("", "*.stdout")
		if err != nil {
			t.Fatalf("failed to create stdout capture file: %v\n", err)
		}
		defer fp.Close()

		// Save the original os.Stdout.
		originalStdout := os.Stdout
		defer func() { os.Stdout = originalStdout }()

		os.Stdout = fp

		// Call the tested function.
		fn()

		// Read the output.
		if err := fp.Sync(); err != nil {
			t.Fatalf("failed to sync stdout capture file: %v\n", err)
		}
		if _, err := fp.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("failed to set read offset in stdout capture file: %v\n", err)
		}
		contents, err := io.ReadAll(fp)
		if err != nil {
			t.Fatalf("failed to read stdout capture file: %v\n", err)
		}

		stdout = string(contents)
	})
	return
}

// WithArgs temporarily replaces os.Args with the provided arguments while executing
// the given function. This function is synchronized to prevent concurrent modifications.
func WithArgs(_ *testing.T, fn func(), args ...string) {
	SyncCall(&osArgsLock, func() {
		// Save the original os.Args
		originalArgs := os.Args
		defer func() { os.Args = originalArgs }()

		os.Args = append([]string{os.Args[0]}, args...)

		// Call the tested function.
		fn()
	})
}

// SyncCall executes the provided function while holding the specified mutex lock.
func SyncCall(lock *sync.Mutex, fn func()) {
	lock.Lock()
	defer lock.Unlock()
	fn()
}

// CreateMockFile creates a temporary file with the given name pattern and contents,
// returning the file path.
func CreateMockFile(t *testing.T, namePattern string, contents []byte) string {
	fp := CreateOpenNewTestFile(t, namePattern)
	defer fp.Close()

	if _, err := fp.Write(contents); err != nil {
		t.Fatalf("failed to write test file: %v\n", err)
	}

	return fp.Name()
}

// CreateOpenNewTestFile creates and opens a new temporary test file with the given name pattern.
// The caller is responsible for closing the file.
func CreateOpenNewTestFile(t *testing.T, namePattern string) *os.File {
	fp, err := os.CreateTemp("", namePattern)
	if err != nil {
		t.Fatalf("failed to create test file: %v\n", err)
	}
	return fp
}

// AssertFileContains checks if a file contains all strings from want slice and none from notWant slice.
func AssertFileContains(t *testing.T, filePath string, want []string, notWant []string) {
	if contents := ReadFile(t, filePath); len(want) > 0 {
		require.NotEmpty(t, contents)
		AssertContainsAll(t, string(contents), want)
		AssertContainsNone(t, string(contents), notWant)
	} else {
		assert.Empty(t, contents)
	}
}

// AssertContainsAll verifies that the given contents string contains all specified elements.
func AssertContainsAll(t *testing.T, contents string, elements []string) {
	for i := range elements {
		assert.Contains(t, string(contents), elements[i])
	}
}

// AssertContainsNone verifies that the given contents string contains none of the specified elements.
func AssertContainsNone(t *testing.T, contents string, elements []string) {
	for i := range elements {
		assert.NotContains(t, string(contents), elements[i])
	}
}

// AssertFileContentsSameAs verifies that two files have identical contents.
func AssertFileContentsSameAs(t *testing.T, wantFilePath string, gotFilePath string) {
	want := ReadFile(t, wantFilePath)
	got := ReadFile(t, gotFilePath)
	assert.Equal(t, string(want), string(got))
}

// ReadFile reads the entire file at the given path and returns its contents.
func ReadFile(t *testing.T, filePath string) []byte {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read test file: %v\n", err)
	}
	return contents
}

// AssertNotBlank asserts that the given string is not blank (i.e., not empty or consisting only of whitespace).
func AssertNotBlank(t *testing.T, value string) {
	assert.NotEmpty(t, strings.TrimSpace(value))
}

// Ptr returns a pointer to the given value.
func Ptr[T any](value T) *T {
	return &value
}

// CreateMockServer creates a test HTTP server with configurable responses.
// It accepts a map of paths to response configurations (status code and content).
// Returns the server which should be closed after use.
func CreateMockServer(t *testing.T, responses map[string]MockHTTPResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if response, ok := responses[r.URL.Path]; ok {
			if response.Delay > 0 {
				time.Sleep(response.Delay)
			}
			w.WriteHeader(response.StatusCode)
			if response.Content != nil {
				if _, err := w.Write(response.Content); err != nil {
					t.Fatalf("failed to write mock response: %v", err)
				}
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// MockHTTPResponse defines a mock HTTP response for testing.
type MockHTTPResponse struct {
	StatusCode int
	Content    []byte
	Delay      time.Duration
}
