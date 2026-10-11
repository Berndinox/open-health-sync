// SPDX-License-Identifier: AGPL-3.0-only

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashDeterminism(t *testing.T) {
	fields := map[string]string{
		"user_id":       "abc",
		"provider":      "apple_health",
		"quantity_type": "heart_rate",
		"start_date":    "2025-03-01T14:30:00Z",
		"value":         "72.5",
	}

	hash1 := ComputeContentHash(fields)
	hash2 := ComputeContentHash(fields)

	require.NotEmpty(t, hash1)
	assert.Equal(t, hash1, hash2, "same fields must produce same hash")
	assert.Len(t, hash1, 64, "SHA-256 hex string should be 64 chars")
}

func TestHashFieldOrderIndependence(t *testing.T) {
	// Two maps with same keys and values but different insertion order
	fields1 := map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
	}
	fields2 := map[string]string{
		"c": "3",
		"a": "1",
		"b": "2",
	}

	hash1 := ComputeContentHash(fields1)
	hash2 := ComputeContentHash(fields2)

	assert.Equal(t, hash1, hash2, "field order must not affect hash")
}

func TestHashDifferentValues(t *testing.T) {
	fields1 := map[string]string{
		"user_id": "abc",
		"value":   "72.5",
	}
	fields2 := map[string]string{
		"user_id": "abc",
		"value":   "73.0",
	}

	hash1 := ComputeContentHash(fields1)
	hash2 := ComputeContentHash(fields2)

	assert.NotEqual(t, hash1, hash2, "different values must produce different hashes")
}

func TestCanonicalFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"trailing zeros", 72.5000, "72.5"},
		{"no trailing zeros", 72.5, "72.5"},
		{"zero", 0.0, "0"},
		{"negative", -1.5, "-1.5"},
		{"integer value", 100.0, "100"},
		{"small decimal", 0.001, "0.001"},
		{"large number", 12345.6789, "12345.6789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanonicalFloat(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanonicalFloatConsistency(t *testing.T) {
	// 72.5000 and 72.5 must produce the same canonical string
	assert.Equal(t, CanonicalFloat(72.5000), CanonicalFloat(72.5))
}

// --- Child content-hash functions ---
//
// These give the four child tables (sleep_phases, route_points, ecg_voltage_samples,
// heartbeat_samples) a deterministic identity so overlapping re-deliveries dedup via
// ON CONFLICT instead of piling up duplicate rows.

func TestSleepPhaseContentHash(t *testing.T) {
	start := time.Date(2025, 3, 1, 22, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	base := SleepPhase{ParentContentHash: "parentA", Phase: "deep", StartDate: start, EndDate: end, DurationS: 1800}

	// Idempotent: the same logical phase re-delivered hashes identically.
	assert.Equal(t, SleepPhaseContentHash(base), SleepPhaseContentHash(base))
	assert.Len(t, SleepPhaseContentHash(base), 64)

	// Sibling phases of the same parent must differ (else they would collapse into one).
	sibling := base
	sibling.Phase = "rem"
	assert.NotEqual(t, SleepPhaseContentHash(base), SleepPhaseContentHash(sibling))

	shifted := base
	shifted.StartDate = start.Add(time.Minute)
	assert.NotEqual(t, SleepPhaseContentHash(base), SleepPhaseContentHash(shifted))

	// Same phase under a different parent must differ.
	otherParent := base
	otherParent.ParentContentHash = "parentB"
	assert.NotEqual(t, SleepPhaseContentHash(base), SleepPhaseContentHash(otherParent))
}

func TestRoutePointContentHash(t *testing.T) {
	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	alt := 12.5
	base := RoutePoint{ParentContentHash: "wkt", Timestamp: ts, Latitude: 37.7749, Longitude: -122.4194, AltitudeM: &alt}

	assert.Equal(t, RoutePointContentHash(base), RoutePointContentHash(base))
	assert.Len(t, RoutePointContentHash(base), 64)

	moved := base
	moved.Latitude = 37.7750
	assert.NotEqual(t, RoutePointContentHash(base), RoutePointContentHash(moved))

	later := base
	later.Timestamp = ts.Add(time.Second)
	assert.NotEqual(t, RoutePointContentHash(base), RoutePointContentHash(later))

	// Nil altitude must not panic and must stay deterministic.
	noAlt := base
	noAlt.AltitudeM = nil
	assert.Equal(t, RoutePointContentHash(noAlt), RoutePointContentHash(noAlt))
	assert.NotEqual(t, RoutePointContentHash(base), RoutePointContentHash(noAlt))
}

func TestECGVoltageSampleContentHash(t *testing.T) {
	base := ECGVoltageSample{ParentContentHash: "ecg1", TimeSinceSampleStart: 0.004, VoltageUV: 120.5}

	assert.Equal(t, ECGVoltageSampleContentHash(base), ECGVoltageSampleContentHash(base))
	assert.Len(t, ECGVoltageSampleContentHash(base), 64)

	nextSample := base
	nextSample.TimeSinceSampleStart = 0.008
	assert.NotEqual(t, ECGVoltageSampleContentHash(base), ECGVoltageSampleContentHash(nextSample))

	diffVolt := base
	diffVolt.VoltageUV = 121.0
	assert.NotEqual(t, ECGVoltageSampleContentHash(base), ECGVoltageSampleContentHash(diffVolt))
}

func TestHeartbeatSampleContentHash(t *testing.T) {
	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	base := HeartbeatSample{SeriesID: "s1", UserID: "u1", Timestamp: ts, RRMS: 812.0}

	assert.Equal(t, HeartbeatSampleContentHash(base), HeartbeatSampleContentHash(base))
	assert.Len(t, HeartbeatSampleContentHash(base), 64)

	later := base
	later.Timestamp = ts.Add(time.Millisecond)
	assert.NotEqual(t, HeartbeatSampleContentHash(base), HeartbeatSampleContentHash(later))

	diffSeries := base
	diffSeries.SeriesID = "s2"
	assert.NotEqual(t, HeartbeatSampleContentHash(base), HeartbeatSampleContentHash(diffSeries))
}

func TestObservationContentHash(t *testing.T) {
	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	v := 5.4
	base := Observation{UserID: "u1", CanonicalCode: "blood_glucose", TS: ts, Provider: "dexcom", ValueNum: &v}

	assert.Equal(t, ObservationContentHash(base), ObservationContentHash(base))
	assert.Len(t, ObservationContentHash(base), 64)

	later := base
	later.TS = ts.Add(5 * time.Minute)
	assert.NotEqual(t, ObservationContentHash(base), ObservationContentHash(later))

	diffVal := base
	v2 := 6.1
	diffVal.ValueNum = &v2
	assert.NotEqual(t, ObservationContentHash(base), ObservationContentHash(diffVal))

	diffCode := base
	diffCode.CanonicalCode = "heart_rate"
	assert.NotEqual(t, ObservationContentHash(base), ObservationContentHash(diffCode))

	// Categorical observation (no numeric value) stays deterministic.
	cat := Observation{UserID: "u1", CanonicalCode: "mood", TS: ts, Provider: "apple_health", ValueCode: "happy"}
	assert.Equal(t, ObservationContentHash(cat), ObservationContentHash(cat))
	assert.Len(t, ObservationContentHash(cat), 64)
}

// TestHashesStayByteIdentical pins known digests so a refactor can never change a
// dedup identity: rows already stored under these hashes would stop deduplicating.
// The expected values were produced by the original implementation.
func TestHashesStayByteIdentical(t *testing.T) {
	ts := time.Date(2025, 3, 1, 22, 0, 0, 0, time.UTC)
	alt := 12.5
	val := 5.4

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ComputeContentHash", ComputeContentHash(map[string]string{
			"user_id": "abc", "provider": "apple_health", "quantity_type": "heart_rate",
			"start_date": "2025-03-01T14:30:00Z", "value": "72.5",
		}), "85f20e24c8a551aab6e98d29e7394307db4fdecbf653b4dbd8acbc0bc450e692"},
		{"SleepPhaseContentHash", SleepPhaseContentHash(SleepPhase{
			ParentContentHash: "parentA", Phase: "deep", StartDate: ts, EndDate: ts.Add(30 * time.Minute), DurationS: 1800,
		}), "475cae71e660fa496c52ee4cfefa841396d26a6a890b1b58e8405929aa67ec81"},
		{"RoutePointContentHash", RoutePointContentHash(RoutePoint{
			ParentContentHash: "wkt", Timestamp: ts, Latitude: 37.7749, Longitude: -122.4194, AltitudeM: &alt,
		}), "75283698825f77ee9c76a8e9920403719cf6dc676d853ac3bd52591ff622a244"},
		{"ObservationContentHash", ObservationContentHash(Observation{
			UserID: "u1", CanonicalCode: "blood_glucose", TS: ts, Provider: "dexcom", ValueNum: &val,
		}), "a6ec7ff5ab7f2e397da126950a8f67083aabf11f1d9797af7ed77201421e67a3"},
		{"HeartbeatSampleContentHash", HeartbeatSampleContentHash(HeartbeatSample{
			SeriesID: "s1", UserID: "u1", Timestamp: ts, RRMS: 812,
		}), "dc2f77783bacbc21af39d90c81cc941a8dcd73c83b380420be457eea7e73992b"},
		{"ECGVoltageSampleContentHash", ECGVoltageSampleContentHash(ECGVoltageSample{
			ParentContentHash: "ecg1", TimeSinceSampleStart: 0.004, VoltageUV: 120.5,
		}), "ee61e4b26be3f970cd1a67697c3074585124c2dee00f49a57e7629317589e704"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, c.got, c.name)
	}
}

// TenantID is a routing field for the writer, not part of a record's identity: the
// database dedup key already carries tenant_id, and the hashes must stay as stored.
func TestTenantIDDoesNotChangeHashes(t *testing.T) {
	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	a := HeartbeatSample{SeriesID: "s1", UserID: "u1", Timestamp: ts, RRMS: 800}
	b := a
	b.TenantID = "t1"
	assert.Equal(t, HeartbeatSampleContentHash(a), HeartbeatSampleContentHash(b))

	oa := Observation{UserID: "u1", CanonicalCode: "mood", TS: ts, Provider: "apple_health", ValueCode: "happy"}
	ob := oa
	ob.TenantID = "t1"
	assert.Equal(t, ObservationContentHash(oa), ObservationContentHash(ob))
}
