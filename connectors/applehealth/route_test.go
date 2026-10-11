// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutePointAllFields(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "route_points.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.RoutePoints, 1)

	rp := result.RoutePoints[0]
	assert.Equal(t, "user-123", rp.UserID)
	assert.Equal(t, "2025-03-01T14:30:00Z", rp.Timestamp.UTC().Format("2006-01-02T15:04:05Z"))
	assert.Equal(t, 10.0, rp.Latitude)
	assert.Equal(t, 20.0, rp.Longitude)
	require.NotNil(t, rp.AltitudeM)
	assert.Equal(t, 10.5, *rp.AltitudeM)
	require.NotNil(t, rp.SpeedMps)
	assert.Equal(t, 3.2, *rp.SpeedMps)
	require.NotNil(t, rp.CourseDeg)
	assert.Equal(t, 180.5, *rp.CourseDeg)
	assert.True(t, result.HighFrequency, "route is high-frequency")
}

func TestRoutePointMissingOptionalFields(t *testing.T) {
	m := New()
	record := map[string]any{
		"timestamp": "2025-03-01T14:30:00Z",
		"latitude":  10.0,
		"longitude": 20.0,
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.RoutePoints, 1)

	rp := result.RoutePoints[0]
	assert.Equal(t, 10.0, rp.Latitude)
	assert.Equal(t, 20.0, rp.Longitude)
	assert.Nil(t, rp.AltitudeM, "altitude should be nil when missing")
	assert.Nil(t, rp.SpeedMps, "speed should be nil when missing")
	assert.Nil(t, rp.CourseDeg, "course should be nil when missing")
	assert.True(t, result.HighFrequency)
}

func TestRoutePointInvalidTimestamp(t *testing.T) {
	m := New()
	record := map[string]any{
		"timestamp": "not-a-date",
		"latitude":  10.0,
		"longitude": 20.0,
	}

	result, err := m.mapRecord(record, "user-123")
	assert.Error(t, err, "invalid timestamp should error")
	assert.Nil(t, result)
}

// TestRoutePoint_ParentContentHash documents the intentional design decision:
// Apple Health route files contain route points for exactly one workout per raw object
// (the workout record's routePath field points to a distinct raw key per workout).
// No linking field (workoutUUID or equivalent) exists in route point records, so
// ParentContentHash is left empty at the mapper level. The writer's single-parent
// orphan fallback correctly assigns route points to the lone parent workout in the batch.
func TestRoutePoint_ParentContentHash(t *testing.T) {
	m := New()
	record := map[string]any{
		"timestamp": "2025-03-01T14:30:00Z",
		"latitude":  10.0,
		"longitude": 20.0,
	}
	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.Len(t, result.RoutePoints, 1)
	// Route mapper intentionally leaves ParentContentHash empty.
	// Writer orphan fallback handles single-parent-per-file Apple Health exports.
	assert.Equal(t, "", result.RoutePoints[0].ParentContentHash,
		"route mapper intentionally leaves ParentContentHash empty; writer orphan fallback handles single-parent files")
}
