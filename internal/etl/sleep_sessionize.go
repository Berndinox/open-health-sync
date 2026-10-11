// SPDX-License-Identifier: AGPL-3.0-only

package etl

import (
	"sort"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// sleepSessionGap is the largest gap between consecutive sleep-stage segments
// that still belongs to the same night. watchOS emits stage segments roughly
// back to back; a larger gap starts a new session.
const sleepSessionGap = time.Hour

// synthesizeSleepSessions builds one SleepSession per night from "orphan" sleep
// phases (ParentContentHash == "", excluding in_bed markers).
//
// Modern watchOS (since ~mid-2024) reports sleep as a series of stage segments
// (canonicalized by the mapper to light/deep/rem and awake) with NO in_bed
// record. The mapper therefore
// emits phases but no session, and writeSleepWithPhases (which writes phases
// per-session) dropped them all, so sleep silently stopped landing in the DB.
// This groups those phases by time gap into sessions, computes the standard
// aggregates, and stamps each grouped phase's ParentContentHash with the
// synthesized session's hash so the existing per-session phase writer persists
// them. provider is the job's provider: it is part of the session identity.
func synthesizeSleepSessions(provider string, phases []model.SleepPhase) []model.SleepSession {
	idx := make([]int, 0, len(phases))
	for i := range phases {
		if phases[i].ParentContentHash == "" && phases[i].Phase != "in_bed" {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return nil
	}
	sort.Slice(idx, func(a, b int) bool {
		return phases[idx[a]].StartDate.Before(phases[idx[b]].StartDate)
	})

	var sessions []model.SleepSession
	group := make([]int, 0, len(idx))
	var prevEnd time.Time

	flush := func() {
		if len(group) == 0 {
			return
		}
		start := phases[group[0]].StartDate
		end := start
		var total, deep, rem, light, awake int
		for _, gi := range group {
			p := phases[gi]
			if p.EndDate.After(end) {
				end = p.EndDate
			}
			switch p.Phase {
			case model.SleepStageDeep:
				deep += p.DurationS
				total += p.DurationS
			case model.SleepStageREM:
				rem += p.DurationS
				total += p.DurationS
			case model.SleepStageLight:
				light += p.DurationS
				total += p.DurationS
			case model.SleepStageAwake:
				awake += p.DurationS
			}
		}
		userID := phases[group[0]].UserID
		hash := model.ComputeContentHash(map[string]string{
			"user_id":       userID,
			"provider":      provider,
			"bedtime_start": start.UTC().Format(time.RFC3339Nano),
			"bedtime_end":   end.UTC().Format(time.RFC3339Nano),
		})
		sessions = append(sessions, model.SleepSession{
			UserID:            userID,
			Provider:          provider,
			BedtimeStart:      start,
			BedtimeEnd:        end,
			TotalSleepSeconds: &total,
			DeepSleepSeconds:  &deep,
			REMSleepSeconds:   &rem,
			LightSleepSeconds: &light,
			AwakeSeconds:      &awake,
			ContentHash:       hash,
		})
		for _, gi := range group {
			phases[gi].ParentContentHash = hash
		}
		group = group[:0]
	}

	for _, i := range idx {
		if len(group) > 0 && phases[i].StartDate.Sub(prevEnd) > sleepSessionGap {
			flush()
		}
		group = append(group, i)
		if phases[i].EndDate.After(prevEnd) {
			prevEnd = phases[i].EndDate
		}
	}
	flush()
	return sessions
}
