# MindTrial

[![Build](https://github.com/petmal/mindtrial/actions/workflows/go.yml/badge.svg)](https://github.com/petmal/mindtrial/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/petmal/mindtrial)](https://goreportcard.com/report/github.com/petmal/mindtrial)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://mozilla.org/MPL/2.0/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/petmal/mindtrial)](https://go.dev/)
[![Go Reference](https://pkg.go.dev/badge/github.com/petmal/mindtrial.svg)](https://pkg.go.dev/github.com/petmal/mindtrial)

**MindTrial** lets you test a single AI language model (LLM) or evaluate multiple models side-by-side. It supports providers like OpenAI, Google, Anthropic, DeepSeek, Mistral AI, xAI, Alibaba, and Moonshot AI. You can create your own custom tasks with text prompts, plain text or structured JSON response formats, optional file attachments, and tool use for enhanced capabilities; validate responses through exact value matching or an LLM judge for semantic evaluation; and get results in easy-to-read HTML and CSV formats.

## Quick Start Guide

1. Install the tool:

   ```bash
   go install github.com/petmal/mindtrial/cmd/mindtrial@latest
   ```

2. Run with default settings:

   ```bash
   mindtrial run
   ```

### Prerequisites

- [Go 1.24](https://golang.org/dl/)
- [Docker](https://www.docker.com/) (for tool execution)
- API keys from your chosen AI providers

## Key Features

- Compare multiple AI models at once
- Create custom evaluation tasks using simple YAML files
- Attach files or images to prompts for visual tasks
- Enable tool use for tasks with secure sandboxed execution
- Use LLM judges for semantic validation of complex and creative tasks
- Get results in HTML and CSV formats
- Easy to extend with new AI models
- Smart rate limiting to prevent API overload
- Interactive mode with terminal-based UI

## Basic Usage

1. Display available commands and options:

   ```bash
   mindtrial help
   ```

2. Run with custom configuration and output options:

   ```bash
   mindtrial --config="custom-config.yaml" --tasks="custom-tasks.yaml" --output-dir="./results" --output-basename="custom-tasks-results" run
   ```

3. Run with specific output formats (CSV only, no HTML):

   ```bash
   mindtrial --csv=true --html=false run
   ```

4. Run in interactive mode to select models and tasks before starting:

   ```bash
   mindtrial --interactive run
   ```

## Configuration Guide

MindTrial uses two simple YAML files to control everything:

### 1. config.yaml - Application Settings

Controls how MindTrial operates, including:

- Where to save results
- Which AI models to use
- API settings and rate limits

### 2. tasks.yaml - Task Definitions

Defines what you want to evaluate, including:

- Questions/prompts for the AI
- Expected answers
- Response format rules

> [!TIP]
> **New to MindTrial?** Start with the example files provided and modify them for your needs.

> [!TIP]
> Use **interactive mode** with the `--interactive` flag to select model configurations and tasks before running, without having to edit configuration files.

### config.yaml

This file defines the tool's settings and target model configurations evaluated during the trial run. The following properties are required:

- **output-dir**: Path to the directory where results will be saved.
- **task-source**: Path to the file with definitions of tasks to run.
- **providers**: List of providers (i.e. target LLM configurations) to execute tasks during the trial run.
  - **name**: Name of the LLM provider (e.g. *openai*).
  - **client-config**: Configuration for this provider's client (e.g. *API key*).
  - **runs**: List of runs (i.e. model configurations) for this provider. Unless disabled, all configurations will be trialed.
    - **name**: A unique display-friendly name to be shown in the results.
    - **model**: Model name must be exactly as defined by the backend service's API (e.g. *gpt-4o-mini*).

> [!IMPORTANT]
> All provider names must match exactly:
>
> - **openai**: OpenAI GPT models
> - **google**: Google Gemini models
> - **anthropic**: Anthropic Claude models
> - **deepseek**: DeepSeek open-source models
> - **mistralai**: Mistral AI models
> - **xai**: xAI (Grok) models
> - **alibaba**: Alibaba (Qwen) models
> - **moonshotai**: Moonshot AI (Kimi) models

> [!NOTE]
> **Anthropic** and **DeepSeek** providers support configurable request timeout in the `client-config` section:
>
> - **request-timeout**: Sets the timeout duration for API requests (i.e. thinking).
>
> **Alibaba** and **Moonshot AI** providers support endpoint configuration in the `client-config` section:
>
> - **endpoint**: Specifies the network endpoint URL for the API. If not specified, defaults are:
>   - **Alibaba**: *Singapore* endpoint (`https://dashscope-intl.aliyuncs.com/compatible-mode/v1`) for better international access. For *China* mainland, use `https://dashscope.aliyuncs.com/compatible-mode/v1`.
>   - **Moonshot AI**: Public API endpoint (`https://api.moonshot.ai/v1`).

> [!NOTE]
> Some models support additional model-specific runtime configuration parameters.
> These can be provided in the `model-parameters` section of the run configuration.
>
> Currently supported parameters for **OpenAI** models include:
>
> - **text-response-format**: If `true`, use plain-text response format (less reliable) for compatibility with models that do not support `JSON`.
> - **reasoning-effort**: Controls effort on reasoning for reasoning models (i.e. *low*, *medium*, *high*).
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 2.0, default: 1.0). Lower values produce more focused and deterministic outputs.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0, default: 1.0). Lower values produce more focused outputs.
> - **presence-penalty**: Penalizes new tokens based on their presence in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use new tokens.
> - **frequency-penalty**: Penalizes new tokens based on their frequency in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use less frequent tokens.
> - **max-completion-tokens**: Controls the maximum number of tokens available to the model for generating a response.
>
> Currently supported parameters for **Anthropic** models include:
>
> - **max-tokens**: Controls the maximum number of tokens available to the model for generating a response.
> - **thinking-budget-tokens**: Enables enhanced reasoning capabilities when set. Specifies the number of tokens the model can use for its internal reasoning process. Must be at least 1024 and less than `max-tokens`.
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 1.0, default: 1.0). Lower values produce more focused and deterministic outputs.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0). Lower values produce more focused outputs.
> - **top-k**: Limits tokens considered for each position to top K options. Higher values allow more diverse outputs.
>
> Currently supported parameters for **Google** models include:
>
> - **text-response-format**: If `true`, use plain-text response format (less reliable) for compatibility with models that do not support `JSON`.
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 2.0, default: 1.0). Lower values produce more focused and deterministic outputs.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0). Lower values produce more focused outputs.
> - **top-k**: Limits tokens considered for each position to top K options. Higher values allow more diverse outputs.
> - **presence-penalty**: Penalizes new tokens based on whether they appear in the text so far. Positive values discourage reuse of tokens, increasing vocabulary. Negative values encourage token reuse.
> - **frequency-penalty**: Penalizes new tokens based on their frequency in the text so far. Positive values discourage frequent tokens proportionally. Negative values encourage token repetition.
> - **seed**: Seed used for deterministic generation. When set, the model attempts to provide consistent responses for identical inputs.
>
> Currently supported parameters for **DeepSeek** models include:
>
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 2.0, default: 1.0). Lower values produce more focused and deterministic outputs.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0). Lower values produce more focused outputs.
> - **presence-penalty**: Penalizes new tokens based on their presence in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use new tokens.
> - **frequency-penalty**: Penalizes new tokens based on their frequency in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use less frequent tokens.
>
> Currently supported parameters for **Mistral AI** models include:
>
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 1.5). Lower values produce more focused and deterministic outputs.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0). Lower values produce more focused outputs.
> - **max-tokens**: Controls the maximum number of tokens available to the model for generating a response.
> - **presence-penalty**: Penalizes new tokens based on their presence in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use new tokens.
> - **frequency-penalty**: Penalizes new tokens based on their frequency in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use less frequent tokens.
> - **random-seed**: Provides the seed to use for random sampling. If set, requests will generate deterministic results.
> - **prompt-mode**: When set to "reasoning", instructs the model to reason if supported.
> - **safe-prompt**: Enables content filtering to ensure outputs comply with usage policies.
>
> Currently supported parameters for **xAI** models include:
>
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 2.0, default: 1.0). Lower values produce more focused and deterministic outputs.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0, default: 1.0). Lower values produce more focused outputs.
> - **max-completion-tokens**: Controls the maximum number of tokens available to the model for generating a response.
> - **presence-penalty**: Penalizes new tokens based on their presence in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use new tokens.
> - **frequency-penalty**: Penalizes new tokens based on their frequency in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use less frequent tokens.
> - **reasoning-effort**: Controls effort on reasoning for supported reasoning-capable models (values: `low`, `high`). Not all xAI reasoning models (i.e. Grok 4) accept this parameter.
> - **seed**: Integer seed to request deterministic sampling when possible. Determinism is best-effort. xAI makes a best-effort to return repeatable outputs for identical inputs when `seed` and other parameters are the same.
>
> Currently supported parameters for **Alibaba** models include:
>
> - **text-response-format**: If `true`, use plain-text response format (less reliable) for compatibility with models that do not support `JSON` (for example, when thinking is enabled on certain Qwen models).
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 2.0, default: 1.0). Lower values produce more focused and deterministic outputs.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0). Lower values produce more focused outputs.
> - **max-tokens**: Controls the maximum number of tokens available to the model for generating a response.
> - **presence-penalty**: Penalizes new tokens based on whether they appear in the text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage introducing new topics.
> - **frequency-penalty**: Penalizes new tokens based on their frequency in text so far (range: -2.0 to 2.0, default: 0.0). Positive values encourage model to use less frequent tokens.
> - **seed**: Makes text generation more deterministic by using the same seed value. When using the same seed and keeping other parameters unchanged, the model makes best-effort to return consistent outputs for identical inputs.
> - **disable-legacy-json-mode**: Compatibility toggle that controls legacy prompt injection for JSON formatting. Default: `false` (legacy mode on), which adds an explicit JSON formatting instruction to the prompt for improved compatibility with most Qwen models. Setting this to `true` disables the legacy prompt injection. For best compatibility and reliable JSON responses, keep this set to `false` unless you are certain the target model works correctly without legacy prompt injection.
>
> Currently supported parameters for **Moonshot AI** models include:
>
> - **temperature**: Controls randomness/creativity of responses (range: 0.0 to 1.0, default: 0.0). Higher values make output more random, while lower values make it more focused and deterministic. Moonshot AI recommends 0.6 for `kimi-k2` models and 1.0 for `kimi-k2-thinking` models.
> - **top-p**: Controls diversity via nucleus sampling (range: 0.0 to 1.0, default: 1.0). Lower values produce more focused outputs. Generally, change either this or temperature, but not both at the same time.
> - **max-tokens**: Controls the maximum number of tokens to generate for the chat completion.
> - **presence-penalty**: Penalizes new tokens based on whether they appear in the text (range: -2.0 to 2.0, default: 0.0). Positive values increase the likelihood of the model discussing new topics.
> - **frequency-penalty**: Penalizes new tokens based on their existing frequency in the text (range: -2.0 to 2.0, default: 0.0). Positive values reduce the likelihood of the model repeating the same phrases verbatim.

> [!NOTE]
> The results will be saved to `<output-dir>/<output-basename>.<format>`. If the result output file already exists, it will be replaced. If the log file already exists, it will be appended to.

> [!TIP]
> The following placeholders are available for output paths and names:
>
> - **{{.Year}}**: Current year
> - **{{.Month}}**: Current month
> - **{{.Day}}**: Current day
> - **{{.Hour}}**: Current hour
> - **{{.Minute}}**: Current minute
> - **{{.Second}}**: Current second

> [!TIP]
> If `log-file` and/or `output-basename` is blank, the log and/or output will be written to the `stdout`.

> [!NOTE]
> MindTrial processes tasks across different AI providers simultaneously (in parallel). However, when running multiple configurations from the same provider (e.g. different OpenAI models), these are processed one after another (sequentially).

> [!TIP]
> Models can use the `max-requests-per-minute` property in their run configurations to limit the number of requests made per minute.

> [!TIP]
> To automatically retry failed requests due to rate limiting or other transient errors, set `retry-policy` at the provider level to apply to all runs.
> An individual run configuration can override this by setting its own `retry-policy`:
>
> - **max-retry-attempts**: Maximum number of retry attempts (default: 0 means no retry).
> - **initial-delay-seconds**: Initial delay before the first retry in seconds.
>
> Retries use exponential backoff starting with the initial delay.

> [!TIP]
> To disable all run configurations for a given provider, set `disabled: true` on that provider.
> An individual run configuration can override this by setting `disabled: false` (e.g. to enable just that one configuration).

Example snippet from `config.yaml`:

```yaml
# config.yaml
config:
  log-file: ""
  output-dir: "./results/{{.Year}}-{{.Month}}-{{.Day}}/"
  output-basename: "{{.Hour}}-{{.Minute}}-{{.Second}}"
  task-source: "./tasks.yaml"
  providers:
    - name: openai
      disabled: true
      client-config:
        api-key: "<your-api-key>"
      retry-policy:
        max-retry-attempts: 5
        initial-delay-seconds: 30
      runs:
        - name: "4o-mini - latest"
          disabled: false
          model: "gpt-4o-mini"
          max-requests-per-minute: 3
        - name: "o1-mini - latest"
          model: "o1-mini"
          max-requests-per-minute: 3
          model-parameters:
            text-response-format: true
        - name: "o3-mini - latest (high reasoning)"
          model: "o3-mini"
          max-requests-per-minute: 3
          model-parameters:
            reasoning-effort: "high"
    - name: anthropic
      client-config:
        api-key: "<your-api-key>"
      runs:
        - name: "Claude 3.7 Sonnet - latest"
          model: "claude-3-7-sonnet-latest"
          max-requests-per-minute: 5
          model-parameters:
            max-tokens: 4096
        - name: "Claude 3.7 Sonnet - latest (extended thinking)"
          model: "claude-3-7-sonnet-latest"
          max-requests-per-minute: 5
          model-parameters:
            max-tokens: 8192
            thinking-budget-tokens: 2048
    - name: deepseek
      client-config:
        api-key: "<your-api-key>"
        request-timeout: 10m
      runs:
        - name: "DeepSeek-V3.1 - latest (thinking mode)"
          model: "deepseek-reasoner"
          max-requests-per-minute: 15
    - name: mistralai
      client-config:
        api-key: "<your-api-key>"
      runs:
        - name: "Mistral Large - latest"
          model: "mistral-large-latest"
          max-requests-per-minute: 5
          retry-policy:
            max-retry-attempts: 5
            initial-delay-seconds: 30
    - name: alibaba
      client-config:
        api-key: "<your-api-key>"
        endpoint: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"  # Singapore region
      retry-policy:
        max-retry-attempts: 5
        initial-delay-seconds: 30
      runs:
        - name: "Qwen3-Max-Preview"
          model: "qwen3-max-preview"
          max-requests-per-minute: 30
        - name: "Qwen-VL-Max-Latest"
          model: "qwen-vl-max-latest"
          max-requests-per-minute: 30
          model-parameters:
            disable-legacy-json-mode: true
        - name: "Qwen3-Next-80B-A3B-Thinking"
          model: "qwen3-next-80b-a3b-thinking"
          max-requests-per-minute: 30
          model-parameters:
            text-response-format: true
```

### tasks.yaml

This file defines the tasks to be executed on all enabled run configurations. Each task must define the following four properties:

- **name**: A unique display-friendly name to be shown in the results.
- **prompt**: The prompt (i.e. task) that will be sent to the AI model.
- **response-result-format**: Defines how the AI should format the final answer to the prompt. This can be either:
  - **Plain text format**: A string instruction describing the expected answer format (e.g., "single number", "list of words separated by commas").
  - **Structured schema format**: A JSON schema object defining the structure of the expected response for complex data (e.g., objects with specific fields and types).
- **expected-result**: Defines the accepted valid answer(s) to the prompt. The format depends on the `response-result-format` type:
  - **For plain text format**: A string value or list of string values that follow the format instruction precisely.
  - **For structured schema format**: An object value or list of object values that conform to the JSON schema definition.
Only one expected result needs to match for the response to be considered correct.

Optionally, a task can include a list of `files` to be sent along with the prompt:

- **files**: A list of files to attach to the prompt. Each file entry defines the following properties:
  - **name**: A unique name for the file, used for reference within the prompt if needed.
  - **uri**: The path or URI to the file. Local file paths and remote HTTP/HTTPS URLs are supported. The file content will be downloaded and sent with the request.
  - **type**: The MIME type of the file (e.g., `image/png`, `image/jpeg`). If omitted, the tool will attempt to infer the type based on the file extension or content.

> [!NOTE]
> If a task includes files, it will be skipped for any provider configuration that does not support file uploads or does not support the specific file type.

> [!NOTE]
> Currently supported image types include: `image/jpeg`, `image/jpg`, `image/png`, `image/gif`, `image/webp`. Support may vary by provider.

> [!TIP]
> To disable all tasks by default, set `disabled: true` in the `task-config` section.
> An individual task can override this by setting `disabled: false` (e.g. to enable just that one task).

#### Structured Response Formats

MindTrial supports two types of response formats for tasks:

##### Plain Text Format

For tasks where the final answer can be represented as a text value:

```yaml
- name: "math problem"
  prompt: "What is 2 + 2?"
  response-result-format: "single number"
  expected-result: "4"
```

##### Structured Schema Format

For tasks requiring complex structured answers, you can define a JSON schema that describes the expected response format:

```yaml
- name: "perfect square check"
  prompt: "For each number in [4, 9, 10, 16], determine if it's a perfect square and if so, provide the square root."
  response-result-format:
    type: array
    items:
      type: object
      additionalProperties: false
      properties:
        number:
          type: integer
        is_perfect_square:
          type: boolean
        square_root:
          type: integer
      required: ["number", "is_perfect_square"]
  expected-result:
    - - number: 4
        is_perfect_square: true
        square_root: 2
      - number: 9
        is_perfect_square: true
        square_root: 3
      - number: 10
        is_perfect_square: false
      - number: 16
        is_perfect_square: true
        square_root: 4
```

> [!IMPORTANT]
> **Structured schema format caveats:**
>
> - Semantic validation (LLM judges) cannot be used with structured schema-based response formats.
> - All expected results must be objects that conform to the same schema. For array schemas, the entire expected array must be wrapped in a single list item under `expected-result` to avoid treating each array element as a separate expected answer.
> - Models must support structured JSON response generation for reliable results.
> - The *OpenAI* provider requires JSON schemas to have `additionalProperties: false` and all fields must be required (no optional fields allowed). Other providers may be more flexible.

#### System Prompt

The system prompt controls how the response format instruction is presented to the AI model.

You can customize this template globally for all tasks in the `task-config` section, and override it for individual tasks if needed. The template uses Go's template syntax and can reference `{{.ResponseResultFormat}}` to include the task's `response-result-format`.

Default system prompt for all tasks is:

> Provide the final answer in exactly this format: {{.ResponseResultFormat}}

- **system-prompt**: A configuration section for the system prompt.
  - **template**: The template string for the system prompt instruction. If not specified, uses the default.
  - **enable-for**: Controls when system prompt should be sent to AI models. Options:
    - `"all"`: Send system prompt for all tasks (both plain text and structured schema formats).
    - `"text"`: Send system prompt only for tasks with plain text response format (default).
    - `"none"`: Do not send system prompt.

> [!NOTE]
> For structured schema response formats, the JSON schema is automatically passed to the AI model through the provider's structured response mechanism, making explicit format instructions in the system prompt optional.

#### Validation Rules

These rules control how the validator compares the model's answer to the expected results.
By default, comparisons are case-insensitive and only trim leading and trailing whitespace.

You can set validation rules globally for all tasks in the `task-config` section, and override them for individual tasks if needed; any option not specified at the task level will inherit the global setting from `task-config`:

- **validation-rules**: Controls how model responses are validated against expected results.
  - **case-sensitive**: If `true`, comparison is case-sensitive. If `false` (default), comparison ignores case.
  - **ignore-whitespace**: If `true`, all whitespace (spaces, tabs, newlines) is removed before comparison. If `false` (default), only leading/trailing whitespace is trimmed, and internal whitespace is preserved.
  - **trim-lines**: If `true`, trims leading and trailing whitespace from each line before comparison while preserving internal spaces within lines. CRLF line endings are normalized to LF. This option is ignored when `ignore-whitespace` is enabled. If `false` (default), lines are not individually trimmed.
  - **judge**: Optional configuration for LLM-based semantic validation instead of exact value matching.
    - **enabled**: If `true`, uses an LLM judge to evaluate semantic equivalence. If `false` (default), uses exact value matching.
    - **name**: The name of the judge configuration defined in the `config.yaml` file.
    - **variant**: The specific run variant from the judge's provider to use.

#### Judge-Based Validation

For complex or open-ended tasks where exact value matching is insufficient, you can configure LLM judges to evaluate responses semantically. This is particularly useful for creative writing, reasoning tasks, or when multiple valid answer formats exist.

**How it works:** Instead of comparing text exactly, an LLM judge evaluates whether the model's response semantically matches the expected result, considering meaning and intent rather than exact wording.

To use judge validation:

1. **Define and configure judge models in `config.yaml`:**

    ```yaml
    config:
      # ... existing configuration ...
      judges:
        - name: "mistral-judge"  # A unique name for the judge configuration.
          provider:
            name: "mistralai"
            client-config:
              api-key: "<your-api-key>"
            runs:
              - name: "fast"
                model: "mistral-medium-latest"
                max-requests-per-minute: 30
                model-parameters:
                  temperature: 0.20
                  random-seed: 847629
              - name: "reasoning"
                model: "magistral-medium-latest"
                max-requests-per-minute: 30
                model-parameters:
                  prompt-mode: "reasoning"
                  temperature: 0.20
                  random-seed: 847629
        - name: "deepseek-judge"
          provider:
            name: "deepseek"
            client-config:
              api-key: "<your-api-key>"
            runs:
              - name: "fast"
                model: "deepseek-chat"
                max-requests-per-minute: 30
                model-parameters:
                  temperature: 0.20
              - name: "reasoning"
                model: "deepseek-reasoner"
                max-requests-per-minute: 30
    ```

2. **Enable judge validation in `tasks.yaml`:**

    ```yaml
    # Enable globally for all tasks.
    task-config:
      validation-rules:
        judge:
          enabled: true
          name: "mistral-judge"
          variant: "fast"
      tasks:
        # ... tasks will use judge validation by default ...

    # Override per-task (inherit global settings and override specific options).
    task-config:
      validation-rules:
        judge:
          enabled: false  # Default: use exact value matching.
          name: "mistral-judge"
          variant: "fast"
      tasks:
        - name: "exact matching task"
          prompt: "What is 2+2?"
          response-result-format: "single number"
          expected-result: "4"
          # Inherits global validation-rules (exact value matching).
        
        - name: "creative writing task"
          prompt: "Write a short story about..."
          response-result-format: "short story narrative"
          expected-result: "A creative and engaging short story"
          validation-rules:
            judge:
              enabled: true  # Override: enable judge validation for this task.
              # Inherits name: "mistral-judge" and variant: "fast" from global config.
        
        - name: "complex reasoning task"
          prompt: "Analyze this philosophical argument..."
          response-result-format: "structured analysis with reasoning"
          expected-result: "A thoughtful analysis with logical reasoning"
          validation-rules:
            judge:
              enabled: true
              variant: "reasoning"  # Override: use reasoning run variant instead of fast.
              # Inherits name: "mistral-judge" from global config.
    ```

#### Judge Prompt Customization

MindTrial automatically applies a built-in semantic evaluation template that compares candidate responses against expected answers. For advanced use cases, you can customize the judge prompt template, response format, and acceptance criteria.

Judge prompts can be customized in the `validation-rules.judge.prompt` section of your `tasks.yaml` file, either globally in `task-config` or individually per task.

**Customization Fields:**

- **template**: Custom prompt template for the judge (supports template variables listed below).
- **verdict-format**: Expected response format from the judge (plain text instruction or JSON schema).
- **passing-verdicts**: Set of verdict values that indicate a passing evaluation.

> [!IMPORTANT]
>
> - If you provide a custom `template`, you **must** also specify both `verdict-format` and `passing-verdicts`.
> - You **cannot** override just `verdict-format` or `passing-verdicts` unless you also override the `template`.
> - All `passing-verdicts` values must conform to the `verdict-format` structure.

> [!TIP]
> The following template variables are available for judge prompts:
>
> - **{{.OriginalTask.Prompt}}**: The original task prompt
> - **{{.OriginalTask.ResponseResultFormat}}**: Format instruction from the task
> - **{{.OriginalTask.ExpectedResults}}**: Array of expected answers
> - **{{.Candidate.Response}}**: The model's response being evaluated
> - **{{.Rules.CaseSensitive}}**: Boolean case-sensitive validation flag
> - **{{.Rules.IgnoreWhitespace}}**: Boolean ignore whitespace flag
> - **{{.Rules.TrimLines}}**: Boolean trim lines flag

A sample task from `tasks.yaml`:

```yaml
# tasks.yaml
task-config:
  disabled: true
  system-prompt:
    enable-for: "text"
    template: |
      Provide the final answer in exactly this format: {{.ResponseResultFormat}}
      Treat every substring enclosed in `<` and `>` as a variable placeholder.
      Substitute only the raw value in place of `<variable name>`, removing the `<` and `>` characters.
      Do not add any extra words, punctuation, quotes, or whitespace beyond what the format string shows.
  validation-rules:
    case-sensitive: false
    ignore-whitespace: false
  tasks:
    - name: "riddle - split words - v1"
      disabled: false
      prompt: |-
        There are four 8-letter words (animals) that have been split into 2-letter pieces.
        Find these four words by putting appropriate pieces back together:

        RR TE KA DG EH AN SQ EL UI OO HE LO AR PE NG OG
      response-result-format: |-
        list of words in alphabetical order separated by ", "
      system-prompt:
        template: "Provide the final answer in exactly this format: {{.ResponseResultFormat}}"
      expected-result: |-
        ANTELOPE, HEDGEHOG, KANGAROO, SQUIRREL
    - name: "visual - shapes - v1"
      prompt: |-
        The attached picture contains various shapes marked by letters.
        It also contains a set of same shapes that have been rotated marked by numbers.
        Your task is to find all matching pairs.
      response-result-format: |-
        <shape number>: <shape letter> pairs separated by ", " and ordered by shape number
      expected-result: |-
        1: G, 2: F, 3: B, 4: A, 5: C, 6: D, 7: E
      validation-rules:
        ignore-whitespace: true
      files:
        - name: "picture"
          uri: "./taskdata/visual-shapes-v1.png"
          type: "image/png"
    - name: "riddle - anagram - v3"
      prompt: |-
        Two words (each individual word is a fruit) have been combined and their letters arranged in alphabetical order forming a single group.
        Find the original words for each of these 2 groups:

        1. AACEEGHPPR
        2. ACEILMNOOPRT
      response-result-format: |-
        1. <word>, <word>
        2. <word>, <word>
        (words in each group must be alphabetically ordered)
      expected-result:
        - |
          1. GRAPE, PEACH
          2. APRICOT, MELON
        - |
          1. GRAPE, PEACH
          2. APRICOT, LEMON
    - name: "chemistry - observable phenomena - v1"
      disabled: true
      prompt: |-
        What are the primary observable results of mixing household vinegar (an aqueous solution of acetic acid, $CH_3COOH$) with baking soda (sodium bicarbonate, $NaHCO_3$)?
      response-result-format: |-
        Provide a bulleted list of the main, directly observable phenomena. Focus on what one would see and hear. Do not include the chemical equation.
      expected-result: |-
        The response must correctly identify the two main observable results of the chemical reaction.
        Crucially, it must mention the production of a gas, described as fizzing, bubbling, or effervescence.
        It should also note that the solid baking soda dissolves or disappears as it reacts with the vinegar.
      validation-rules:
        judge:
          enabled: true
          name: "mistral-judge"
          variant: "reasoning"
          # Uses default judge prompt configuration for semantic evaluation.
    - name: "code quality - custom judge"
      prompt: |-
        Write a Python function that finds the maximum value in a list.
      response-result-format: |-
        complete Python function with proper naming and structure
      expected-result: |-
        A well-written Python function that correctly finds the maximum value with good practices
      validation-rules:
        judge:
          enabled: true
          name: "mistral-judge"
          variant: "reasoning"
          prompt:
            template: |-
              Evaluate this Python code for both correctness and quality:
              {{.Candidate.Response}}

              Criteria: 1) Correctly finds max value, 2) Proper function name/structure, 3) Handles edge cases, 4) Good Python style
            verdict-format:
              type: object
              properties:
                quality_score:
                  type: string
                  enum: ["excellent", "good", "poor"]
              required: ["quality_score"]
              additionalProperties: false
            passing-verdicts:
              - quality_score: "excellent"
              - quality_score: "good"
    - name: "structured response - log parsing"
      prompt: |-
        Parse the following log lines and extract the timestamp, log level, and message for each. If a user ID is present, extract that as well.
        Log lines:
        [2025-09-14 10:30:00] INFO: User 'admin' logged in successfully.
        [2025-09-14 10:31:15] WARN: System memory usage is high.
      response-result-format:
        type: array
        items:
          type: object
          additionalProperties: false
          properties:
            timestamp:
              type: string
              format: "date-time"
            level:
              type: string
              enum: ["INFO", "WARN", "ERROR"]
            message:
              type: string
            user_id:
              type: string
          required: ["timestamp", "level", "message", "user_id"]
      expected-result:
        - - timestamp: "2025-09-14T10:30:00Z"
            level: "INFO"
            message: "User 'admin' logged in successfully."
            user_id: "admin"
          - timestamp: "2025-09-14T10:31:15Z"
            level: "WARN"
            message: "System memory usage is high."
            user_id: ""
```

#### Tools

MindTrial supports tool use for tasks, allowing AI models to execute external tools during task solving. Tools are executed in sandboxed Docker containers with resource limits and network isolation.

##### Tool Definitions

Tools must be defined in `config.yaml` under the `tools` section. Each tool defines how to execute a specific capability:

- **name**: A unique name for the tool.
- **image**: Docker image to use for the tool execution.
- **description**: A detailed description of what the tool does and how to use it. This description is provided to the LLM to help it understand when and how to use the tool. Be specific and avoid ambiguity to help the LLM choose the correct tool and provide appropriate parameters.
- **parameters**: JSON schema defining the tool's input parameters. The LLM will generate the actual parameter values based on this schema. Provide comprehensive descriptions that explain parameter purpose and format.
- **parameter-files**: Mapping of parameter names to container file paths where argument values should be written. Argument values are converted to strings, non-string values are marshaled to JSON. The tool's command should read these files as needed.
- **auxiliary-dir**: Directory path inside the container where task files will be automatically mounted. If specified, copies of all files attached to the task will be mounted to this directory using each file's unique reference `name` exactly as provided. Files in this directory are reset between tool calls.
- **shared-dir**: Directory path inside the container that persists across all tool calls within a single task. If specified, files created in this directory will be available for any subsequent tool calls but will be removed when the task completes.
- **command**: Command to run inside the container. The standard output of the command execution is captured and passed back to the LLM as is.
- **env**: Environment variables to set in the container.

> [!IMPORTANT]
> Tool use requires Docker to be installed and running on the system. Tools are executed in isolated containers with no network access by default.

Example tool definition in `config.yaml`:

```yaml
config:
  tools:
    - name: python-code-executor
      image: python:latest
      description: |
        Executes Python 3 code in a secure, sandboxed environment to perform calculations, data manipulation, or algorithmic tasks.
        IMPORTANT:
        - Only the Python standard library is available. No third-party packages (like pandas or numpy) can be imported.
        - The environment has no network access.
        - Any files mentioned in the conversation (shown as [file: filename]) are automatically mounted to /app/data/ with their exact filenames.
        - Use standard file operations like open('/app/data/filename', 'r') to read attached files, where 'filename' matches the name shown in [file: filename] references.
        - A persistent shared directory is available at /app/shared/ that persists across ALL tool calls within the same task (regardless of which tool is being called). Files created in this directory will be available in any subsequent tool call.
        - Any files or changes outside of /app/shared/ are ephemeral and will be reset between tool calls.
        - The code must print its final result to standard output to be returned.
      parameters:
        type: object
        properties:
          code:
            type: string
            description: "A string containing a self-contained Python 3 script. The script must use the `print()` function to return a final result. Example: `print(sum([i for i in range(101) if i % 2 == 0]))`. To read attached files, use open('/app/data/filename', 'r') where 'filename' matches what appears in [file: filename] references."
        required:
          - code
        additionalProperties: false
      parameter-files:
        code: /app/main.py
      auxiliary-dir: /app/data
      shared-dir: /app/shared
      command:
        - python
        - /app/main.py
      env:
        PYTHONIOENCODING: "UTF-8"
        PYTHONUNBUFFERED: "1"
        PYTHONHASHSEED: "847629"
```

##### Tool Selection

You can configure tool selection globally for all tasks in the `task-config` section, and override it for individual tasks if needed. Tools must be defined in `config.yaml` first.

- **tool-selector**: Configuration for tool availability during task execution.
  - **disabled**: If `true`, no tools are available for tasks (default: `false`).
  - **tools**: List of tools to make available, with per-tool limits.
    - **name**: Name of the tool as defined in `config.yaml`.
    - **disabled**: If `true`, this tool is not available (default: `false`).
    - **max-calls**: Maximum number of times this tool can be called per task (optional).
    - **timeout**: Maximum execution time per tool call (e.g., `60s`, optional).
    - **max-memory-mb**: Maximum memory usage in MB per tool call (optional).
    - **cpu-percent**: Maximum CPU usage as percentage per tool call (optional).

Example tool configuration in `tasks.yaml`:

```yaml
task-config:
  tool-selector:
    disabled: false
    tools:
      - name: python-code-executor
        disabled: false
        max-calls: 10
        timeout: 60s
        max-memory-mb: 512
        cpu-percent: 25
  tasks:
    - name: "math calculation"
      prompt: "Calculate the sum of even numbers from 1 to 100."
      response-result-format: "single number"
      expected-result: "2550"
      # Inherits global tool-selector configuration.
    - name: "simple math"
      prompt: "What is 2 + 2?"
      response-result-format: "single number"
      expected-result: "4"
      tool-selector:
        tools:
          - name: python-code-executor
            disabled: true  # Selectively disable tool for this simple task.
```

## Command Reference

```bash
mindtrial [options] [command]

Commands:
  run                       Start the trials
  help                      Show help
  version                   Show version

Options:
  --config string           Configuration file path (default: config.yaml)
  --tasks string            Task definitions file path
  --output-dir string       Results output directory
  --output-basename string  Base filename for results; replace if exists; blank = stdout
  --html                    Generate HTML output (default: true)
  --csv                     Generate CSV output (default: false)
  --log string              Log file path; append if exists; blank = stdout
  --verbose                 Enable detailed logging
  --debug                   Enable low-level debug logging (implies --verbose)
  --interactive             Enable interactive interface for run configuration, and real-time progress monitoring (default: false)
```

## Contributing

Contributions are welcome! Please review our [CONTRIBUTING.md](CONTRIBUTING.md) guidelines for more details.

### Getting the Source Code

Clone the repository and install dependencies:

```bash
git clone https://github.com/petmal/mindtrial.git
cd mindtrial
go mod download
```

### Running Tests

Execute the unit tests with:

```bash
go test -tags=test -race -v ./...
```

### Project Details

```text
/
├── cmd/
│   └── mindtrial/       # Command-line interface and main entry point
│       └── tui/         # Terminal-based UI and interactive mode functionality
├── config/              # Data models and management for configuration and task definitions
├── formatters/          # Output formatting for results
├── pkg/                 # Shared packages and utilities
├── providers/           # AI model service provider connectors
│   ├── execution/       # Provider run execution utilities and coordination
│   └── tools/           # Execution engine for external tools used by models
├── runners/             # Task execution and result aggregation
├── taskdata/            # Auxiliary files referenced by tasks in tasks.yaml
├── validators/          # Result validation logic
└── version/             # Application metadata
```

### License

This project is licensed under the Mozilla Public License 2.0 - see the [LICENSE](LICENSE) file for details.
