// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCategoryMenstrualFlow(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "category_menstrual.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.CategorySamples, 1)

	s := result.CategorySamples[0]
	assert.Equal(t, "menstrual_flow", s.CategoryType)
	assert.Equal(t, 2, s.ValueInt)
	assert.Equal(t, "light", s.ValueLabel, "value 2 should map to 'light'")
	assert.Equal(t, "user-123", s.UserID)
	assert.Equal(t, "apple_health", s.Provider)
	assert.NotEmpty(t, s.ContentHash)
}

func TestCategoryAppleStandHour(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T14:00:00Z",
		"endDate":      "2025-03-01T15:00:00Z",
		"categoryType": "HKCategoryTypeIdentifierAppleStandHour",
		"value":        float64(1),
		"sourceName":   "Apple Watch",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.CategorySamples, 1)

	s := result.CategorySamples[0]
	assert.Equal(t, "apple_stand_hour", s.CategoryType)
	assert.Equal(t, 1, s.ValueInt)
	assert.Equal(t, "stood", s.ValueLabel)
}

func TestCategoryHighHeartRateEvent(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T14:00:00Z",
		"endDate":      "2025-03-01T14:05:00Z",
		"categoryType": "HKCategoryTypeIdentifierHighHeartRateEvent",
		"value":        float64(0),
		"sourceName":   "Apple Watch",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.CategorySamples, 1)

	s := result.CategorySamples[0]
	assert.Equal(t, "high_heart_rate_event", s.CategoryType)
}

func TestCategorySleepNotRoutedHere(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "category_sleep.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Sleep should produce SleepPhases, not CategorySamples
	assert.Empty(t, result.CategorySamples, "sleep should not produce CategorySample")
	assert.NotEmpty(t, result.SleepPhases, "sleep should produce SleepPhase")
}

func TestCategoryUnknownType(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T14:00:00Z",
		"endDate":      "2025-03-01T14:05:00Z",
		"categoryType": "HKCategoryTypeIdentifierFutureType",
		"value":        float64(0),
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	assert.NoError(t, err)
	assert.Nil(t, result, "unknown category type should return nil")
}

func TestCategoryContentHash(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "category_menstrual.json")

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.CategorySamples[0].ContentHash,
		result2.CategorySamples[0].ContentHash,
		"same record must produce same hash")
}
