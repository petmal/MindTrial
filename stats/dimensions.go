// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"errors"
	"fmt"
	"strings"

	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
)

// Dimension identifies a result/task attribute that stats records can be grouped or filtered by.
type Dimension string

const (
	// DimensionProvider groups/filters by the AI provider name.
	DimensionProvider Dimension = "provider"
	// DimensionRun groups/filters by the provider's run configuration name.
	DimensionRun Dimension = "run"
	// DimensionModel groups/filters by the resolved model identifier.
	DimensionModel Dimension = "model"
	// DimensionSuite groups/filters by the task's suite label.
	DimensionSuite Dimension = "suite"
	// DimensionCategory groups/filters by the task's category label.
	DimensionCategory Dimension = "category"
	// DimensionDifficulty groups/filters by the task's difficulty label.
	DimensionDifficulty Dimension = "difficulty"
	// DimensionTag groups/filters by task tag. Unlike the other dimensions, a single result
	// can carry multiple tags, so grouping by tag causes that result to be counted in every
	// tag group it belongs to (groups overlap and are not additive).
	DimensionTag Dimension = "tag"
)

// unspecifiedValue is used as a placeholder grouping/filter value when a result has no
// value for a given dimension, so such results are grouped explicitly rather than dropped.
const unspecifiedValue = "(unspecified)"

var validDimensions = map[Dimension]bool{
	DimensionProvider:   true,
	DimensionRun:        true,
	DimensionModel:      true,
	DimensionSuite:      true,
	DimensionCategory:   true,
	DimensionDifficulty: true,
	DimensionTag:        true,
}

// ErrInvalidDimension is returned when a group-by dimension name is not recognized.
var ErrInvalidDimension = errors.New("invalid group-by dimension")

// ParseDimensions parses a comma-separated list of dimension names into validated,
// lower-cased Dimension values. Blank entries are ignored. Each dimension may appear at
// most once: a repeated dimension has no sensible use case and would otherwise produce
// ambiguous combinations downstream (see dimensionCombinations/buildRecord).
func ParseDimensions(commaSeparated string) ([]Dimension, error) {
	var dims []Dimension
	var names []string
	for _, name := range strings.Split(commaSeparated, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		dim := Dimension(strings.ToLower(name))
		if !validDimensions[dim] {
			return nil, fmt.Errorf("%w: %q", ErrInvalidDimension, name)
		}
		dims = append(dims, dim)
		names = append(names, string(dim))
	}
	if len(dims) == 0 {
		return nil, fmt.Errorf("%w: group-by must specify at least one dimension", ErrInvalidDimension)
	}
	// A shorter unique set than the parsed list means some dimension was repeated.
	if unique := utils.NewStringSet(names...).Values(); len(unique) != len(dims) {
		return nil, fmt.Errorf("%w: each dimension may appear at most once in %q", ErrInvalidDimension, commaSeparated)
	}
	return dims, nil
}

func orUnspecified(value string) string {
	if strings.TrimSpace(value) == "" {
		return unspecifiedValue
	}
	return value
}

// dimensionValues returns all values a result contributes for the given dimension. Every
// dimension except DimensionTag contributes exactly one value.
func dimensionValues(r runners.RunResult, dim Dimension) []string {
	switch dim {
	case DimensionProvider:
		return []string{orUnspecified(r.Provider)}
	case DimensionRun:
		return []string{orUnspecified(r.Run)}
	case DimensionModel:
		return []string{orUnspecified(r.RunConfig.Model)}
	case DimensionSuite:
		return []string{orUnspecified(r.TaskMetadata.Suite)}
	case DimensionCategory:
		return []string{orUnspecified(r.TaskMetadata.Category)}
	case DimensionDifficulty:
		return []string{orUnspecified(r.TaskMetadata.Difficulty)}
	case DimensionTag:
		if len(r.TaskMetadata.Tags) == 0 {
			return []string{unspecifiedValue}
		}
		// Task tags are nominally a set, but the source config does not enforce uniqueness;
		// dedupe them here so a duplicate tag doesn't double-count its result in that tag group.
		return utils.NewStringSet(r.TaskMetadata.Tags...).Values()
	default:
		return nil
	}
}

// dimensionCombinations returns every distinct combination of dimension values (in groupBy
// order) that the result contributes to. A result contributes to more than one combination
// only when groupBy includes a multi-valued dimension (currently only DimensionTag), in
// which case it is exploded into one combination per value of that dimension.
func dimensionCombinations(r runners.RunResult, groupBy []Dimension) [][]string {
	combos := [][]string{{}}
	for _, dim := range groupBy {
		values := dimensionValues(r, dim)
		next := make([][]string, 0, len(combos)*len(values))
		for _, combo := range combos {
			for _, value := range values {
				extended := make([]string, len(combo), len(combo)+1)
				copy(extended, combo)
				next = append(next, append(extended, value))
			}
		}
		combos = next
	}
	return combos
}
