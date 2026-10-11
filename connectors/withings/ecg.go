// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"fmt"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// mapECG maps one ECG line. The AFib classification always becomes a
// QuantitySample. When the line carries the waveform ("signal"), it also
// becomes one ECGRecording plus one ECGVoltageSample per value: Withings
// returns microvolts, so values pass through unchanged, and the offset of
// sample i is i / sampling frequency (1 Hz when the frequency is missing,
// degraded but lossless).
func mapECG(line []byte, userID string) (*model.MapResult, error) {
	var r HeartRecord
	if err := decode(DataTypeECG, line, &r); err != nil {
		return nil, err
	}
	if r.Timestamp == 0 {
		return nil, fmt.Errorf("withings: ecg: missing timestamp")
	}
	ts := ParseWithingsTimestamp(r.Timestamp)
	externalID := fmt.Sprintf("withings_ecg_%d_%d", r.Timestamp, r.ECG.SignalID)

	md := withTimezone(map[string]any{
		"heart_rate": r.HeartRate,
		"signal_id":  r.ECG.SignalID,
		"device_id":  r.DeviceID,
		"model":      r.Model,
		"afib":       r.ECG.AFib,
	}, r.Timezone)
	afib := model.QuantitySample{
		UserID:          userID,
		Provider:        Slug,
		SourceSlug:      Slug,
		SourceName:      sourceName,
		DeviceID:        r.DeviceID,
		ExternalID:      externalID,
		QuantityType:    "ecg_afib_classification",
		Value:           float64(r.ECG.AFib),
		Unit:            "classification",
		IsPrimarySource: true,
		StartDate:       ts,
		EndDate:         ts,
		Metadata:        md,
	}
	afib.ContentHash = model.ComputeContentHash(map[string]string{
		"user_id":     userID,
		"provider":    Slug,
		"type":        "ecg_afib_classification",
		"external_id": externalID,
		"start_date":  ts.Format(time.RFC3339),
	})
	res := &model.MapResult{QuantitySamples: []model.QuantitySample{afib}}

	if r.Signal == nil || len(r.Signal.Signal) == 0 {
		return res, nil
	}
	sig := r.Signal
	n := len(sig.Signal)
	var dur time.Duration
	if sig.SamplingFrequency > 0 {
		dur = time.Duration(float64(n) / sig.SamplingFrequency * float64(time.Second))
	}
	recMeta := withTimezone(map[string]any{
		"signal_id":          r.ECG.SignalID,
		"afib":               r.ECG.AFib,
		"device_id":          r.DeviceID,
		"model":              r.Model,
		"wear_position":      sig.WearPosition,
		"sampling_frequency": sig.SamplingFrequency,
	}, r.Timezone)
	rec := model.ECGRecording{
		UserID:               userID,
		StartDate:            ts,
		EndDate:              ts.Add(dur),
		Classification:       r.ECG.AFib,
		SamplingFrequency:    sig.SamplingFrequency,
		NumberOfMeasurements: n,
		Metadata:             recMeta,
	}
	if r.HeartRate > 0 {
		rec.AverageHeartRate = ptrFloat(r.HeartRate)
	}
	rec.ContentHash = model.ComputeContentHash(map[string]string{
		"user_id":        userID,
		"provider":       Slug,
		"type":           "ecg_recording",
		"start_date":     ts.Format(time.RFC3339),
		"classification": fmt.Sprintf("%d", r.ECG.AFib),
	})
	res.ECGRecordings = []model.ECGRecording{rec}
	res.HighFrequency = true

	hz := sig.SamplingFrequency
	if hz <= 0 {
		hz = 1
	}
	res.ECGSamples = make([]model.ECGVoltageSample, 0, n)
	for i, uv := range sig.Signal {
		res.ECGSamples = append(res.ECGSamples, model.ECGVoltageSample{
			UserID:               userID,
			TimeSinceSampleStart: float64(i) / hz,
			VoltageUV:            uv,
			ParentContentHash:    rec.ContentHash,
		})
	}
	return res, nil
}
