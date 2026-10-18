package attendance

import (
	"testing"
	"time"
)

func TestReminderPAPMonthWindowAndUnknownEvidence(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	report := AlliancePAPReport{Month: "2026-10", Available: true, Complete: true, LastSyncedAt: &now, Points: 2, Characters: []AlliancePAPCharacter{{CharacterID: 101}}}
	keys, err := requirementReminderKeys(report, 3, now)
	if err != nil || len(keys) != 1 {
		t.Fatal(keys, err)
	}
	keys, err = requirementReminderKeys(report, 3, now.AddDate(0, 0, -14))
	if err != nil || len(keys) != 0 {
		t.Fatal("early reminder", keys, err)
	}
	for _, kind := range []string{"stale", "incomplete", "old_month", "unavailable"} {
		r := report
		switch kind {
		case "stale":
			old := now.Add(-25 * time.Hour)
			r.LastSyncedAt = &old
		case "incomplete":
			r.Complete = false
		case "old_month":
			r.Month = "2026-09"
		case "unavailable":
			r.Available = false
		}
		if _, err = requirementReminderKeys(r, 3, now); err == nil {
			t.Fatal(kind)
		}
	}
	report.Points = 3
	keys, err = requirementReminderKeys(report, 3, now)
	if err != nil || len(keys) != 0 {
		t.Fatal("achieved", keys, err)
	}
}
