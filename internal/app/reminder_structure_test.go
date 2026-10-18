package app

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/reminder"
	"glorynavy.local/seat/internal/modules/structures"
	"reflect"
	"testing"
	"time"
)

func TestReminderStructureSeverityJitterAndPermissionFailure(t *testing.T) {
	now := time.Now().UTC()
	fuel := now.Add(20 * time.Hour)
	unanchor := now.Add(3 * time.Hour)
	readError := error(nil)
	s := &reminder.Service{}
	h := structures.Handler{Read: func(context.Context, string, int64) ([]eve.Structure, error) {
		return []eve.Structure{{ID: 10, FuelExpires: &fuel, UnanchorsAt: &unanchor}}, readError
	}}
	wireEveReminders(s, nil, nil, nil, nil, h, []string{"structures"})
	source := s.Sources["structure_alert"]
	keys, e := source(context.Background(), "allowed", now)
	if e != nil || len(keys) != 2 {
		t.Fatal(keys, e)
	}
	fuel = fuel.Add(time.Second)
	same, e := source(context.Background(), "allowed", now)
	if e != nil || !reflect.DeepEqual(keys, same) {
		t.Fatal("sampling jitter duplicated fuel alert", same, e)
	}
	fuel = now.Add(100 * time.Hour)
	keys, e = source(context.Background(), "allowed", now)
	if e != nil || len(keys) != 1 {
		t.Fatal("safe fuel alerted", keys, e)
	}
	readError = errors.New("permission changed")
	if _, e = source(context.Background(), "denied", now); e == nil {
		t.Fatal("failed authorized source hidden")
	}
}
