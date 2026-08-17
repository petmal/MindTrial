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

	"github.com/petmal/mindtrial/runners"
)

// TagMode determines how multiple Filters.Tags values are combined.
type TagMode string

const (
	// TagModeAll requires a result to carry every filtered tag (logical AND).
	TagModeAll TagMode = "all"
	// TagModeAny requires a result to carry at least one filtered tag (logical OR).
	TagModeAny TagMode = "any"
	// TagModeDefault is the mode used when Filters.TagMode/a --tag-mode value is blank.
	TagModeDefault = TagModeAll
)

var (
	// ErrInvalidStatus is returned when a --status filter value is not recognized.
	ErrInvalidStatus = errors.New("invalid status filter value")
	// ErrInvalidTagMode is returned when a --tag-mode value is not recognized.
	ErrInvalidTagMode = errors.New("invalid tag-mode value")
)

var statusKinds = map[string]runners.ResultKind{
	"passed":  runners.Success,
	"failed":  runners.Failure,
	"error":   runners.Error,
	"skipped": runners.NotSupported,
}

// ParseTagMode validates and normalizes a --tag-mode value. A blank value defaults to
// TagModeDefault.
func ParseTagMode(value string) (TagMode, error) {
	switch normalized := TagMode(strings.ToLower(strings.TrimSpace(value))); normalized {
	case "":
		return TagModeDefault, nil
	case TagModeAll, TagModeAny:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidTagMode, value)
	}
}

// Filters selects the subset of results that contribute to computed stats. Each field is
// matched case-insensitively; multiple values within one field are combined with a logical
// OR (any match is sufficient), except Tags, whose combination is controlled by TagMode.
// A nil/empty field imposes no restriction on that attribute.
type Filters struct {
	Providers    []string
	Runs         []string
	Models       []string
	Suites       []string
	Categories   []string
	Difficulties []string
	Statuses     []string
	Tags         []string
	TagMode      TagMode
}

// Validate checks that every filter value is well-formed (currently only Statuses needs
// parsing), without requiring any results to be present. Useful for failing fast on a bad
// CLI filter before loading input files.
func (f Filters) Validate() error {
	_, err := f.resolvedStatuses()
	return err
}

// resolvedStatuses parses and validates Statuses, returning the corresponding result kinds,
// or nil if no status filter was configured.
func (f Filters) resolvedStatuses() (map[runners.ResultKind]bool, error) {
	if len(f.Statuses) == 0 {
		return nil, nil
	}
	kinds := make(map[runners.ResultKind]bool, len(f.Statuses))
	for _, status := range f.Statuses {
		kind, ok := statusKinds[strings.ToLower(strings.TrimSpace(status))]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, status)
		}
		kinds[kind] = true
	}
	return kinds, nil
}

// matches reports whether the result satisfies every configured filter. allowedStatuses is
// the pre-resolved set from resolvedStatuses (nil means no status restriction).
func (f Filters) matches(r runners.RunResult, allowedStatuses map[runners.ResultKind]bool) bool {
	if allowedStatuses != nil && !allowedStatuses[r.Kind] {
		return false
	}
	// Compare against orUnspecified(value) rather than the raw value so a filter can match
	// the same "(unspecified)" placeholder that grouping displays for blank metadata.
	if !matchesAny(orUnspecified(r.Provider), f.Providers) {
		return false
	}
	if !matchesAny(orUnspecified(r.Run), f.Runs) {
		return false
	}
	if !matchesAny(orUnspecified(r.RunConfig.Model), f.Models) {
		return false
	}
	if !matchesAny(orUnspecified(r.TaskMetadata.Suite), f.Suites) {
		return false
	}
	if !matchesAny(orUnspecified(r.TaskMetadata.Category), f.Categories) {
		return false
	}
	if !matchesAny(orUnspecified(r.TaskMetadata.Difficulty), f.Difficulties) {
		return false
	}

	tagMode := f.TagMode
	if tagMode == "" {
		tagMode = TagModeDefault
	}
	return matchesTags(r.TaskMetadata.Tags, f.Tags, tagMode)
}

// matchesAny reports whether value case-insensitively equals one of candidates. An empty
// candidates list imposes no restriction and always matches.
func matchesAny(value string, candidates []string) bool {
	if len(candidates) == 0 {
		return true
	}
	for _, candidate := range candidates {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

// matchesTags reports whether tags satisfies the filter values under the given mode. An
// empty filter list imposes no restriction and always matches. A result with no tags is
// treated as carrying the "(unspecified)" placeholder, mirroring dimensionValues, so a
// filter can target untagged results the same way it targets any other blank metadata.
func matchesTags(tags []string, filter []string, mode TagMode) bool {
	if len(filter) == 0 {
		return true
	}
	if len(tags) == 0 {
		tags = []string{unspecifiedValue}
	}
	matched := 0
	for _, want := range filter {
		for _, tag := range tags {
			if strings.EqualFold(tag, want) {
				matched++
				break
			}
		}
	}
	if mode == TagModeAny {
		return matched > 0
	}
	return matched == len(filter)
}
