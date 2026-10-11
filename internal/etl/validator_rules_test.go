// SPDX-License-Identifier: AGPL-3.0-only

package etl

import (
	"math"
	"testing"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRangeRules_CoverEveryDeclaredMetric is the guard that makes the range
// table hard to forget. Adding a metric to model.storageUnits without saying
// what a plausible value looks like used to mean it reached the hypertables
// unchecked; now it fails here.
func TestRangeRules_CoverEveryDeclaredMetric(t *testing.T) {
	for qt := range model.StorageUnits() {
		_, ok := rangeRules[qt]
		assert.Truef(t, ok, "quantity type %q declares a storage unit but has no range rule", qt)
	}
}

// TestRangeRules_UnitMatchesStorageUnit stops the two tables drifting. A rule
// whose bounds assume kg while the pipeline stores lb would silently pass every
// wrong value, which is the exact failure the storage-unit contract exists to
// prevent.
func TestRangeRules_UnitMatchesStorageUnit(t *testing.T) {
	for qt, rule := range rangeRules {
		storage, declared := model.StorageUnit(qt)
		if declared {
			assert.Equalf(t, storage, rule.Unit,
				"rule for %q assumes unit %q but the storage unit is %q", qt, rule.Unit, storage)
			continue
		}
		// No declared storage unit means CanonicalizeUnits leaves the sample's
		// unit alone, so the rule must not enforce one.
		assert.Emptyf(t, rule.Unit,
			"rule for %q declares unit %q but the type has no storage unit, so any spelling can arrive", qt, rule.Unit)
	}
}

// TestRangeRules_BandsNest checks the invariant the severity split relies on:
// HardMin <= SoftMin < SoftMax <= HardMax. A rule that forgot its soft band
// leaves SoftMin == SoftMax == 0, which this catches.
func TestRangeRules_BandsNest(t *testing.T) {
	for qt, rule := range rangeRules {
		assert.LessOrEqualf(t, rule.HardMin, rule.SoftMin, "%s: soft floor below hard floor", qt)
		assert.Lessf(t, rule.SoftMin, rule.SoftMax, "%s: empty or missing soft band", qt)
		assert.LessOrEqualf(t, rule.SoftMax, rule.HardMax, "%s: soft ceiling above hard ceiling", qt)
	}
}

// TestRangeRules_UnboundedOnlyAbove guards the phrasing rule: a rule may be
// unbounded above (cumulative counts, masses) but never below, because
// hardRangeMessage reads HardMax to pick "non-negative" vs "out of range".
func TestRangeRules_UnboundedOnlyAbove(t *testing.T) {
	for qt, rule := range rangeRules {
		assert.Falsef(t, math.IsInf(rule.HardMin, -1), "%s: hard floor must be finite", qt)
	}
}

func TestValidate_SeveritySplit(t *testing.T) {
	// 300 bpm sits on the hard ceiling: possible to record, impossible to have.
	// 301 is past it and must be rejected outright.
	rejected := ValidateMapResult(qs("heart_rate", 301))
	require.Len(t, rejected, 1)
	assert.Equal(t, SeverityReject, rejected[0].Severity)
	assert.Len(t, Rejections(rejected), 1)
	assert.Empty(t, Suspicions(rejected))

	// 250 bpm is inside the hard band but past the soft one: a plausible sensor
	// artifact during exercise, so it is kept and flagged rather than dropped.
	suspect := ValidateMapResult(qs("heart_rate", 250))
	require.Len(t, suspect, 1)
	assert.Equal(t, SeveritySuspect, suspect[0].Severity)
	assert.Empty(t, Rejections(suspect))
	assert.Len(t, Suspicions(suspect), 1)
	assert.Contains(t, suspect[0].Message, "implausible")

	// A value inside both bands produces nothing.
	assert.Empty(t, ValidateMapResult(qs("heart_rate", 62)))
}

// TestValidate_OneErrorPerBadValue: a value outside BOTH bands must not report
// twice, or the recovery loop would double-count the same sample.
func TestValidate_OneErrorPerBadValue(t *testing.T) {
	errs := ValidateMapResult(qs("body_temperature", 60))
	require.Len(t, errs, 1)
	assert.Equal(t, SeverityReject, errs[0].Severity)
}

// TestValidate_NonCanonicalUnitRejected covers the sample that reached the
// validator without passing model.CanonicalizeUnits. Its magnitude is unknown,
// so it must park as recoverable rather than be range-checked against the wrong
// scale.
func TestValidate_NonCanonicalUnitRejected(t *testing.T) {
	errs := ValidateMapResult(qsUnit("body_mass", 165, "lb"))
	require.Len(t, errs, 1)
	assert.Equal(t, "unit", errs[0].Field)
	assert.Equal(t, SeverityReject, errs[0].Severity)

	// The same value in the storage unit passes.
	assert.Empty(t, ValidateMapResult(qsUnit("body_mass", 74.8, "kg")))
	// Case differences are not a violation.
	assert.Empty(t, ValidateMapResult(qsUnit("body_mass", 74.8, "KG")))
	// A legacy alias with no declared storage unit accepts any spelling.
	assert.Empty(t, ValidateMapResult(qsUnit("spo2", 97, "whatever")))
}

// TestValidate_UnitContractPairsWithConverter proves the split of duties: the
// validator no longer knows about mmol/L, because model.ToStorageUnit converts
// it before the value is ever range-checked.
func TestValidate_UnitContractPairsWithConverter(t *testing.T) {
	v, unit, err := model.ToStorageUnit("blood_glucose", 6.5, "mmol/L")
	require.NoError(t, err)
	require.Equal(t, "mg/dL", unit)
	assert.InDelta(t, 117.1, v, 0.1)
	assert.Empty(t, ValidateMapResult(qsUnit("blood_glucose", v, unit)))
}

func TestFlagSuspects_AnnotatesTheRightSample(t *testing.T) {
	mr := &model.MapResult{
		QuantitySamples: []model.QuantitySample{
			{QuantityType: "heart_rate", Value: 62, StartDate: time.Now().Add(-time.Hour)},
			{QuantityType: "heart_rate", Value: 250, StartDate: time.Now().Add(-time.Hour)},
		},
	}
	errs := ValidateMapResult(mr)
	require.Len(t, errs, 1)

	FlagSuspects(mr, Suspicions(errs))

	assert.Nil(t, mr.QuantitySamples[0].Metadata, "a clean sample must not be annotated")
	require.NotNil(t, mr.QuantitySamples[1].Metadata)
	assert.Equal(t, string(SeveritySuspect), mr.QuantitySamples[1].Metadata["quality_flag"])
	assert.Contains(t, mr.QuantitySamples[1].Metadata["quality_note"], "implausible")
}

// TestValidate_PreviouslyUncheckedMetrics covers the two thirds of the canonical
// vocabulary that had no rule before the table existed. One representative bad
// value per family is enough: the coverage guard above is what proves the rest
// are wired.
func TestValidate_PreviouslyUncheckedMetrics(t *testing.T) {
	cases := []struct {
		qt   string
		v    float64
		want Severity
	}{
		{"height", 1.75, SeverityReject}, // metres slipped through as centimetres
		{"height", 178, ""},              // clean
		{"vo2_max", 480, SeverityReject}, // decimal shift
		{"vo2_max", 52, ""},              // clean
		{"resting_heart_rate", 180, SeveritySuspect},
		{"resting_heart_rate", 54, ""},
		{"body_mass_index", 0, SeverityReject},
		{"body_mass_index", 23.4, ""},
		{"skin_temperature", 90, SeverityReject},
		{"skin_temperature", 33.2, ""},
		{"body_temperature_deviation", -0.4, ""},
		{"body_temperature_deviation", -40, SeverityReject},
		{"strain", 45, SeveritySuspect}, // WHOOP strain tops out at 21
		{"strain", 14.2, ""},
		{"readiness_score", 82, ""},
		{"readiness_score", 140, SeverityReject},
		{"visceral_fat_rating", 8, ""},
		{"flights_climbed", -3, SeverityReject},
	}
	for _, c := range cases {
		errs := ValidateMapResult(qs(c.qt, c.v))
		if c.want == "" {
			assert.Emptyf(t, errs, "%s=%v should be clean", c.qt, c.v)
			continue
		}
		require.Lenf(t, errs, 1, "%s=%v should produce one error", c.qt, c.v)
		assert.Equalf(t, c.want, errs[0].Severity, "%s=%v severity", c.qt, c.v)
	}
}
