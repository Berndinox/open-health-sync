// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadTestFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))
	return record
}

func TestQuantityHeartRate(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "quantity_heart_rate.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.QuantitySamples, 1)

	s := result.QuantitySamples[0]
	assert.Equal(t, "heart_rate", s.QuantityType)
	assert.Equal(t, 72.5, s.Value)
	assert.Equal(t, "bpm", s.Unit, "count/min should be normalized to bpm")
	assert.Equal(t, "user-123", s.UserID)
	assert.Equal(t, "apple_health", s.Provider)
	assert.Equal(t, "Apple Watch", s.SourceName)
	assert.NotEmpty(t, s.ContentHash, "content hash must be set")
	assert.NotNil(t, s.Metadata)
	assert.Equal(t, "count/min", s.Metadata["original_unit"])
}

// TestQuantityMissingValueSkipped: a quantity record with no value field carries
// no measurement, so it is skipped (no result, no error) instead of being failed
// into the failure log (regression: user-entered ElectrodermalActivity).
func TestQuantityMissingValueSkipped(t *testing.T) {
	m := New()
	record := map[string]any{
		"quantityType":   "HKQuantityTypeIdentifierElectrodermalActivity",
		"startDate":      "2024-02-18T17:06:00.000000000Z",
		"endDate":        "2024-02-18T17:06:00.000000000Z",
		"sourceName":     "Health",
		"sourceBundleId": "com.apple.Health",
		// intentionally no "value"
	}
	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err, "a value-less quantity record must not error")
	if result != nil {
		assert.Empty(t, result.QuantitySamples, "a value-less record produces no sample")
	}
}

func TestQuantitySteps(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "quantity_steps.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.QuantitySamples, 1)

	s := result.QuantitySamples[0]
	assert.Equal(t, "step_count", s.QuantityType)
	assert.Equal(t, 1234.0, s.Value)
	assert.Equal(t, "count", s.Unit)
}

func TestQuantityBodyMass(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "quantity_body_mass.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.QuantitySamples, 1)

	s := result.QuantitySamples[0]
	assert.Equal(t, "body_mass", s.QuantityType)
	assert.Equal(t, 75.5, s.Value)
	assert.Equal(t, "kg", s.Unit)
}

func TestQuantityBodyMassLbConversion(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T07:00:00Z",
		"endDate":      "2025-03-01T07:00:00Z",
		"quantityType": "HKQuantityTypeIdentifierBodyMass",
		"value":        150.0,
		"unit":         "lb",
		"sourceName":   "Scale",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.QuantitySamples, 1)

	s := result.QuantitySamples[0]
	assert.Equal(t, "body_mass", s.QuantityType)
	assert.Equal(t, "kg", s.Unit)
	assert.True(t, math.Abs(s.Value-68.0388555) < 0.001,
		"expected ~68.04 kg, got %f", s.Value)
	assert.Equal(t, "lb", s.Metadata["original_unit"])
	assert.Equal(t, 150.0, s.Metadata["original_value"])
}

func TestCorrelationBloodPressure(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "correlation_blood_pressure.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.QuantitySamples, 2, "blood pressure should expand to 2 samples")

	// Find systolic and diastolic
	var systolic, diastolic *struct {
		quantityType string
		value        float64
	}
	for _, s := range result.QuantitySamples {
		if s.QuantityType == "blood_pressure_systolic" {
			systolic = &struct {
				quantityType string
				value        float64
			}{s.QuantityType, s.Value}
		}
		if s.QuantityType == "blood_pressure_diastolic" {
			diastolic = &struct {
				quantityType string
				value        float64
			}{s.QuantityType, s.Value}
		}
	}

	require.NotNil(t, systolic, "should have systolic sample")
	assert.Equal(t, 120.0, systolic.value)

	require.NotNil(t, diastolic, "should have diastolic sample")
	assert.Equal(t, 80.0, diastolic.value)
}

func TestQuantityUnknownType(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "unknown_type.json")

	result, err := m.mapRecord(record, "user-123")
	assert.NoError(t, err)
	assert.Nil(t, result, "unknown type should return nil")
}

func TestQuantityBadValueCoercion(t *testing.T) {
	m := New()
	// String value that CAN be parsed
	record := map[string]any{
		"startDate":    "2025-03-01T14:30:00Z",
		"endDate":      "2025-03-01T14:30:00Z",
		"quantityType": "HKQuantityTypeIdentifierHeartRate",
		"value":        "72.5",
		"unit":         "count/min",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 72.5, result.QuantitySamples[0].Value)
}

func TestQuantityUnparseableValue(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "bad_values.json")

	result, err := m.mapRecord(record, "user-123")
	assert.Error(t, err, "truly unparseable value should return error")
	assert.Nil(t, result)
}

func TestQuantityContentHashDeterminism(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "quantity_heart_rate.json")

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.QuantitySamples[0].ContentHash,
		result2.QuantitySamples[0].ContentHash,
		"same record must produce same content hash")
}

func TestQuantityContentHashDiffers(t *testing.T) {
	m := New()
	record1 := loadTestFixture(t, "quantity_heart_rate.json")
	record2 := loadTestFixture(t, "quantity_steps.json")

	result1, err := m.mapRecord(record1, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record2, "user-123")
	require.NoError(t, err)

	assert.NotEqual(t, result1.QuantitySamples[0].ContentHash,
		result2.QuantitySamples[0].ContentHash,
		"different records must produce different hashes")
}

func TestQuantityMetadataPreservesOriginal(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "quantity_heart_rate.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	md := result.QuantitySamples[0].Metadata
	assert.Equal(t, "count/min", md["original_unit"])
	assert.Equal(t, 72.5, md["original_value"])
	assert.Equal(t, "Europe/Paris", md["timezone"])
}

func TestQuantityDietaryTypeRoutesToNutrition(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T12:00:00Z",
		"endDate":      "2025-03-01T12:00:00Z",
		"quantityType": "HKQuantityTypeIdentifierDietaryProtein",
		"value":        25.0,
		"unit":         "g",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	assert.NoError(t, err)
	require.NotNil(t, result, "dietary types should route to nutrition handler")
	assert.Len(t, result.NutritionLogs, 1, "dietary type should produce a NutritionLog")
	assert.NotNil(t, result.NutritionLogs[0].ProteinG)
	assert.Equal(t, 25.0, *result.NutritionLogs[0].ProteinG)
}
