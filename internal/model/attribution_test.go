// SPDX-License-Identifier: AGPL-3.0-only

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFromRecord(t *testing.T) {
	assert.Equal(t, "fitbit", UpstreamFromRecord(map[string]any{"source_slug": "fitbit"}))
	assert.Equal(t, "polar", UpstreamFromRecord(map[string]any{"sourceProvider": "polar"}))
	assert.Empty(t, UpstreamFromRecord(map[string]any{"provider": "oura"}),
		"the provider field is the integration, not the upstream")
	assert.Empty(t, UpstreamFromRecord(map[string]any{}))
}

// A direct integration must come out with attribution equal to the provider, which is
// what makes this change additive for oura, withings, whoop and apple_health.
func TestCanonicalizeSourceAttribution_DirectProvider(t *testing.T) {
	mr := &MapResult{QuantitySamples: []QuantitySample{
		{QuantityType: QTHeartRate, Provider: "oura"},
		{QuantityType: QTStepCount, Provider: "oura"},
	}}
	CanonicalizeSourceAttribution(mr, map[string]any{"provider": "Oura"}, "Oura")

	for _, s := range mr.QuantitySamples {
		assert.Equal(t, "oura", s.SourceSlug)
	}
}

// An aggregator's samples must carry the real sensor vendor, not the integration.
func TestCanonicalizeSourceAttribution_Aggregator(t *testing.T) {
	mr := &MapResult{QuantitySamples: []QuantitySample{
		{QuantityType: QTHeartRate, Provider: "aggregator"},
	}}
	CanonicalizeSourceAttribution(mr, map[string]any{"source_slug": "Fitbit"}, "aggregator")

	assert.Equal(t, "fitbit", mr.QuantitySamples[0].SourceSlug)
	assert.Equal(t, "aggregator", mr.QuantitySamples[0].Provider,
		"provider stays the integration: it is who holds the token and who to re-pull from")
}

// A free-text device label still yields usable attribution rather than being dropped
// or folded into the aggregator.
func TestCanonicalizeSourceAttribution_FreeTextUpstream(t *testing.T) {
	mr := &MapResult{QuantitySamples: []QuantitySample{{QuantityType: QTHeartRate, Provider: "aggregator"}}}
	CanonicalizeSourceAttribution(mr, map[string]any{"source_slug": "Fitbit Charge 6"}, "aggregator")
	assert.Equal(t, "fitbit_charge_6", mr.QuantitySamples[0].SourceSlug)
}

// A mapper that read per-record attribution out of its own payload knows better than
// the file-level fallback, so its value survives (normalized).
func TestCanonicalizeSourceAttribution_MapperSetValueWins(t *testing.T) {
	mr := &MapResult{QuantitySamples: []QuantitySample{
		{QuantityType: QTHeartRate, Provider: "aggregator", SourceSlug: "Withings"},
		{QuantityType: QTStepCount, Provider: "aggregator"},
	}}
	CanonicalizeSourceAttribution(mr, map[string]any{"source_slug": "fitbit"}, "aggregator")

	assert.Equal(t, "withings", mr.QuantitySamples[0].SourceSlug, "per-sample value wins, normalized")
	assert.Equal(t, "fitbit", mr.QuantitySamples[1].SourceSlug, "the rest take the record fallback")
}

func TestCanonicalizeSourceAttribution_IsIdempotent(t *testing.T) {
	mr := &MapResult{QuantitySamples: []QuantitySample{{QuantityType: QTHeartRate, Provider: "aggregator"}}}
	record := map[string]any{"source_slug": "Fitbit"}

	CanonicalizeSourceAttribution(mr, record, "aggregator")
	first := mr.QuantitySamples[0].SourceSlug
	CanonicalizeSourceAttribution(mr, record, "aggregator")

	assert.Equal(t, first, mr.QuantitySamples[0].SourceSlug)
}

func TestCanonicalizeSourceAttribution_NilAndEmpty(t *testing.T) {
	CanonicalizeSourceAttribution(nil, nil, "oura") // must not panic
	mr := &MapResult{}
	CanonicalizeSourceAttribution(mr, nil, "oura")
	require.Empty(t, mr.QuantitySamples)
}
