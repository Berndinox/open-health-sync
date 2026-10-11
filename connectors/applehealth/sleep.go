// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"fmt"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// mapSleep transforms a raw Apple Health sleep analysis record into
// SleepPhase records and optionally a SleepSession (for in_bed records).
func (m *mapper) mapSleep(record map[string]any, userID string) (*model.MapResult, error) {
	// Extract sleep value (integer enum)
	valueRaw, _ := record["value"].(float64)
	valueInt := int(valueRaw)

	phase, ok := sleepAnalysisValues[valueInt]
	if !ok {
		phase = fmt.Sprintf("unknown_%d", valueInt)
	}
	// Canonicalize every stage label through the shared four-value enum so
	// apple_health stores the same vocabulary as the other providers. in_bed is
	// the one exception: it stays verbatim (never folded to awake) because
	// session boundary logic keys on it.
	if phase != "in_bed" {
		phase = model.CanonicalSleepStage(phase)
	}

	startDate, endDate, tz, err := ParseTimestamp(record)
	if err != nil {
		return nil, fmt.Errorf("sleep: %w", err)
	}

	durationS := int(endDate.Sub(startDate).Seconds())

	result := &model.MapResult{
		SleepPhases: []model.SleepPhase{
			{
				UserID:    userID,
				Phase:     phase,
				StartDate: startDate,
				EndDate:   endDate,
				DurationS: durationS,
			},
		},
	}

	// For "in_bed" records (value 0), also create a SleepSession
	if valueInt == 0 {
		metadata := buildMetadata(record, nil)
		if tz != "" {
			metadata["timezone"] = tz
		}

		session := model.SleepSession{
			UserID:       userID,
			Provider:     "apple_health",
			BedtimeStart: startDate,
			BedtimeEnd:   endDate,
			Metadata:     metadata,
		}

		session.ContentHash = model.ComputeContentHash(map[string]string{
			"user_id":       userID,
			"provider":      "apple_health",
			"bedtime_start": startDate.UTC().Format(time.RFC3339Nano),
			"bedtime_end":   endDate.UTC().Format(time.RFC3339Nano),
		})

		result.SleepSessions = []model.SleepSession{session}
		// Set ParentContentHash on the in_bed phase to match the sibling session.
		result.SleepPhases[0].ParentContentHash = session.ContentHash
	}

	return result, nil
}
