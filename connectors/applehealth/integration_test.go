// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationMixedTypes verifies that the mapper correctly dispatches
// mixed record types to their respective handlers and produces the expected
// canonical output types.
func TestIntegrationMixedTypes(t *testing.T) {
	m := New()

	data, err := os.ReadFile("testdata/mixed_types.json")
	require.NoError(t, err)

	var records []map[string]any
	require.NoError(t, json.Unmarshal(data, &records))
	require.Len(t, records, 7, "fixture should have 7 records")

	var (
		quantityCount  int
		categoryCount  int
		workoutCount   int
		ecgCount       int
		nutritionCount int
		skippedCount   int
	)

	for _, record := range records {
		result, err := m.mapRecord(record, "user-integration")
		require.NoError(t, err)

		if result == nil {
			skippedCount++
			continue
		}

		quantityCount += len(result.QuantitySamples)
		categoryCount += len(result.CategorySamples)
		workoutCount += len(result.Workouts)
		ecgCount += len(result.ECGRecordings)
		nutritionCount += len(result.NutritionLogs)
	}

	assert.Equal(t, 1, quantityCount, "heart rate -> QuantitySample")
	assert.Equal(t, 1, categoryCount, "menstrual flow -> CategorySample")
	assert.Equal(t, 1, workoutCount, "running workout -> Workout")
	assert.Equal(t, 1, ecgCount, "ECG metadata -> ECGRecording")
	assert.Equal(t, 1, nutritionCount, "dietary protein -> NutritionLog")
	assert.Equal(t, 2, skippedCount, "clinical record (not stored) and unknown type -> nil (skipped)")
}

// TestIntegrationSupportedTypesCount verifies that SupportedTypes returns
// a comprehensive list covering all registered type identifiers.
func TestIntegrationSupportedTypesCount(t *testing.T) {
	m := New()
	types := m.supportedTypes()

	// Expected counts:
	// ~50 quantity types + ~48 category types (49 minus sleep_analysis which is separate)
	// + ~39 dietary types + sleep + workout + ecg_metadata + ecg_voltage
	// + heartbeat + route + food_correlation
	// Total should be > 140
	assert.Greater(t, len(types), 140,
		"SupportedTypes should include all registered types (got %d)", len(types))

	// Verify some specific types are present
	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}

	for _, expected := range []string{
		"HKQuantityTypeIdentifierHeartRate",
		"HKCategoryTypeIdentifierMenstrualFlow",
		"HKCategoryTypeIdentifierSleepAnalysis",
		"workout",
		"ecg_metadata",
		"ecg_voltage",
		"heartbeat",
		"route",
		"HKCorrelationTypeIdentifierFood",
		"HKQuantityTypeIdentifierDietaryProtein",
		"HKQuantityTypeIdentifierDietaryEnergyConsumed",
	} {
		assert.True(t, typeSet[expected], "SupportedTypes should include %q", expected)
	}
}

// TestIntegrationNoRegressionPlan01 verifies that all Plan 01 handler tests
// still pass by running quantity, category, workout, and sleep through the model.
func TestIntegrationNoRegressionPlan01(t *testing.T) {
	m := New()

	// Quantity: heart rate
	hr := loadTestFixture(t, "quantity_heart_rate.json")
	result, err := m.mapRecord(hr, "user-regression")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.QuantitySamples, 1)
	assert.Equal(t, "heart_rate", result.QuantitySamples[0].QuantityType)

	// Category: menstrual flow
	mf := loadTestFixture(t, "category_menstrual.json")
	result, err = m.mapRecord(mf, "user-regression")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.CategorySamples, 1)

	// Workout: running
	wr := loadTestFixture(t, "workout_running.json")
	result, err = m.mapRecord(wr, "user-regression")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Workouts, 1)

	// Sleep: in_bed produces both session and phase
	sl := loadTestFixture(t, "category_sleep.json")
	result, err = m.mapRecord(sl, "user-regression")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.SleepPhases)
}
