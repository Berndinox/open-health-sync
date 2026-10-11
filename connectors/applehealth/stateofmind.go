// SPDX-License-Identifier: AGPL-3.0-only

package applehealth

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// mapStateOfMind handles state-of-mind records from Apple Health.
// These arrive with quantity_type = "HKStateOfMindType".
// Fields: kind (int: 1=momentaryEmotion, 2=dailyMood), valence (float -1..1),
// labels ([]int), associations ([]int), all in _raw.
// Output: one CategorySample with category_type "state_of_mind" where
// value_int = kind and metadata contains valence, labels, associations.
func (m *mapper) mapStateOfMind(record map[string]any, userID string) (*model.MapResult, error) {
	startDate, endDate, tz, err := ParseTimestamp(record)
	if err != nil {
		return nil, fmt.Errorf("stateofmind: %w", err)
	}

	externalID := stringField(record, "external_id")
	if externalID == "" {
		externalID = stringField(record, "uuid")
	}
	sourceName := stringField(record, "sourceName")

	// Extract state of mind fields from _raw
	var kind int
	var valence float64
	var labels, associations any

	if raw, ok := record["_raw"].(map[string]any); ok {
		if k, ok := raw["kind"].(float64); ok {
			kind = int(k)
		}
		if v, ok := raw["valence"].(float64); ok {
			valence = v
		}
		labels = raw["labels"]
		associations = raw["associations"]
	}

	meta := map[string]any{
		"provider": "apple_health",
		"valence":  fmt.Sprintf("%.4f", valence),
	}
	if tz != "" {
		meta["timezone"] = tz
	}
	if labels != nil {
		if lJSON, err := json.Marshal(labels); err == nil {
			meta["labels"] = string(lJSON)
		}
	}
	if associations != nil {
		if aJSON, err := json.Marshal(associations); err == nil {
			meta["associations"] = string(aJSON)
		}
	}

	sample := model.CategorySample{
		UserID:       userID,
		Provider:     "apple_health",
		CategoryType: "state_of_mind",
		ValueInt:     kind,
		StartDate:    startDate,
		EndDate:      endDate,
		ExternalID:   externalID,
		SourceName:   sourceName,
		Metadata:     meta,
		ContentHash: model.ComputeContentHash(map[string]string{
			"user_id":     userID,
			"provider":    "apple_health",
			"type":        "state_of_mind",
			"external_id": externalID,
			"start_date":  startDate.Format(time.RFC3339),
		}),
	}

	return &model.MapResult{CategorySamples: []model.CategorySample{sample}}, nil
}
