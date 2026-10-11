// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSleepAsleepCore(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "category_sleep.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should produce a SleepPhase but NOT a SleepSession (value=3 is not in_bed)
	require.Len(t, result.SleepPhases, 1)
	assert.Empty(t, result.SleepSessions, "asleep_core should not produce session")

	phase := result.SleepPhases[0]
	assert.Equal(t, "light", phase.Phase, "asleep_core canonicalizes to light")
	assert.Equal(t, "user-123", phase.UserID)
	assert.True(t, phase.DurationS > 0, "duration should be positive")
}

func TestSleepInBedProducesSession(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T23:00:00Z",
		"endDate":      "2025-03-02T07:00:00Z",
		"categoryType": "HKCategoryTypeIdentifierSleepAnalysis",
		"value":        float64(0), // in_bed
		"sourceName":   "Apple Watch",
		"metadata": map[string]any{
			"HKTimeZone": "Europe/Paris",
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should produce both SleepPhase AND SleepSession
	require.Len(t, result.SleepPhases, 1)
	require.Len(t, result.SleepSessions, 1)

	phase := result.SleepPhases[0]
	assert.Equal(t, "in_bed", phase.Phase)

	session := result.SleepSessions[0]
	assert.Equal(t, "user-123", session.UserID)
	assert.Equal(t, "apple_health", session.Provider)
	assert.NotEmpty(t, session.ContentHash)
}

func TestSleepAllPhaseValues(t *testing.T) {
	// Every HK stage label is canonicalized to the shared four-value enum at
	// emission. in_bed stays verbatim (session boundary logic keys on it) and
	// unknown values fall back to awake per the helper's unknown policy.
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"in_bed", 0, "in_bed"},
		{"asleep_unspecified", 1, "light"},
		{"awake", 2, "awake"},
		{"asleep_core", 3, "light"},
		{"asleep_deep", 4, "deep"},
		{"asleep_rem", 5, "rem"},
		{"unknown_value", 42, "awake"},
	}

	m := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := map[string]any{
				"startDate":    "2025-03-01T23:00:00Z",
				"endDate":      "2025-03-02T00:30:00Z",
				"categoryType": "HKCategoryTypeIdentifierSleepAnalysis",
				"value":        tt.value,
				"sourceName":   "Apple Watch",
				"metadata":     map[string]any{},
			}

			result, err := m.mapRecord(record, "user-123")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result.SleepPhases, 1)
			assert.Equal(t, tt.expected, result.SleepPhases[0].Phase)
		})
	}
}

func TestSleepSessionContentHash(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T23:00:00Z",
		"endDate":      "2025-03-02T07:00:00Z",
		"categoryType": "HKCategoryTypeIdentifierSleepAnalysis",
		"value":        float64(0), // in_bed
		"sourceName":   "Apple Watch",
		"metadata":     map[string]any{},
	}

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.SleepSessions[0].ContentHash,
		result2.SleepSessions[0].ContentHash,
		"same record must produce same session hash")
}

func TestSleepInBed_ParentContentHash(t *testing.T) {
	// For in_bed records (value 0), the SleepPhase must carry ParentContentHash
	// matching the sibling SleepSession's ContentHash.
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T23:00:00Z",
		"endDate":      "2025-03-02T07:00:00Z",
		"categoryType": "HKCategoryTypeIdentifierSleepAnalysis",
		"value":        float64(0), // in_bed
		"sourceName":   "Apple Watch",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.Len(t, result.SleepSessions, 1)
	require.Len(t, result.SleepPhases, 1)

	session := result.SleepSessions[0]
	phase := result.SleepPhases[0]
	assert.NotEmpty(t, session.ContentHash)
	assert.Equal(t, session.ContentHash, phase.ParentContentHash,
		"in_bed phase ParentContentHash must match session ContentHash")
}

func TestSleepNonInBed_ParentContentHashEmpty(t *testing.T) {
	// For non-in_bed records, no parent session exists, so ParentContentHash is empty.
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T23:30:00Z",
		"endDate":      "2025-03-02T01:15:00Z",
		"categoryType": "HKCategoryTypeIdentifierSleepAnalysis",
		"value":        float64(3), // asleep_core (not in_bed)
		"sourceName":   "Apple Watch",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.Len(t, result.SleepPhases, 1)
	assert.Empty(t, result.SleepSessions, "non-in_bed should not produce session")
	assert.Empty(t, result.SleepPhases[0].ParentContentHash,
		"non-in_bed phase should have empty ParentContentHash")
}

func TestSleepPhaseDuration(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T23:30:00Z",
		"endDate":      "2025-03-02T01:15:00Z",
		"categoryType": "HKCategoryTypeIdentifierSleepAnalysis",
		"value":        float64(3), // asleep_core
		"sourceName":   "Apple Watch",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	// 1h45m = 6300 seconds
	assert.Equal(t, 6300, result.SleepPhases[0].DurationS)
}
