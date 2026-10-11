// SPDX-License-Identifier: AGPL-3.0-only

//go:build docs

package example

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/config"
	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

// TestFixture is the fixture pattern: raw lines in testdata, mapped through
// Map, compared with the canonical records written out by hand.
func TestFixture(t *testing.T) {
	f, err := os.Open("testdata/weigh_in.ndjson")
	require.NoError(t, err)
	defer f.Close()

	c := New()
	var got []float64
	var days []time.Time
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var fields map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &fields))
		res, err := c.Map(connector.RawRecord{UserID: "u1", Provider: Slug, DataType: "weigh_in", Fields: fields})
		require.NoError(t, err)
		require.Len(t, res.QuantitySamples, 1)
		s := res.QuantitySamples[0]
		assert.Equal(t, "body_mass", s.QuantityType)
		assert.Equal(t, "kg", s.Unit)
		assert.Equal(t, "Europe/Paris", s.Metadata["timezone"])
		assert.NotEmpty(t, s.ContentHash)
		got = append(got, s.Value)
		days = append(days, s.StartDate)
	}
	require.NoError(t, sc.Err())
	assert.Equal(t, []float64{72.4, 72.1}, got)
	assert.Equal(t, time.Date(2026, 3, 1, 7, 30, 0, 0, time.UTC), days[0])
}

func TestMapIsDeterministic(t *testing.T) {
	fields := map[string]any{"measuredAt": "2026-03-01T07:30:00Z", "kg": 72.4}
	a, err := New().Map(connector.RawRecord{UserID: "u1", Fields: fields})
	require.NoError(t, err)
	b, err := New().Map(connector.RawRecord{UserID: "u1", Fields: fields})
	require.NoError(t, err)
	assert.Equal(t, a.QuantitySamples[0].ContentHash, b.QuantitySamples[0].ContentHash)
}

func TestMapRejectsMissingTime(t *testing.T) {
	_, err := New().Map(connector.RawRecord{UserID: "u1", Fields: map[string]any{"kg": 70.0}})
	assert.Error(t, err)
}

func TestParseUpload(t *testing.T) {
	c := New()
	require.NoError(t, c.ParseUpload(connector.UploadMeta{}, strings.NewReader(`{"measuredAt":"2026-03-01T07:30:00Z","kg":72.4}`+"\n"), nil))
	assert.Error(t, c.ParseUpload(connector.UploadMeta{}, strings.NewReader("not json\n"), nil))
	assert.Error(t, c.ParseUpload(connector.UploadMeta{}, strings.NewReader("\n"), nil))
}

func TestEnabledWithoutKeys(t *testing.T) {
	r := connector.NewRegistry()
	r.Register(New())
	on := r.Enabled(map[string]config.ProviderKeys{})
	require.Len(t, on, 1)
	_, device := connector.AsDevice(on[0])
	assert.True(t, device)
}
