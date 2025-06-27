// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/v2/progress"
	"github.com/charmbracelet/bubbles/v2/viewport"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/runners"
)

const (
	padding             = 2
	viewportBorderColor = "62"
	minViewportHeight   = 5
)

// progressModel represents the model for interactive task progress monitoring.
type progressModel struct {
	uiIsReady       bool
	progressBar     progress.Model // component for displaying progress
	viewport        viewport.Model // component for displaying scrollable messages
	resultManager   runners.AsyncResultSet
	progressPercent float64
	messages        *ConsoleBuffer
	action          UserInputEvent
}

// progressMsg represents a UI event for task progress update.
// The value is between 0.0 and 1.0.
type progressMsg float32

// messageMsg represents a UI event carrying a new log message from the task.
type messageMsg string

func newProgressModel(consoleBuffer *ConsoleBuffer, resultManager runners.AsyncResultSet) progressModel {
	return progressModel{
		messages:        consoleBuffer,
		resultManager:   resultManager,
		progressPercent: 0.0,
		action:          Continue,
	}
}

func (m progressModel) Init() tea.Cmd {
	return tea.Batch(
		waitForProgress(m.resultManager.ProgressEvents()),
		waitForMessage(m.resultManager.MessageEvents()),
	)
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.resultManager.Cancel()
			m.action = Exit
			return m, tea.Quit
		case "q", "esc":
			m.action = Quit
			return m, tea.Quit
		case "up", "down", "pgup", "pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	case tea.WindowSizeMsg:
		// Calculate space needed for other UI elements.
		titleHeight := 2    // title + spacing
		progressHeight := 4 // progress stats + progress bar + spacing
		helpHeight := 2     // help text + spacing
		borderHeight := 2   // viewport border
		totalReservedHeight := titleHeight + progressHeight + helpHeight + borderHeight + 2*padding

		viewportHeight := max(msg.Height-totalReservedHeight, minViewportHeight)
		componentWidth := msg.Width - 2*padding

		if !m.uiIsReady {
			m.progressBar = progress.New(
				progress.WithDefaultGradient(),
				progress.WithWidth(componentWidth),
			)

			m.viewport = viewport.New(
				viewport.WithWidth(componentWidth),
				viewport.WithHeight(viewportHeight),
			)
			m.viewport.Style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(viewportBorderColor)).
				PaddingRight(padding)
			m.uiIsReady = true
		} else {
			m.viewport.SetWidth(componentWidth)
			m.viewport.SetHeight(viewportHeight)
			m.progressBar.SetWidth(componentWidth)
		}

		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		m.progressBar, cmd = m.progressBar.Update(msg)
		cmds = append(cmds, cmd)

	case progressMsg:
		percent := float64(msg)
		m.progressPercent = percent
		cmds = append(cmds, m.progressBar.SetPercent(percent))
		cmds = append(cmds, waitForProgress(m.resultManager.ProgressEvents()))

	case messageMsg:
		m.viewport.SetContent(m.messages.String()) // read all current messages directly from the buffer
		m.viewport.GotoBottom()
		cmds = append(cmds, waitForMessage(m.resultManager.MessageEvents()))

	case progress.FrameMsg:
		var cmd tea.Cmd
		m.progressBar, cmd = m.progressBar.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m progressModel) View() string {
	if !m.uiIsReady {
		return initializingMsg
	}

	var s strings.Builder

	// Title.
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("Task Progress"))
	s.WriteString("\n\n")

	// Progress stats.
	s.WriteString(fmt.Sprintf("Progress: %.1f%%\n\n", m.progressPercent*100.0))

	// Progress bar.
	s.WriteString(m.progressBar.View())
	s.WriteString("\n\n")

	// Log messages.
	s.WriteString(m.viewport.View())
	s.WriteString("\n\n")

	// Help text.
	helpText := lipgloss.NewStyle().Foreground(lipgloss.Color(helpTextColor)).Render(
		"↑/↓: scroll log • q/esc: close • ctrl+c: exit",
	)
	s.WriteString(helpText)

	return lipgloss.NewStyle().Padding(1, 2).Render(s.String())
}

func waitForProgress(progressEvents <-chan float32) tea.Cmd {
	return func() tea.Msg {
		if progressEvents == nil {
			return nil
		}
		progress, ok := <-progressEvents
		if !ok {
			return tea.Quit() // channel closed
		}
		return progressMsg(progress)
	}
}

func waitForMessage(messageEvents <-chan string) tea.Cmd {
	return func() tea.Msg {
		if messageEvents == nil {
			return nil
		}
		message, ok := <-messageEvents
		if !ok {
			return tea.Quit() // channel closed
		}
		return messageMsg(message)
	}
}

// ConsoleBuffer is a thread-safe buffer for storing console logs.
type ConsoleBuffer struct {
	sync.RWMutex
	buffer strings.Builder
}

// Write writes p to the buffer in a thread-safe manner.
// It implements the io.Writer interface.
func (cb *ConsoleBuffer) Write(p []byte) (int, error) {
	cb.Lock()
	defer cb.Unlock()
	return cb.buffer.Write(p)
}

// String returns the accumulated string in a thread-safe manner.
func (cb *ConsoleBuffer) String() string {
	cb.RLock()
	defer cb.RUnlock()
	return cb.buffer.String()
}

// NewTaskMonitor initializes and returns a TaskMonitor.
// It accepts a pointer to a console buffer where an external logger writes during task execution.
// The TaskMonitor can read directly from this buffer to update the UI console component.
func NewTaskMonitor(runner runners.Runner, console *ConsoleBuffer) *TaskMonitor {
	return &TaskMonitor{
		runner:  runner,
		console: console,
	}
}

// TaskMonitor represents an interactive terminal UI for monitoring task execution.
// It displays real-time progress, logs, and handles user input during task execution.
// It wraps a runners.Runner implementation to execute tasks while providing visual feedback.
type TaskMonitor struct {
	runner  runners.Runner
	console *ConsoleBuffer
}

// Run runs tasks in an interactive UI, displaying real-time progress and logs.
// It returns the user action and the run result set.
func (t *TaskMonitor) Run(ctx context.Context, tasks []config.Task) (userAction UserInputEvent, result runners.AsyncResultSet, err error) {
	if !IsTerminal() {
		return Exit, nil, fmt.Errorf("%w: %v", ErrInteractiveMode, ErrTerminalRequired)
	}

	// Start tasks asynchronously.
	result, err = t.runner.Start(ctx, tasks)
	if err != nil {
		return
	}

	// Create and run the model.
	model := newProgressModel(t.console, result)
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run() // blocking call
	if err != nil {
		return Exit, result, fmt.Errorf("%w: progress monitor: %v", ErrInteractiveMode, err)
	}

	progressModel := finalModel.(progressModel)
	return progressModel.action, result, nil
}
