// SPDX-License-Identifier: AGPL-3.0-only

package etl

import (
	"testing"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// qs builds a single-sample MapResult with a safe (recent-past) timestamp so
// only the value-range rule under test can fire.
func qs(qt string, v float64) *model.MapResult {
	return &model.MapResult{
		QuantitySamples: []model.QuantitySample{{
			QuantityType: qt,
			Value:        v,
			StartDate:    time.Now().Add(-1 * time.Hour),
			EndDate:      time.Now().Add(-59 * time.Minute),
		}},
	}
}

func TestValidate_NonNegativeMetrics(t *testing.T) {
	// hrv_sdnn used to sit in this list. It moved out when the soft band landed:
	// an SDNN of exactly 0 ms means no beat-to-beat variability at all and 1234 ms
	// is off any physiological scale, so both are now suspect rather than clean.
	// See TestValidate_HRVSDNNBands.
	for _, qt := range []string{"step_count", "active_energy_burned", "distance_walking_running", "intraday_activity"} {
		t.Run(qt+"/valid-zero", func(t *testing.T) {
			assert.Empty(t, ValidateMapResult(qs(qt, 0)))
		})
		t.Run(qt+"/valid-positive", func(t *testing.T) {
			assert.Empty(t, ValidateMapResult(qs(qt, 1234.5)))
		})
		t.Run(qt+"/invalid-negative", func(t *testing.T) {
			errs := ValidateMapResult(qs(qt, -1))
			require.Len(t, errs, 1)
			assert.Equal(t, "value", errs[0].Field)
			assert.Contains(t, errs[0].Message, "non-negative")
		})
	}
}

// TestValidate_HRVSDNNBands: the hard rule stays "non-negative", the soft band
// is what catches the two garbage shapes a real feed produces.
func TestValidate_HRVSDNNBands(t *testing.T) {
	assert.Empty(t, ValidateMapResult(qs("hrv_sdnn", 42)))
	assert.Empty(t, ValidateMapResult(qs("hrv_sdnn", 180)))

	for _, v := range []float64{0, 1234.5} {
		errs := ValidateMapResult(qs("hrv_sdnn", v))
		require.Lenf(t, errs, 1, "hrv_sdnn=%v", v)
		assert.Equal(t, SeveritySuspect, errs[0].Severity, "kept and flagged, not dropped")
	}

	errs := ValidateMapResult(qs("hrv_sdnn", -1))
	require.Len(t, errs, 1)
	assert.Equal(t, SeverityReject, errs[0].Severity)
	assert.Contains(t, errs[0].Message, "non-negative")
}

func TestValidate_BodyFatPercentageRange(t *testing.T) {
	assert.Empty(t, ValidateMapResult(qs("body_fat_percentage", 22.5)))

	// 0 and 100 are inside the hard band but nobody has either: essential fat is
	// about 3 percent. They now come back flagged rather than clean, which is the
	// point of the soft band.
	for _, v := range []float64{0, 100} {
		errs := ValidateMapResult(qs("body_fat_percentage", v))
		require.Lenf(t, errs, 1, "body_fat_percentage=%v", v)
		assert.Equal(t, SeveritySuspect, errs[0].Severity)
	}

	require.Len(t, ValidateMapResult(qs("body_fat_percentage", 150)), 1)
	require.Len(t, ValidateMapResult(qs("body_fat_percentage", -5)), 1)
}

func TestValidate_PositiveMasses(t *testing.T) {
	// One realistic value per metric. The old single 12.3 kg stood in for all
	// five, which is nonsense for bone_mass (an adult skeleton is 2 to 4 kg) and
	// now trips its soft band.
	valid := map[string]float64{
		"lean_body_mass": 55.2,
		"body_fat_mass":  18.4,
		"bone_mass":      3.1,
		"muscle_mass":    32.7,
		"water_mass":     40.5,
	}
	for qt, v := range valid {
		t.Run(qt+"/valid", func(t *testing.T) {
			assert.Empty(t, ValidateMapResult(qs(qt, v)))
		})
		t.Run(qt+"/invalid-zero", func(t *testing.T) {
			errs := ValidateMapResult(qs(qt, 0))
			require.Len(t, errs, 1)
			assert.Contains(t, errs[0].Message, "positive")
			assert.Equal(t, SeverityReject, errs[0].Severity)
		})
		t.Run(qt+"/invalid-negative", func(t *testing.T) {
			require.Len(t, ValidateMapResult(qs(qt, -1)), 1)
		})
	}
}

// Real-data guard: the values actually present in the live dataset must all
// pass the new rules (these are the observed extremes from the audit).
func TestValidate_RealDataExtremesPass(t *testing.T) {
	samples := []struct {
		qt string
		v  float64
	}{
		{"step_count", 68302},
		{"active_energy_burned", 17536.07},
		{"distance_walking_running", 70961.63},
		{"body_fat_percentage", 41.109},
		{"body_fat_percentage", 16.179},
		{"lean_body_mass", 130.244},
		{"bone_mass", 4.38},
		{"hrv_sdnn", 42},
	}
	for _, s := range samples {
		assert.Emptyf(t, ValidateMapResult(qs(s.qt, s.v)), "real %s=%v should pass", s.qt, s.v)
	}
}

// qsUnit is qs with an explicit unit so unit-aware rules can be exercised.
func qsUnit(qt string, v float64, unit string) *model.MapResult {
	mr := qs(qt, v)
	mr.QuantitySamples[0].Unit = unit
	return mr
}

// TestValidate_BloodGlucoseUnitAware guards the regression where the validator
// assumed mmol/L [0,50] unconditionally and dropped every mg/dL reading (the
// values 88-161 seen in the live dataset are normal mg/dL glucose).
//
// The validator no longer branches on the unit itself. mg/dL is the storage unit
// and model.ToStorageUnit converts mmol/L before validation runs, so the
// mmol/L half of this test moved to TestValidate_UnitContractPairsWithConverter,
// where it exercises the converter and the range check together.
func TestValidate_BloodGlucoseUnitAware(t *testing.T) {
	// mg/dL (Apple Health default): normal-to-high readings must pass.
	for _, v := range []float64{72, 88, 117, 159, 240, 400} {
		assert.Emptyf(t, ValidateMapResult(qsUnit("blood_glucose", v, "mg/dL")), "mg/dL %v should pass", v)
	}
	// Empty unit defaults to mg/dL.
	assert.Empty(t, ValidateMapResult(qsUnit("blood_glucose", 117, "")))
	// Absurd mg/dL still rejected.
	require.Len(t, ValidateMapResult(qsUnit("blood_glucose", 5000, "mg/dL")), 1)
	require.Len(t, ValidateMapResult(qsUnit("blood_glucose", -1, "mg/dL")), 1)
	// An unconverted mmol/L value reaching the validator means the sample skipped
	// the choke point, so it parks instead of being silently mis-scaled.
	unconverted := ValidateMapResult(qsUnit("blood_glucose", 6.5, "mmol/L"))
	require.Len(t, unconverted, 1)
	assert.Equal(t, "unit", unconverted[0].Field)
	assert.Equal(t, SeverityReject, unconverted[0].Severity)
}
