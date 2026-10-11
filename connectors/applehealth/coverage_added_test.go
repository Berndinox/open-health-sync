// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMapNewlyCoveredQuantityTypes guards the regression where WalkingSpeed,
// VO2Max, and CyclingPower were absent from quantityTypeCanonical, so mapQuantity
// returned (nil, nil) and the records were silently dropped (no row, no failure).
func TestMapNewlyCoveredQuantityTypes(t *testing.T) {
	m := New()
	cases := []struct {
		hkType    string
		unit      string
		canonical string
	}{
		{"HKQuantityTypeIdentifierWalkingSpeed", "m/s", "walking_speed"},
		{"HKQuantityTypeIdentifierVO2Max", "mL/kg/min", "vo2_max"},
		{"HKQuantityTypeIdentifierCyclingPower", "W", "cycling_power"},
	}
	for _, c := range cases {
		t.Run(c.canonical, func(t *testing.T) {
			rec := map[string]any{
				"quantityType": c.hkType,
				"value":        1.5,
				"unit":         c.unit,
				"startDate":    "2024-01-01T00:00:00Z",
				"endDate":      "2024-01-01T00:01:00Z",
				"sourceName":   "Apple Watch",
			}
			result, err := m.mapRecord(rec, "user-123")
			require.NoError(t, err)
			require.NotNil(t, result, "%s must map to a record, not be skipped", c.hkType)
			require.Len(t, result.QuantitySamples, 1)
			assert.Equal(t, c.canonical, result.QuantitySamples[0].QuantityType)
			assert.Equal(t, 1.5, result.QuantitySamples[0].Value)
		})
	}
}

// TestMapAudioExposureEventCategory guards coverage of the AudioExposureEvent
// category type, which once had no canonical mapping.
func TestMapAudioExposureEventCategory(t *testing.T) {
	m := New()
	rec := map[string]any{
		"categoryType": "HKCategoryTypeIdentifierAudioExposureEvent",
		"value":        float64(1),
		"startDate":    "2024-01-01T00:00:00Z",
		"endDate":      "2024-01-01T00:00:00Z",
		"sourceName":   "Apple Watch",
	}
	result, err := m.mapRecord(rec, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result, "AudioExposureEvent must map, not be skipped")
	require.Len(t, result.CategorySamples, 1)
	assert.Equal(t, "audio_exposure_event", result.CategorySamples[0].CategoryType)
}
