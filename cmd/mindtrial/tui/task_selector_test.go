// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package tui

import (
	"sort"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/petmal/mindtrial/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCycleValue(t *testing.T) {
	values := []string{"easy", "hard", "medium"}

	tests := []struct {
		name    string
		current string
		want    string
	}{
		{name: "no filter cycles to first value", current: "", want: "easy"},
		{name: "first value cycles to second", current: "easy", want: "hard"},
		{name: "last value cycles back to no filter", current: "medium", want: ""},
		{name: "value no longer present resets to no filter", current: "unknown", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cycleValue(tt.current, values))
		})
	}

	t.Run("no values to cycle through", func(t *testing.T) {
		assert.Empty(t, cycleValue("", nil))
	})
}

func TestTaskSelectionItemFilterValue(t *testing.T) {
	item := taskSelectionItem{tags: []string{"smoke test", "nightly"}}
	assert.Equal(t, "smoke test"+tagValueSeparator+"nightly", item.FilterValue(),
		"tags must be joined with a separator that cannot appear in a multi-word tag, so a tag containing a space round-trips intact")
}

func TestTagPrefixFilter(t *testing.T) {
	join := func(tags ...string) string {
		return strings.Join(tags, tagValueSeparator)
	}

	tests := []struct {
		name    string
		term    string
		targets []string
		want    []int
	}{
		{name: "empty term matches all", term: "", targets: []string{join("nightly"), join("smoke")}, want: []int{0, 1}},
		{name: "case-insensitive exact tag match", term: "VISION", targets: []string{join("vision"), join("smoke")}, want: []int{0}},
		{name: "prefix of a single-word tag matches", term: "vis", targets: []string{join("visual"), join("regression")}, want: []int{0}},
		{name: "extra task tags do not prevent a match", term: "smoke", targets: []string{join("visual", "smoke"), join("visual")}, want: []int{0}},
		{name: "multi-word tag matches while still being typed", term: "smoke t", targets: []string{join("smoke test")}, want: []int{0}},
		{name: "multi-word tag matches on trailing space", term: "smoke ", targets: []string{join("smoke test")}, want: []int{0}},
		{name: "multi-word tag does not match a divergent continuation", term: "smoke x", targets: []string{join("smoke test")}, want: []int{}},
		{name: "second word alone does not match a multi-word tag", term: "test", targets: []string{join("smoke test")}, want: []int{}},
		{name: "space no longer combines prefixes of two distinct tags", term: "smoke slow", targets: []string{join("smoke", "slow")}, want: []int{}},
		{name: "comma combines prefixes of two distinct tags", term: "smoke,slow", targets: []string{join("smoke", "slow"), join("smoke")}, want: []int{0}},
		{name: "comma-separated tokens tolerate surrounding whitespace", term: " smoke , slow ", targets: []string{join("smoke test", "slow down")}, want: []int{0}},
		{name: "comma-separated query requires every token to match, possibly different tags", term: "smoke,slow", targets: []string{join("smoke"), join("slow"), join("smoke", "slow"), join("smoke", "fast")}, want: []int{2}},
		{name: "blank tokens from stray commas are ignored", term: ",,smoke,,", targets: []string{join("smoke"), join("visual")}, want: []int{0}},
		{name: "only stray commas matches all, same as empty term", term: " , ", targets: []string{join("smoke"), join("visual")}, want: []int{0, 1}},
		{name: "no matching prefix", term: "regression", targets: []string{join("visual"), join("smoke")}, want: []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranks := tagPrefixFilter(tt.term, tt.targets)
			got := make([]int, len(ranks))
			for i, rank := range ranks {
				got[i] = rank.Index
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTaskSelectionModelAvailableMetadataValues(t *testing.T) {
	tasks := []config.Task{
		{Name: "vis-spa-hard", Suite: "visual", Category: "spatial awareness", Difficulty: "hard"},
		{Name: "vis-logic-medium", Suite: "visual", Category: "logic math", Difficulty: "medium"},
		{Name: "rid-anagram-medium", Suite: "riddle", Category: "anagram", Difficulty: "medium"},
		{Name: "rid-logic-easy", Suite: "riddle", Category: "logic math", Difficulty: "easy"},
	}

	// expectedAvailable computes, independently of the production code, the sorted unique
	// non-empty values for dimension among tasks matching every OTHER given active filter.
	// This is the ground truth that m.available*Values() is checked against below.
	expectedAvailable := func(dimension string, suite string, category string, difficulty string) []string {
		values := make(map[string]struct{})
		for _, task := range tasks {
			if dimension != "suite" && suite != "" && task.Suite != suite {
				continue
			}
			if dimension != "category" && category != "" && task.Category != category {
				continue
			}
			if dimension != "difficulty" && difficulty != "" && task.Difficulty != difficulty {
				continue
			}

			var value string
			switch dimension {
			case "suite":
				value = task.Suite
			case "category":
				value = task.Category
			case "difficulty":
				value = task.Difficulty
			}
			if value != "" {
				values[value] = struct{}{}
			}
		}

		result := make([]string, 0, len(values))
		for value := range values {
			result = append(result, value)
		}
		sort.Strings(result)
		return result
	}

	// assertAvailable checks that every dimension's available values match the ground truth
	// for the model's CURRENT active filters, regardless of which order they were set in.
	assertAvailable := func(t *testing.T, m taskSelectionModel) {
		t.Helper()
		assert.Equal(t, expectedAvailable("suite", m.activeSuite, m.activeCategory, m.activeDifficulty), m.availableSuiteValues(), "suite")
		assert.Equal(t, expectedAvailable("category", m.activeSuite, m.activeCategory, m.activeDifficulty), m.availableCategoryValues(), "category")
		assert.Equal(t, expectedAvailable("difficulty", m.activeSuite, m.activeCategory, m.activeDifficulty), m.availableDifficultyValues(), "difficulty")
	}

	newModel := func() taskSelectionModel {
		return newTaskSelectionModel(&config.TaskConfig{Tasks: tasks})
	}

	t.Run("no filters active", func(t *testing.T) {
		assertAvailable(t, newModel())
	})

	t.Run("single filter active", func(t *testing.T) {
		t.Run("suite only", func(t *testing.T) {
			m := newModel()
			m.activeSuite = "visual"
			assertAvailable(t, m)
		})
		t.Run("category only", func(t *testing.T) {
			m := newModel()
			m.activeCategory = "logic math"
			assertAvailable(t, m)
		})
		t.Run("difficulty only", func(t *testing.T) {
			m := newModel()
			m.activeDifficulty = "medium"
			assertAvailable(t, m)
		})
	})

	// setFilter applies one of the three active filter dimensions using the same value
	// for every test, keeping the combination matrix below concise.
	setFilter := map[string]func(*taskSelectionModel){
		"suite":      func(m *taskSelectionModel) { m.activeSuite = "visual" },
		"category":   func(m *taskSelectionModel) { m.activeCategory = "logic math" },
		"difficulty": func(m *taskSelectionModel) { m.activeDifficulty = "medium" },
	}

	t.Run("two filters active in every order", func(t *testing.T) {
		pairs := [][2]string{
			{"suite", "category"}, {"category", "suite"},
			{"suite", "difficulty"}, {"difficulty", "suite"},
			{"category", "difficulty"}, {"difficulty", "category"},
		}
		for _, pair := range pairs {
			t.Run(strings.Join(pair[:], "-then-"), func(t *testing.T) {
				m := newModel()
				for _, dimension := range pair {
					setFilter[dimension](&m)
					assertAvailable(t, m)
				}
			})
		}
	})

	t.Run("three filters active in every order", func(t *testing.T) {
		orders := [][3]string{
			{"suite", "category", "difficulty"},
			{"suite", "difficulty", "category"},
			{"category", "suite", "difficulty"},
			{"category", "difficulty", "suite"},
			{"difficulty", "suite", "category"},
			{"difficulty", "category", "suite"},
		}
		for _, order := range orders {
			t.Run(strings.Join(order[:], "-then-"), func(t *testing.T) {
				m := newModel()
				for _, dimension := range order {
					setFilter[dimension](&m)
					assertAvailable(t, m)
				}
			})
		}
	})
}

func TestTaskSelectionModelMetadataFiltering(t *testing.T) {
	m := newTaskSelectionModel(&config.TaskConfig{Tasks: []config.Task{
		{Name: "t1", Suite: "core", Category: "math", Difficulty: "easy", Tags: []string{"smoke"}},
		{Name: "t2", Suite: "extended", Category: "coding", Difficulty: "hard", Tags: []string{"nightly"}},
		{Name: "t3", Suite: "core", Category: "coding", Difficulty: "hard", Tags: []string{"regression"}},
	}})

	m.activeSuite = "core"
	m.applyMetadataFilters()
	assert.Equal(t, []string{"t1", "t3"}, visibleTaskNames(m))

	m.activeCategory = "coding"
	m.applyMetadataFilters()
	assert.Equal(t, []string{"t3"}, visibleTaskNames(m))

	m.list.SetFilterText("regression")
	assert.Equal(t, []string{"t3"}, visibleTaskNames(m))

	m.clearFilters()
	assert.Equal(t, []string{"t1", "t2", "t3"}, visibleTaskNames(m))
	assert.Empty(t, m.list.FilterValue())
}

func TestTaskSelectionModelSelection(t *testing.T) {
	m := newTaskSelectionModel(&config.TaskConfig{Tasks: []config.Task{
		{Name: "t1", Suite: "core"},
		{Name: "t2", Suite: "extended"},
		{Name: "t3", Suite: "core"},
	}})

	m.activeSuite = "core"
	m.applyMetadataFilters()
	m.setVisibleSelection(false)

	assert.False(t, m.selected[0])
	assert.True(t, m.selected[1], "hidden task should be left unchanged")
	assert.False(t, m.selected[2])

	m.toggleSelectedTask()
	assert.True(t, m.selected[0])
}

func TestApplyTaskSelection(t *testing.T) {
	t.Run("sets global disabled when some tasks are deselected", func(t *testing.T) {
		taskConfig := config.TaskConfig{Tasks: []config.Task{{Name: "t1"}, {Name: "t2"}}}
		m := newTaskSelectionModel(&taskConfig)
		m.selected[1] = false

		applyTaskSelection(&taskConfig, m)

		assert.True(t, taskConfig.Disabled)
		require.NotNil(t, taskConfig.Tasks[0].Disabled)
		require.NotNil(t, taskConfig.Tasks[1].Disabled)
		assert.False(t, *taskConfig.Tasks[0].Disabled)
		assert.True(t, *taskConfig.Tasks[1].Disabled)
	})

	t.Run("leaves global disabled unchanged for empty task list", func(t *testing.T) {
		taskConfig := config.TaskConfig{Disabled: true}
		m := newTaskSelectionModel(&taskConfig)

		applyTaskSelection(&taskConfig, m)

		assert.True(t, taskConfig.Disabled)
	})
}

func TestTaskSelectionModelFilteringKeyInput(t *testing.T) {
	model := newTaskSelectionModel(&config.TaskConfig{Tasks: []config.Task{
		{Name: "t1", Suite: "core", Tags: []string{"ssss"}},
	}})

	updated, _ := model.Update(keyPress("/"))
	model = updated.(taskSelectionModel)
	require.True(t, model.list.SettingFilter())

	for range 4 {
		updated, _ = model.Update(keyPress("s"))
		model = updated.(taskSelectionModel)
	}

	assert.Equal(t, "ssss", model.list.FilterValue())
	assert.Empty(t, model.activeSuite, "typing s in tag search must not cycle suite")
}

// TestTaskSelectionModelTagSearchNoMatchesShowsNoTasks is a regression test: bubbles/list's
// own accept-filter handling silently discards a tag search and reverts to the previously
// applied filters whenever it matches nothing, which is inconsistent with every other filter
// dimension (suite/category/difficulty), where narrowing down to zero matches simply shows
// "No tasks.". Confirming a non-matching tag search must behave the same way.
func TestTaskSelectionModelTagSearchNoMatchesShowsNoTasks(t *testing.T) {
	model := newTaskSelectionModel(&config.TaskConfig{Tasks: []config.Task{
		{Name: "t1", Tags: []string{"widget"}},
	}})

	// Put the list into the same state it would be in after the user types a
	// non-matching tag search term and before they press enter: filtering in
	// progress, with filteredItems already computed for the current input.
	model.list.SetFilterText("nomatch")
	model.list.SetFilterState(list.Filtering)
	require.True(t, model.list.SettingFilter())
	require.Empty(t, model.list.VisibleItems(), "tag search term must not match any task's tags")

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(taskSelectionModel)

	assert.False(t, model.list.SettingFilter(), "filter editing should end on enter")
	assert.True(t, model.list.IsFiltered(), "filter must stay applied instead of being reset")
	assert.Equal(t, "nomatch", model.list.FilterValue(), "the typed tag search term must not be discarded")
	assert.Empty(t, model.list.VisibleItems())
}

func TestTaskSelectionModelStatusLinesAreTruncatedIndependently(t *testing.T) {
	m := newTaskSelectionModel(&config.TaskConfig{Tasks: []config.Task{
		{Name: "t1", Suite: "very-long-suite", Category: "very-long-category", Difficulty: "very-long-difficulty", Tags: []string{"very-long-tag"}},
	}})
	m.width = 40
	m.activeSuite = "very-long-suite"
	m.activeCategory = "very-long-category"
	m.activeDifficulty = "very-long-difficulty"
	m.list.SetFilterText("very-long-tag")

	assert.LessOrEqual(t, len([]rune(m.countsLine())), 40)
	assert.LessOrEqual(t, len([]rune(m.filtersLine())), 40)
}

// TestTaskSelectionModelFiltersLineKeepsAllActiveFilters is a regression test: when the
// counts and filters summaries shared a single line, a long category name could push an
// earlier-set difficulty filter past the truncation width, silently hiding it. Rendering
// them on separate lines gives the filters their own width budget.
func TestTaskSelectionModelFiltersLineKeepsAllActiveFilters(t *testing.T) {
	m := newTaskSelectionModel(&config.TaskConfig{Tasks: []config.Task{
		{Name: "t1", Suite: "visual2", Category: "numerical awareness", Difficulty: "hard"},
	}})
	m.width = 91
	m.activeSuite = "visual2"
	m.activeCategory = "numerical awareness"
	m.activeDifficulty = "hard"

	assert.Contains(t, m.filtersLine(), "difficulty=hard")
}

func visibleTaskNames(m taskSelectionModel) []string {
	items := m.visibleTaskItems()
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.name
	}
	return names
}

func keyPress(text string) tea.KeyPressMsg {
	if text == "/" || text == "s" {
		return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
	}
	return tea.KeyPressMsg(tea.Key{Code: []rune(text)[0]})
}
