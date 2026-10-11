// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimestampRFC3339(t *testing.T) {
	record := map[string]any{
		"startDate": "2025-03-01T14:30:00Z",
		"endDate":   "2025-03-01T14:30:00Z",
	}

	start, end, tz, err := ParseTimestamp(record)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2025, 3, 1, 14, 30, 0, 0, time.UTC), start)
	assert.Equal(t, time.Date(2025, 3, 1, 14, 30, 0, 0, time.UTC), end)
	assert.Empty(t, tz)
}

func TestParseTimestampRFC3339Nano(t *testing.T) {
	record := map[string]any{
		"startDate": "2025-03-01T14:30:00.123456789Z",
		"endDate":   "2025-03-01T14:30:00.123456789Z",
	}

	start, _, _, err := ParseTimestamp(record)
	require.NoError(t, err)
	assert.Equal(t, 123456789, start.Nanosecond())
}

func TestParseTimestampTimezoneExtraction(t *testing.T) {
	record := map[string]any{
		"startDate": "2025-03-01T14:30:00Z",
		"endDate":   "2025-03-01T14:30:00Z",
		"metadata": map[string]any{
			"HKTimeZone": "Europe/Paris",
		},
	}

	_, _, tz, err := ParseTimestamp(record)
	require.NoError(t, err)
	assert.Equal(t, "Europe/Paris", tz)
}

func TestParseTimestampNoTimezone(t *testing.T) {
	record := map[string]any{
		"startDate": "2025-03-01T14:30:00Z",
		"endDate":   "2025-03-01T14:30:00Z",
		"metadata":  map[string]any{},
	}

	_, _, tz, err := ParseTimestamp(record)
	require.NoError(t, err)
	assert.Empty(t, tz)
}

func TestParseTimestampMissingStartDate(t *testing.T) {
	record := map[string]any{
		"endDate": "2025-03-01T14:30:00Z",
	}

	_, _, _, err := ParseTimestamp(record)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing startDate")
}

func TestParseTimestampInvalidDate(t *testing.T) {
	record := map[string]any{
		"startDate": "not-a-date",
		"endDate":   "2025-03-01T14:30:00Z",
	}

	_, _, _, err := ParseTimestamp(record)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid startDate")
}

func TestParseTimestampUTCNormalization(t *testing.T) {
	// Even with an offset, the result should be UTC
	record := map[string]any{
		"startDate": "2025-03-01T14:30:00+05:00",
		"endDate":   "2025-03-01T14:30:00+05:00",
	}

	start, _, _, err := ParseTimestamp(record)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, start.Location())
	assert.Equal(t, 9, start.Hour(), "14:30+05:00 should become 09:30 UTC")
}

func TestParseTimestampMissingEndDateDefaultsToStart(t *testing.T) {
	record := map[string]any{
		"startDate": "2025-03-01T14:30:00Z",
	}

	start, end, _, err := ParseTimestamp(record)
	require.NoError(t, err)
	assert.Equal(t, start, end, "missing endDate should default to startDate")
}
