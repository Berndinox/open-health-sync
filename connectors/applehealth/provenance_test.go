// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func provMapper() *Connector { return New() }

func heartRateRecord(extra map[string]any) map[string]any {
	rec := map[string]any{
		"quantityType":   "HKQuantityTypeIdentifierHeartRate",
		"value":          float64(58),
		"unit":           "count/min",
		"startDate":      "2026-08-31T10:00:00.000000000Z",
		"endDate":        "2026-08-31T10:01:00.000000000Z",
		"sourceName":     "Test Apple Watch",
		"sourceBundleId": "com.apple.health",
		"device":         "Apple Watch",
	}
	for k, v := range extra {
		rec[k] = v
	}
	return rec
}

// The whole point of the change: device_id is HKDevice.name, "Apple Watch",
// which no per-model accuracy row can match. productType is "Watch7,2".
func TestQuantity_CarriesTheModelIdentifiers(t *testing.T) {
	res, err := provMapper().mapRecord(heartRateRecord(map[string]any{
		"productType": "Watch7,2",
		"deviceModel": "Watch",
	}), "user-1")
	require.NoError(t, err)
	require.Len(t, res.QuantitySamples, 1)
	s := res.QuantitySamples[0]

	assert.Equal(t, "Watch7,2", s.ProductType)
	assert.Equal(t, "Watch", s.DeviceModel)
	assert.Equal(t, "Apple Watch", s.DeviceID,
		"device_id keeps its old meaning, it is not repurposed into the model")
}

// Three states, not two. A client that predates the field genuinely does not
// know, and storing that as "measured" would be the same fabrication this
// chantier exists to remove.
func TestQuantity_WasUserEnteredHasThreeStates(t *testing.T) {
	entered, err := provMapper().mapRecord(heartRateRecord(map[string]any{"wasUserEntered": true}), "u")
	require.NoError(t, err)
	require.NotNil(t, entered.QuantitySamples[0].WasUserEntered)
	assert.True(t, *entered.QuantitySamples[0].WasUserEntered)

	measured, err := provMapper().mapRecord(heartRateRecord(map[string]any{"wasUserEntered": false}), "u")
	require.NoError(t, err)
	require.NotNil(t, measured.QuantitySamples[0].WasUserEntered,
		"false must survive as false rather than collapsing into absent")
	assert.False(t, *measured.QuantitySamples[0].WasUserEntered)

	silent, err := provMapper().mapRecord(heartRateRecord(nil), "u")
	require.NoError(t, err)
	assert.Nil(t, silent.QuantitySamples[0].WasUserEntered,
		"an older client that never sent the field must stay unknown")
}

// buildMetadata is an allow-list, not a passthrough. The four provenance fields
// that get no column survive only because they are named in it, and a field
// dropped there is dropped without a trace: the write succeeds, the sample looks
// healthy, and the value is simply gone.
func TestQuantity_ForensicProvenanceSurvivesTheAllowList(t *testing.T) {
	res, err := provMapper().mapRecord(heartRateRecord(map[string]any{
		"osVersion":             "18.1.0",
		"deviceHardwareVersion": "Watch7,2",
		"deviceSoftwareVersion": "11.1",
		"deviceManufacturer":    "Apple Inc.",
	}), "u")
	require.NoError(t, err)
	md := res.QuantitySamples[0].Metadata

	assert.Equal(t, "18.1.0", md["osVersion"])
	assert.Equal(t, "Watch7,2", md["deviceHardwareVersion"])
	assert.Equal(t, "11.1", md["deviceSoftwareVersion"])
	assert.Equal(t, "Apple Inc.", md["deviceManufacturer"])
}

// A record from a client that predates 0.2.0 must map exactly as it did.
func TestQuantity_PreProvenanceRecordIsUnchanged(t *testing.T) {
	res, err := provMapper().mapRecord(heartRateRecord(nil), "u")
	require.NoError(t, err)
	s := res.QuantitySamples[0]

	assert.Empty(t, s.ProductType)
	assert.Empty(t, s.DeviceModel)
	assert.Nil(t, s.WasUserEntered)
	assert.InDelta(t, 58, s.Value, 0.001, "the reading itself is untouched")
	assert.NotEmpty(t, s.ContentHash)
}
