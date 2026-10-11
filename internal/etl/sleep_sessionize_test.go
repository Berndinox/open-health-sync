// SPDX-License-Identifier: AGPL-3.0-only

package etl

import (
	"testing"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

func mkPhase(phase string, start time.Time, durMin int) model.SleepPhase {
	return model.SleepPhase{
		UserID:    "u1",
		Phase:     phase,
		StartDate: start,
		EndDate:   start.Add(time.Duration(durMin) * time.Minute),
		DurationS: durMin * 60,
	}
}

// Regression: modern watchOS sends sleep as stage segments (canonicalized by
// the mapper to light/deep/rem/awake) with NO in_bed record, so the mapper
// produces orphan phases and no session, and the per-session writer dropped
// them all. synthesizeSleepSessions must build one session per night and
// parent the phases to it.
func TestSynthesizeSleepSessions_modernStages(t *testing.T) {
	base := time.Date(2026, 6, 26, 6, 0, 0, 0, time.UTC)
	phases := []model.SleepPhase{
		mkPhase(model.SleepStageLight, base, 30),
		mkPhase(model.SleepStageDeep, base.Add(30*time.Minute), 20),
		mkPhase(model.SleepStageAwake, base.Add(50*time.Minute), 5),
		mkPhase(model.SleepStageREM, base.Add(55*time.Minute), 25),
	}
	sessions := synthesizeSleepSessions("apple_health", phases)
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	deref := func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	}
	if deref(s.TotalSleepSeconds) != (30+20+25)*60 {
		t.Errorf("total sleep = %d, want %d", deref(s.TotalSleepSeconds), (30+20+25)*60)
	}
	if deref(s.DeepSleepSeconds) != 20*60 || deref(s.REMSleepSeconds) != 25*60 ||
		deref(s.LightSleepSeconds) != 30*60 || deref(s.AwakeSeconds) != 5*60 {
		t.Errorf("breakdown deep=%d rem=%d light=%d awake=%d", deref(s.DeepSleepSeconds), deref(s.REMSleepSeconds), deref(s.LightSleepSeconds), deref(s.AwakeSeconds))
	}
	if !s.BedtimeStart.Equal(base) || !s.BedtimeEnd.Equal(base.Add(80*time.Minute)) {
		t.Errorf("span %v..%v wrong", s.BedtimeStart, s.BedtimeEnd)
	}
	if s.ContentHash == "" {
		t.Error("session content hash empty")
	}
	for i := range phases {
		if phases[i].ParentContentHash != s.ContentHash {
			t.Errorf("phase %d (%s) not parented to the synthesized session", i, phases[i].Phase)
		}
	}
}

// A gap larger than the threshold starts a new night.
func TestSynthesizeSleepSessions_twoNights(t *testing.T) {
	base := time.Date(2026, 6, 25, 6, 0, 0, 0, time.UTC)
	phases := []model.SleepPhase{
		mkPhase(model.SleepStageLight, base, 10),
		mkPhase(model.SleepStageLight, base.Add(130*time.Minute), 10), // 120 min gap > 1h
	}
	if got := len(synthesizeSleepSessions("apple_health", phases)); got != 2 {
		t.Fatalf("want 2 sessions, got %d", got)
	}
}

// Phases already attached to a real in_bed session are left alone.
func TestSynthesizeSleepSessions_skipsParentedAndInBed(t *testing.T) {
	base := time.Date(2026, 6, 25, 6, 0, 0, 0, time.UTC)
	phases := []model.SleepPhase{
		{UserID: "u1", Phase: model.SleepStageLight, StartDate: base, EndDate: base.Add(10 * time.Minute), DurationS: 600, ParentContentHash: "existing"},
		mkPhase("in_bed", base, 60),
	}
	if got := len(synthesizeSleepSessions("apple_health", phases)); got != 0 {
		t.Fatalf("want 0 synthesized sessions, got %d", got)
	}
}
