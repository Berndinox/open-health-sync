// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"fmt"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// mapRoute handles GPS route point records from workout routes.
// Each record contains a timestamp, latitude, longitude, and optional
// altitude, speed, and course fields. Route points are high-frequency
// (one per GPS sample, typically 1Hz).
func (m *mapper) mapRoute(record map[string]any, userID string) (*model.MapResult, error) {
	tsStr, ok := record["timestamp"].(string)
	if !ok {
		return nil, fmt.Errorf("route: missing timestamp field")
	}

	ts, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		ts, err = time.Parse(time.RFC3339Nano, tsStr)
		if err != nil {
			return nil, fmt.Errorf("route: invalid timestamp %q: %w", tsStr, err)
		}
	}
	ts = ts.UTC()

	lat, err := extractFloat(record, "latitude")
	if err != nil {
		return nil, fmt.Errorf("route: %w", err)
	}

	lng, err := extractFloat(record, "longitude")
	if err != nil {
		return nil, fmt.Errorf("route: %w", err)
	}

	rp := model.RoutePoint{
		UserID:    userID,
		Timestamp: ts,
		Latitude:  lat,
		Longitude: lng,
		// ParentContentHash is intentionally left empty (zero value).
		// Apple Health route files contain route points for exactly one workout per raw object
		// (the workout record's routePath field points to a distinct raw key per workout).
		// The writer's single-parent orphan fallback assigns these route points to the lone
		// parent workout in the batch, which is the correct and sufficient behavior.
		// Multi-parent batches (multiple workouts in one file) are not produced by the
		// Apple Health SDK; route points with empty ParentContentHash are
		// safely skipped in that rare edge case.
	}

	// Optional fields
	if alt, err := extractFloat(record, "altitude"); err == nil {
		rp.AltitudeM = &alt
	}
	if speed, err := extractFloat(record, "speed"); err == nil {
		rp.SpeedMps = &speed
	}
	if course, err := extractFloat(record, "course"); err == nil {
		rp.CourseDeg = &course
	}

	return &model.MapResult{
		RoutePoints:   []model.RoutePoint{rp},
		HighFrequency: true,
	}, nil
}
