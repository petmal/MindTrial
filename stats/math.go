// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package stats

import (
	"math"
	"sort"
	"time"
)

// median returns the statistical median of values, or nil when there are fewer than two
// samples (matching the HTML report's dynamic summary, which considers a single sample
// insufficient to be meaningful).
func median(values []float64) *float64 {
	if len(values) < 2 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	n := len(sorted)
	mid := n / 2
	var result float64
	if n%2 != 0 {
		result = sorted[mid]
	} else {
		result = (sorted[mid-1] + sorted[mid]) / 2
	}
	return &result
}

// stddev returns the population standard deviation of values (not the sample/n-1 variant),
// or nil when there are fewer than two samples. Matches the HTML report's dynamic summary.
func stddev(values []float64) *float64 {
	if len(values) < 2 {
		return nil
	}
	n := float64(len(values))

	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / n

	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= n

	result := math.Sqrt(variance)
	return &result
}

// medianDuration/stddevDuration are convenience wrappers around median/stddev for
// nanosecond-valued samples, returning a *time.Duration instead of a raw *float64.
func medianDuration(nanoseconds []float64) *time.Duration {
	if v := median(nanoseconds); v != nil {
		d := time.Duration(*v)
		return &d
	}
	return nil
}

func stddevDuration(nanoseconds []float64) *time.Duration {
	if v := stddev(nanoseconds); v != nil {
		d := time.Duration(*v)
		return &d
	}
	return nil
}
