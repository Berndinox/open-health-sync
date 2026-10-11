// SPDX-License-Identifier: AGPL-3.0-only

package model

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ComputeContentHash produces a deterministic SHA-256 hex string from
// the given key-value pairs. Keys are sorted lexicographically and
// concatenated as "key=value\n" before hashing.
func ComputeContentHash(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write([]byte(fields[k]))
		h.Write([]byte("\n"))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// CanonicalFloat converts a float64 to a deterministic string representation.
// Uses 'f' format with 10 decimal places, then trims trailing zeros and
// trailing decimal point for consistent hashing.
func CanonicalFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 10, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// canonicalTime formats a timestamp deterministically in UTC for hashing.
func canonicalTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// canonicalFloatPtr renders an optional float for hashing; nil becomes empty.
func canonicalFloatPtr(f *float64) string {
	if f == nil {
		return ""
	}
	return CanonicalFloat(*f)
}

// SleepPhaseContentHash derives a stable identity for a sleep phase from its parent
// session hash plus its own distinguishing fields, so sibling phases differ and a
// re-delivered phase dedups via ON CONFLICT (content_hash).
func SleepPhaseContentHash(p SleepPhase) string {
	return ComputeContentHash(map[string]string{
		"parent":     p.ParentContentHash,
		"phase":      p.Phase,
		"start":      canonicalTime(p.StartDate),
		"end":        canonicalTime(p.EndDate),
		"duration_s": strconv.Itoa(p.DurationS),
	})
}

// RoutePointContentHash derives a stable identity for a GPS route point from its parent
// workout hash plus timestamp and position.
func RoutePointContentHash(p RoutePoint) string {
	return ComputeContentHash(map[string]string{
		"parent":    p.ParentContentHash,
		"timestamp": canonicalTime(p.Timestamp),
		"latitude":  CanonicalFloat(p.Latitude),
		"longitude": CanonicalFloat(p.Longitude),
		"altitude":  canonicalFloatPtr(p.AltitudeM),
	})
}

// ECGVoltageSampleContentHash derives a stable identity for an ECG voltage sample from
// its parent recording hash plus its time offset and voltage.
func ECGVoltageSampleContentHash(s ECGVoltageSample) string {
	return ComputeContentHash(map[string]string{
		"parent":  s.ParentContentHash,
		"offset":  CanonicalFloat(s.TimeSinceSampleStart),
		"voltage": CanonicalFloat(s.VoltageUV),
	})
}

// HeartbeatSampleContentHash derives a stable identity for a heartbeat (RR interval)
// sample. Heartbeats have no parent record, so identity is the series plus timestamp.
func HeartbeatSampleContentHash(s HeartbeatSample) string {
	return ComputeContentHash(map[string]string{
		"series_id": s.SeriesID,
		"user_id":   s.UserID,
		"timestamp": canonicalTime(s.Timestamp),
		"rr_ms":     CanonicalFloat(s.RRMS),
	})
}

// ObservationContentHash derives a stable identity for a generic observation from its
// owner, code, timestamp, provider, and value, so a re-delivered observation dedups.
func ObservationContentHash(o Observation) string {
	valNum := ""
	if o.ValueNum != nil {
		valNum = CanonicalFloat(*o.ValueNum)
	}
	return ComputeContentHash(map[string]string{
		"user_id":    o.UserID,
		"code":       o.CanonicalCode,
		"ts":         canonicalTime(o.TS),
		"provider":   o.Provider,
		"value_num":  valNum,
		"value_code": o.ValueCode,
		"value_text": o.ValueText,
	})
}
