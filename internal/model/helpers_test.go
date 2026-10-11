// SPDX-License-Identifier: AGPL-3.0-only

package model

import "testing"

// TestCanonicalSleepStage verifies every provider phase variant collapses to the
// canonical four-value vocabulary {deep, rem, light, awake}, including the
// asleep_core -> light mapping and the awake fallback for unknown labels.
func TestCanonicalSleepStage(t *testing.T) {
	cases := map[string]string{
		// Already canonical.
		"deep":  SleepStageDeep,
		"rem":   SleepStageREM,
		"light": SleepStageLight,
		"awake": SleepStageAwake,
		// Apple Health variants.
		"asleep_deep":        SleepStageDeep,
		"asleep_rem":         SleepStageREM,
		"asleep_core":        SleepStageLight,
		"asleep_unspecified": SleepStageLight,
		"in_bed":             SleepStageAwake,
		// Case / whitespace tolerance.
		"ASLEEP_CORE": SleepStageLight,
		"  Deep  ":    SleepStageDeep,
		"REM":         SleepStageREM,
		// Other provider spellings.
		"core":   SleepStageLight,
		"asleep": SleepStageLight,
		"wake":   SleepStageAwake,
		"inbed":  SleepStageAwake,
		// Unknown -> awake fallback (never counts as scored sleep).
		"gibberish": SleepStageAwake,
		"":          SleepStageAwake,
	}
	for in, want := range cases {
		if got := CanonicalSleepStage(in); got != want {
			t.Errorf("CanonicalSleepStage(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCanonicalSleepStageClosedSet guarantees the helper only ever emits one of the
// four allowed values, so the sleep phase column can never carry a stray label.
func TestCanonicalSleepStageClosedSet(t *testing.T) {
	allowed := map[string]bool{
		SleepStageDeep:  true,
		SleepStageREM:   true,
		SleepStageLight: true,
		SleepStageAwake: true,
	}
	inputs := []string{
		"deep", "rem", "light", "awake", "asleep_deep", "asleep_rem",
		"asleep_core", "asleep_unspecified", "in_bed", "core", "asleep",
		"wake", "inbed", "unknown_value", "N3", "",
	}
	for _, in := range inputs {
		if got := CanonicalSleepStage(in); !allowed[got] {
			t.Errorf("CanonicalSleepStage(%q) = %q: outside canonical set", in, got)
		}
	}
}
