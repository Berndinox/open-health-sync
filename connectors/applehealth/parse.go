// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"fmt"
	"time"
)

// ParseTimestamp extracts start_date and end_date from a record,
// normalizing to UTC. If HKTimeZone is present in metadata,
// it is preserved in the returned tz string but does NOT affect the
// timestamp value (the SDK already writes UTC via ISO8601DateFormatter).
func ParseTimestamp(record map[string]any) (startDate, endDate time.Time, tz string, err error) {
	startStr, ok := record["startDate"].(string)
	if !ok {
		return time.Time{}, time.Time{}, "", fmt.Errorf("missing startDate")
	}

	startDate, err = parseISO8601(startStr)
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("invalid startDate %q: %w", startStr, err)
	}

	// endDate is optional for some record types; default to startDate if missing
	if endStr, ok := record["endDate"].(string); ok {
		endDate, err = parseISO8601(endStr)
		if err != nil {
			return time.Time{}, time.Time{}, "", fmt.Errorf("invalid endDate %q: %w", endStr, err)
		}
	} else {
		endDate = startDate
	}

	tz = timezoneOf(record)

	// Ensure UTC (should already be UTC from ISO8601DateFormatter)
	startDate = startDate.UTC()
	endDate = endDate.UTC()

	return startDate, endDate, tz, nil
}

// parseISO8601 tries RFC3339Nano first, then RFC3339 to handle
// varying precision from iOS's ISO8601DateFormatter.
func parseISO8601(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// withTimezone records the sample's IANA time zone, when the line gave one,
// under metadata["timezone"]: the ETL derives local days from it.
func withTimezone(md map[string]any, tz string) map[string]any {
	if tz != "" {
		md["timezone"] = tz
	}
	return md
}

// timezoneOf returns the IANA zone HealthKit recorded in the line's metadata
// (HKTimeZone), or "".
func timezoneOf(record map[string]any) string {
	if md, ok := record["metadata"].(map[string]any); ok {
		if tz, ok := md["HKTimeZone"].(string); ok {
			return tz
		}
	}
	return ""
}
