// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

// Package main provides the command-line interface and the main entry point for MindTrial.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/formatters"
	"github.com/petmal/mindtrial/runners"
	"github.com/petmal/mindtrial/version"
)

const (
	runCommandName             = "run"
	helpCommandName            = "help"
	versionCommandName         = "version"
	unsetFlagValue             = "\x00"
	exitCodeBadCommand         = 2
	exitCodeFinishedWithErrors = 3
	loggerPrefix               = version.Name + ": "
	defaultConfigFile          = "config.yaml"
)

var (
	commandDoc = map[string]string{
		runCommandName:     "start the trials",
		helpCommandName:    "show help",
		versionCommandName: "show version",
	}
)

var (
	csvFormatter        = formatters.NewCSVFormatter()
	htmlFormatter       = formatters.NewHTMLFormatter()
	logFormatter        = formatters.NewLogFormatter()
	summaryLogFormatter = formatters.NewSummaryLogFormatter()
)

var (
	configFilePath     = flag.String("config", defaultConfigFile, "configuration file path")
	tasksFilePath      = flag.String("tasks", unsetFlagValue, "task definitions file path")
	outputFileDir      = flag.String("output-dir", unsetFlagValue, "results output directory")
	outputFileBasename = flag.String("output-basename", unsetFlagValue, "base filename for results; replace if exists; blank = stdout")
	formatHTML         = formatFlag(htmlFormatter, true)
	formatCSV          = formatFlag(csvFormatter, false)
	logFilePath        = flag.String("log", unsetFlagValue, "log file path; append if exists; blank = stdout")
)

func formatFlag(formatter formatters.Formatter, defaultValue bool) *bool {
	fileExt := formatter.FileExt()
	return flag.Bool(strings.ToLower(fileExt), defaultValue, fmt.Sprintf("generate %s output", strings.ToUpper(fileExt)))
}

func init() {
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		fmt.Fprintf(w, "Usage: %s [options] [command]\n", os.Args[0])
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Commands:")
		printCommandHelp(w, runCommandName, helpCommandName, versionCommandName)
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
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			switch arg {
			case helpCommandName:
				printHelp(os.Stdout)
				return
			case versionCommandName:
				printVersion(os.Stdout)
				return
			case runCommandName:
				if ok, err := run(); err != nil {
					log.Fatal(err)
				} else if !ok {
					os.Exit(exitCodeFinishedWithErrors)
				}
				return
			}
		}
	}
	printHelp(nil) // os.Stderr
	os.Exit(exitCodeBadCommand)
}

func run() (ok bool, err error) {
	ctx := context.Background()
	flag.Parse()

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

	// Time to be used to resolve name patterns.
	timeRef := time.Now()

	// Configure logger.
	logFile := os.Stdout // default
	if fp, logPath, err := createOutputFile(getFlagValueIfSet(logFilePath, config.MakeAbs(configDir, cfg.Config.LogFile)), timeRef, true); err != nil {
		return ok, err
	} else if fp != nil {
		fmt.Printf("Log messages will be saved to: %s\n", logPath)
		defer fp.Close()
		logFile = fp
	}
	logger := log.New(logFile, loggerPrefix, log.LstdFlags|log.Lmsgprefix)

	// Create output files.
	outputWriters := make(map[formatters.Formatter]io.Writer)
	for _, formatter := range enabledFormatters() {
		outputWriters[formatter] = os.Stdout // default
		if fileName := getFlagValueIfSet(outputFileBasename, cfg.Config.OutputBaseName); config.IsNotBlank(fileName) {
			fileName = fmt.Sprintf("%s.%s", fileName, formatter.FileExt())
			if fp, outputPath, err := createOutputFile(config.MakeAbs(
				getFlagValueIfSet(outputFileDir, config.MakeAbs(configDir, cfg.Config.OutputDir)), fileName), timeRef, false); err != nil {
				return ok, err
			} else if fp != nil {
				defer fp.Close()
				fmt.Printf("Results will be saved to: %s\n", outputPath)
				outputWriters[formatter] = fp
			}
		}
	}

	// Run tasks.
	exec, err := runners.NewDefaultRunner(ctx, targetProviders, logger)
	if err != nil {
		return
	}
	defer exec.Close(ctx)
	if err = exec.Run(ctx, targetTasks); err != nil { // blocking call
		return
	}

	// Print and save the results.
	results := exec.GetResults()
	ok = !logResults(results, logger)
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

func logResults(results runners.Results, logger *log.Logger) (finishedWithErrors bool) {
	logger.SetFlags(0)
	logger.SetPrefix("")
	out := logger.Writer()
	fmt.Fprintln(out)
	if err := summaryLogFormatter.Write(results, out); err != nil {
		log.Println(err)
		finishedWithErrors = true
	}
	fmt.Fprintln(out)
	if err := logFormatter.Write(results, out); err != nil {
		log.Println(err)
		finishedWithErrors = true
	}
	fmt.Fprintln(out)
	return
}

func saveResults(results runners.Results, outputWriters map[formatters.Formatter]io.Writer) (finishedWithErrors bool) {
	for formatter, out := range outputWriters {
		if err := formatter.Write(results, out); err != nil {
			log.Println(err)
			finishedWithErrors = true
		}
	}
	return
}
