// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"fmt"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// mapECG handles ECG metadata records. ECG metadata records contain
// classification, sampling frequency, heart rate, and a reference to
// the voltage data path. The metadata itself is NOT high-frequency
// (single record per ECG session).
func (m *mapper) mapECG(record map[string]any, userID string) (*model.MapResult, error) {
	startDate, endDate, tz, err := ParseTimestamp(record)
	if err != nil {
		return nil, fmt.Errorf("ecg metadata: %w", err)
	}

	classification, err := extractInt(record, "classification")
	if err != nil {
		return nil, fmt.Errorf("ecg metadata: %w", err)
	}

	symptomsStatus, _ := extractInt(record, "symptomsStatus")
	samplingFrequency, _ := extractFloat(record, "samplingFrequency")
	numberOfMeasurements, _ := extractInt(record, "numberOfVoltageMeasurements")
	voltagePath := stringField(record, "voltagePath")

	var avgHR *float64
	if hr, err := extractFloat(record, "averageHeartRate"); err == nil {
		avgHR = &hr
	}

	metadata := buildMetadata(record, nil)
	if tz != "" {
		metadata["timezone"] = tz
	}

	ecg := model.ECGRecording{
		UserID:               userID,
		StartDate:            startDate,
		EndDate:              endDate,
		Classification:       classification,
		SymptomsStatus:       symptomsStatus,
		AverageHeartRate:     avgHR,
		SamplingFrequency:    samplingFrequency,
		NumberOfMeasurements: numberOfMeasurements,
		VoltagePath:          voltagePath,
		Metadata:             metadata,
	}

	ecg.ContentHash = model.ComputeContentHash(map[string]string{
		"user_id":        userID,
		"provider":       "apple_health",
		"start_date":     startDate.UTC().Format(time.RFC3339Nano),
		"classification": fmt.Sprintf("%d", classification),
	})

	return &model.MapResult{
		ECGRecordings: []model.ECGRecording{ecg},
		HighFrequency: false, // ECG metadata is a single record
	}, nil
}

// mapECGVoltage handles ECG voltage sample records. Each record is a
// single voltage measurement from a ~30-second ECG recording at ~512Hz.
// Value is in volts; we convert to microvolts (multiply by 1,000,000).
func (m *mapper) mapECGVoltage(record map[string]any, userID string) (*model.MapResult, error) {
	timeSinceSampleStart, err := extractFloat(record, "timeSinceSampleStart")
	if err != nil {
		return nil, fmt.Errorf("ecg voltage: %w", err)
	}

	volts, err := extractFloat(record, "value")
	if err != nil {
		return nil, fmt.Errorf("ecg voltage: %w", err)
	}

	// Convert volts to microvolts
	voltageUV := volts * 1_000_000

	sample := model.ECGVoltageSample{
		ECGID:                "", // set by the ETL
		UserID:               userID,
		TimeSinceSampleStart: timeSinceSampleStart,
		VoltageUV:            voltageUV,
		// ParentContentHash is intentionally left empty (zero value).
		// Apple Health ECG voltage files contain samples for exactly one ECG recording per raw object
		// (the ECG metadata record's voltagePath field points to a distinct raw key per recording).
		// The writer's single-parent orphan fallback assigns these samples to the lone
		// parent ECG recording in the batch, which is the correct and sufficient behavior.
		// Multi-parent batches (multiple ECG recordings in one file) are not produced by the
		// Apple Health SDK; samples with empty ParentContentHash are
		// safely skipped in that rare edge case.
	}

	return &model.MapResult{
		ECGSamples:    []model.ECGVoltageSample{sample},
		HighFrequency: true,
	}, nil
}

// extractInt extracts an integer from a record field with best-effort coercion.
func extractInt(record map[string]any, field string) (int, error) {
	v, ok := record[field]
	if !ok {
		return 0, fmt.Errorf("missing %s field", field)
	}

	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case string:
		f, err := extractFloat(record, field)
		if err != nil {
			return 0, err
		}
		return int(f), nil
	default:
		return 0, fmt.Errorf("unexpected type for %s: %T", field, v)
	}
}
