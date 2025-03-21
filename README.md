# MindTrial

[![Build](https://github.com/petmal/mindtrial/actions/workflows/go.yml/badge.svg)](https://github.com/petmal/mindtrial/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/petmal/mindtrial)](https://goreportcard.com/report/github.com/petmal/mindtrial)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://mozilla.org/MPL/2.0/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/petmal/mindtrial)](https://go.dev/)

**MindTrial** helps you assess and compare the performance of AI language models (LLMs) on text-based tasks. Use it to evaluate a single model or test multiple models from OpenAI, Google, Anthropic, and DeepSeek side by side, and get easy-to-read results in HTML and CSV formats.

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

- [Go 1.23](https://golang.org/dl/)
- API keys from your chosen AI providers

## Key Features

- Compare multiple AI models at once
- Create custom test tasks using simple YAML files
- Get results in HTML and CSV formats
- Easy to extend with new AI models
- Smart rate limiting to prevent API overload

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

## Configuration Guide

MindTrial uses two simple YAML files to control everything:

### 1. config.yaml - Application Settings

Controls how MindTrial operates, including:

- Where to save results
- Which AI models to use
- API settings and rate limits

### 2. tasks.yaml - Task Definitions

Defines what you want to test, including:

- Questions/prompts for the AI
- Expected answers
- Response format rules

> [!TIP]
> **New to MindTrial?** Start with the example files provided and modify them for your needs.

### config.yaml

This file defines the tool's settings and target model configurations evaluated during the trial run. The following properties are required:

- **output-dir**: Path to the directory where results will be saved.
- **task-source**: Path to the file with definitions of tasks to run.
- **providers**: List of providers (i.e. target LLM configurations) to execute tasks during the trial run.
  - **name**: Name of the LLM provider (e.g. *openai*).
  - **client-config**: Configuration for this provider's client (e.g. *API key*).
  - **runs**: List of runs (i.e. model configurations) for this provider. Unless disabled, all configurations will be trialed.
    - **name**: Display-friendly name to be shown in the results.
    - **model**: Model name must be exactly as defined by the backend service's API (e.g. *gpt-4o-mini*).

> [!IMPORTANT]
> All provider names must match exactly:
>
> - **openai**: OpenAI GPT models
> - **google**: Google Gemini models
> - **anthropic**: Anthropic Claude models
> - **deepseek**: DeepSeek open-source models

> [!NOTE]
> Some models (**openai**) support additional model-specific runtime configuration.
> These can be provided in the `model-parameters` section of the run configuration.
> Currently supported parameters for OpenAI models include:
>
> - **text-response-format**: If `true`, use plain-text response format (less reliable) for compatibility with models that do not support `JSON`.
> - **reasoning-effort**: Controls effort on reasoning for reasoning models (i.e. *low*, *medium*, *high*).

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
```

### tasks.yaml

This file defines the tasks to be executed on all enabled run configurations. Each task must define the following four properties:

- **name**: Display-friendly name to be shown in the results.
- **prompt**: The prompt (i.e. task) that will be sent to the AI model.
- **response-result-format**: Defines how the AI should format the final answer to the prompt. This is important because the final answer will be compared to the `expected-result` and it needs to consistently follow the same format.
- **expected-result**: This defines the expected (i.e. valid) final answer to the prompt. It must follow the `response-result-format` precisely.

> [!NOTE]
> Currently, the letter-case is ignored when comparing the final answer to the `expected-result`.

> [!TIP]
> To disable all tasks by default, set `disabled: true` in the `task-config` section.
> An individual task can override this by setting `disabled: false` (e.g. to enable just that one task).

A sample task from `tasks.yaml`:

```yaml
# tasks.yaml
task-config:
  disabled: true
  tasks:
    - name: "riddle - anagram - v1"
      disabled: false
      prompt: |-
        There are four 8-letter words (animals) that have been split into 2-letter pieces.
        Find these four words by putting appropriate pieces back together:
        RR TE KA DG EH AN SQ EL UI OO HE LO AR PE NG OG
      response-result-format: |-
        list of words in alphabetical order separated by ", "
      expected-result: |-
        ANTELOPE, HEDGEHOG, KANGAROO, SQUIRREL
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
│   └── mindtrial/    # Command-line interface and main entry point
├── config/           # Data models and management for configuration and task definitions
├── formatters/       # Output formatting for results
├── pkg/              # Shared packages and utilities
├── providers/        # AI model service provider connectors
├── runners/          # Task execution and result aggregation
└── version/          # Application metadata
```

### License

This project is licensed under the Mozilla Public License 2.0 - see the [LICENSE](LICENSE) file for details.
