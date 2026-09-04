// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// mockTaskFileWithAccess creates a TaskFile with explicit access and resolved options.
func mockTaskFileWithAccess(t *testing.T, name, uri, mimeType string, access []config.FileAccess) config.TaskFile {
	t.Helper()
	yamlStr := fmt.Sprintf("name: %s\nuri: %s\ntype: %s", name, uri, mimeType)
	var file config.TaskFile
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &file))
	if access != nil {
		file.Options = &config.FileOptions{Access: access}
		// Resolve with empty defaults to set resolvedFileOptions.
		file.ResolveFileOptions(config.FileOptions{})
	} else {
		// Nil access means omitted -> default [native, local]
		file.ResolveFileOptions(config.FileOptions{})
	}
	return file
}

func TestApiFilenameForFile(t *testing.T) {
	ctx := context.Background()
	t.Run("name already has extension", func(t *testing.T) {
		f := mockTaskFileWithAccess(t, "report.pdf", "file:///docs/report.pdf", "application/pdf", []config.FileAccess{config.FileAccessNative})
		name, err := apiFilenameForFile(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, "report.pdf", name)
	})
	t.Run("extensionless name with pdf mime", func(t *testing.T) {
		f := mockTaskFileWithAccess(t, "report", "file:///docs/report.pdf", "application/pdf", []config.FileAccess{config.FileAccessNative})
		name, err := apiFilenameForFile(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, "report.pdf", name)
	})
	t.Run("extensionless name with remote URI and query", func(t *testing.T) {
		// The URI path extension wins over the MIME table, which is host-dependent.
		f := mockTaskFileWithAccess(t, "report", "https://example.test/assets/report.pdf?token=abc", "application/pdf", []config.FileAccess{config.FileAccessNative})
		name, err := apiFilenameForFile(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, "report.pdf", name)
	})
	t.Run("extensionless name prefers URI extension over MIME table", func(t *testing.T) {
		// mime.ExtensionsByType("text/html") is sorted and may yield ".htm" first.
		f := mockTaskFileWithAccess(t, "page", "file:///docs/page.html", "text/html", []config.FileAccess{config.FileAccessNative})
		name, err := apiFilenameForFile(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, "page.html", name)
	})
	t.Run("extensionless name with explicit mime", func(t *testing.T) {
		// No URI extension, explicit mime should provide extension.
		f := mockTaskFileWithAccess(t, "data", "file:///docs/data", "text/csv", []config.FileAccess{config.FileAccessNative})
		name, err := apiFilenameForFile(ctx, f)
		require.NoError(t, err)
		// mime.ExtensionsByType for text/csv returns .csv
		assert.Equal(t, "data.csv", name)
	})
	t.Run("unknown mime returns name as-is", func(t *testing.T) {
		f := mockTaskFileWithAccess(t, "unknown", "file:///docs/unknown", "application/octet-stream", []config.FileAccess{config.FileAccessNative})
		name, err := apiFilenameForFile(ctx, f)
		require.NoError(t, err)
		// application/octet-stream maps to .bin, so it will return unknown.bin
		// If no extension found, it returns name as-is. We accept either.
		assert.True(t, name == "unknown" || name == "unknown.bin")
	})
	t.Run("name with extension takes precedence over mime", func(t *testing.T) {
		f := mockTaskFileWithAccess(t, "archive.tar.gz", "file:///docs/archive.tar.gz", "application/octet-stream", []config.FileAccess{config.FileAccessNative})
		name, err := apiFilenameForFile(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, "archive.tar.gz", name)
	})
}

func TestTaskFilesToDataMap_AccessFiltering(t *testing.T) {
	ctx := context.Background()
	t.Run("omitted access included", func(t *testing.T) {
		data := []byte("hello")
		path := testutils.CreateMockFile(t, "test-*.txt", data)
		f := mockTaskFile(t, "test", path, "text/plain")
		// mockTaskFile via yaml without access, need to resolve
		f.ResolveFileOptions(config.FileOptions{})
		m, err := taskFilesToDataMap(ctx, []config.TaskFile{f})
		require.NoError(t, err)
		assert.Equal(t, map[string][]byte{"test": data}, m)
	})
	t.Run("local only included", func(t *testing.T) {
		data := []byte("local")
		path := testutils.CreateMockFile(t, "test-*.txt", data)
		f := mockTaskFileWithAccess(t, "test", path, "text/plain", []config.FileAccess{config.FileAccessLocal})
		m, err := taskFilesToDataMap(ctx, []config.TaskFile{f})
		require.NoError(t, err)
		assert.Equal(t, map[string][]byte{"test": data}, m)
	})
	t.Run("native+local included", func(t *testing.T) {
		data := []byte("both")
		path := testutils.CreateMockFile(t, "test-*.txt", data)
		f := mockTaskFileWithAccess(t, "test", path, "text/plain", []config.FileAccess{config.FileAccessNative, config.FileAccessLocal})
		m, err := taskFilesToDataMap(ctx, []config.TaskFile{f})
		require.NoError(t, err)
		assert.Equal(t, map[string][]byte{"test": data}, m)
	})
	t.Run("native only excluded", func(t *testing.T) {
		// Use nonexistent file to prove it is not read.
		f := mockTaskFileWithAccess(t, "native", "/nonexistent/path.txt", "text/plain", []config.FileAccess{config.FileAccessNative})
		m, err := taskFilesToDataMap(ctx, []config.TaskFile{f})
		require.NoError(t, err)
		assert.Equal(t, map[string][]byte{}, m)
	})
	t.Run("mixed files", func(t *testing.T) {
		dataLocal := []byte("local")
		pathLocal := testutils.CreateMockFile(t, "local-*.txt", dataLocal)
		fLocal := mockTaskFileWithAccess(t, "local.txt", pathLocal, "text/plain", []config.FileAccess{config.FileAccessLocal})
		fNative := mockTaskFileWithAccess(t, "native.txt", "/nonexistent/native.txt", "text/plain", []config.FileAccess{config.FileAccessNative})
		dataBoth := []byte("both")
		pathBoth := testutils.CreateMockFile(t, "both-*.txt", dataBoth)
		fBoth := mockTaskFileWithAccess(t, "both.txt", pathBoth, "text/plain", []config.FileAccess{config.FileAccessNative, config.FileAccessLocal})
		m, err := taskFilesToDataMap(ctx, []config.TaskFile{fLocal, fNative, fBoth})
		require.NoError(t, err)
		expected := map[string][]byte{"local.txt": dataLocal, "both.txt": dataBoth}
		assert.Equal(t, expected, m)
	})
	t.Run("local read error preserved", func(t *testing.T) {
		f := mockTaskFileWithAccess(t, "missing", "/nonexistent/missing.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})
		_, err := taskFilesToDataMap(ctx, []config.TaskFile{f})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read content")
	})
	t.Run("native only not read even with missing file", func(t *testing.T) {
		f := mockTaskFileWithAccess(t, "native-missing", "/nonexistent/native-missing.txt", "text/plain", []config.FileAccess{config.FileAccessNative})
		m, err := taskFilesToDataMap(ctx, []config.TaskFile{f})
		require.NoError(t, err)
		assert.Empty(t, m)
	})
}

func TestProvider_LocalOnlyBypass(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NewTestLogger(t)
	// Use a MIME that is unsupported for most providers when native, but should be bypassed for local-only.
	// For providers that support text/plain natively (OpenAI, Google), use application/octet-stream.
	// For others, text/plain is already unsupported.
	tests := []struct {
		name     string
		provider string
		run      func(t *testing.T) error
	}{
		{
			name:     "openai local-only bypass",
			provider: "openai",
			run: func(t *testing.T) error {
				p := NewOpenAI(config.OpenAIClientConfig{APIKey: "test"}, nil)
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.bin", "file://file.bin", "application/octet-stream", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.completionProvider.createPromptMessage(ctx, logger, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "openai native unsupported still errors",
			provider: "openai",
			run: func(t *testing.T) error {
				p := NewOpenAI(config.OpenAIClientConfig{APIKey: "test"}, nil)
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.bin", "file://file.bin", "application/octet-stream", []config.FileAccess{config.FileAccessNative})}}
				_, err := p.completionProvider.createPromptMessage(ctx, logger, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "google local-only bypass",
			provider: "google",
			run: func(t *testing.T) error {
				p := &GoogleAI{}
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.bin", "file://file.bin", "application/octet-stream", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.createPromptMessageParts(ctx, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "anthropic local-only bypass",
			provider: "anthropic",
			run: func(t *testing.T) error {
				p := &Anthropic{}
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.txt", "file://file.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.createPromptMessageParts(ctx, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "mistral local-only bypass",
			provider: "mistral",
			run: func(t *testing.T) error {
				p := &MistralAI{}
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.txt", "file://file.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.createPromptMessage(ctx, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "deepseek local-only bypass",
			provider: "deepseek",
			run: func(t *testing.T) error {
				p := &Deepseek{}
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.txt", "file://file.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.createPromptMessageParts(ctx, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "xai local-only bypass",
			provider: "xai",
			run: func(t *testing.T) error {
				p := &XAI{}
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.txt", "file://file.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.createPromptMessageParts(ctx, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "openrouter local-only bypass",
			provider: "openrouter",
			run: func(t *testing.T) error {
				p := NewOpenRouter(config.OpenRouterClientConfig{APIKey: "test"}, nil)
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.txt", "file://file.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.openaiProvider.createPromptMessage(ctx, logger, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "alibaba local-only bypass",
			provider: "alibaba",
			run: func(t *testing.T) error {
				p := NewAlibaba(config.AlibabaClientConfig{APIKey: "test"}, nil)
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.txt", "file://file.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.openaiProvider.createPromptMessage(ctx, logger, "prompt", task.Files, &Result{})
				return err
			},
		},
		{
			name:     "moonshot local-only bypass",
			provider: "moonshot",
			run: func(t *testing.T) error {
				p := NewMoonshotAI(config.MoonshotAIClientConfig{APIKey: "test"}, nil)
				task := config.Task{Name: "t", Files: []config.TaskFile{mockTaskFileWithAccess(t, "file.txt", "file://file.txt", "text/plain", []config.FileAccess{config.FileAccessLocal})}}
				_, err := p.openaiProvider.createPromptMessage(ctx, logger, "prompt", task.Files, &Result{})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(t)
			if tt.name == "openai native unsupported still errors" {
				require.ErrorIs(t, err, ErrFileNotSupported)
			} else {
				require.NoError(t, err, "local-only file should bypass native MIME check for %s", tt.provider)
			}
		})
	}
}

// mockNativePDF returns a native-access PDF task file backed by a real temporary file.
func mockNativePDF(t *testing.T, name string) config.TaskFile {
	t.Helper()
	path := testutils.CreateMockFile(t, "doc-*.pdf", []byte("%PDF-1.7\ntest"))
	return mockTaskFileWithAccess(t, name, path, "application/pdf", []config.FileAccess{config.FileAccessNative})
}

// mockLocalCSV returns a local-only CSV task file backed by a real temporary file.
func mockLocalCSV(t *testing.T, name string) config.TaskFile {
	t.Helper()
	path := testutils.CreateMockFile(t, "data-*.csv", []byte("a,b\n1,2"))
	return mockTaskFileWithAccess(t, name, path, "text/csv", []config.FileAccess{config.FileAccessLocal})
}

const testPDFDataURLPrefix = "data:application/pdf;base64,"

// TestProvider_NativeDocumentRequestShape asserts the outgoing request shape for a
// native document, that a local-only file contributes its marker but no native part,
// and that markers stay ordered before the task prompt.
func TestProvider_NativeDocumentRequestShape(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NewTestLogger(t)

	t.Run("openai chat completions", func(t *testing.T) {
		p := NewOpenAI(config.OpenAIClientConfig{APIKey: "test"}, nil)
		result := &Result{}
		files := []config.TaskFile{mockNativePDF(t, "report.pdf"), mockLocalCSV(t, "data.csv")}

		message, err := p.completionProvider.createPromptMessage(ctx, logger, "the prompt", files, result)
		require.NoError(t, err)

		parts := message.OfUser.Content.OfArrayOfContentParts
		require.Len(t, parts, 4)
		require.NotNil(t, parts[0].OfText)
		assert.Equal(t, "[file: report.pdf]", parts[0].OfText.Text)
		require.NotNil(t, parts[1].OfFile, "native PDF must be sent as a file content part")
		assert.Equal(t, "report.pdf", parts[1].OfFile.File.Filename.Value)
		assert.True(t, strings.HasPrefix(parts[1].OfFile.File.FileData.Value, testPDFDataURLPrefix),
			"file data must be a normalized PDF data URL, got %q", parts[1].OfFile.File.FileData.Value)
		require.NotNil(t, parts[2].OfText)
		assert.Equal(t, "[file: data.csv]", parts[2].OfText.Text, "local-only file keeps its marker")
		require.NotNil(t, parts[3].OfText)
		assert.Equal(t, "the prompt", parts[3].OfText.Text)
		assert.Equal(t, []string{"[file: report.pdf]", "[file: data.csv]", "the prompt"}, result.GetPrompts())
	})

	t.Run("openai responses", func(t *testing.T) {
		p := NewOpenAI(config.OpenAIClientConfig{APIKey: "test"}, nil)
		result := &Result{}
		files := []config.TaskFile{mockNativePDF(t, "report.pdf"), mockLocalCSV(t, "data.csv")}

		items, err := p.responsesProvider.createPromptInputItems(ctx, logger, "the prompt", files, result)
		require.NoError(t, err)
		require.Len(t, items, 1)

		parts := items[0].OfMessage.Content.OfInputItemContentList
		require.Len(t, parts, 4)
		require.NotNil(t, parts[0].OfInputText)
		assert.Equal(t, "[file: report.pdf]", parts[0].OfInputText.Text)
		require.NotNil(t, parts[1].OfInputFile, "native PDF must be sent as an input_file part")
		require.Nil(t, parts[1].OfInputImage, "documents must not use the image part")
		assert.Equal(t, "report.pdf", parts[1].OfInputFile.Filename.Value)
		assert.True(t, strings.HasPrefix(parts[1].OfInputFile.FileData.Value, testPDFDataURLPrefix))
		assert.Equal(t, responses.ResponseInputFileDetailAuto, parts[1].OfInputFile.Detail, "unset image-detail leaves the provider default")
		require.NotNil(t, parts[2].OfInputText)
		assert.Equal(t, "[file: data.csv]", parts[2].OfInputText.Text)
		require.NotNil(t, parts[3].OfInputText)
		assert.Equal(t, "the prompt", parts[3].OfInputText.Text)
	})

	t.Run("google inline document", func(t *testing.T) {
		p := &GoogleAI{}
		result := &Result{}
		files := []config.TaskFile{mockNativePDF(t, "report.pdf"), mockLocalCSV(t, "data.csv")}

		parts, err := p.createPromptMessageParts(ctx, "the prompt", files, result)
		require.NoError(t, err)
		require.Len(t, parts, 4)
		assert.Equal(t, "[file: report.pdf]", parts[0].Text)
		require.NotNil(t, parts[1].InlineData, "native PDF must be sent as inline data")
		assert.Equal(t, "application/pdf", parts[1].InlineData.MIMEType)
		assert.NotEmpty(t, parts[1].InlineData.Data)
		assert.Equal(t, "[file: data.csv]", parts[2].Text)
		assert.Equal(t, "the prompt", parts[3].Text)
	})

	t.Run("google normalizes inferred mime type", func(t *testing.T) {
		p := &GoogleAI{}
		path := testutils.CreateMockFile(t, "page-*.html", []byte("<html></html>"))
		// No explicit type: TypeValue infers "text/html; charset=utf-8".
		file := mockTaskFileWithAccess(t, "page.html", path, "", []config.FileAccess{config.FileAccessNative})

		parts, err := p.createPromptMessageParts(ctx, "the prompt", []config.TaskFile{file}, &Result{})
		require.NoError(t, err)
		require.Len(t, parts, 3)
		require.NotNil(t, parts[1].InlineData)
		assert.Equal(t, "text/html", parts[1].InlineData.MIMEType, "parameters must be stripped before the API call")
	})

	t.Run("openai responses applies image detail to pdf rendering", func(t *testing.T) {
		p := NewOpenAI(config.OpenAIClientConfig{APIKey: "test"}, nil)
		file := mockNativePDF(t, "report.pdf")
		file.Options = &config.FileOptions{ImageDetail: testutils.Ptr(config.ImageDetailLow)}
		file.ResolveFileOptions(config.FileOptions{})

		items, err := p.responsesProvider.createPromptInputItems(ctx, logger, "the prompt", []config.TaskFile{file}, &Result{})
		require.NoError(t, err)
		parts := items[0].OfMessage.Content.OfInputItemContentList
		require.NotNil(t, parts[1].OfInputFile)
		assert.Equal(t, responses.ResponseInputFileDetailLow, parts[1].OfInputFile.Detail)
	})

	t.Run("anthropic document block", func(t *testing.T) {
		p := &Anthropic{}
		result := &Result{}
		files := []config.TaskFile{mockNativePDF(t, "report.pdf"), mockLocalCSV(t, "data.csv")}

		parts, err := p.createPromptMessageParts(ctx, "the prompt", files, result)
		require.NoError(t, err)
		require.Len(t, parts, 4)
		require.NotNil(t, parts[0].OfText)
		assert.Equal(t, "[file: report.pdf]", parts[0].OfText.Text)
		require.NotNil(t, parts[1].OfDocument, "native PDF must be sent as a document block")
		require.NotNil(t, parts[1].OfDocument.Source.OfBase64)
		assert.NotEmpty(t, parts[1].OfDocument.Source.OfBase64.Data)
		require.NotNil(t, parts[2].OfText)
		assert.Equal(t, "[file: data.csv]", parts[2].OfText.Text)
		require.NotNil(t, parts[3].OfText)
		assert.Equal(t, "the prompt", parts[3].OfText.Text)
	})

	t.Run("anthropic plain text document block", func(t *testing.T) {
		p := &Anthropic{}
		const text = "line one\nline two"
		path := testutils.CreateMockFile(t, "notes-*.txt", []byte(text))
		file := mockTaskFileWithAccess(t, "notes.txt", path, "text/plain", []config.FileAccess{config.FileAccessNative})

		parts, err := p.createPromptMessageParts(ctx, "the prompt", []config.TaskFile{file}, &Result{})
		require.NoError(t, err)
		require.Len(t, parts, 3)
		require.NotNil(t, parts[1].OfDocument, "plain text must be sent as a document block")
		require.NotNil(t, parts[1].OfDocument.Source.OfText, "a text source carries the document verbatim")
		assert.Equal(t, text, parts[1].OfDocument.Source.OfText.Data)
		assert.Nil(t, parts[1].OfDocument.Source.OfBase64, "plain text must not be base64-encoded")
	})

	t.Run("mistral document url chunk", func(t *testing.T) {
		p := &MistralAI{}
		result := &Result{}
		files := []config.TaskFile{mockNativePDF(t, "report.pdf"), mockLocalCSV(t, "data.csv")}

		message, err := p.createPromptMessage(ctx, "the prompt", files, result)
		require.NoError(t, err)
		require.NotNil(t, message.UserMessage)
		content := message.UserMessage.Content.Get()
		require.NotNil(t, content)
		require.NotNil(t, content.ArrayOfContentChunk)

		chunks := *content.ArrayOfContentChunk
		require.Len(t, chunks, 4)
		require.NotNil(t, chunks[0].TextChunk)
		assert.Equal(t, "[file: report.pdf]", chunks[0].TextChunk.Text)
		require.NotNil(t, chunks[1].DocumentURLChunk, "native PDF must be sent as a document_url chunk")
		assert.Equal(t, "report.pdf", chunks[1].DocumentURLChunk.GetDocumentName())
		assert.True(t, strings.HasPrefix(chunks[1].DocumentURLChunk.DocumentUrl, testPDFDataURLPrefix))
		require.NotNil(t, chunks[2].TextChunk)
		assert.Equal(t, "[file: data.csv]", chunks[2].TextChunk.Text)
		require.NotNil(t, chunks[3].TextChunk)
		assert.Equal(t, "the prompt", chunks[3].TextChunk.Text)
	})

	t.Run("deepseek local-only uses text request and records each marker once", func(t *testing.T) {
		p := &Deepseek{}
		result := &Result{}
		files := []config.TaskFile{mockLocalCSV(t, "data.csv"), mockLocalCSV(t, "more.csv")}
		task := config.Task{Name: "t", Prompt: "the prompt", Files: files}
		require.False(t, task.RequiresNativeFileInput())

		parts, err := p.createPromptMessageParts(ctx, "the prompt", files, result)
		require.NoError(t, err)
		require.Len(t, parts, 3, "local-only files contribute markers only")
		assert.Equal(t, "[file: data.csv]", parts[0].Text)
		assert.Equal(t, "[file: more.csv]", parts[1].Text)
		assert.Equal(t, "the prompt", parts[2].Text)
	})
}

func TestGoogleSupportedDocumentMimeTypes(t *testing.T) {
	// The full "Supported content types" list from the Gemini file input guide.
	documented := []string{
		"application/pdf",
		"application/json",
		"text/plain",
		"text/html",
		"text/css",
		"text/xml",
		"text/csv",
		"text/rtf",
		"text/javascript",
	}
	for _, mimeType := range documented {
		assert.True(t, googleSupportedDocumentMimeTypes[mimeType], "Gemini documents %s", mimeType)
	}
	assert.Len(t, googleSupportedDocumentMimeTypes, len(documented), "no undocumented type may be added")
	// Spellings Gemini does not document, including the one Go infers for ".rtf".
	for _, undocumented := range []string{"application/rtf", "text/markdown", "text/md", "application/x-javascript", "text/x-python"} {
		assert.False(t, googleSupportedDocumentMimeTypes[undocumented], "%s is not a documented Gemini type", undocumented)
	}
}

func TestOpenAIAdapterLeakage(t *testing.T) {
	// OpenAI falls back to native behaviour (nil validator), OpenRouter overrides
	// documents to pdf-only, Alibaba/Moonshot disable documents (images-only).
	openAI := NewOpenAI(config.OpenAIClientConfig{APIKey: "test"}, nil)
	require.Nil(t, openAI.completionProvider.FileValidator, "OpenAI should fall back to native behaviour")
	require.Nil(t, openAI.responsesProvider.FileValidator, "OpenAI responses should fall back to native behaviour")
	openRouter := NewOpenRouter(config.OpenRouterClientConfig{APIKey: "test"}, nil)
	require.NotNil(t, openRouter.openaiProvider.FileValidator, "OpenRouter should override file validator")
	alibaba := NewAlibaba(config.AlibabaClientConfig{APIKey: "test"}, nil)
	require.NotNil(t, alibaba.openaiProvider.FileValidator, "Alibaba should disable documents")
	moonshot := NewMoonshotAI(config.MoonshotAIClientConfig{APIKey: "test"}, nil)
	require.NotNil(t, moonshot.openaiProvider.FileValidator, "Moonshot should disable documents")

	// Verify OpenRouter validator is pdf-only (plus images): text/plain should fail, pdf and images should succeed.
	require.True(t, openRouter.openaiProvider.FileValidator.IsSupportedDocument("application/pdf"), "OpenRouter should support pdf")
	require.False(t, openRouter.openaiProvider.FileValidator.IsSupportedDocument("text/plain"), "OpenRouter should not support text/plain")
	require.True(t, openRouter.openaiProvider.FileValidator.IsSupportedImage("image/png"), "OpenRouter should support images")

	// Nil validator falls back to native OpenAI behaviour.
	var native *openAIFileValidator
	require.True(t, native.IsSupportedDocument("text/plain"), "OpenAI should support text/plain")
	require.True(t, native.IsSupportedDocument("application/pdf"), "OpenAI should support pdf")
	require.True(t, native.IsSupportedImage("image/png"), "OpenAI should support images")

	// Documented categories from OpenAI's accepted file types table.
	for _, documented := range []string{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.oasis.opendocument.text",
		"application/rtf",
		"text/csv",
		"text/markdown",
		"text/xml",
		"text/x-python",
		"text/x-r",
	} {
		assert.True(t, native.IsSupportedDocument(documented), "OpenAI should support documented type %s", documented)
	}
	// Types absent from the documented table must not be accepted.
	for _, undocumented := range []string{"text/json", "application/x-python", "application/octet-stream"} {
		assert.False(t, native.IsSupportedDocument(undocumented), "%s is not a documented OpenAI type", undocumented)
	}

	// Responses provider uses the same approach: image check via validator, file check via validator.
	assert.True(t, openAI.responsesProvider.FileValidator.IsSupportedImage("image/png"), "responses validator should support images")
	assert.False(t, openAI.responsesProvider.FileValidator.IsSupportedImage("application/octet-stream"), "responses validator should reject non-images")
	require.True(t, openAI.responsesProvider.FileValidator.IsSupportedDocument("text/plain"), "OpenAI responses should support text/plain")
	require.True(t, openAI.responsesProvider.FileValidator.IsSupportedDocument("application/pdf"), "OpenAI responses should support pdf")
	require.True(t, openAI.responsesProvider.FileValidator.IsSupportedImage("image/png"), "OpenAI responses should support images")

	// Explicit empty documents disables non-image input; images still pass.
	imagesOnly := newOpenAIFileValidator(nil, map[string]bool{})
	assert.True(t, imagesOnly.IsSupportedImage("image/png"), "images-only validator should still support images")
	assert.False(t, imagesOnly.IsSupportedDocument("text/plain"), "images-only validator should reject non-images")

	// Image override is honoured by both methods.
	customImages := newOpenAIFileValidator(map[string]bool{"image/png": true}, nil)
	assert.True(t, customImages.IsSupportedImage("image/png"))
	assert.False(t, customImages.IsSupportedImage("image/jpeg"))
	assert.False(t, customImages.IsSupportedDocument("image/jpeg"), "custom images should reject image/jpeg as document")
	assert.True(t, customImages.IsSupportedDocument("text/plain"), "custom images should fall back to default documents")
}
