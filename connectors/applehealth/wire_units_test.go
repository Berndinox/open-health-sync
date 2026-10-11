// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// TestEveryQuantityTypeLandsInItsStorageUnit maps one line per HealthKit
// quantity type, in the unit the SDK sends it (HKUnit.unitString of the SDK
// catalog's unit), through the ETL's unit choke point. A declared storage unit
// the wire unit cannot reach would park every sample of that type.
func TestEveryQuantityTypeLandsInItsStorageUnit(t *testing.T) {
	wire := map[string]string{
		"HKQuantityTypeIdentifierHeartRate":                     "count/min",
		"HKQuantityTypeIdentifierRestingHeartRate":              "count/min",
		"HKQuantityTypeIdentifierWalkingHeartRateAverage":       "count/min",
		"HKQuantityTypeIdentifierHeartRateVariabilitySDNN":      "ms",
		"HKQuantityTypeIdentifierOxygenSaturation":              "%",
		"HKQuantityTypeIdentifierBloodPressureSystolic":         "mmHg",
		"HKQuantityTypeIdentifierBloodPressureDiastolic":        "mmHg",
		"HKQuantityTypeIdentifierRespiratoryRate":               "count/min",
		"HKQuantityTypeIdentifierBodyTemperature":               "degC",
		"HKQuantityTypeIdentifierBodyMass":                      "kg",
		"HKQuantityTypeIdentifierHeight":                        "m",
		"HKQuantityTypeIdentifierBodyMassIndex":                 "count",
		"HKQuantityTypeIdentifierBodyFatPercentage":             "%",
		"HKQuantityTypeIdentifierLeanBodyMass":                  "kg",
		"HKQuantityTypeIdentifierWaistCircumference":            "m",
		"HKQuantityTypeIdentifierStepCount":                     "count",
		"HKQuantityTypeIdentifierDistanceWalkingRunning":        "m",
		"HKQuantityTypeIdentifierDistanceCycling":               "m",
		"HKQuantityTypeIdentifierDistanceSwimming":              "m",
		"HKQuantityTypeIdentifierDistanceWheelchair":            "m",
		"HKQuantityTypeIdentifierDistanceDownhillSnowSports":    "m",
		"HKQuantityTypeIdentifierBasalEnergyBurned":             "kcal",
		"HKQuantityTypeIdentifierActiveEnergyBurned":            "kcal",
		"HKQuantityTypeIdentifierFlightsClimbed":                "count",
		"HKQuantityTypeIdentifierPushCount":                     "count",
		"HKQuantityTypeIdentifierNikeFuel":                      "count",
		"HKQuantityTypeIdentifierAppleExerciseTime":             "min",
		"HKQuantityTypeIdentifierAppleMoveTime":                 "min",
		"HKQuantityTypeIdentifierAppleStandTime":                "min",
		"HKQuantityTypeIdentifierRunningStrideLength":           "m",
		"HKQuantityTypeIdentifierRunningVerticalOscillation":    "cm",
		"HKQuantityTypeIdentifierRunningGroundContactTime":      "ms",
		"HKQuantityTypeIdentifierRunningPower":                  "W",
		"HKQuantityTypeIdentifierRunningSpeed":                  "m/s",
		"HKQuantityTypeIdentifierWalkingSpeed":                  "m/s",
		"HKQuantityTypeIdentifierPhysicalEffort":                "kcal/hr·kg",
		"HKQuantityTypeIdentifierVO2Max":                        "mL/min·kg",
		"HKQuantityTypeIdentifierCyclingPower":                  "W",
		"HKQuantityTypeIdentifierEnvironmentalAudioExposure":    "dBASPL",
		"HKQuantityTypeIdentifierHeadphoneAudioExposure":        "dBASPL",
		"HKQuantityTypeIdentifierEnvironmentalSoundReduction":   "dBASPL",
		"HKQuantityTypeIdentifierBloodGlucose":                  "mg/dL",
		"HKQuantityTypeIdentifierElectrodermalActivity":         "mcS",
		"HKQuantityTypeIdentifierForcedExpiratoryVolume1":       "L",
		"HKQuantityTypeIdentifierForcedVitalCapacity":           "L",
		"HKQuantityTypeIdentifierInhalerUsage":                  "count",
		"HKQuantityTypeIdentifierInsulinDelivery":               "IU",
		"HKQuantityTypeIdentifierPeakExpiratoryFlowRate":        "L/min",
		"HKQuantityTypeIdentifierNumberOfTimesFallen":           "count",
		"HKQuantityTypeIdentifierUVExposure":                    "count",
		"HKQuantityTypeIdentifierBasalBodyTemperature":          "degC",
		"HKQuantityTypeIdentifierBloodAlcoholContent":           "%",
		"HKQuantityTypeIdentifierNumberOfAlcoholicBeverages":    "count",
		"HKQuantityTypeIdentifierAppleSleepingWristTemperature": "degC",
		"HKQuantityTypeIdentifierWaterTemperature":              "degC",
		"HKQuantityTypeIdentifierUnderwaterDepth":               "m",
	}
	require.Len(t, wire, len(quantityTypeCanonical), "one wire unit per mapped quantity type")

	m := newMapper()
	for hk, unit := range wire {
		res, err := m.mapRecord(map[string]any{
			"quantityType": hk, "value": 1.0, "unit": unit,
			"startDate": "2026-01-10T14:30:00Z", "endDate": "2026-01-10T14:31:00Z",
		}, "u")
		require.NoError(t, err, hk)
		require.NotNil(t, res, hk)
		errs := model.CanonicalizeUnits(res)
		assert.Empty(t, errs, "%s sent in %q does not reach its storage unit", hk, unit)
		want, declared := model.StorageUnit(res.QuantitySamples[0].QuantityType)
		assert.True(t, declared, hk)
		assert.Equal(t, want, res.QuantitySamples[0].Unit, hk)
	}

	// HealthKit's 0..1 fraction becomes a percentage: 0.0008 is 0.08 percent.
	res, err := m.mapRecord(map[string]any{
		"quantityType": "HKQuantityTypeIdentifierBloodAlcoholContent", "value": 0.0008, "unit": "%",
		"startDate": "2026-01-10T14:30:00Z",
	}, "u")
	require.NoError(t, err)
	require.Empty(t, model.CanonicalizeUnits(res))
	assert.Equal(t, "percent", res.QuantitySamples[0].Unit)
	assert.InDelta(t, 0.08, res.QuantitySamples[0].Value, 1e-12)
}
