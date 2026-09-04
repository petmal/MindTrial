// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/option"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/logging"
	"github.com/petmal/mindtrial/pkg/utils"
)

// openAISupportedDocumentMimeTypes lists the non-image types accepted as native file
// input. Transcribed in full from the "Full list of accepted file types" table in
// OpenAI's File inputs guide, lower-cased for lookup.
// See: https://platform.openai.com/docs/guides/pdf-files
var openAISupportedDocumentMimeTypes = map[string]bool{
	"application/csv":                          true,
	"application/graphql":                      true,
	"application/javascript":                   true,
	"application/json":                         true,
	"application/json5":                        true,
	"application/msword":                       true,
	"application/pdf":                          true,
	"application/rtf":                          true,
	"application/toml":                         true,
	"application/typescript":                   true,
	"application/vnd.apple.iwork":              true,
	"application/vnd.apple.keynote":            true,
	"application/vnd.apple.pages":              true,
	"application/vnd.google-apps.document":     true,
	"application/vnd.google-apps.presentation": true,
	"application/vnd.google-apps.spreadsheet":  true,
	"application/vnd.ms-excel":                 true,
	"application/vnd.ms-powerpoint":            true,
	"application/vnd.oasis.opendocument.text":  true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/x-awk":              true,
	"application/x-bash":             true,
	"application/x-graphql":          true,
	"application/x-httpd-php":        true,
	"application/x-httpd-php-source": true,
	"application/x-iif":              true,
	"application/x-json5":            true,
	"application/x-ndjson":           true,
	"application/x-patch":            true,
	"application/x-php":              true,
	"application/x-powershell":       true,
	"application/x-protobuf":         true,
	"application/x-rust":             true,
	"application/x-scala":            true,
	"application/x-sql":              true,
	"application/x-subrip":           true,
	"application/x-terraform":        true,
	"application/x-toml":             true,
	"application/x-yaml":             true,
	"application/yaml":               true,
	"message/rfc822":                 true,
	"text/calendar":                  true,
	"text/css":                       true,
	"text/csv":                       true,
	"text/html":                      true,
	"text/javascript":                true,
	"text/jsx":                       true,
	"text/markdown":                  true,
	"text/plain":                     true,
	"text/rtf":                       true,
	"text/srt":                       true,
	"text/tsv":                       true,
	"text/tsx":                       true,
	"text/vbscript":                  true,
	"text/vtt":                       true,
	"text/x-asm":                     true,
	"text/x-astro":                   true,
	"text/x-awk":                     true,
	"text/x-bash":                    true,
	"text/x-c":                       true,
	"text/x-c++":                     true,
	"text/x-clojure":                 true,
	"text/x-cmake":                   true,
	"text/x-csharp":                  true,
	"text/x-dart":                    true,
	"text/x-diff":                    true,
	"text/x-dockerfile":              true,
	"text/x-ejs":                     true,
	"text/x-elixir":                  true,
	"text/x-erb":                     true,
	"text/x-erlang":                  true,
	"text/x-go":                      true,
	"text/x-golang":                  true,
	"text/x-gradle":                  true,
	"text/x-graphql":                 true,
	"text/x-groovy":                  true,
	"text/x-handlebars":              true,
	"text/x-haskell":                 true,
	"text/x-hcl":                     true,
	"text/x-iif":                     true,
	"text/x-ini":                     true,
	"text/x-jade":                    true,
	"text/x-java":                    true,
	"text/x-jinja2":                  true,
	"text/x-julia":                   true,
	"text/x-kotlin":                  true,
	"text/x-less":                    true,
	"text/x-liquid":                  true,
	"text/x-lisp":                    true,
	"text/x-lua":                     true,
	"text/x-makefile":                true,
	"text/x-mustache":                true,
	"text/x-objectivec":              true,
	"text/x-objectivec++":            true,
	"text/x-patch":                   true,
	"text/x-perl":                    true,
	"text/x-php":                     true,
	"text/x-properties":              true,
	"text/x-protobuf":                true,
	"text/x-pug":                     true,
	"text/x-python":                  true,
	"text/x-r":                       true,
	"text/x-rst":                     true,
	"text/x-ruby":                    true,
	"text/x-rust":                    true,
	"text/x-sass":                    true,
	"text/x-scala":                   true,
	"text/x-script.python":           true,
	"text/x-scss":                    true,
	"text/x-sh":                      true,
	"text/x-shellscript":             true,
	"text/x-sql":                     true,
	"text/x-subrip":                  true,
	"text/x-swift":                   true,
	"text/x-terraform":               true,
	"text/x-tex":                     true,
	"text/x-tmpl":                    true,
	"text/x-toml":                    true,
	"text/x-twig":                    true,
	"text/x-typescript":              true,
	"text/x-vcard":                   true,
	"text/x-yaml":                    true,
	"text/x-zsh":                     true,
	"text/xml":                       true,
}

// NewOpenAI creates a new OpenAI provider instance with the given configuration.
// Both inner providers leave FileValidator nil to fall back to native OpenAI behaviour.
func NewOpenAI(cfg config.OpenAIClientConfig, availableTools []config.ToolConfig) *OpenAI {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	return &OpenAI{
		completionProvider: newOpenAICompletionsProvider(availableTools, opts...),
		responsesProvider:  newOpenAIResponsesProvider(availableTools, opts...),
	}
}

// OpenAI implements the Provider interface for OpenAI generative models.
type OpenAI struct {
	completionProvider *openAICompletionsProvider
	responsesProvider  *openAIResponsesProvider
}

func (o OpenAI) Name() string {
	return config.OPENAI
}

func (o *OpenAI) Run(ctx context.Context, logger logging.Logger, cfg config.RunConfig, task config.Task) (result Result, err error) {
	openAIV3Params := openAIV3ModelParams{}

	if cfg.ModelParams != nil {
		if openAIModelParams, ok := cfg.ModelParams.(config.OpenAIModelParams); ok {
			o.copyToOpenAIV3Params(openAIModelParams, &openAIV3Params)
		} else {
			return result, fmt.Errorf("%w: %s", ErrInvalidModelParams, cfg.Name)
		}
	}

	// Automatically derive a stable prompt cache key from the run configuration.
	// This requires no user configuration and improves prompt cache hit-rate
	// consistency whenever request content happens to be cacheable; it is a
	// routing hint only and has no effect otherwise. GPT-5.6 and later models
	// require a prompt cache key to use the more reliable cache matching. Name
	// is a required field so it is always non-empty, and hashing an empty
	// string would still be a valid, stable key regardless.
	openAIV3Params.PromptCacheKey = utils.Ptr(promptCacheKeyFor(cfg))

	cfg.ModelParams = openAIV3Params
	if useChatCompletionsAPI(cfg.Model) {
		logger.Message(ctx, logging.LevelInfo, "using Chat Completions API")
		return o.completionProvider.Run(ctx, logger, cfg, task)
	}
	logger.Message(ctx, logging.LevelInfo, "using Responses API")
	return o.responsesProvider.Run(ctx, logger, cfg, task)
}

func (o *OpenAI) Close(ctx context.Context) error {
	return errors.Join(
		o.completionProvider.Close(ctx),
		o.responsesProvider.Close(ctx),
	)
}

// copyToOpenAIV3Params copies relevant fields from OpenAIModelParams to openAIV3ModelParams.
func (o *OpenAI) copyToOpenAIV3Params(openAIModelParams config.OpenAIModelParams, openAIV3Params *openAIV3ModelParams) {
	if openAIModelParams.TextResponseFormat {
		openAIV3Params.ResponseFormat = ResponseFormatText.Ptr()
	}

	openAIV3Params.ReasoningEffort = openAIModelParams.ReasoningEffort
	openAIV3Params.ReasoningContext = openAIModelParams.ReasoningContext
	openAIV3Params.ReasoningMode = openAIModelParams.ReasoningMode
	openAIV3Params.Verbosity = openAIModelParams.Verbosity
	if openAIModelParams.Temperature != nil {
		openAIV3Params.Temperature = utils.Ptr(float64(*openAIModelParams.Temperature))
	}
	if openAIModelParams.TopP != nil {
		openAIV3Params.TopP = utils.Ptr(float64(*openAIModelParams.TopP))
	}
	if openAIModelParams.MaxCompletionTokens != nil {
		openAIV3Params.MaxCompletionTokens = utils.Ptr(int64(*openAIModelParams.MaxCompletionTokens))
	}
	if openAIModelParams.MaxTokens != nil {
		openAIV3Params.MaxTokens = utils.Ptr(int64(*openAIModelParams.MaxTokens))
	}
	if openAIModelParams.PresencePenalty != nil {
		openAIV3Params.PresencePenalty = utils.Ptr(float64(*openAIModelParams.PresencePenalty))
	}
	if openAIModelParams.FrequencyPenalty != nil {
		openAIV3Params.FrequencyPenalty = utils.Ptr(float64(*openAIModelParams.FrequencyPenalty))
	}
	openAIV3Params.Seed = openAIModelParams.Seed
}

// chatCompletionModelPrefixes lists model name prefixes that should be routed to
// the legacy Chat Completions API. All other models default to the Responses API.
var chatCompletionModelPrefixes = []string{
	"gpt-3", // gpt-3.5-turbo (sunset Sep 2026)
	"gpt-4", // gpt-4o, gpt-4o-mini, gpt-4.1, gpt-4.1-mini, gpt-4.1-nano
	"o1",    // o1 reasoning model
	"o3",    // o3 reasoning model
	"o4",    // o4-mini reasoning model
}

// useChatCompletionsAPI reports whether the given model should use the legacy
// Chat Completions API instead of the Responses API. Known pre-GPT-5 model
// families are routed to Chat Completions; all other models (including future
// ones) default to the Responses API.
func useChatCompletionsAPI(model string) bool {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	return slices.ContainsFunc(chatCompletionModelPrefixes, func(prefix string) bool {
		return strings.HasPrefix(normalizedModel, prefix)
	})
}
