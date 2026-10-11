// SPDX-License-Identifier: AGPL-3.0-only

package etl

import "github.com/soma-intelligence/open-health-sync/internal/model"

// prunableRecordTypes are the record types whose rejection can be scoped to the
// one offending entry, because the entry is a LEAF: removing it leaves the rest
// of the mapped record coherent and writable.
//
// Anything absent from this list is structural. A sleep session whose bedtimes
// are out of order, or a workout that ends before it starts, is the parent its
// phases and route points hang off: writing the rest without it would persist
// children with nothing to attach to. An unrecognised record type is treated as
// structural too, so a new validator rule fails safe (whole record parked) until
// someone decides how to scope it.
var prunableRecordTypes = map[string]bool{
	"quantity_sample":    true,
	"category_sample":    true,
	"route_point":        true,
	"nutrition_log":      true,
	"ecg_voltage_sample": true,
	"heartbeat_sample":   true,
}

// pruneRejected removes from mr exactly the entries named by rejects, leaving
// everything else writable. It returns the rejects it could NOT scope; when that
// is non-empty mr is left untouched and the caller drops the whole record.
//
// This exists because one raw record is not one sample. A provider's daily
// record can map to a summary plus the whole day's heart-rate, HRV and
// saturation series; a sleep record maps to a session plus its phases plus the
// night's HR series.
// Dropping the MapResult over a single 310 bpm optical artifact erased the entire
// day, and reason_code 'validation' is deterministic-permanent to the drainer, so
// it was never replayed: the loss was permanent and invisible.
func pruneRejected(mr *model.MapResult, rejects []ValidationError) []ValidationError {
	if mr == nil || len(rejects) == 0 {
		return nil
	}

	var structural []ValidationError
	for _, e := range rejects {
		if !prunableRecordTypes[e.RecordType] {
			structural = append(structural, e)
		}
	}
	if len(structural) > 0 {
		return structural
	}

	drop := make(map[string]map[int]bool, 2)
	for _, e := range rejects {
		byType, ok := drop[e.RecordType]
		if !ok {
			byType = make(map[int]bool)
			drop[e.RecordType] = byType
		}
		byType[e.Index] = true
	}

	for recordType, idx := range drop {
		switch recordType {
		case "quantity_sample":
			mr.QuantitySamples = dropIndices(mr.QuantitySamples, idx)
		case "category_sample":
			mr.CategorySamples = dropIndices(mr.CategorySamples, idx)
		case "route_point":
			mr.RoutePoints = dropIndices(mr.RoutePoints, idx)
		case "nutrition_log":
			mr.NutritionLogs = dropIndices(mr.NutritionLogs, idx)
		case "ecg_voltage_sample":
			mr.ECGSamples = dropIndices(mr.ECGSamples, idx)
		case "heartbeat_sample":
			mr.HeartbeatSamples = dropIndices(mr.HeartbeatSamples, idx)
		}
	}
	return nil
}

// dropIndices removes the positions named by idx, preserving order. The slice is
// compacted in place: the caller owns the MapResult by this point.
func dropIndices[T any](s []T, idx map[int]bool) []T {
	if len(idx) == 0 {
		return s
	}
	out := s[:0]
	for i, v := range s {
		if idx[i] {
			continue
		}
		out = append(out, v)
	}
	return out
}
