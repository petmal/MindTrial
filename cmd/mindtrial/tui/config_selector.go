// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/petmal/mindtrial/config"
)

const (
	childIndentation = "    "
)

// checkItem represents an item in a checklist.
type checkItem struct {
	label     string
	checked   bool
	isParent  bool
	children  []int // indices of children
	parentIdx int   // index of parent, -1 if no parent
}

// checklistModel is a model for an interactive checklist.
type checklistModel struct {
	uiIsReady bool
	title     string
	items     []checkItem
	cursor    int
	action    UserInputEvent
}

func (m checklistModel) Init() tea.Cmd {
	return nil
}

func (m checklistModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) { //nolint:gocritic
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.action = Exit
			return m, tea.Quit
		case "q", "esc":
			m.action = Quit
			return m, tea.Quit
		case "enter":
			m.action = Continue
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "space":
			// toggle the selected item
			m.items[m.cursor].checked = !m.items[m.cursor].checked

			// if this is a parent, toggle all children
			if m.items[m.cursor].isParent {
				for _, childIdx := range m.items[m.cursor].children {
					m.items[childIdx].checked = m.items[m.cursor].checked
				}
			} else if m.items[m.cursor].parentIdx >= 0 {
				// if this is a child, update parent based on siblings
				parentIdx := m.items[m.cursor].parentIdx
				allChecked := true
				for _, childIdx := range m.items[parentIdx].children {
					if !m.items[childIdx].checked {
						allChecked = false
						break
					}
				}
				m.items[parentIdx].checked = allChecked
			}
		}

	case tea.WindowSizeMsg:
		m.uiIsReady = true
	}
	return m, nil
}

func (m checklistModel) View() string {
	if !m.uiIsReady {
		return initializingMsg
	}

	var s strings.Builder

	// checklist title
	titleStyle := lipgloss.NewStyle().Bold(true).Margin(0, 0, 1, 0)
	s.WriteString(titleStyle.Render(m.title) + "\n")

	// checklist items
	for i, item := range m.items {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}

		checked := "[ ]"
		if item.checked {
			checked = "[x]"
		}

		line := fmt.Sprintf("%s %s %s", cursor, checked, item.label)

		// indent child items
		if !item.isParent {
			line = childIndentation + line
		}

		// highlight the selected line
		if i == m.cursor {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color(highlightColor)).Render(line)
		} else if item.isParent {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}

		s.WriteString(line + "\n")
	}

	// help text
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(helpTextColor)).Margin(1, 0, 0, 0)
	s.WriteString(helpStyle.Render("↑/↓: navigate • space: toggle • enter: confirm • q/esc: cancel • ctrl+c: exit"))

	return s.String()
}

// DisplayRunConfigurationPicker displays a terminal UI for enabling or disabling run configurations.
// It returns the selected user action and an error if the selection fails.
// This function modifies the providers slice directly.
func DisplayRunConfigurationPicker(providers []config.ProviderConfig) (UserInputEvent, error) {
	if !IsTerminal() {
		return Exit, fmt.Errorf("%w: %v", ErrInteractiveMode, ErrTerminalRequired)
	}

	items := []checkItem{}
	providerToItemIdx := map[int]int{} // Maps provider index to item index
	runToItemIdx := map[string]int{}   // Maps "provider-run" index to item index

	// build checklist
	for i, provider := range providers {
		// add provider item
		providerIdx := len(items)
		providerToItemIdx[i] = providerIdx

		items = append(items, checkItem{
			label:     provider.Name,
			checked:   !provider.Disabled,
			isParent:  true,
			children:  []int{},
			parentIdx: -1,
		})

		childIndices := []int{}

		// add run configuration items
		for j, run := range provider.Runs {
			childIdx := len(items)
			runKey := makeRunLookupKey(i, j)
			runToItemIdx[runKey] = childIdx
			childIndices = append(childIndices, childIdx)

			items = append(items, checkItem{
				label:     run.Name,
				checked:   !config.ResolveFlagOverride(run.Disabled, provider.Disabled),
				isParent:  false,
				children:  []int{},
				parentIdx: providerIdx,
			})
		}

		// set children for provider
		items[providerIdx].children = childIndices
	}

	// create and run the model
	model := checklistModel{
		title:  "Select Provider Configurations",
		items:  items,
		cursor: 0,
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)
	finalModel, err := p.Run() // blocking call
	if err != nil {
		return Exit, fmt.Errorf("%w: provider configuration selection: %v", ErrInteractiveMode, err)
	}

	checklist := finalModel.(checklistModel)
	if checklist.action == Continue {
		// update providers based on selection
		for i := range providers {
			// update provider disabled state
			if itemIdx, ok := providerToItemIdx[i]; ok {
				providers[i].Disabled = !checklist.items[itemIdx].checked
			}

			// update run configurations
			for j := range providers[i].Runs {
				runKey := makeRunLookupKey(i, j)
				if itemIdx, ok := runToItemIdx[runKey]; ok {
					disabled := !checklist.items[itemIdx].checked
					providers[i].Runs[j].Disabled = &disabled
				}
			}
		}
	}

	return checklist.action, nil // if dialog canceled, return without changes
}

// makeRunLookupKey creates a unique key for identifying a run configuration.
func makeRunLookupKey(providerIdx int, runIdx int) string {
	return fmt.Sprintf("%d-%d", providerIdx, runIdx)
}

// DisplayTaskPicker displays a terminal UI for enabling or disabling tasks.
// It returns the selected user action and an error if the selection fails.
// This function modifies the provided taskConfig directly.
func DisplayTaskPicker(taskConfig *config.TaskConfig) (UserInputEvent, error) {
	if !IsTerminal() {
		return Exit, fmt.Errorf("%w: %v", ErrInteractiveMode, ErrTerminalRequired)
	}

	items := []checkItem{}
	taskToItemIdx := map[int]int{} // Maps task index to item index

	// add parent item for all tasks
	parentIdx := 0
	items = append(items, checkItem{
		label:     "All Tasks",
		checked:   !taskConfig.Disabled,
		isParent:  true,
		children:  []int{},
		parentIdx: -1,
	})

	childIndices := []int{}

	// build checklist for individual tasks
	for i, task := range taskConfig.Tasks {
		childIdx := len(items)
		taskToItemIdx[i] = childIdx
		childIndices = append(childIndices, childIdx)

		items = append(items, checkItem{
			label:     task.Name,
			checked:   !config.ResolveFlagOverride(task.Disabled, taskConfig.Disabled),
			isParent:  false,
			children:  []int{},
			parentIdx: parentIdx,
		})
	}

	// set children for parent item
	items[parentIdx].children = childIndices

	// create and run the model
	model := checklistModel{
		title:  "Select Tasks",
		items:  items,
		cursor: 0,
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)
	finalModel, err := p.Run() // blocking call
	if err != nil {
		return Exit, fmt.Errorf("%w: task selection: %v", ErrInteractiveMode, err)
	}

	checklist := finalModel.(checklistModel)
	if checklist.action == Continue {
		// update global disabled flag
		taskConfig.Disabled = !checklist.items[parentIdx].checked

		// update individual tasks
		for i := range taskConfig.Tasks {
			if itemIdx, ok := taskToItemIdx[i]; ok {
				disabled := !checklist.items[itemIdx].checked
				taskConfig.Tasks[i].Disabled = &disabled
			}
		}
	}

	return checklist.action, nil // if dialog canceled, return without changes
}
