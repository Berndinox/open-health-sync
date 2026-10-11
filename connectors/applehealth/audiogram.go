// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// mapAudiogram handles audiogram records from Apple Health.
// These arrive with quantity_type = "HKDataTypeIdentifierAudiogramSample".
// The sensitivity points are stored in _raw as a JSON array.
// Output: one QuantitySample with quantity_type "audiogram" where
// value = number of sensitivity points and the array is JSON-encoded in metadata.
func (m *mapper) mapAudiogram(record map[string]any, userID string) (*model.MapResult, error) {
	startDate, endDate, tz, err := ParseTimestamp(record)
	if err != nil {
		return nil, fmt.Errorf("audiogram: %w", err)
	}

	externalID := stringField(record, "external_id")
	if externalID == "" {
		externalID = stringField(record, "uuid")
	}
	sourceName := stringField(record, "sourceName")
	deviceID := stringField(record, "device")

	// Extract sensitivity points from _raw
	var sensitivityPoints any
	var pointCount float64
	if raw, ok := record["_raw"].(map[string]any); ok {
		sensitivityPoints = raw["sensitivityPoints"]
		if pts, ok := sensitivityPoints.([]any); ok {
			pointCount = float64(len(pts))
		}
	}

	meta := map[string]any{
		"provider": "apple_health",
	}
	if tz != "" {
		meta["timezone"] = tz
	}
	if sensitivityPoints != nil {
		if spJSON, err := json.Marshal(sensitivityPoints); err == nil {
			meta["sensitivity_points"] = string(spJSON)
		}
	}

	sample := model.QuantitySample{
		UserID:       userID,
		Provider:     "apple_health",
		QuantityType: "audiogram",
		Value:        pointCount,
		Unit:         "points",
		StartDate:    startDate,
		EndDate:      endDate,
		ExternalID:   externalID,
		SourceName:   sourceName,
		DeviceID:     deviceID,
		Metadata:     meta,
		ContentHash: model.ComputeContentHash(map[string]string{
			"user_id":     userID,
			"provider":    "apple_health",
			"type":        "audiogram",
			"external_id": externalID,
			"start_date":  startDate.Format(time.RFC3339),
		}),
	}

	return &model.MapResult{QuantitySamples: []model.QuantitySample{sample}}, nil
}
