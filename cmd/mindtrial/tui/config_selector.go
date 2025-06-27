// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/v2/viewport"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/petmal/mindtrial/config"
)

const (
	childIndentation = "    "
	selectAllText    = "Select All"
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
	viewport  viewport.Model
}

func (m checklistModel) Init() tea.Cmd {
	return nil
}

func (m checklistModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

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
				m.updateViewContent()
				m.scrollViewContent()
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.updateViewContent()
				m.scrollViewContent()
			}
		case "space":
			// Toggle the selected item.
			m.items[m.cursor].checked = !m.items[m.cursor].checked

			// If this is a parent, toggle all descendants recursively.
			if m.items[m.cursor].isParent {
				m.toggleDescendants(m.cursor, m.items[m.cursor].checked)
			}

			// Update all ancestors.
			m.updateAncestors(m.cursor)

			// Repopulate the view with updated items.
			m.updateViewContent()
		}

	case tea.WindowSizeMsg:
		headerHeight := 4 // title + spacing + help text
		viewportHeight := msg.Height - headerHeight
		if !m.uiIsReady {
			m.viewport = viewport.New(
				viewport.WithWidth(msg.Width),
				viewport.WithHeight(viewportHeight),
			)
			m.setViewContent()
			m.uiIsReady = true
		} else {
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(viewportHeight)
		}

		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		// Ensure the cursor is visible after resize.
		m.scrollViewContent()
	}

	return m, tea.Batch(cmds...)
}

// updateViewContent updates the viewport's content with the rendered checklist items.
func (m *checklistModel) updateViewContent() {
	if m.uiIsReady {
		m.setViewContent()
	}
}

func (m *checklistModel) setViewContent() {
	m.viewport.SetContent(m.renderItems())
}

// scrollViewContent adjusts the viewport's Y-offset with the cursor's position.
func (m *checklistModel) scrollViewContent() {
	if m.uiIsReady {
		if m.cursor < m.viewport.YOffset {
			m.viewport.SetYOffset(m.cursor)
		} else if m.cursor >= m.viewport.YOffset+m.viewport.Height() {
			m.viewport.SetYOffset(m.cursor - m.viewport.Height() + 1)
		}
	}
}

// toggleDescendants recursively sets all descendants to the given state.
func (m *checklistModel) toggleDescendants(itemIdx int, state bool) {
	for _, childIdx := range m.items[itemIdx].children {
		m.items[childIdx].checked = state
		if m.items[childIdx].isParent {
			m.toggleDescendants(childIdx, state)
		}
	}
}

// updateAncestors updates a parent's state based on children and propagates up the hierarchy.
func (m *checklistModel) updateAncestors(itemIdx int) {
	if m.items[itemIdx].parentIdx < 0 {
		return // this is a root item, nothing to update
	}

	parentIdx := m.items[itemIdx].parentIdx

	// Check the parent item if all children are checked.
	// Uncheck the parent item if any child is unchecked.
	allChecked := true
	for _, childIdx := range m.items[parentIdx].children {
		if !m.items[childIdx].checked {
			allChecked = false
			break
		}
	}
	m.items[parentIdx].checked = allChecked

	// Continue updating ancestors up the hierarchy.
	m.updateAncestors(parentIdx)
}

func (m checklistModel) View() string {
	if !m.uiIsReady {
		return initializingMsg
	}

	var s strings.Builder

	// Checklist title.
	titleStyle := lipgloss.NewStyle().Bold(true).Margin(0, 0, 1, 0)
	s.WriteString(titleStyle.Render(m.title) + "\n")

	// Checklist items in viewport.
	s.WriteString(m.viewport.View())

	// Help text.
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(helpTextColor)).Margin(1, 0, 0, 0)
	s.WriteString(helpStyle.Render("↑/↓: navigate • space: toggle • enter: confirm • q/esc: cancel • ctrl+c: exit"))

	return s.String()
}

// renderItems renders the checklist items as a string.
func (m checklistModel) renderItems() string {
	var s strings.Builder

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

		// Indent child items.
		line = m.getIndentation(i) + line

		// Highlight the selected line.
		if i == m.cursor {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color(highlightColor)).Render(line)
		} else if item.isParent {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}

		s.WriteString(line + "\n")
	}

	return s.String()
}

// getIndentation returns the indentation string for an item.
func (m checklistModel) getIndentation(itemIdx int) string {
	var indentation strings.Builder
	currentIdx := itemIdx

	for m.items[currentIdx].parentIdx >= 0 {
		indentation.WriteString(childIndentation)
		currentIdx = m.items[currentIdx].parentIdx
	}

	return indentation.String()
}

// DisplayRunConfigurationPicker displays a terminal UI for enabling or disabling run configurations.
// It returns the selected user action and an error if the selection fails.
// This function modifies the providers slice directly.
func DisplayRunConfigurationPicker(providers []config.ProviderConfig) (UserInputEvent, error) {
	if !IsTerminal() {
		return Exit, fmt.Errorf("%w: %v", ErrInteractiveMode, ErrTerminalRequired)
	}

	items := []checkItem{}
	providerToItemIdx := map[int]int{} // maps provider index to item index
	runToItemIdx := map[string]int{}   // maps "provider-run" index to item index

	// Add parent item for all providers.
	providerRootIdx := 0
	allChecked := true

	items = append(items, checkItem{
		label:     selectAllText,
		checked:   allChecked,
		isParent:  true,
		children:  []int{},
		parentIdx: -1,
	})

	childIndices := []int{}

	// Build checklist.
	for i, provider := range providers {
		// Add provider item.
		providerIdx := len(items)
		providerToItemIdx[i] = providerIdx
		childIndices = append(childIndices, providerIdx)

		items = append(items, checkItem{
			label:     provider.Name,
			checked:   !provider.Disabled,
			isParent:  true,
			children:  []int{},
			parentIdx: providerRootIdx,
		})

		providerChildIndices := []int{}

		// Add run configuration items.
		for j, run := range provider.Runs {
			childIdx := len(items)
			runKey := makeRunLookupKey(i, j)
			runToItemIdx[runKey] = childIdx
			providerChildIndices = append(providerChildIndices, childIdx)

			isRunDisabled := config.ResolveFlagOverride(run.Disabled, provider.Disabled)
			if isRunDisabled { // if any run is disabled the root item should be unchecked
				allChecked = false
			}

			items = append(items, checkItem{
				label:     run.Name,
				checked:   !isRunDisabled,
				isParent:  false,
				children:  []int{},
				parentIdx: providerIdx,
			})
		}

		// Set children for provider.
		items[providerIdx].children = providerChildIndices
	}

	// Update the root item's checked state.
	items[providerRootIdx].checked = allChecked

	// Set children for the root item.
	items[providerRootIdx].children = childIndices

	// Create and run the model.
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
		// Update providers based on selection.
		for i := range providers {
			// Update provider disabled state.
			if itemIdx, ok := providerToItemIdx[i]; ok {
				providers[i].Disabled = !checklist.items[itemIdx].checked
			}

			// Update run configurations.
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
	taskToItemIdx := map[int]int{} // maps task index to item index

	// Add parent item for all tasks.
	taskRootIdx := 0
	items = append(items, checkItem{
		label:     selectAllText,
		checked:   !taskConfig.Disabled,
		isParent:  true,
		children:  []int{},
		parentIdx: -1,
	})

	childIndices := []int{}

	// Build checklist for individual tasks.
	for i, task := range taskConfig.Tasks {
		childIdx := len(items)
		taskToItemIdx[i] = childIdx
		childIndices = append(childIndices, childIdx)

		items = append(items, checkItem{
			label:     task.Name,
			checked:   !config.ResolveFlagOverride(task.Disabled, taskConfig.Disabled),
			isParent:  false,
			children:  []int{},
			parentIdx: taskRootIdx,
		})
	}

	// Set children for parent item.
	items[taskRootIdx].children = childIndices

	// Create and run the model.
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
		// Update global disabled flag.
		taskConfig.Disabled = !checklist.items[taskRootIdx].checked

		// Update individual tasks.
		for i := range taskConfig.Tasks {
			if itemIdx, ok := taskToItemIdx[i]; ok {
				disabled := !checklist.items[itemIdx].checked
				taskConfig.Tasks[i].Disabled = &disabled
			}
		}
	}

	return checklist.action, nil // if dialog canceled, return without changes
}
