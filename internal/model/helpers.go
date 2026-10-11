// SPDX-License-Identifier: AGPL-3.0-only

package model

import (
	"fmt"
	"strings"
	"time"
)

// Canonical sleep-stage vocabulary for the sleep phase column. Exactly four
// lowercase values are ever stored, so a phase carries one key regardless of the
// provider naming it (Apple's asleep_core, oura's "light", withings numeric, etc.).
const (
	SleepStageDeep  = "deep"
	SleepStageREM   = "rem"
	SleepStageLight = "light"
	SleepStageAwake = "awake"
)

// sleepStageAliases maps provider phase variants to the canonical four-value set.
// Apple Health uses asleep_deep/asleep_rem/asleep_core/asleep_unspecified and
// in_bed; oura/whoop/withings emit their own words. asleep_core maps to "light"
// (Apple "core" is the light/N1-N2 band). Anything in_bed or unclassified counts
// as awake so it never inflates a sleep-stage total. Note: the apple_health
// mapper deliberately does NOT route its in_bed marker through this helper; that
// label stays verbatim because sleep session boundary logic keys on it.
var sleepStageAliases = map[string]string{
	"deep":               SleepStageDeep,
	"asleep_deep":        SleepStageDeep,
	"deep_sleep":         SleepStageDeep,
	"rem":                SleepStageREM,
	"asleep_rem":         SleepStageREM,
	"rem_sleep":          SleepStageREM,
	"light":              SleepStageLight,
	"core":               SleepStageLight,
	"asleep_core":        SleepStageLight,
	"asleep_unspecified": SleepStageLight,
	"asleep":             SleepStageLight,
	"light_sleep":        SleepStageLight,
	"awake":              SleepStageAwake,
	"in_bed":             SleepStageAwake,
	"inbed":              SleepStageAwake,
	"wake":               SleepStageAwake,
}

// CanonicalSleepStage normalizes a provider-specific sleep phase label into one of
// the canonical values {deep, rem, light, awake}. Matching is case-insensitive and
// tolerates surrounding whitespace. Unknown labels fall back to "awake" (the safe
// choice: an unrecognized band is never counted as scored sleep). Provider mapper
// streams call this instead of hand-mapping stage strings.
func CanonicalSleepStage(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	if c, ok := sleepStageAliases[key]; ok {
		return c
	}
	return SleepStageAwake
}

// ParseNormalizedDates parses start_date and end_date from a normalized record JSON map.
// Dates are RFC3339 strings written by the collectors. Handles both RFC3339Nano and RFC3339.
func ParseNormalizedDates(record map[string]any) (time.Time, time.Time, error) {
	startStr, ok := record["start_date"].(string)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("missing or invalid start_date field")
	}
	startDate, err := time.Parse(time.RFC3339Nano, startStr)
	if err != nil {
		// Try RFC3339 without nano
		startDate, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse start_date %q: %w", startStr, err)
		}
	}

	endStr, ok := record["end_date"].(string)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("missing or invalid end_date field")
	}
	endDate, err := time.Parse(time.RFC3339Nano, endStr)
	if err != nil {
		endDate, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse end_date %q: %w", endStr, err)
		}
	}

	return startDate.UTC(), endDate.UTC(), nil
}
