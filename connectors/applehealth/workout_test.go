// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkoutRunning(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "workout_running.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Workouts, 1)

	w := result.Workouts[0]
	assert.Equal(t, "running", w.ActivityType)
	assert.Equal(t, "user-123", w.UserID)
	assert.Equal(t, "apple_health", w.Provider)

	require.NotNil(t, w.DurationS)
	assert.Equal(t, 2700, *w.DurationS)

	require.NotNil(t, w.ActiveEnergyKcal)
	assert.Equal(t, 350.5, *w.ActiveEnergyKcal)

	require.NotNil(t, w.DistanceM)
	assert.Equal(t, 5234.0, *w.DistanceM)

	require.NotNil(t, w.AvgHeartRate)
	assert.Equal(t, 155.0, *w.AvgHeartRate)

	require.NotNil(t, w.MaxHeartRate)
	assert.Equal(t, 178.0, *w.MaxHeartRate)

	assert.NotEmpty(t, w.ContentHash)
	assert.Equal(t, "workouts/UUID-HERE/route", w.Metadata["routePath"])
}

func TestWorkoutUnknownActivityType(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":           "2025-03-01T07:00:00Z",
		"endDate":             "2025-03-01T07:45:00Z",
		"workoutActivityType": float64(9999),
		"duration":            2700.0,
		"sourceName":          "Apple Watch",
		"metadata":            map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Workouts, 1)

	assert.Equal(t, "other", result.Workouts[0].ActivityType)
}

func TestWorkoutMissingOptionalFields(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":           "2025-03-01T07:00:00Z",
		"endDate":             "2025-03-01T07:45:00Z",
		"workoutActivityType": float64(37),
		"sourceName":          "Apple Watch",
		"metadata":            map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Workouts, 1)

	w := result.Workouts[0]
	assert.Equal(t, "running", w.ActivityType)
	assert.Nil(t, w.DurationS, "missing duration should be nil")
	assert.Nil(t, w.ActiveEnergyKcal, "missing energy should be nil")
	assert.Nil(t, w.DistanceM, "missing distance should be nil")
	assert.Nil(t, w.AvgHeartRate, "missing HR should be nil")
	assert.Nil(t, w.MaxHeartRate, "missing max HR should be nil")
}

// TestWorkoutSDKWire pins the wire format the shipping iOS exporter
// (the SDK sample source) actually produces, with synthetic values. It
// carries durationS, not duration: reading only "duration" would leave
// duration_s NULL on every workout.
func TestWorkoutSDKWire(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "workout_sdk.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.Len(t, result.Workouts, 1)

	w := result.Workouts[0]
	assert.Equal(t, "walking", w.ActivityType)

	require.NotNil(t, w.DurationS, "durationS must populate duration_s")
	assert.Equal(t, 1800, *w.DurationS)

	// Speed and elevation also arrive inside the metadata blob ("1.25 m/s").
	// They are promoted to their own columns, which also makes them
	// backfillable from raw objects already stored.
	require.NotNil(t, w.AvgSpeedMps)
	assert.InDelta(t, 1.25, *w.AvgSpeedMps, 1e-9)

	require.NotNil(t, w.ElevationGainM)
	assert.InDelta(t, 10.5, *w.ElevationGainM, 1e-9)
}

// TestWorkoutSDKFull covers the wire format once the exporter also
// emits the workout.statistics(for:) values it currently drops on the floor.
func TestWorkoutSDKFull(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "workout_sdk_full.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.Len(t, result.Workouts, 1)

	w := result.Workouts[0]
	require.NotNil(t, w.DurationS)
	assert.Equal(t, 1800, *w.DurationS)

	require.NotNil(t, w.ActiveEnergyKcal)
	assert.Equal(t, 200.0, *w.ActiveEnergyKcal)

	require.NotNil(t, w.DistanceM)
	assert.Equal(t, 2000.5, *w.DistanceM)

	require.NotNil(t, w.AvgHeartRate)
	assert.Equal(t, 120.0, *w.AvgHeartRate)

	require.NotNil(t, w.MaxHeartRate)
	assert.Equal(t, 150.0, *w.MaxHeartRate)

	// Typed fields from the exporter win over the HK metadata strings.
	require.NotNil(t, w.AvgSpeedMps)
	assert.InDelta(t, 1.25, *w.AvgSpeedMps, 1e-9)

	require.NotNil(t, w.ElevationGainM)
	assert.InDelta(t, 10.5, *w.ElevationGainM, 1e-9)
}

func TestParseHKQuantityString(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"1.628 m/s", 1.628, true},
		{"23.8397 m", 23.8397, true},
		{"0 m/s", 0, true},
		{"12 km/h", 12.0 / 3.6, true},
		{"1.2 mi", 1.2 * 1609.344, true},
		{"120 ft", 120 * 0.3048, true},
		{"", 0, false},
		{"not a number", 0, false},
		{"0", 0, true},
	}
	for _, c := range cases {
		got, ok := parseHKQuantityString(c.in)
		assert.Equal(t, c.ok, ok, "ok mismatch for %q", c.in)
		if c.ok {
			assert.InDelta(t, c.want, got, 1e-9, "value mismatch for %q", c.in)
		}
	}
}

func TestWorkoutContentHash(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "workout_running.json")

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.Workouts[0].ContentHash,
		result2.Workouts[0].ContentHash,
		"same record must produce same hash")
}
