// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudiogramValidRecord(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T10:00:00Z",
		"endDate":       "2025-03-01T10:30:00Z",
		"uuid":          "audiogram-uuid-001",
		"sourceName":    "Mimi Hearing Test",
		"quantity_type": "HKDataTypeIdentifierAudiogramSample",
		"_raw": map[string]any{
			"sensitivityPoints": []any{
				map[string]any{"frequency": 500.0, "leftEarSensitivity": 10.0, "rightEarSensitivity": 15.0},
				map[string]any{"frequency": 1000.0, "leftEarSensitivity": 12.0, "rightEarSensitivity": 18.0},
				map[string]any{"frequency": 2000.0, "leftEarSensitivity": 20.0, "rightEarSensitivity": 25.0},
			},
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.QuantitySamples, 1)

	s := result.QuantitySamples[0]
	assert.Equal(t, "audiogram", s.QuantityType)
	assert.Equal(t, "user-123", s.UserID)
	assert.Equal(t, "apple_health", s.Provider)
	assert.Equal(t, "Mimi Hearing Test", s.SourceName)
	assert.Equal(t, "audiogram-uuid-001", s.ExternalID)
}

func TestAudiogramValueIsPointCount(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T10:00:00Z",
		"endDate":       "2025-03-01T10:30:00Z",
		"uuid":          "audiogram-uuid-002",
		"quantity_type": "HKDataTypeIdentifierAudiogramSample",
		"_raw": map[string]any{
			"sensitivityPoints": []any{
				map[string]any{"frequency": 500.0},
				map[string]any{"frequency": 1000.0},
				map[string]any{"frequency": 2000.0},
				map[string]any{"frequency": 4000.0},
			},
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	s := result.QuantitySamples[0]
	assert.Equal(t, 4.0, s.Value, "value should equal number of sensitivity points")
	assert.Equal(t, "points", s.Unit)
}

func TestAudiogramSensitivityPointsInMetadata(t *testing.T) {
	m := New()
	pts := []any{
		map[string]any{"frequency": 500.0, "leftEarSensitivity": 10.0, "rightEarSensitivity": 15.0},
		map[string]any{"frequency": 1000.0, "leftEarSensitivity": 12.0, "rightEarSensitivity": 18.0},
	}
	record := map[string]any{
		"startDate":     "2025-03-01T10:00:00Z",
		"endDate":       "2025-03-01T10:30:00Z",
		"uuid":          "audiogram-uuid-003",
		"quantity_type": "HKDataTypeIdentifierAudiogramSample",
		"_raw": map[string]any{
			"sensitivityPoints": pts,
		},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	s := result.QuantitySamples[0]
	require.NotNil(t, s.Metadata)
	spStr, ok := s.Metadata["sensitivity_points"].(string)
	require.True(t, ok, "sensitivity_points should be a string in metadata")

	// Verify it's valid JSON
	var decoded []any
	require.NoError(t, json.Unmarshal([]byte(spStr), &decoded))
	assert.Len(t, decoded, 2)
}

func TestAudiogramContentHashDeterministic(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T10:00:00Z",
		"endDate":       "2025-03-01T10:30:00Z",
		"uuid":          "audiogram-uuid-004",
		"quantity_type": "HKDataTypeIdentifierAudiogramSample",
		"_raw": map[string]any{
			"sensitivityPoints": []any{
				map[string]any{"frequency": 500.0},
			},
		},
	}

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.QuantitySamples[0].ContentHash,
		result2.QuantitySamples[0].ContentHash,
		"same input must produce same content hash")
	assert.NotEmpty(t, result1.QuantitySamples[0].ContentHash)
}

func TestAudiogramMissingDates(t *testing.T) {
	m := New()
	record := map[string]any{
		"uuid":          "audiogram-no-dates",
		"quantity_type": "HKDataTypeIdentifierAudiogramSample",
		"_raw": map[string]any{
			"sensitivityPoints": []any{},
		},
	}

	result, err := m.mapRecord(record, "user-123")
	assert.Error(t, err, "missing startDate should return error")
	assert.Nil(t, result)
}

func TestAudiogramEmptyRaw(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":     "2025-03-01T10:00:00Z",
		"endDate":       "2025-03-01T10:30:00Z",
		"uuid":          "audiogram-uuid-005",
		"quantity_type": "HKDataTypeIdentifierAudiogramSample",
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	s := result.QuantitySamples[0]
	assert.Equal(t, 0.0, s.Value, "no _raw should produce 0 value")
	_, hasSP := s.Metadata["sensitivity_points"]
	assert.False(t, hasSP, "no _raw should not set sensitivity_points in metadata")
}
