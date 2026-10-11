// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePart(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func TestComputeCountsEachKindOnce(t *testing.T) {
	dir := t.TempDir()
	hr := `{"quantityType":"HKQuantityTypeIdentifierHeartRate","startDate":"2026-01-10T10:00:00.000Z","endDate":"2026-01-10T10:00:00.000Z","value":60,"sourceName":"W"}`
	writePart(t, dir, "0_HKQuantityTypeIdentifierHeartRate.ndjson", hr+"\n"+
		`{"quantityType":"HKQuantityTypeIdentifierHeartRate","startDate":"2026-01-10T10:01:00.000Z","endDate":"2026-01-10T10:01:00.000Z","value":61,"sourceName":"W"}`+"\n\n")
	// The same line again in another part is one record.
	writePart(t, dir, "1_HKQuantityTypeIdentifierHeartRate.ndjson", hr+"\n")
	writePart(t, dir, "2_HKQuantityTypeIdentifierStepCount.ndjson",
		`{"quantityType":"HKQuantityTypeIdentifierStepCount","startDate":"2026-01-10T10:00:00Z","endDate":"2026-01-10T10:15:00Z","value":100.5}`+"\n"+
			`{"quantityType":"HKQuantityTypeIdentifierStepCount","startDate":"2026-01-10T10:15:00Z","endDate":"2026-01-10T10:30:00Z","value":20}`+"\n")
	writePart(t, dir, "10_HKCategoryTypeIdentifierSleepAnalysis.ndjson",
		`{"categoryType":"HKCategoryTypeIdentifierSleepAnalysis","startDate":"2026-01-09T22:00:00Z","endDate":"2026-01-10T06:00:00Z","value":0}`+"\n"+
			`{"categoryType":"HKCategoryTypeIdentifierSleepAnalysis","startDate":"2026-01-09T22:10:00Z","endDate":"2026-01-09T23:00:00Z","value":3}`+"\n")
	writePart(t, dir, "4_HKQuantityTypeIdentifierOxygenSaturation.ndjson",
		`{"quantityType":"HKQuantityTypeIdentifierOxygenSaturation","startDate":"2026-01-10T10:00:00Z","endDate":"2026-01-10T10:00:00Z","value":0.97}`+"\n")
	writePart(t, dir, "3_HKWorkoutTypeIdentifier.ndjson",
		`{"workoutActivityType":37,"startDate":"2026-01-10T07:00:00Z","endDate":"2026-01-10T07:45:00Z"}`+"\n")

	e, err := Compute(dir)
	require.NoError(t, err)
	assert.Equal(t, Expectation{Parts: 6, HeartRates: 2, Steps: 2, StepSum: 120.5, SpO2: 1, SleepSessions: 1, Workouts: 1}, e)

	files, err := Parts(dir)
	require.NoError(t, err)
	assert.Equal(t, "10_HKCategoryTypeIdentifierSleepAnalysis.ndjson", filepath.Base(files[5]), "ordered by part number")

	var out bytes.Buffer
	require.NoError(t, e.WriteShell(&out))
	assert.Equal(t, "PARTS=6\nHR_COUNT=2\nSTEP_COUNT=2\nSTEP_SUM=120.500\nSPO2_COUNT=1\nSLEEP_SESSIONS=1\nWORKOUTS=1\n", out.String())
}

func TestComputeRefusesBadInput(t *testing.T) {
	dir := t.TempDir()
	_, err := Compute(dir)
	assert.Error(t, err, "an empty directory is not a fixture")

	writePart(t, dir, "nopart.ndjson", "{}\n")
	_, err = Compute(dir)
	assert.ErrorContains(t, err, "want <part>_<type>.ndjson")

	dir = t.TempDir()
	writePart(t, dir, "0_X.ndjson", "not json\n")
	_, err = Compute(dir)
	assert.ErrorContains(t, err, "line 1")
}

func TestGenerateIsDistinctAndStable(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	require.NoError(t, Generate(a, 3, 50))
	require.NoError(t, Generate(b, 3, 50))
	e, err := Compute(a)
	require.NoError(t, err)
	assert.Equal(t, 3, e.Parts)
	assert.Equal(t, 100, e.HeartRates, "parts 0 and 2")
	assert.Equal(t, 50, e.Steps, "part 1")
	for _, f := range []string{"0_HKQuantityTypeIdentifierHeartRate.ndjson", "1_HKQuantityTypeIdentifierStepCount.ndjson"} {
		x, err := os.ReadFile(filepath.Join(a, f))
		require.NoError(t, err)
		y, err := os.ReadFile(filepath.Join(b, f))
		require.NoError(t, err)
		assert.Equal(t, x, y, "same bytes on every run")
	}
}

// The committed quickstart fixture: these are the numbers the suites assert.
func TestQuickstartFixture(t *testing.T) {
	e, err := Compute("../../connectors/applehealth/testdata/e2e")
	require.NoError(t, err)
	assert.Equal(t, 6, e.Parts)
	assert.Equal(t, 288, e.HeartRates)
	assert.Equal(t, 96, e.Steps)
	assert.Equal(t, 24, e.SpO2)
	assert.Equal(t, 2, e.SleepSessions)
	assert.Equal(t, 3, e.Workouts)
}
