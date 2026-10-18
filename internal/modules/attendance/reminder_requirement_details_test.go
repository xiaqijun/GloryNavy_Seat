package attendance

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestReminderPAPDetailsCurrentTargetAndEvidence(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	report := AlliancePAPReport{Month: "2026-10", Available: true, Complete: true, LastSyncedAt: &now, Points: 2.5, Characters: []AlliancePAPCharacter{{CharacterID: 101}}}
	items, err := requirementReminderDetails(report, 5, now)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	content := fmt.Sprint(items)
	for _, want := range []string{"2026-10", "当前 PAP 2.5", "目标 PAP 5", "还差 PAP 2.5", "2026-11-01 00:00 UTC"} {
		if !strings.Contains(content, want) {
			t.Fatal(want, content)
		}
	}
	report.Complete = false
	if items, err = requirementReminderDetails(report, 5, now); err == nil || len(items) != 0 {
		t.Fatal("unknown evidence mailed", items, err)
	}
}
