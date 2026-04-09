// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package main provides the command-line interface and the main entry point for MindTrial.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/petmal/mindtrial/cmd/mindtrial/tui"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/formatters"
	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
	"github.com/petmal/mindtrial/version"
)

const (
	runCommandName             = "run"
	mergeResultsCommandName    = "merge-results"
	helpCommandName            = "help"
	versionCommandName         = "version"
	unsetFlagValue             = "\x00"
	exitCodeBadCommand         = 2
	exitCodeFinishedWithErrors = 3
	defaultConfigFile          = "config.yaml"
	msgInteractiveExited       = "Interactive session exited by user."
)

var (
	commandDoc = map[string]string{
		runCommandName:          "start the trials",
		mergeResultsCommandName: "merge results from multiple runs",
		helpCommandName:         "show help",
		versionCommandName:      "show version",
	}
)

var (
	csvFormatter        = formatters.NewCSVFormatter()
	htmlFormatter       = formatters.NewHTMLFormatter()
	jsonCodec           = formatters.NewJSONCodec()
	logFormatter        = formatters.NewLogFormatter()
	summaryLogFormatter = formatters.NewSummaryLogFormatter()
)

var (
	configFilePath     *string
	tasksFilePath      *string
	outputFileDir      *string
	outputFileBasename *string
	formatHTML         *bool
	formatCSV          *bool
	formatJSON         *bool
	logFilePath        *string
	verbose            *bool
	debug              *bool
	interactive        *bool
)

var inputFiles stringSliceFlag

// stringSliceFlag implements flag.Value for collecting multiple string flag values.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// outputTarget pairs a formatter with its destination writer.
type outputTarget struct {
	formatter formatters.Formatter
	writer    io.Writer
}

func formatFlag(formatter formatters.Formatter, defaultValue bool) *bool {
	fileExt := formatter.FileExt()
	return flag.Bool(strings.ToLower(fileExt), defaultValue, fmt.Sprintf("generate %s output", strings.ToUpper(fileExt)))
}

var stderr = zerolog.New(zerolog.NewConsoleWriter(
	func(w *zerolog.ConsoleWriter) {
		w.Out = os.Stderr
		w.TimeFormat = time.DateTime
		w.NoColor = true
	},
)).Level(zerolog.TraceLevel).With().Timestamp().Logger()

func init() {
	registerFlags()
}

func registerFlags() {
	configFilePath = flag.String("config", defaultConfigFile, "configuration file path")
	tasksFilePath = flag.String("tasks", unsetFlagValue, "task definitions file path")
	outputFileDir = flag.String("output-dir", unsetFlagValue, "results output directory")
	outputFileBasename = flag.String("output-basename", unsetFlagValue, "base filename for results; replace if exists; blank = stdout")
	formatHTML = formatFlag(htmlFormatter, true)
	formatCSV = formatFlag(csvFormatter, false)
	formatJSON = formatFlag(jsonCodec, false)
	logFilePath = flag.String("log", unsetFlagValue, "log file path; append if exists; blank = stdout")
	verbose = flag.Bool("verbose", false, "enable detailed logging")
	debug = flag.Bool("debug", false, "enable low-level debug logging")
	interactive = flag.Bool("interactive", false, "enable interactive interface for run configuration, and real-time progress monitoring")
	flag.Var(&inputFiles, "input", "input result file path for merge-results; can be specified multiple times")

	flag.Usage = func() {
		w := flag.CommandLine.Output()
		fmt.Fprintf(w, "Usage: %s [options] [command]\n", os.Args[0])
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Commands:")
		printCommandHelp(w, runCommandName, mergeResultsCommandName, helpCommandName, versionCommandName)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		flag.PrintDefaults()
	}
}

func printCommandHelp(out io.Writer, commands ...string) {
	for _, cmdName := range commands {
		formatCommandHelp(out, cmdName, commandDoc[cmdName])
	}
}

func formatCommandHelp(out io.Writer, name string, usage string) {
	fmt.Fprintf(out, "  %s\n", name)
	fmt.Fprintf(out, "        %s\n", usage)
}

func main() {
	flag.Parse()
	for _, arg := range flag.Args() {
		switch arg {
		case helpCommandName:
			printHelp(os.Stdout)
			return
		case versionCommandName:
			printVersion(os.Stdout)
			return
		case runCommandName:
			if ok, err := run(context.Background()); err != nil {
				stderr.Fatal().Err(err).Send()
			} else if !ok {
				os.Exit(exitCodeFinishedWithErrors)
			}
			return
		case mergeResultsCommandName:
			if ok, err := mergeResults(context.Background()); err != nil {
				stderr.Fatal().Err(err).Send()
			} else if !ok {
				os.Exit(exitCodeFinishedWithErrors)
			}
			return
		}
	}
	printHelp(nil) // os.Stderr
	os.Exit(exitCodeBadCommand)
}

func run(ctx context.Context) (ok bool, err error) {
	if err = validateFlags(runCommandName,
		"config", "tasks", "output-dir", "output-basename",
		"html", "csv", "json", "log", "verbose", "debug", "interactive",
	); err != nil {
		return
	}

	configPath := filepath.Clean(*configFilePath)
	workingDir, configDir, err := getWorkingDirectories(configPath)
	if err != nil {
		return
	}
	fmt.Printf("Current working directory: %s\n", workingDir)
	fmt.Printf("Configuration directory: %s\n", configDir)

	// Load configuration.
	fmt.Printf("Loading configuration from file: %s\n", configPath)
	cfg, err := config.LoadConfigFromFile(ctx, configPath)
	if err != nil {
		return
	}

	// Load tasks.
	tasksFile := config.CleanIfNotBlank(getFlagValueIfSet(tasksFilePath, config.MakeAbs(configDir, cfg.Config.TaskSource)))
	fmt.Printf("Loading tasks from file: %s\n", tasksFile)
	tasks, err := config.LoadTasksFromFile(ctx, tasksFile)
	if err != nil {
		return
	}

	// Interactive configuration if enabled.
	if isEnabled(interactive) {
		if userAction, err := tui.DisplayRunConfigurationPicker(cfg.Config.Providers); err != nil { // blocking call
			return ok, err
		} else if userAction == tui.Exit { //nolint:gocritic
			fmt.Println(msgInteractiveExited)
			return true, nil
		} else if userAction == tui.Quit {
			fmt.Println("No changes applied: provider configuration selection was cancelled.")
		} else {
			fmt.Println("Changes applied: selected provider configurations have been enabled.")
		}

		if userAction, err := tui.DisplayTaskPicker(&tasks.TaskConfig); err != nil { // blocking call
			return ok, err
		} else if userAction == tui.Exit { //nolint:gocritic
			fmt.Println(msgInteractiveExited)
			return true, nil
		} else if userAction == tui.Quit {
			fmt.Println("No changes applied: task selection was cancelled.")
		} else {
			fmt.Println("Changes applied: selected tasks have been enabled.")
		}
	}

	// Filter out disabled providers and runs.
	targetProviders := cfg.Config.GetProvidersWithEnabledRuns()
	if len(targetProviders) < 1 {
		fmt.Println("Nothing to run: all providers are disabled or have no enabled run configurations.")
		return true, nil
	}

	// Filter out disabled tasks.
	targetTasks := tasks.TaskConfig.GetEnabledTasks()
	if len(targetTasks) < 1 {
		fmt.Println("Nothing to run: all tasks are disabled.")
		return true, nil
	}

	// Set the base path for each task context file to the location of the task definition file.
	taskFileDir := filepath.Dir(tasksFile)
	for _, task := range targetTasks {
		if err = task.SetBaseFilePath(taskFileDir); err != nil {
			return
		}
	}

	// Time to be used to resolve name patterns.
	timeRef := time.Now()

	// Create output files.
	var outputWriters []outputTarget
	for _, formatter := range enabledFormatters() {
		out := os.Stdout // default
		if fileName := getFlagValueIfSet(outputFileBasename, cfg.Config.OutputBaseName); config.IsNotBlank(fileName) {
			fileName = fmt.Sprintf("%s.%s", fileName, formatter.FileExt())
			if fp, outputPath, err := createOutputFile(config.MakeAbs(
				getFlagValueIfSet(outputFileDir, config.MakeAbs(configDir, cfg.Config.OutputDir)), fileName), timeRef, false); err != nil {
				return ok, err
			} else if fp != nil {
				defer fp.Close()
				fmt.Printf("Results in %s format will be saved to: %s\n", strings.ToUpper(formatter.FileExt()), outputPath)
				out = fp
			}
		}
		outputWriters = append(outputWriters, outputTarget{formatter: formatter, writer: out})
	}

	// Configure logger.
	var consoleBuffer io.Writer = os.Stdout
	if isEnabled(interactive) {
		consoleBuffer = &tui.ConsoleBuffer{}
	}
	logWriters := []io.Writer{zerolog.NewConsoleWriter(
		func(w *zerolog.ConsoleWriter) {
			w.Out = consoleBuffer
			w.TimeFormat = time.DateTime
			w.NoColor = false
		},
	)}
	logFile := os.Stdout
	if fp, logPath, err := createOutputFile(getFlagValueIfSet(logFilePath, config.MakeAbs(configDir, cfg.Config.LogFile)), timeRef, true); err != nil {
		return ok, err
	} else if fp != nil {
		fmt.Printf("Log messages will be saved to: %s\n", logPath)
		defer fp.Close()
		logFile = fp
		logWriters = append(logWriters, zerolog.NewConsoleWriter(
			func(w *zerolog.ConsoleWriter) {
				w.Out = logFile
				w.TimeFormat = time.DateTime
				w.NoColor = true
			},
		)) // format the file output as plain-text without color codes
	}
	logger := zerolog.New(zerolog.MultiLevelWriter(logWriters...)).Level(getEnabledLogLevel()).With().Timestamp().Logger()

	// Filter out disabled judges and runs.
	availableJudges := cfg.Config.GetJudgesWithEnabledRuns()

	// Run tasks.
	exec, err := runners.NewDefaultRunner(ctx, targetProviders, availableJudges, cfg.Config.Tools, logger)
	if err != nil {
		return
	}
	defer exec.Close(ctx)

	var runResult runners.ResultSet
	if isEnabled(interactive) {
		var userAction tui.UserInputEvent
		if userAction, runResult, err = tui.NewTaskMonitor(exec, consoleBuffer.(*tui.ConsoleBuffer)).Run(ctx, targetTasks); err != nil { // blocking call
			return ok, err
		} else if userAction == tui.Exit {
			fmt.Println(msgInteractiveExited)
			return true, nil
		} else if userAction == tui.Quit {
			fmt.Println("Interactive UI closed: tasks will continue running in the background.")
		}
	} else {
		if runResult, err = exec.Run(ctx, targetTasks); err != nil { // blocking call
			return
		}
	}

	// If this was an async run that is still in progress, the call will block until it is finished.
	results := runResult.GetResults()

	// Print and save the results.
	ok = !logResults(results, logFile)
	ok = ok && !saveResults(results, outputWriters)

	return
}

func enabledFormatters() (enabled []formatters.Formatter) {
	if isEnabled(formatHTML) {
		enabled = append(enabled, htmlFormatter)
	}
	if isEnabled(formatCSV) {
		enabled = append(enabled, csvFormatter)
	}
	if isEnabled(formatJSON) {
		enabled = append(enabled, jsonCodec)
	}
	return enabled
}

func isEnabled(value *bool) bool {
	return value != nil && *value
}

func getWorkingDirectories(configFilePath string) (workingDir string, configDir string, err error) {
	workingDir, err = os.Getwd()
	if err != nil {
		return
	}

	// If the path is not absolute it will be joined with the current working directory.
	absConfigPath, err := filepath.Abs(configFilePath)
	if err != nil {
		return
	}
	configDir = filepath.Dir(absConfigPath)

	return
}

func getEnabledLogLevel() zerolog.Level {
	if isEnabled(debug) {
		return zerolog.TraceLevel
	} else if isEnabled(verbose) {
		return zerolog.DebugLevel
	}
	return zerolog.InfoLevel
}

func getFlagValueIfSet(value *string, defaultValue string) string {
	if (value != nil) && *value != unsetFlagValue {
		return *value
	}
	return defaultValue
}

func printHelp(out io.Writer) {
	flag.CommandLine.SetOutput(out)
	flag.Usage()
}

func printVersion(out io.Writer) {
	fmt.Fprintf(out, "%s %s\n", version.Name, version.GetVersion())
}

func createOutputFile(outputFilePath string, timeRef time.Time, append bool) (outputFile *os.File, outputPath string, err error) {
	if outputPath = config.CleanIfNotBlank(config.ResolveFileNamePattern(outputFilePath, timeRef)); config.IsNotBlank(outputPath) {
		if err = os.MkdirAll(filepath.Dir(outputPath), os.ModePerm); err != nil {
			return
		}
		if append {
			outputFile, err = os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		} else {
			outputFile, err = os.Create(outputPath)
		}
	}
	return
}

func logResults(results runners.Results, out io.Writer) (finishedWithErrors bool) {
	fmt.Fprintln(out)
	if err := summaryLogFormatter.Write(results, out); err != nil {
		stderr.Warn().Err(err).Msg("failed to log summary")
		finishedWithErrors = true
	}
	fmt.Fprintln(out)
	if err := logFormatter.Write(results, out); err != nil {
		stderr.Warn().Err(err).Msg("failed to log results")
		finishedWithErrors = true
	}
	fmt.Fprintln(out)
	return
}

func saveResults(results runners.Results, outputWriters []outputTarget) (finishedWithErrors bool) {
	for _, ow := range outputWriters {
		if err := ow.formatter.Write(results, ow.writer); err != nil {
			stderr.Warn().Err(err).Msgf("failed to write %s output", strings.ToUpper(ow.formatter.FileExt()))
			finishedWithErrors = true
		}
	}
	return
}

var errUnsupportedFlag = errors.New("unsupported flag for command")

func validateFlags(command string, supported ...string) error {
	allowed := make(map[string]bool, len(supported))
	for _, name := range supported {
		allowed[name] = true
	}
	var unsupported string
	flag.Visit(func(f *flag.Flag) {
		if !allowed[f.Name] {
			unsupported = f.Name
		}
	})
	if unsupported != "" {
		return fmt.Errorf("%w: --%s is not supported by %q", errUnsupportedFlag, unsupported, command)
	}
	return nil
}

func mergeResults(_ context.Context) (ok bool, err error) {
	if err = validateFlags(mergeResultsCommandName,
		"input", "output-dir", "output-basename", "html", "csv", "json", "verbose",
	); err != nil {
		return
	}

	if len(inputFiles) < 1 {
		fmt.Println("Nothing to merge: no input files provided.")
		return true, nil
	}

	// Read all input files.
	resultSets := make([]runners.Results, 0, len(inputFiles))
	for _, inputPath := range inputFiles {
		fmt.Printf("Loading results from file: %s\n", inputPath)
		rs, readErr := formatters.ReadResultsFromFile(inputPath)
		if readErr != nil {
			return false, readErr
		}
		resultSets = append(resultSets, rs)
	}

	// Merge results.
	results, stats := runners.MergeResults(resultSets...)

	// Time to be used to resolve name patterns.
	timeRef := time.Now()

	// Create output files.
	var outputWriters []outputTarget
	for _, formatter := range enabledFormatters() {
		out := os.Stdout // default
		if fileName := getFlagValueIfSet(outputFileBasename, ""); config.IsNotBlank(fileName) {
			fileName = fmt.Sprintf("%s.%s", fileName, formatter.FileExt())
			outputDir := getFlagValueIfSet(outputFileDir, "")
			if config.IsNotBlank(outputDir) {
				fileName = filepath.Join(outputDir, fileName)
			}
			if fp, outputPath, createErr := createOutputFile(fileName, timeRef, false); createErr != nil {
				return ok, createErr
			} else if fp != nil {
				defer fp.Close()
				fmt.Printf("Results in %s format will be saved to: %s\n", strings.ToUpper(formatter.FileExt()), outputPath)
				out = fp
			}
		}
		outputWriters = append(outputWriters, outputTarget{formatter: formatter, writer: out})
	}

	// Print merge summary.
	fmt.Println()
	fmt.Println("Merged results:")
	for _, provider := range utils.SortedKeys(stats.Runs) {
		fmt.Printf("  %s:\n", provider)
		for _, run := range utils.SortedKeys(stats.Runs[provider]) {
			rs := stats.Runs[provider][run]
			if rs.Updated > 0 {
				fmt.Printf("    %s: %d total, %d updated\n", run, rs.Total, rs.Updated)
			} else {
				fmt.Printf("    %s: %d total\n", run, rs.Total)
			}
		}
	}
	fmt.Println()

	// Print and save the results.
	ok = !isEnabled(verbose) || !logResults(results, os.Stdout)
	ok = ok && !saveResults(results, outputWriters)

	return
}
