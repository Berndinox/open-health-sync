// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECGRecordingMetadata(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "ecg_recording.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.ECGRecordings, 1)

	ecg := result.ECGRecordings[0]
	assert.Equal(t, "user-123", ecg.UserID)
	assert.Equal(t, 1, ecg.Classification)
	assert.Equal(t, 0, ecg.SymptomsStatus)
	assert.Equal(t, 512.0, ecg.SamplingFrequency)
	assert.Equal(t, 15000, ecg.NumberOfMeasurements)
	assert.NotNil(t, ecg.AverageHeartRate)
	assert.Equal(t, 72.0, *ecg.AverageHeartRate)
	assert.Equal(t, "user-123/Apple Health/ECG/ecg_2025-03-01.json", ecg.VoltagePath)
	assert.NotEmpty(t, ecg.ContentHash)
	assert.False(t, result.HighFrequency, "ECG metadata is NOT high-frequency")
}

func TestECGRecordingTimestamps(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "ecg_recording.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	ecg := result.ECGRecordings[0]
	assert.Equal(t, "2025-03-01T14:30:00Z", ecg.StartDate.UTC().Format("2006-01-02T15:04:05Z"))
	assert.Equal(t, "2025-03-01T14:30:30Z", ecg.EndDate.UTC().Format("2006-01-02T15:04:05Z"))
}

func TestECGRecordingContentHashDeterminism(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "ecg_recording.json")

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.ECGRecordings[0].ContentHash,
		result2.ECGRecordings[0].ContentHash,
		"same ECG record must produce same content hash")
}

func TestECGVoltageSample(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "ecg_voltage.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.ECGSamples, 1)

	sample := result.ECGSamples[0]
	assert.Equal(t, "user-123", sample.UserID)
	assert.Equal(t, "", sample.ECGID, "ECGID empty at mapper level (set by pipeline)")
	assert.Equal(t, 0.001953125, sample.TimeSinceSampleStart)
	assert.Equal(t, 45.0, sample.VoltageUV, "volts -> microvolts: 0.000045 * 1_000_000 = 45")
	assert.True(t, result.HighFrequency, "ECG voltage is high-frequency")
}

func TestECGMissingClassification(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":                   "2025-03-01T14:30:00Z",
		"endDate":                     "2025-03-01T14:30:30Z",
		"symptomsStatus":              0,
		"numberOfVoltageMeasurements": 15000,
		"samplingFrequency":           512.0,
		"sourceName":                  "Apple Watch",
	}

	// Record without classification should not match ECG handler
	// (classification is the discriminator for ECG metadata records)
	result, err := m.mapRecord(record, "user-123")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestECGVoltageMissingValue(t *testing.T) {
	m := New()
	record := map[string]any{
		"timeSinceSampleStart": 0.001953125,
		// missing "value" field -- record won't match voltage dispatch
		// (requires both timeSinceSampleStart AND value)
	}

	result, err := m.mapRecord(record, "user-123")
	assert.NoError(t, err, "incomplete voltage record is unrecognized, returns nil")
	assert.Nil(t, result)
}

// TestECGVoltage_ParentContentHash documents the intentional design decision:
// Apple Health ECG voltage files contain samples for exactly one ECG recording per raw object
// (the ECG metadata record's voltagePath field points to a distinct raw key per recording).
// No linking field (ecgUUID or equivalent) exists in ECG voltage sample records, so
// ParentContentHash is left empty at the mapper level. The writer's single-parent
// orphan fallback correctly assigns samples to the lone parent ECG recording in the batch.
func TestECGVoltage_ParentContentHash(t *testing.T) {
	m := New()
	record := map[string]any{
		"timeSinceSampleStart": 0.001953125,
		"value":                0.000045,
	}
	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.Len(t, result.ECGSamples, 1)
	// ECG voltage mapper intentionally leaves ParentContentHash empty.
	// Writer orphan fallback handles single-parent-per-file Apple Health exports.
	assert.Equal(t, "", result.ECGSamples[0].ParentContentHash,
		"ECG voltage mapper intentionally leaves ParentContentHash empty; writer orphan fallback handles single-parent files")
}
