// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateOfMindValidRecord(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T09:00:00Z",
		"endDate":       "2025-03-01T09:00:00Z",
		"uuid":          "stateofmind-uuid-001",
		"sourceName":    "Health",
		"quantity_type": "HKStateOfMindType",
		"_raw": map[string]any{
			"kind":    float64(1),
			"valence": float64(0.75),
			"labels":  []any{float64(1), float64(3)},
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.CategorySamples, 1)

	s := result.CategorySamples[0]
	assert.Equal(t, "state_of_mind", s.CategoryType)
	assert.Equal(t, 1, s.ValueInt, "kind=1 (momentaryEmotion) should be value_int=1")
	assert.Equal(t, "user-123", s.UserID)
	assert.Equal(t, "apple_health", s.Provider)
	assert.Equal(t, "Health", s.SourceName)
	assert.Equal(t, "stateofmind-uuid-001", s.ExternalID)
}

func TestStateOfMindValenceInMetadata(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T09:00:00Z",
		"endDate":       "2025-03-01T09:00:00Z",
		"uuid":          "stateofmind-uuid-002",
		"quantity_type": "HKStateOfMindType",
		"_raw": map[string]any{
			"kind":    float64(1),
			"valence": float64(0.75),
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	s := result.CategorySamples[0]
	require.NotNil(t, s.Metadata)
	valence, ok := s.Metadata["valence"].(string)
	require.True(t, ok, "valence should be a string in metadata")
	assert.Equal(t, "0.7500", valence, "valence should be formatted to 4 decimal places")
}

func TestStateOfMindLabelsAndAssociationsInMetadata(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T09:00:00Z",
		"endDate":       "2025-03-01T09:00:00Z",
		"uuid":          "stateofmind-uuid-003",
		"quantity_type": "HKStateOfMindType",
		"_raw": map[string]any{
			"kind":         float64(2),
			"valence":      float64(-0.25),
			"labels":       []any{float64(2), float64(5), float64(8)},
			"associations": []any{float64(1), float64(4)},
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	s := result.CategorySamples[0]
	require.NotNil(t, s.Metadata)

	// Verify labels are JSON-encoded
	labelsStr, ok := s.Metadata["labels"].(string)
	require.True(t, ok, "labels should be JSON string in metadata")
	var labels []any
	require.NoError(t, json.Unmarshal([]byte(labelsStr), &labels))
	assert.Len(t, labels, 3)

	// Verify associations are JSON-encoded
	assocStr, ok := s.Metadata["associations"].(string)
	require.True(t, ok, "associations should be JSON string in metadata")
	var assoc []any
	require.NoError(t, json.Unmarshal([]byte(assocStr), &assoc))
	assert.Len(t, assoc, 2)
}

func TestStateOfMindContentHashDeterministic(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T09:00:00Z",
		"endDate":       "2025-03-01T09:00:00Z",
		"uuid":          "stateofmind-uuid-004",
		"quantity_type": "HKStateOfMindType",
		"_raw": map[string]any{
			"kind":    float64(1),
			"valence": float64(0.5),
		},
	}

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.CategorySamples[0].ContentHash,
		result2.CategorySamples[0].ContentHash,
		"same input must produce same content hash")
	assert.NotEmpty(t, result1.CategorySamples[0].ContentHash)
}

func TestStateOfMindMissingDates(t *testing.T) {
	m := New()
	record := map[string]any{
		"uuid":          "stateofmind-no-dates",
		"quantity_type": "HKStateOfMindType",
		"_raw": map[string]any{
			"kind":    float64(1),
			"valence": float64(0.0),
		},
	}

	result, err := m.mapRecord(record, "user-123")
	assert.Error(t, err, "missing startDate should return error")
	assert.Nil(t, result)
}

func TestStateOfMindDailyMood(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T09:00:00Z",
		"endDate":       "2025-03-01T09:00:00Z",
		"uuid":          "stateofmind-uuid-005",
		"quantity_type": "HKStateOfMindType",
		"_raw": map[string]any{
			"kind":    float64(2),
			"valence": float64(-0.5),
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	s := result.CategorySamples[0]
	assert.Equal(t, 2, s.ValueInt, "kind=2 (dailyMood) should be value_int=2")
	assert.Equal(t, "-0.5000", s.Metadata["valence"])
}
