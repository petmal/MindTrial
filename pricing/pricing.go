// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package pricing provides per-model token pricing data for estimating
// the dollar cost of AI model evaluation runs.
//
// Prices are expressed in USD per million tokens and sourced from each
// provider's published pricing pages. Update this file when providers
// change their rates or new models are added.
package pricing

import "strings"

// ModelPrice holds input and output token costs in USD per million tokens.
type ModelPrice struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// Lookup returns the pricing for a given provider and model.
// Returns zero pricing and false if the model is not found.
func Lookup(provider, model string) (ModelPrice, bool) {
	provider = strings.ToLower(provider)
	model = strings.ToLower(model)

	if models, ok := catalog[provider]; ok {
		if price, ok := models[model]; ok {
			return price, true
		}
	}
	return ModelPrice{}, false
}

// Cost computes the estimated dollar cost given token counts and a ModelPrice.
func Cost(price ModelPrice, inputTokens, outputTokens int64) float64 {
	inputCost := float64(inputTokens) * price.InputPerMillion / 1_000_000
	outputCost := float64(outputTokens) * price.OutputPerMillion / 1_000_000
	return inputCost + outputCost
}

// EstimateCost is a convenience that combines Lookup and Cost.
// Returns 0.0 if pricing data is not available for the given model.
func EstimateCost(provider, model string, inputTokens, outputTokens int64) float64 {
	price, ok := Lookup(provider, model)
	if !ok {
		return 0
	}
	return Cost(price, inputTokens, outputTokens)
}

// catalog maps provider → model → pricing.
// Keep models in lowercase; Lookup normalises keys before searching.
//
// Sources (update links when refreshing prices):
//   - OpenAI:    https://openai.com/api/pricing/
//   - Google:    https://ai.google.dev/pricing
//   - Anthropic: https://www.anthropic.com/pricing
//   - DeepSeek:  https://platform.deepseek.com/api-docs/pricing
//   - Mistral:   https://mistral.ai/products/pricing
//   - xAI:       https://docs.x.ai/docs/models#models-and-pricing
//   - Alibaba:   https://www.alibabacloud.com/help/en/model-studio/developer-reference/billing
//   - Moonshot:  https://platform.moonshot.cn/docs/pricing
var catalog = map[string]map[string]ModelPrice{
	"openai": {
		"gpt-4o-mini":  {InputPerMillion: 0.15, OutputPerMillion: 0.60},
		"o1-mini":      {InputPerMillion: 1.10, OutputPerMillion: 4.40},
		"o3-mini":      {InputPerMillion: 1.10, OutputPerMillion: 4.40},
		"o4-mini":      {InputPerMillion: 1.10, OutputPerMillion: 4.40},
		"o3":           {InputPerMillion: 10.00, OutputPerMillion: 40.00},
		"gpt-5-mini":   {InputPerMillion: 1.50, OutputPerMillion: 6.00},
		"gpt-5":        {InputPerMillion: 5.00, OutputPerMillion: 20.00},
		"gpt-5.2":      {InputPerMillion: 5.00, OutputPerMillion: 20.00},
		"gpt-5.4":      {InputPerMillion: 2.50, OutputPerMillion: 15.00},
		"gpt-5.4-pro":  {InputPerMillion: 30.00, OutputPerMillion: 180.00},
	},
	"openrouter": {
		"openai/gpt-5.2":                    {InputPerMillion: 5.00, OutputPerMillion: 20.00},
		"google/gemma-3-27b-it:free":        {InputPerMillion: 0, OutputPerMillion: 0},
		"prime-intellect/intellect-3":       {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"inception/mercury":                 {InputPerMillion: 0.25, OutputPerMillion: 1.00},
		"bytedance-seed/seed-1.6":           {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"z-ai/glm-4.6v":                    {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"z-ai/glm-4.7":                     {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"z-ai/glm-5":                       {InputPerMillion: 0.75, OutputPerMillion: 3.00},
		"stepfun-ai/step3":                  {InputPerMillion: 0.50, OutputPerMillion: 2.00},
	},
	"google": {
		"gemini-1.5-flash":                    {InputPerMillion: 0.075, OutputPerMillion: 0.30},
		"gemini-2.0-flash":                    {InputPerMillion: 0.10, OutputPerMillion: 0.40},
		"gemini-2.0-flash-thinking-exp":       {InputPerMillion: 0.10, OutputPerMillion: 0.40},
		"gemini-2.5-flash":                    {InputPerMillion: 0.15, OutputPerMillion: 0.60},
		"gemini-2.5-pro":                      {InputPerMillion: 1.25, OutputPerMillion: 5.00},
		"gemini-3-pro-preview":                {InputPerMillion: 2.00, OutputPerMillion: 8.00},
		"gemini-3.1-pro-preview":              {InputPerMillion: 2.00, OutputPerMillion: 8.00},
		"gemini-3.1-pro-preview-customtools":  {InputPerMillion: 2.00, OutputPerMillion: 8.00},
		"gemini-3-flash-preview":              {InputPerMillion: 0.15, OutputPerMillion: 0.60},
		"gemini-3.1-flash-lite-preview":       {InputPerMillion: 0.075, OutputPerMillion: 0.30},
	},
	"anthropic": {
		"claude-3-7-sonnet-latest": {InputPerMillion: 3.00, OutputPerMillion: 15.00},
		"claude-sonnet-4-0":        {InputPerMillion: 3.00, OutputPerMillion: 15.00},
		"claude-opus-4-0":          {InputPerMillion: 15.00, OutputPerMillion: 75.00},
		"claude-opus-4-1":          {InputPerMillion: 15.00, OutputPerMillion: 75.00},
		"claude-sonnet-4-5":        {InputPerMillion: 3.00, OutputPerMillion: 15.00},
		"claude-opus-4-5":          {InputPerMillion: 15.00, OutputPerMillion: 75.00},
		"claude-haiku-4-5":         {InputPerMillion: 0.80, OutputPerMillion: 4.00},
		"claude-sonnet-4-6":        {InputPerMillion: 3.00, OutputPerMillion: 15.00},
		"claude-opus-4-6":          {InputPerMillion: 5.00, OutputPerMillion: 25.00},
	},
	"deepseek": {
		"deepseek-reasoner": {InputPerMillion: 0.55, OutputPerMillion: 2.19},
		"deepseek-chat":     {InputPerMillion: 0.27, OutputPerMillion: 1.10},
	},
	"mistralai": {
		"mistral-large-latest":    {InputPerMillion: 2.00, OutputPerMillion: 6.00},
		"magistral-medium-latest": {InputPerMillion: 2.00, OutputPerMillion: 5.00},
		"pixtral-large-latest":    {InputPerMillion: 2.00, OutputPerMillion: 6.00},
		"mistral-medium-latest":   {InputPerMillion: 0.40, OutputPerMillion: 2.00},
	},
	"xai": {
		"grok-4-latest":                     {InputPerMillion: 3.00, OutputPerMillion: 15.00},
		"grok-4-1-fast-reasoning-latest":    {InputPerMillion: 3.00, OutputPerMillion: 15.00},
	},
	"alibaba": {
		"qwen3-max-preview":              {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"qwen-vl-max-latest":             {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"qwen3-235b-a22b-thinking-2507":  {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"qwen3-next-80b-a3b-thinking":    {InputPerMillion: 0.30, OutputPerMillion: 1.20},
		"qwen3-max-2026-01-23":           {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"qvq-max":                        {InputPerMillion: 0.50, OutputPerMillion: 2.00},
		"qwq-plus":                       {InputPerMillion: 0.30, OutputPerMillion: 1.20},
	},
	"moonshotai": {
		"kimi-k2-thinking": {InputPerMillion: 0.60, OutputPerMillion: 2.40},
		"kimi-k2.5":        {InputPerMillion: 0.60, OutputPerMillion: 2.40},
	},
}
