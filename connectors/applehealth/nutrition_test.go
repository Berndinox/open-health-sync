// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/soma-intelligence/open-health-sync/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNutritionDietaryProtein(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "nutrition_dietary.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.NutritionLogs, 1)

	nl := result.NutritionLogs[0]
	assert.Equal(t, "user-123", nl.UserID)
	assert.Equal(t, "apple_health", nl.Provider)
	require.NotNil(t, nl.ProteinG)
	assert.Equal(t, 25.0, *nl.ProteinG)
	assert.NotEmpty(t, nl.ContentHash)
}

func TestNutritionDietaryEnergy(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T12:00:00Z",
		"endDate":      "2025-03-01T12:30:00Z",
		"quantityType": "HKQuantityTypeIdentifierDietaryEnergyConsumed",
		"value":        500.0,
		"unit":         "kcal",
		"sourceName":   "Example Food Log",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.NutritionLogs, 1)

	nl := result.NutritionLogs[0]
	require.NotNil(t, nl.EnergyKcal)
	assert.Equal(t, 500.0, *nl.EnergyKcal)
}

func TestNutritionFoodCorrelation(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "correlation_food.json")

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.NutritionLogs, 1)

	nl := result.NutritionLogs[0]
	assert.Equal(t, "user-123", nl.UserID)
	assert.Equal(t, "apple_health", nl.Provider)

	require.NotNil(t, nl.EnergyKcal)
	assert.Equal(t, 450.0, *nl.EnergyKcal)

	require.NotNil(t, nl.ProteinG)
	assert.Equal(t, 25.0, *nl.ProteinG)

	require.NotNil(t, nl.CarbsG)
	assert.Equal(t, 55.0, *nl.CarbsG)

	require.NotNil(t, nl.FatG)
	assert.Equal(t, 12.0, *nl.FatG)

	assert.NotEmpty(t, nl.ContentHash)
}

func TestNutritionUnknownDietaryType(t *testing.T) {
	m := New()
	record := map[string]any{
		"startDate":    "2025-03-01T12:00:00Z",
		"endDate":      "2025-03-01T12:30:00Z",
		"quantityType": "HKQuantityTypeIdentifierDietaryVitaminB6",
		"value":        1.5,
		"unit":         "mg",
		"sourceName":   "Example Food Log",
		"metadata":     map[string]any{},
	}

	result, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.NutritionLogs, 1)

	nl := result.NutritionLogs[0]
	// vitamin_b6 is not in the core fields -- should be in NutrientsJSON
	assert.NotNil(t, nl.NutrientsJSON)
	assert.Equal(t, 1.5, nl.NutrientsJSON["vitamin_b6"])
}

func TestNutritionContentHash(t *testing.T) {
	m := New()
	record := loadTestFixture(t, "nutrition_dietary.json")

	result1, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	result2, err := m.mapRecord(record, "user-123")
	require.NoError(t, err)

	assert.Equal(t, result1.NutritionLogs[0].ContentHash,
		result2.NutritionLogs[0].ContentHash,
		"same nutrition record must produce same content hash")
}

// TestMapDietaryQuantity_ScalesMassToTheDestinationColumn guards against mass values stored in the wrong unit.
//
// Every dietary mass reached the database in KILOGRAMS. The SDK's
// extractQuantityValue walks an ordered unit list and returns the FIRST unit the
// sample is compatible with; "kg" sits at index 4 and "g"/"mg"/"mcg" at 20/21/22,
// and every mass quantity is compatible with kilograms, so the later entries are
// unreachable code. The wire record then says unit "kg" honestly, and this mapper
// wrote it straight into a column named _g.
//
// Unscaled, a gram column reads 1,000x low and a milligram column 1,000,000x
// low, while energy_kcal is right because energy is not mass.
//
// The conversion must be scoped to nutrition. A global kg->g rule would also hit
// bodyMass, which is legitimately kilograms, and turn 78 kg into 78,000.
func TestMapDietaryQuantity_ScalesMassToTheDestinationColumn(t *testing.T) {
	m := newMapper()
	cases := []struct {
		name, qt, unit string
		in, want       float64
		get            func(*model.NutritionLog) *float64
	}{
		{"protein in kg lands as grams", "HKQuantityTypeIdentifierDietaryProtein", "kg",
			0.136, 136, func(n *model.NutritionLog) *float64 { return n.ProteinG }},
		{"protein already in grams is untouched", "HKQuantityTypeIdentifierDietaryProtein", "g",
			136, 136, func(n *model.NutritionLog) *float64 { return n.ProteinG }},
		{"fat in kg lands as grams", "HKQuantityTypeIdentifierDietaryFatTotal", "kg",
			0.28, 280, func(n *model.NutritionLog) *float64 { return n.FatG }},
		{"sodium in kg lands as milligrams", "HKQuantityTypeIdentifierDietarySodium", "kg",
			0.0538, 53800, func(n *model.NutritionLog) *float64 { return n.SodiumMg }},
		{"sodium already in mg is untouched", "HKQuantityTypeIdentifierDietarySodium", "mg",
			2300, 2300, func(n *model.NutritionLog) *float64 { return n.SodiumMg }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := m.mapDietaryQuantity(map[string]any{
				"quantityType": c.qt,
				"value":        c.in,
				"unit":         c.unit,
				"startDate":    "2026-09-01T12:00:00Z",
				"endDate":      "2026-09-01T12:00:00Z",
			}, "user-1")
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Len(t, res.NutritionLogs, 1)
			got := c.get(&res.NutritionLogs[0])
			require.NotNil(t, got, "the nutrient field must be set")
			assert.InDelta(t, c.want, *got, 0.001,
				"%s in %s must be stored in the destination column's unit", c.qt, c.unit)
		})
	}
}
