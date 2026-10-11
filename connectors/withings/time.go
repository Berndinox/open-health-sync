// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"time"
	// Embed the zone database so LoadLocation works in minimal images.
	_ "time/tzdata"
)

// ParseWithingsTimestamp converts a Withings Unix epoch to time.Time UTC.
func ParseWithingsTimestamp(epoch int64) time.Time {
	return time.Unix(epoch, 0).UTC()
}

// zone returns the IANA location named by Withings, or nil when the name is
// empty or unknown.
func zone(name string) *time.Location {
	if name == "" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

// withTimezone sets metadata "timezone" (R17) when name is a valid IANA zone.
func withTimezone(md map[string]any, name string) map[string]any {
	if zone(name) == nil {
		return md
	}
	if md == nil {
		md = map[string]any{}
	}
	md["timezone"] = name
	return md
}

// timeSpan is one request window of a chunked fetch.
type timeSpan struct {
	start, end time.Time
}

// getChunksBetween splits [start, end) into consecutive spans of at most
// chunk. An empty or inverted window gives no span.
func getChunksBetween(start, end time.Time, chunk time.Duration) []timeSpan {
	var spans []timeSpan
	for s := start; s.Before(end); {
		e := s.Add(chunk)
		if e.After(end) {
			e = end
		}
		spans = append(spans, timeSpan{start: s, end: e})
		s = e
	}
	return spans
}

// utcDay is t's calendar day in UTC, at midnight. Raw objects are grouped by
// it.
func utcDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
