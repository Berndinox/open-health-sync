// SPDX-License-Identifier: AGPL-3.0-only

package model

import "testing"

// BenchmarkCanonicalQuantityType measures the per-value canonicalization cost.
// Every quantity_type passes through this on the ETL hot path. The mix covers
// the three branches: already-snake (pass-through), camelCase conversion, and
// the override-map lookup.
func BenchmarkCanonicalQuantityType(b *testing.B) {
	inputs := []string{
		"heart_rate",               // already snake_case (fast path)
		"heartRate",                // camelCase -> snake_case
		"heartRateVariabilitySDNN", // override-map lookup (acronym)
		"fatMass",                  // override-map lookup
		"distanceWalkingRunning",   // multi-word camelCase
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = CanonicalQuantityType(inputs[i%len(inputs)])
	}
}
