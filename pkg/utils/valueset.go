// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package utils

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"
)

// ErrInvalidValueSetValue indicates invalid ValueSet definition.
var ErrInvalidValueSetValue = errors.New("invalid set value")

// ValueSet represents a set of unique values.
// The type is generic and can handle both string values and object values.
type ValueSet struct {
	values []interface{}
}

// NewValueSet creates a new ValueSet from the given items, discarding duplicates.
func NewValueSet(items ...interface{}) ValueSet {
	unique := make([]interface{}, 0, len(items))
	for _, item := range items {
		// Check if item already exists using deep equality.
		if !slices.ContainsFunc(unique, func(existing interface{}) bool {
			return reflect.DeepEqual(existing, item)
		}) {
			unique = append(unique, item)
		}
	}
	return ValueSet{values: unique}
}

// Values returns a copy of the set's values.
func (v ValueSet) Values() []interface{} {
	return slices.Clone(v.values)
}

// Any returns true if any value in the set satisfies the given condition.
func (v ValueSet) Any(condition func(interface{}) bool) bool {
	return slices.ContainsFunc(v.values, condition)
}

// Map applies a transformation function to all values and returns a new ValueSet with duplicates removed.
func (v ValueSet) Map(transform func(interface{}) interface{}) ValueSet {
	transformed := make([]interface{}, len(v.values))
	for i, val := range v.values {
		transformed[i] = transform(val)
	}
	return NewValueSet(transformed...)
}

// AsStringSet returns the values as a StringSet if they are all strings.
// Returns (values, true) if all values are strings, (empty StringSet, false) otherwise.
func (v ValueSet) AsStringSet() (StringSet, bool) {
	strings := make([]string, 0, len(v.values))
	for _, val := range v.values {
		if str, ok := val.(string); ok {
			strings = append(strings, str)
		} else {
			return StringSet{}, false
		}
	}
	return NewStringSet(strings...), true
}

// UnmarshalYAML implements custom YAML unmarshaling for ValueSet.
func (v *ValueSet) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var single interface{}
		if err := value.Decode(&single); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidValueSetValue, err)
		}
		v.values = []interface{}{single}
	case yaml.MappingNode:
		var m map[string]interface{}
		if err := value.Decode(&m); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidValueSetValue, err)
		}
		v.values = []interface{}{m}
	case yaml.SequenceNode:
		var list []interface{}
		if err := value.Decode(&list); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidValueSetValue, err)
		}
		v.values = list
	default:
		return fmt.Errorf("%w: must be a value or list of values", ErrInvalidValueSetValue)
	}
	return nil
}

// MarshalYAML implements custom YAML marshaling for ValueSet.
func (v ValueSet) MarshalYAML() (interface{}, error) {
	if len(v.values) == 1 {
		return v.values[0], nil
	}
	return v.values, nil
}
