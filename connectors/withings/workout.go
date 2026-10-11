// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"fmt"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// workoutDataKeys are the "data" fields copied into the workout metadata.
var workoutDataKeys = []string{
	"calories", "steps", "distance", "hr_average", "hr_min", "hr_max",
	"hr_zone_0", "hr_zone_1", "hr_zone_2", "hr_zone_3", "hr_zone_4",
	"spo2_average", "intensity", "manual_distance", "manual_calories", "pause_duration", "elevation",
}

// mapWorkout turns one getworkouts series into a Workout.
func mapWorkout(line []byte, userID string) (*model.MapResult, error) {
	var head struct {
		Startdate *int64 `json:"startdate"`
		Enddate   *int64 `json:"enddate"`
	}
	if err := decode(DataTypeWorkout, line, &head); err != nil {
		return nil, err
	}
	if head.Startdate == nil || head.Enddate == nil {
		return nil, fmt.Errorf("withings: workout: missing startdate or enddate")
	}
	var w Workout
	if err := decode(DataTypeWorkout, line, &w); err != nil {
		return nil, err
	}
	start := ParseWithingsTimestamp(w.Startdate)
	end := ParseWithingsTimestamp(w.Enddate)
	dur := int(end.Sub(start).Seconds())
	data := w.Data
	if data == nil {
		data = map[string]any{}
	}

	wt := withingsWorkoutCategory(w.Category)
	activity := wt.activity
	md := map[string]any{
		"activity_type":     activity,
		"withings_category": w.Category,
	}
	if wt.indoor {
		md["indoor"] = true
	}
	if stepBasedCategories[w.Category] {
		md["distance_is_step_based"] = true
	}
	for _, k := range workoutDataKeys {
		if v, ok := data[k]; ok && v != nil {
			md[k] = v
		}
	}
	if v, ok := floatField(data, "hr_average"); ok {
		md["avg_heart_rate"] = v
	}
	if v, ok := floatField(data, "hr_min"); ok {
		md["min_heart_rate"] = v
	}
	if v, ok := floatField(data, "hr_max"); ok {
		md["max_heart_rate"] = v
	}
	md = withTimezone(md, w.Timezone)

	externalID := string(w.ID)
	out := model.Workout{
		UserID:           userID,
		Provider:         Slug,
		ExternalID:       externalID,
		ActivityType:     activity,
		StartDate:        start,
		EndDate:          end,
		DurationS:        &dur,
		ActiveEnergyKcal: optFloat(data, "calories"),
		DistanceM:        optFloat(data, "distance"),
		AvgHeartRate:     optFloat(data, "hr_average"),
		MaxHeartRate:     optFloat(data, "hr_max"),
		Metadata:         md,
	}
	out.ContentHash = model.ComputeContentHash(map[string]string{
		"user_id":     userID,
		"provider":    Slug,
		"start_date":  start.Format(time.RFC3339),
		"external_id": externalID,
	})
	return &model.MapResult{Workouts: []model.Workout{out}}, nil
}

// workoutType is the canonical activity of a Withings workout category, in
// the vocabulary the Apple Health connector uses, and whether Withings
// recorded it indoors.
type workoutType struct {
	activity string
	indoor   bool
}

// withingsWorkoutCategory maps a getworkouts category id to the canonical
// activity name ("other" for an unknown id).
// https://developer.withings.com/api-reference#tag/measure/operation/measure-getworkouts
func withingsWorkoutCategory(cat int) workoutType {
	if t, ok := workoutCategories[cat]; ok {
		return t
	}
	return workoutType{activity: "other"}
}

var workoutCategories = map[int]workoutType{
	1: {activity: "walking"}, 2: {activity: "running"}, 3: {activity: "hiking"},
	4: {activity: "skating_sports"}, 5: {activity: "cycling"}, 6: {activity: "cycling"},
	7: {activity: "swimming"}, 8: {activity: "surfing_sports"}, 9: {activity: "water_sports"},
	10: {activity: "water_sports"}, 11: {activity: "surfing_sports"}, 12: {activity: "tennis"},
	13: {activity: "table_tennis"}, 14: {activity: "squash"}, 15: {activity: "badminton"},
	16: {activity: "traditional_strength_training"}, 17: {activity: "functional_strength_training"},
	18: {activity: "elliptical"}, 19: {activity: "pilates"}, 20: {activity: "basketball"},
	21: {activity: "soccer"}, 22: {activity: "american_football"}, 23: {activity: "rugby"},
	24: {activity: "volleyball"}, 25: {activity: "water_polo"}, 26: {activity: "equestrian_sports"},
	27: {activity: "golf"}, 28: {activity: "yoga"}, 29: {activity: "dance"}, 30: {activity: "boxing"},
	31: {activity: "fencing"}, 32: {activity: "wrestling"}, 33: {activity: "martial_arts"},
	34: {activity: "downhill_skiing"}, 35: {activity: "snowboarding"}, 36: {activity: "other"},
	128: {activity: "other"}, 187: {activity: "rowing"}, 188: {activity: "dance"},
	191: {activity: "baseball"}, 192: {activity: "handball"}, 193: {activity: "hockey"},
	194: {activity: "hockey"}, 195: {activity: "climbing"}, 196: {activity: "skating_sports"},
	272: {activity: "mixed_cardio"},
	306: {activity: "running", indoor: true}, 307: {activity: "cycling", indoor: true},
	308: {activity: "walking", indoor: true},
}

// stepBasedCategories are the categories whose distance Withings derives from
// steps rather than from GPS or a sensor.
var stepBasedCategories = map[int]bool{1: true, 2: true, 3: true, 306: true, 308: true}
