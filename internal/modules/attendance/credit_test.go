package attendance

import (
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

func TestConfirmedAllianceCreditPoints(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	points, ok := confirmedAllianceCreditPoints(store.AlliancePAPAccountReport{
		Month:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Points:   "12.50",
		State:    "ready",
		Complete: true,
	}, now)
	if !ok || points != 12.5 {
		t.Fatalf("confirmed alliance PAP = (%v, %v), want (12.5, true)", points, ok)
	}
}

func TestConfirmedAllianceCreditPointsIgnoresStaleOrIncomplete(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	cases := []store.AlliancePAPAccountReport{
		{Month: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), Points: "99", State: "ready", Complete: true},
		{Month: now, Points: "99", State: "syncing", Complete: false},
		{Month: now, Points: "not-a-number", State: "ready", Complete: true},
	}
	for _, report := range cases {
		if points, ok := confirmedAllianceCreditPoints(report, now); ok || points != 0 {
			t.Fatalf("unexpected alliance PAP for %+v: (%v, %v)", report, points, ok)
		}
	}
}
