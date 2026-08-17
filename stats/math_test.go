// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petmal/mindtrial/pkg/testutils"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   *float64
	}{
		{name: "empty", values: nil, want: nil},
		{name: "single sample", values: []float64{42}, want: nil},
		{name: "odd count", values: []float64{1, 3, 2}, want: testutils.Ptr(2.0)},
		{name: "even count", values: []float64{1, 4, 2, 3}, want: testutils.Ptr(2.5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := median(tt.values)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.InDelta(t, *tt.want, *got, 0.0001)
			}
		})
	}
}

func TestStddev(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   *float64
	}{
		{name: "empty", values: nil, want: nil},
		{name: "single sample", values: []float64{42}, want: nil},
		{name: "known population stddev", values: []float64{2, 4, 4, 4, 5, 5, 7, 9}, want: testutils.Ptr(2.0)},
		{name: "repeated equal values", values: []float64{5, 5, 5}, want: testutils.Ptr(0.0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stddev(tt.values)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.InDelta(t, *tt.want, *got, 0.0001)
			}
		})
	}
}

func TestMedianDurationAndStddevDuration(t *testing.T) {
	nsValues := []float64{
		float64(time.Second.Nanoseconds()), float64((2 * time.Second).Nanoseconds()), float64((3 * time.Second).Nanoseconds()), float64((4 * time.Second).Nanoseconds()),
	}

	median := medianDuration(nsValues)
	require.NotNil(t, median)
	assert.Equal(t, 2500*time.Millisecond, *median)

	assert.Nil(t, medianDuration(nil))
	assert.Nil(t, stddevDuration(nil))

	sd := stddevDuration(nsValues)
	require.NotNil(t, sd)
	assert.Greater(t, *sd, time.Duration(0))
}
