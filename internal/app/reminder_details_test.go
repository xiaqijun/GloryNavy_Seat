package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/reminder"
	"glorynavy.local/seat/internal/modules/structures"
)

func TestReminderStructureDetailsMatchAuthorizedReferences(t *testing.T) {
	now := time.Now().UTC()
	fuel := now.Add(20 * time.Hour)
	unanchor := now.Add(2 * time.Hour)
	row := eve.Structure{ID: 123, Name: "示例堡垒", CorporationID: 98530802, CorporationName: "测试军团", SolarSystemID: 30000142, SolarSystemName: "吉他", FuelExpires: &fuel, UnanchorsAt: &unanchor, ObservedAt: now}
	s := &reminder.Service{}
	wireEveReminders(s, nil, nil, nil, nil, structures.Handler{Read: func(context.Context, string, int64) ([]eve.Structure, error) { return []eve.Structure{row}, nil }}, []string{"structures"})
	keys, err := s.Sources["structure_alert"](context.Background(), "user", now)
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.Details["structure_alert"](context.Background(), "user", now)
	if err != nil || len(items) != 2 || items[0].Key != keys[0] || items[1].Key != keys[1] {
		t.Fatal(items, keys, err)
	}
	content := fmt.Sprint(items)
	for _, want := range []string{"示例堡垒", "测试军团", "吉他", "预计耗尽时间", "拆除时间", "UTC"} {
		if !strings.Contains(content, want) {
			t.Fatal(want, content)
		}
	}
}
