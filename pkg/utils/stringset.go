// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package utils

import (
	"errors"
	"fmt"

	"slices"

	"gopkg.in/yaml.v3"
)

// ErrInvalidStringSetValue indicates invalid StringSet definition.
var ErrInvalidStringSetValue = errors.New("invalid string-set value")

// StringSet represents a set of unique string values.
type StringSet struct {
	values []string
}

// NewStringSet creates a new StringSet from the given items, discarding duplicates.
func NewStringSet(items ...string) StringSet {
	set := make(map[string]struct{}, len(items))
	unique := make([]string, 0, len(items))
	for _, v := range items {
		if _, exists := set[v]; !exists {
			unique = append(unique, v)
			set[v] = struct{}{}
		}
	}
	return StringSet{values: unique}
}

// Values returns a copy of the set's values.
func (s StringSet) Values() []string {
	return slices.Clone(s.values)
}

// Any returns true if any value in the set satisfies the given condition.
func (s StringSet) Any(condition func(string) bool) bool {
	return slices.ContainsFunc(s.values, condition)
}

// Map returns a new StringSet with f applied to each value, discarding duplicates.
func (s StringSet) Map(f func(string) string) StringSet {
	mapped := make([]string, len(s.values))
	for i, v := range s.values {
		mapped[i] = f(v)
	}
	return NewStringSet(mapped...)
}

// UnmarshalYAML allows StringSet to be loaded from either a string or a list of strings.
func (s *StringSet) UnmarshalYAML(value *yaml.Node) error {
	var items []string
	switch value.Kind {
	case yaml.ScalarNode:
		var single string
		if err := value.Decode(&single); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidStringSetValue, err)
		}
		items = append(items, single)
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidStringSetValue, err)
		}
		items = list
	default:
		return fmt.Errorf("%w: must be a string or list of strings", ErrInvalidStringSetValue)
	}
	*s = NewStringSet(items...)
	return nil
}

// MarshalYAML implements YAML marshaling for StringSet.
func (s StringSet) MarshalYAML() (interface{}, error) {
	if len(s.values) == 1 {
		return s.values[0], nil
	}
	return s.values, nil
}
