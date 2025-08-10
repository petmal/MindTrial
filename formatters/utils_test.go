// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/petmal/mindtrial/pkg/utils"
	"github.com/petmal/mindtrial/runners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToStatus(t *testing.T) {
	tests := []struct {
		name string
		kind runners.ResultKind
		want string
	}{
		{
			name: "Success",
			kind: runners.Success,
			want: Passed,
		},
		{
			name: "Failure",
			kind: runners.Failure,
			want: Failed,
		},
		{
			name: "Error",
			kind: runners.Error,
			want: Error,
		},
		{
			name: "NotSupported",
			kind: runners.NotSupported,
			want: Skipped,
		},
		{
			name: "Unknown",
			kind: runners.ResultKind(999),
			want: "Unknown (999)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ToStatus(tt.kind))
		})
	}
}

func TestCountByKind(t *testing.T) {
	tests := []struct {
		name          string
		resultsByKind map[runners.ResultKind][]runners.RunResult
		kind          runners.ResultKind
		want          int
	}{
		{
			name: "no results of given kind",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {},
				runners.Failure: {},
			},
			kind: runners.Success,
			want: 0,
		},
		{
			name: "one result of given kind",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}},
				runners.Failure: {},
			},
			kind: runners.Success,
			want: 1,
		},
		{
			name: "multiple results of given kind",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}, {Duration: time.Minute}},
				runners.Failure: {},
			},
			kind: runners.Success,
			want: 2,
		},
		{
			name: "results of different kinds",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}, {Duration: time.Hour}},
				runners.Failure: {{Duration: time.Minute}},
			},
			kind: runners.Failure,
			want: 1,
		},
		{
			name: "kind not present in map",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}},
			},
			kind: runners.Failure,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CountByKind(tt.resultsByKind, tt.kind))
		})
	}
}
func TestTotalDuration(t *testing.T) {
	tests := []struct {
		name          string
		resultsByKind map[runners.ResultKind][]runners.RunResult
		include       []runners.ResultKind
		want          time.Duration
	}{
		{
			name: "no results",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {},
				runners.Failure: {},
			},
			include: []runners.ResultKind{runners.Success},
			want:    0,
		},
		{
			name: "single result",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}},
				runners.Failure: {},
			},
			include: []runners.ResultKind{runners.Success},
			want:    time.Second,
		},
		{
			name: "multiple results of one kind",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}, {Duration: time.Minute}},
				runners.Failure: {},
			},
			include: []runners.ResultKind{runners.Success},
			want:    time.Second + time.Minute,
		},
		{
			name: "multiple results of different kinds",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}, {Duration: time.Minute}},
				runners.Failure: {{Duration: time.Hour}},
			},
			include: []runners.ResultKind{runners.Success, runners.Failure},
			want:    time.Second + time.Minute + time.Hour,
		},
		{
			name: "kind not present in map",
			resultsByKind: map[runners.ResultKind][]runners.RunResult{
				runners.Success: {{Duration: time.Second}},
			},
			include: []runners.ResultKind{runners.Failure},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, TotalDuration(tt.resultsByKind, tt.include...))
		})
	}
}

func TestFormatAnswer(t *testing.T) {
	tests := []struct {
		name    string
		result  runners.RunResult
		useHTML bool
		want    []string
	}{
		{
			name: "success result without HTML",
			result: runners.RunResult{
				Kind: runners.Success,
				Got:  "Success output",
			},
			useHTML: false,
			want:    []string{"Success output"},
		},
		{
			name: "error result without HTML",
			result: runners.RunResult{
				Kind: runners.Error,
				Got:  "Error output",
			},
			useHTML: false,
			want:    []string{"Error output"},
		},
		{
			name: "failure result with HTML",
			result: runners.RunResult{
				Kind: runners.Failure,
				Want: utils.NewStringSet("Expected output"),
				Got:  "Actual output",
			},
			useHTML: true,
			want:    []string{htmlDiffContentPrefix + DiffHTML("Expected output", "Actual output")},
		},
		{
			name: "failure result without HTML",
			result: runners.RunResult{
				Kind: runners.Failure,
				Want: utils.NewStringSet("Expected output"),
				Got:  "Actual output",
			},
			useHTML: false,
			want:    []string{DiffText("Expected output", "Actual output")},
		},
		{
			name: "failure result multiple answers with HTML",
			result: runners.RunResult{
				Kind: runners.Failure,
				Want: utils.NewStringSet("Expected output", "Other output"),
				Got:  "Actual output",
			},
			useHTML: true,
			want: []string{
				htmlDiffContentPrefix + DiffHTML("Expected output", "Actual output"),
				htmlDiffContentPrefix + DiffHTML("Other output", "Actual output"),
			},
		},
		{
			name: "failure result multiple answers without HTML",
			result: runners.RunResult{
				Kind: runners.Failure,
				Want: utils.NewStringSet("Expected output", "Other output"),
				Got:  "Actual output",
			},
			useHTML: false,
			want: []string{
				DiffText("Expected output", "Actual output"),
				DiffText("Other output", "Actual output"),
			},
		},
		{
			name: "not supported result without HTML",
			result: runners.RunResult{
				Kind: runners.NotSupported,
				Got:  "Skipped output",
			},
			useHTML: false,
			want:    []string{"Skipped output"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatAnswer(tt.result, tt.useHTML))
		})
	}
}

func TestDiffHTML(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     string
	}{
		{
			name:     "no differences",
			expected: "Quas minima minima rem rerum et quisquam excepturi commodi. Aliquid voluptatibus excepturi placeat non eos dolorem. Veritatis commodi autem enim.",
			actual:   "Quas minima minima rem rerum et quisquam excepturi commodi. Aliquid voluptatibus excepturi placeat non eos dolorem. Veritatis commodi autem enim.",
			want:     `<span>Quas minima minima rem rerum et quisquam excepturi commodi. Aliquid voluptatibus excepturi placeat non eos dolorem. Veritatis commodi autem enim.</span>`,
		},
		{
			name:     "with differences",
			expected: "Est maxime dolor numquam enim ut a. Expedita cumque facere inventore impedit molestias iste veritatis maiores. Sit et a nulla deleniti laborum at ipsa.",
			actual:   "Est maxime dolor numquam enim ut a. Excepturi delectus ut qui non nemo rerum delectus necessitatibus numquam. Sit et a nulla deleniti laborum at ipsa.",
			want:     `<span>Est maxime dolor numquam enim ut a. Ex</span><del style="background:#ffe6e6;">pedita cumque facere inventore impedit molestias iste veritatis maiores</del><ins style="background:#e6ffe6;">cepturi delectus ut qui non nemo rerum delectus necessitatibus numquam</ins><span>. Sit et a nulla deleniti laborum at ipsa.</span>`,
		},
		{
			name:     "empty expected",
			expected: "",
			actual:   "actual text",
			want:     `<ins style="background:#e6ffe6;">actual text</ins>`,
		},
		{
			name:     "empty actual",
			expected: "expected text",
			actual:   "",
			want:     `<del style="background:#ffe6e6;">expected text</del>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DiffHTML(tt.expected, tt.actual))
		})
	}
}

func TestDiffText(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     string
	}{
		{
			name:     "no differences",
			expected: "Quasi ut dolores possimus maiores doloremque quia. Quaerat excepturi architecto qui molestiae fugiat enim enim eveniet consequuntur. Excepturi ullam fugit quo.",
			actual:   "Quasi ut dolores possimus maiores doloremque quia. Quaerat excepturi architecto qui molestiae fugiat enim enim eveniet consequuntur. Excepturi ullam fugit quo.",
			want:     "Quasi ut dolores possimus maiores doloremque quia. Quaerat excepturi architecto qui molestiae fugiat enim enim eveniet consequuntur. Excepturi ullam fugit quo.",
		},
		{
			name:     "with differences",
			expected: "Ea ut quisquam iure aut molestiae. Mollitia saepe magnam nihil. Quisquam beatae autem.",
			actual:   "Ea ut quisquam iure aut molestiae. Nulla ut molestiae. Quisquam beatae autem.",
			want:     "@@ -32,35 +32,26 @@\n ae. \n-Mollitia saepe magnam nihil\n+Nulla ut molestiae\n . Qu\n",
		},
		{
			name:     "empty expected",
			expected: "",
			actual:   "actual text",
			want:     "@@ -0,0 +1,11 @@\n+actual text\n",
		},
		{
			name:     "empty actual",
			expected: "expected text",
			actual:   "",
			want:     "@@ -1,13 +0,0 @@\n-expected text\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DiffText(tt.expected, tt.actual))
		})
	}
}

func TestForEachOrdered(t *testing.T) {
	actualValuesByTestName := newValuesByName(make(map[string][]string))
	tests := []struct {
		name    string
		input   map[int]string
		fn      func(key int, value string) error
		want    []string
		wantErr bool
	}{
		{
			name: "no error",
			input: map[int]string{
				2: "two",
				1: "one",
				3: "three",
			},
			fn: func(_ int, value string) error {
				actualValuesByTestName.Add("no error", value)
				return nil
			},
			want:    []string{"one", "two", "three"},
			wantErr: false,
		},
		{
			name: "error on key 2",
			input: map[int]string{
				2: "two",
				1: "one",
				3: "three",
			},
			fn: func(key int, value string) error {
				actualValuesByTestName.Add("error on key 2", value)
				if key == 2 {
					return errors.ErrUnsupported
				}
				return nil
			},
			wantErr: true,
		},
		{
			name:  "empty map",
			input: map[int]string{},
			fn: func(_ int, value string) error {
				actualValuesByTestName.Add("empty map", value)
				return nil
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ForEachOrdered(tt.input, tt.fn)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, actualValuesByTestName.Get(tt.name))
			}
		})
	}
}

func newValuesByName(init map[string][]string) *valuesByName {
	return &valuesByName{m: init}
}

type valuesByName struct {
	sync.Mutex
	m map[string][]string
}

func (o *valuesByName) Add(name string, value string) {
	o.Lock()
	defer o.Unlock()
	o.m[name] = append(o.m[name], value)
}

func (o *valuesByName) Get(name string) []string {
	return o.m[name]
}

func TestSortedKeys(t *testing.T) {
	tests := []struct {
		name string
		m    map[int]interface{}
		want []int
	}{
		{
			name: "empty map",
			m:    map[int]interface{}{},
			want: []int{},
		},
		{
			name: "single element",
			m:    map[int]interface{}{1: nil},
			want: []int{1},
		},
		{
			name: "multiple elements",
			m:    map[int]interface{}{3: nil, 1: nil, 2: nil},
			want: []int{1, 2, 3},
		},
		{
			name: "negative and positive keys",
			m:    map[int]interface{}{-1: nil, 2: nil, -3: nil, 0: nil},
			want: []int{-3, -1, 0, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SortedKeys(tt.m))
		})
	}
}

func TestRoundToMS(t *testing.T) {
	tests := []struct {
		name     string
		value    time.Duration
		expected time.Duration
	}{
		{
			name:     "rounds down to nearest millisecond",
			value:    1234 * time.Microsecond,
			expected: 1 * time.Millisecond,
		},
		{
			name:     "rounds up to nearest millisecond",
			value:    1500 * time.Microsecond,
			expected: 2 * time.Millisecond,
		},
		{
			name:     "exact millisecond value",
			value:    2 * time.Millisecond,
			expected: 2 * time.Millisecond,
		},
		{
			name:     "zero duration",
			value:    0,
			expected: 0,
		},
		{
			name:     "negative duration",
			value:    -1500 * time.Microsecond,
			expected: -2 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RoundToMS(tt.value))
		})
	}
}

func TestTimestamp(t *testing.T) {
	want := time.Now()
	got := Timestamp()

	parsedTime, err := time.Parse(time.RFC1123Z, got)

	require.NoError(t, err)
	assert.WithinDuration(t, want, parsedTime, time.Second)
}

func TestGroupParagraphs(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
	}{
		{
			name:  "empty slice",
			lines: []string{},
			want:  [][]string{},
		},
		{
			name:  "only blank lines",
			lines: []string{"", " ", "\t", ""},
			want:  [][]string{},
		},
		{
			name:  "single line",
			lines: []string{"Line 1"},
			want:  [][]string{{"Line 1"}},
		},
		{
			name:  "multiple lines single paragraph",
			lines: []string{"Line 1", "Line 2", "Line 3"},
			want:  [][]string{{"Line 1", "Line 2", "Line 3"}},
		},
		{
			name:  "two paragraphs separated by blank",
			lines: []string{"Line 1", "Line 2", "", "Line 3"},
			want:  [][]string{{"Line 1", "Line 2"}, {"Line 3"}},
		},
		{
			name:  "leading and trailing blanks trimmed",
			lines: []string{"", "Line 1", "Line 2", ""},
			want:  [][]string{{"Line 1", "Line 2"}},
		},
		{
			name:  "consecutive blank lines collapse",
			lines: []string{"P1L1", "", "", "P2L1"},
			want:  [][]string{{"P1L1"}, {"P2L1"}},
		},
		{
			name:  "whitespace-only lines each end paragraph",
			lines: []string{"P1L1", "   ", "\t", "P2L1", " ", "P2L2"},
			want:  [][]string{{"P1L1"}, {"P2L1"}, {"P2L2"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GroupParagraphs(tt.lines))
		})
	}
}

func TestUniqueRuns(t *testing.T) {
	tests := []struct {
		name  string
		input runners.Results
		want  []string
	}{
		{
			name:  "empty results",
			input: runners.Results{},
			want:  []string{},
		},
		{
			name: "single run single provider",
			input: runners.Results{
				"provA": {{Run: "run1"}},
			},
			want: []string{"run1"},
		},
		{
			name: "duplicate runs different providers",
			input: runners.Results{
				"provA": {{Run: "run1"}, {Run: "run2"}},
				"provB": {{Run: "run2"}, {Run: "run3"}},
			},
			want: []string{"run1", "run2", "run3"},
		},
		{
			name: "already sorted",
			input: runners.Results{
				"provA": {{Run: "a"}, {Run: "b"}},
			},
			want: []string{"a", "b"},
		},
		{
			name: "unsorted with repeats",
			input: runners.Results{
				"provA": {{Run: "z"}, {Run: "a"}},
				"provB": {{Run: "m"}, {Run: "a"}, {Run: "z"}},
			},
			want: []string{"a", "m", "z"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UniqueRuns(tt.input))
		})
	}
}
