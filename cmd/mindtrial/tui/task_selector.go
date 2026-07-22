// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tui

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/petmal/mindtrial/config"
	"github.com/petmal/mindtrial/pkg/utils"
)

const (
	taskPickerTitle          = "Select Tasks"
	taskPickerReservedLines  = 5
	taskPickerFilterItemName = "task"
)

type selectionListKeyMap struct {
	Confirm         key.Binding
	Cancel          key.Binding
	Exit            key.Binding
	Toggle          key.Binding
	SelectVisible   key.Binding
	DeselectVisible key.Binding
	ClearFilters    key.Binding
}

func newSelectionListKeyMap() selectionListKeyMap {
	return selectionListKeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q/esc", "cancel"),
		),
		Exit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "exit"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "toggle"),
		),
		SelectVisible: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "select visible"),
		),
		DeselectVisible: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "deselect visible"),
		),
		ClearFilters: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "clear filters"),
		),
	}
}

type taskFilterKeyMap struct {
	CycleSuite      key.Binding
	ClearSuite      key.Binding
	CycleCategory   key.Binding
	ClearCategory   key.Binding
	CycleDifficulty key.Binding
	ClearDifficulty key.Binding
}

func newTaskFilterKeyMap() taskFilterKeyMap {
	return taskFilterKeyMap{
		CycleSuite: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "suite"),
		),
		ClearSuite: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "clear suite"),
		),
		CycleCategory: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "category"),
		),
		ClearCategory: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "clear category"),
		),
		CycleDifficulty: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "difficulty"),
		),
		ClearDifficulty: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "clear difficulty"),
		),
	}
}

type taskSelectionModel struct {
	uiIsReady bool
	action    UserInputEvent
	width     int
	height    int

	list      list.Model
	items     []taskSelectionItem
	selected  []bool
	listKeys  selectionListKeyMap
	filterKey taskFilterKeyMap

	activeSuite      string
	activeCategory   string
	activeDifficulty string
}

type taskSelectionItem struct {
	taskIndex  int
	name       string
	suite      string
	category   string
	difficulty string
	tags       []string
}

// tagValueSeparator joins a task's tags into a single FilterValue() string. It uses a
// non-printable separator (rather than a space or the user-facing tagQuerySeparator) so
// that a multi-word tag round-trips through FilterValue() and back intact instead of being
// fragmented into separate words - see tagPrefixFilter, which matches each tag as a
// single, whole unit.
const tagValueSeparator = "\x1f"

// tagQuerySeparator separates multiple tag queries typed into the same filter input (e.g.
// "smoke, nightly" searches for tasks having a tag prefixed by "smoke" AND a - possibly
// different - tag prefixed by "nightly"). A comma is used rather than whitespace so that a
// single query token may itself contain a multi-word tag (e.g. "smoke test") without being
// fragmented; see tagPrefixFilter.
const tagQuerySeparator = ","

func (i taskSelectionItem) FilterValue() string {
	return strings.Join(i.tags, tagValueSeparator)
}

func newTaskSelectionModel(taskConfig *config.TaskConfig) taskSelectionModel {
	items := make([]taskSelectionItem, 0, len(taskConfig.Tasks))
	selected := make([]bool, len(taskConfig.Tasks))

	for i, task := range taskConfig.Tasks {
		items = append(items, taskSelectionItem{
			taskIndex:  i,
			name:       task.Name,
			suite:      task.Suite,
			category:   task.Category,
			difficulty: task.Difficulty,
			tags:       task.Tags,
		})
		selected[i] = !config.ResolveFlagOverride(task.Disabled, taskConfig.Disabled)
	}

	delegate := taskSelectionDelegate{selected: &selected}
	taskList := list.New(toListItems(items), delegate, 0, 0)
	taskList.Title = taskPickerTitle
	taskList.Filter = tagPrefixFilter
	taskList.SetStatusBarItemName(taskPickerFilterItemName, taskPickerFilterItemName+"s")
	taskList.SetShowStatusBar(false)
	taskList.SetShowPagination(false)
	taskList.SetShowHelp(false)
	taskList.FilterInput.Prompt = "Tag: "
	taskList.DisableQuitKeybindings()
	applyTaskListStyles(&taskList)

	return taskSelectionModel{
		list:      taskList,
		items:     items,
		selected:  selected,
		listKeys:  newSelectionListKeyMap(),
		filterKey: newTaskFilterKeyMap(),
	}
}

func addIfNotBlank(values map[string]struct{}, value string) {
	if value != "" {
		values[value] = struct{}{}
	}
}

// cycleValue returns the next value in values after current, cycling through "" (no
// filter) followed by each value in order, then back to "".
func cycleValue(current string, values []string) string {
	if current == "" {
		if len(values) == 0 {
			return ""
		}
		return values[0]
	}
	if idx := slices.Index(values, current); idx >= 0 && idx+1 < len(values) {
		return values[idx+1]
	}
	return ""
}

func applyTaskListStyles(taskList *list.Model) {
	styles := taskList.Styles
	styles.TitleBar = lipgloss.NewStyle().Margin(0, 0, 1, 0)
	styles.Title = lipgloss.NewStyle().Bold(true)
	styles.Filter.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(highlightColor))
	styles.Filter.Blurred.Prompt = styles.Filter.Focused.Prompt
	styles.NoItems = lipgloss.NewStyle().Foreground(lipgloss.Color(helpTextColor))
	taskList.Styles = styles
}

func toListItems(items []taskSelectionItem) []list.Item {
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	return listItems
}

// tagPrefixFilter matches an item when EVERY comma-separated query token of the typed
// term is, taken as a whole (including any internal whitespace), a case-insensitive
// prefix of at least one of the item's tags (e.g. "visu" matches the tag "visual", and
// "smoke t" matches the tag "smoke test" while still being typed out). Multiple tags can
// be searched for at once by separating query tokens with a comma (e.g. "smoke, slow"
// matches an item having a tag prefixed by "smoke" AND a - possibly different - tag
// prefixed by "slow"). Blank tokens (from blank input, or leading/trailing/repeated
// commas) are ignored; a term with no non-blank tokens matches every item. A task's extra
// tags do not prevent a match. Unlike splitting a token on whitespace, each token is
// treated as a single, indivisible unit when matched against a tag, so a multi-word tag is
// never fragmented into independently-matchable words.
func tagPrefixFilter(term string, targets []string) []list.Rank {
	queries := tagQueryTokens(term)
	if len(queries) == 0 {
		ranks := make([]list.Rank, len(targets))
		for i := range targets {
			ranks[i] = list.Rank{Index: i}
		}
		return ranks
	}

	ranks := []list.Rank{}
	for i, target := range targets {
		if matchesAllTagPrefixes(itemTags(target), queries) {
			ranks = append(ranks, list.Rank{Index: i})
		}
	}
	return ranks
}

// tagQueryTokens splits term on tagQuerySeparator into lowercased, whitespace-trimmed,
// non-blank query tokens.
func tagQueryTokens(term string) []string {
	parts := strings.Split(term, tagQuerySeparator)
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if token := strings.ToLower(strings.TrimSpace(part)); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// itemTags recovers the original, whole tag strings from a FilterValue() string.
func itemTags(filterValue string) []string {
	if filterValue == "" {
		return nil
	}
	return strings.Split(strings.ToLower(filterValue), tagValueSeparator)
}

// matchesAllTagPrefixes reports whether every query in queries is a case-insensitive
// prefix of at least one tag in tags (queries may each match a different tag).
func matchesAllTagPrefixes(tags []string, queries []string) bool {
	for _, query := range queries {
		if !matchesAnyTagPrefix(tags, query) {
			return false
		}
	}
	return true
}

func matchesAnyTagPrefix(tags []string, query string) bool {
	return slices.ContainsFunc(tags, func(tag string) bool {
		return strings.HasPrefix(tag, query)
	})
}

func (m taskSelectionModel) Init() tea.Cmd {
	return nil
}

func (m taskSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) { //nolint:gocritic
	case tea.WindowSizeMsg:
		m.uiIsReady = true
		m.width = msg.Width
		m.height = msg.Height
		m.resizeList()
		return m, nil
	case tea.KeyPressMsg:
		if key.Matches(msg, m.listKeys.Exit) {
			m.action = Exit
			return m, tea.Quit
		}
		if m.list.SettingFilter() {
			if key.Matches(msg, m.listKeys.Confirm) && len(m.list.VisibleItems()) == 0 {
				// bubbles/list's own accept-filter handling silently discards the typed
				// tag search and reverts to the previously applied filters whenever it
				// matches nothing (list.go: "If we've filtered down to nothing, clear
				// the filter"). That is inconsistent with every other filter dimension,
				// where narrowing down to zero matches simply shows "No tasks.". Apply
				// the filter ourselves instead of forwarding the key to the list, so a
				// tag search with no matches behaves the same way.
				m.list.SetFilterState(list.FilterApplied)
				m.list.FilterInput.Blur()
				return m, tea.ClearScreen
			}
			model, cmd := m.updateList(msg)
			// Typing in the tag filter reshuffles filtersLine's content, which can
			// trip up the terminal renderer's incremental line-diffing (content
			// shifting position within a line rather than just growing/shrinking
			// at the end). Force a full redraw, matching what a terminal resize
			// already does to work around the same class of artifact.
			return model, tea.Batch(cmd, tea.ClearScreen)
		}
		return m.handleBrowsingKey(msg)
	default:
		return m.updateList(msg)
	}
}

func (m taskSelectionModel) handleBrowsingKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.listKeys.Cancel):
		m.action = Quit
		return m, tea.Quit
	case key.Matches(msg, m.listKeys.Confirm):
		m.action = Continue
		return m, tea.Quit
	case key.Matches(msg, m.listKeys.Toggle):
		m.toggleSelectedTask()
		return m, tea.ClearScreen
	case key.Matches(msg, m.listKeys.SelectVisible):
		m.setVisibleSelection(true)
		return m, tea.ClearScreen
	case key.Matches(msg, m.listKeys.DeselectVisible):
		m.setVisibleSelection(false)
		return m, tea.ClearScreen
	case key.Matches(msg, m.listKeys.ClearFilters):
		m.clearFilters()
		return m, tea.ClearScreen
	case key.Matches(msg, m.filterKey.CycleSuite):
		m.activeSuite = cycleValue(m.activeSuite, m.availableSuiteValues())
		m.applyMetadataFilters()
		return m, tea.ClearScreen
	case key.Matches(msg, m.filterKey.ClearSuite):
		m.activeSuite = ""
		m.applyMetadataFilters()
		return m, tea.ClearScreen
	case key.Matches(msg, m.filterKey.CycleCategory):
		m.activeCategory = cycleValue(m.activeCategory, m.availableCategoryValues())
		m.applyMetadataFilters()
		return m, tea.ClearScreen
	case key.Matches(msg, m.filterKey.ClearCategory):
		m.activeCategory = ""
		m.applyMetadataFilters()
		return m, tea.ClearScreen
	case key.Matches(msg, m.filterKey.CycleDifficulty):
		m.activeDifficulty = cycleValue(m.activeDifficulty, m.availableDifficultyValues())
		m.applyMetadataFilters()
		return m, tea.ClearScreen
	case key.Matches(msg, m.filterKey.ClearDifficulty):
		m.activeDifficulty = ""
		m.applyMetadataFilters()
		return m, tea.ClearScreen
	default:
		return m.updateList(msg)
	}
}

func (m taskSelectionModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *taskSelectionModel) resizeList() {
	m.list.SetSize(m.width, max(1, m.height-taskPickerReservedLines))
}

func (m *taskSelectionModel) toggleSelectedTask() {
	item, ok := m.list.SelectedItem().(taskSelectionItem)
	if !ok {
		return
	}
	m.selected[item.taskIndex] = !m.selected[item.taskIndex]
}

func (m *taskSelectionModel) setVisibleSelection(selected bool) {
	for _, item := range m.visibleTaskItems() {
		m.selected[item.taskIndex] = selected
	}
}

func (m *taskSelectionModel) clearFilters() {
	m.activeSuite = ""
	m.activeCategory = ""
	m.activeDifficulty = ""
	m.list.ResetFilter()
	m.applyMetadataFilters()
}

func (m *taskSelectionModel) applyMetadataFilters() {
	filterValue := m.list.FilterValue()
	m.list.SetItems(toListItems(m.metadataFilteredItems()))
	if filterValue != "" {
		m.list.SetFilterText(filterValue)
	}
}

func (m taskSelectionModel) metadataFilteredItems() []taskSelectionItem {
	if !m.hasMetadataFilters() {
		return m.items
	}

	items := make([]taskSelectionItem, 0, len(m.items))
	for _, item := range m.items {
		if m.taskMatchesMetadataFilters(item) {
			items = append(items, item)
		}
	}
	return items
}

type metadataFilterDimension int

const (
	metadataFilterSuite metadataFilterDimension = iota
	metadataFilterCategory
	metadataFilterDifficulty
)

func (m taskSelectionModel) availableSuiteValues() []string {
	return m.availableMetadataValues(metadataFilterSuite, func(item taskSelectionItem) string {
		return item.suite
	})
}

func (m taskSelectionModel) availableCategoryValues() []string {
	return m.availableMetadataValues(metadataFilterCategory, func(item taskSelectionItem) string {
		return item.category
	})
}

func (m taskSelectionModel) availableDifficultyValues() []string {
	return m.availableMetadataValues(metadataFilterDifficulty, func(item taskSelectionItem) string {
		return item.difficulty
	})
}

func (m taskSelectionModel) availableMetadataValues(ignored metadataFilterDimension, value func(taskSelectionItem) string) []string {
	values := make(map[string]struct{})
	for _, item := range m.items {
		if m.taskMatchesMetadataFiltersExcept(item, ignored) {
			addIfNotBlank(values, value(item))
		}
	}
	return utils.SortedKeys(values)
}

func (m taskSelectionModel) taskMatchesMetadataFiltersExcept(item taskSelectionItem, ignored metadataFilterDimension) bool {
	if ignored != metadataFilterSuite && m.activeSuite != "" && item.suite != m.activeSuite {
		return false
	}
	if ignored != metadataFilterCategory && m.activeCategory != "" && item.category != m.activeCategory {
		return false
	}
	if ignored != metadataFilterDifficulty && m.activeDifficulty != "" && item.difficulty != m.activeDifficulty {
		return false
	}
	return true
}

func (m taskSelectionModel) hasMetadataFilters() bool {
	return m.activeSuite != "" || m.activeCategory != "" || m.activeDifficulty != ""
}

func (m taskSelectionModel) taskMatchesMetadataFilters(item taskSelectionItem) bool {
	if m.activeSuite != "" && item.suite != m.activeSuite {
		return false
	}
	if m.activeCategory != "" && item.category != m.activeCategory {
		return false
	}
	if m.activeDifficulty != "" && item.difficulty != m.activeDifficulty {
		return false
	}
	return true
}

func (m taskSelectionModel) visibleTaskItems() []taskSelectionItem {
	visible := m.list.VisibleItems()
	items := make([]taskSelectionItem, 0, len(visible))
	for _, item := range visible {
		if taskItem, ok := item.(taskSelectionItem); ok {
			items = append(items, taskItem)
		}
	}
	return items
}

func (m taskSelectionModel) selectedCount() int {
	count := 0
	for _, selected := range m.selected {
		if selected {
			count++
		}
	}
	return count
}

func (m taskSelectionModel) allTasksSelected() bool {
	for _, selected := range m.selected {
		if !selected {
			return false
		}
	}
	return true
}

// countsLine renders the "N visible / N total • N selected" summary. It is rendered on
// its own line (separate from filtersLine) so that a long list of active filters cannot
// push it out of the available width, and vice versa.
func (m taskSelectionModel) countsLine() string {
	line := fmt.Sprintf(
		"%d visible / %d total • %d selected",
		len(m.visibleTaskItems()),
		len(m.items),
		m.selectedCount(),
	)
	if m.width > 0 {
		return ansi.Truncate(line, m.width, "…")
	}
	return line
}

// filtersLine renders the active suite/category/difficulty/tag filters, or a blank line
// if none are active. It is rendered on its own line (separate from countsLine) so that
// truncation never silently drops one of the active filters, such as a difficulty set
// earlier being pushed past the width limit by a longer category name.
func (m taskSelectionModel) filtersLine() string {
	var line string
	if activeFilters := m.activeFilters(); len(activeFilters) > 0 {
		line = "filters: " + strings.Join(activeFilters, ", ")
	}
	if m.width > 0 {
		return ansi.Truncate(line, m.width, "…")
	}
	return line
}

func (m taskSelectionModel) activeFilters() []string {
	filters := []string{}
	if m.activeSuite != "" {
		filters = append(filters, "suite="+m.activeSuite)
	}
	if m.activeCategory != "" {
		filters = append(filters, "category="+m.activeCategory)
	}
	if m.activeDifficulty != "" {
		filters = append(filters, "difficulty="+m.activeDifficulty)
	}
	if m.list.SettingFilter() {
		filters = append(filters, "tag="+m.list.FilterValue()+"_")
	} else if m.list.FilterValue() != "" {
		filters = append(filters, "tag="+m.list.FilterValue())
	}
	return filters
}

func (m taskSelectionModel) helpText() string {
	if m.list.SettingFilter() {
		return "type: tag search • enter: apply • esc: clear tag search • ctrl+c: exit"
	}
	return "↑/↓: navigate • space: toggle • a/A: select/deselect visible • enter: confirm • q/esc: cancel • ctrl+c: exit • s/S: suite • c/C: category • d/D: difficulty • r: clear filters • /: tag search"
}

func (m taskSelectionModel) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if !m.uiIsReady {
		v.SetContent(initializingMsg)
		return v
	}

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(helpTextColor)).Margin(1, 0, 0, 0)
	content := strings.Join([]string{
		m.list.View(),
		"",
		m.countsLine(),
		m.filtersLine(),
		helpStyle.Render(m.helpText()),
	}, "\n")
	v.SetContent(content)
	return v
}

type taskSelectionDelegate struct {
	selected *[]bool
}

func (d taskSelectionDelegate) Height() int {
	return 1
}

func (d taskSelectionDelegate) Spacing() int {
	return 0
}

func (d taskSelectionDelegate) Update(tea.Msg, *list.Model) tea.Cmd {
	return nil
}

func (d taskSelectionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	taskItem, ok := item.(taskSelectionItem)
	if !ok {
		return
	}

	cursor := " "
	if index == m.Index() && !m.SettingFilter() {
		cursor = ">"
	}

	checked := "[ ]"
	if taskItem.taskIndex < len(*d.selected) && (*d.selected)[taskItem.taskIndex] {
		checked = "[x]"
	}

	line := fmt.Sprintf("%s %s %s", cursor, checked, taskItem.name)
	if m.Width() > 0 {
		line = ansi.Truncate(line, m.Width(), "…")
	}
	if index == m.Index() && !m.SettingFilter() {
		line = lipgloss.NewStyle().Foreground(lipgloss.Color(highlightColor)).Render(line)
	}
	fmt.Fprint(w, line) //nolint:errcheck
}

// DisplayTaskPicker displays a terminal UI for enabling or disabling tasks.
// It returns the selected user action and an error if the selection fails.
// This function modifies the provided taskConfig directly.
func DisplayTaskPicker(taskConfig *config.TaskConfig) (UserInputEvent, error) {
	if !IsTerminal() {
		return Exit, fmt.Errorf("%w: %v", ErrInteractiveMode, ErrTerminalRequired)
	}

	p := tea.NewProgram(newTaskSelectionModel(taskConfig))
	finalModel, err := p.Run() // blocking call
	if err != nil {
		return Exit, fmt.Errorf("%w: task selection: %v", ErrInteractiveMode, err)
	}

	picker := finalModel.(taskSelectionModel)
	if picker.action == Continue {
		applyTaskSelection(taskConfig, picker)
	}

	return picker.action, nil // if dialog canceled, return without changes
}

func applyTaskSelection(taskConfig *config.TaskConfig, picker taskSelectionModel) {
	if len(taskConfig.Tasks) > 0 {
		taskConfig.Disabled = !picker.allTasksSelected()
	}
	for i := range taskConfig.Tasks {
		disabled := !picker.selected[i]
		taskConfig.Tasks[i].Disabled = &disabled
	}
}
