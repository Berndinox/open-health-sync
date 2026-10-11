// SPDX-License-Identifier: AGPL-3.0-only

package model

import (
	"regexp"
	"testing"
)

// snakeCaseRe matches a canonical quantity_type: starts lowercase, then only
// lowercase letters, digits, and underscores. Any uppercase letter (camelCase
// leak) fails the match.
var snakeCaseRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// normalizedScalarQuantityTypes is the full set of scalar quantity_type values the
// collectors emit in camelCase.
// Every one must canonicalize to a snake_case key, so a given metric is stored
// under one consistent vocabulary regardless of which provider produced it.
var normalizedScalarQuantityTypes = []string{
	"stepCount", "activeEnergyBurned", "basalEnergyBurned", "distanceWalkingRunning",
	"distanceCycling", "distanceSwimming", "flightsClimbed", "appleExerciseTime",
	"appleMoveTime", "appleStandTime", "heartRate", "restingHeartRate",
	"heartRateVariabilitySDNN", "vo2Max", "oxygenSaturation", "respiratoryRate",
	"bodyMass", "bodyFatPercentage", "fatMass", "leanBodyMass", "muscleMass",
	"boneMass", "bodyWaterMass", "bodyTemperature", "bloodPressureSystolic",
	"bloodPressureDiastolic", "basalMetabolicRate", "visceralFat", "vascularAge",
	"pulseWaveVelocity", "walkingSpeed", "runningSpeed", "runningPower", "cyclingPower",
	"readinessScore", "activityScore", "recoveryScore",
	"daily_sleep", "daily_stress", "daily_spo2", "cardiovascular_age", "resilience", "vo2_max",
}

// No collector quantity_type may canonicalize to a camelCase value: that would
// store data under a key the API (snake_case) never queries.
func TestCanonicalQuantityType_AlwaysSnakeCase(t *testing.T) {
	for _, qt := range normalizedScalarQuantityTypes {
		got := CanonicalQuantityType(qt)
		if !snakeCaseRe.MatchString(got) {
			t.Errorf("CanonicalQuantityType(%q) = %q: not snake_case (camelCase leak)", qt, got)
		}
	}
}

// Metrics shared with Apple Health must canonicalize to the SAME key Apple uses
// (see apple_health/types.go), so cross-provider data is queryable uniformly.
func TestCanonicalQuantityType_MatchesAppleVocabulary(t *testing.T) {
	want := map[string]string{
		"heartRate":                "heart_rate",
		"restingHeartRate":         "resting_heart_rate",
		"heartRateVariabilitySDNN": "hrv_sdnn",
		"oxygenSaturation":         "oxygen_saturation",
		"respiratoryRate":          "respiratory_rate",
		"bodyMass":                 "body_mass",
		"bodyFatPercentage":        "body_fat_percentage",
		"stepCount":                "step_count",
		"activeEnergyBurned":       "active_energy_burned",
		"bloodPressureSystolic":    "blood_pressure_systolic",
		"bloodPressureDiastolic":   "blood_pressure_diastolic",
		"appleExerciseTime":        "apple_exercise_time",
		"vo2Max":                   "vo2_max",
		"vo2_max":                  "vo2_max",
		// Acronym/renamed overrides that a naive converter would get wrong.
		"fatMass":       "body_fat_mass",
		"bodyWaterMass": "water_mass",
		"visceralFat":   "visceral_fat_rating",
	}
	for in, exp := range want {
		if got := CanonicalQuantityType(in); got != exp {
			t.Errorf("CanonicalQuantityType(%q) = %q, want %q", in, got, exp)
		}
	}
}
