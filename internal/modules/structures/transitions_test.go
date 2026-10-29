package structures

import (
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestTransitionsUseOnlyNewerKnownObservations(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	old := eve.Structure{ID: 1, CorporationID: 42, State: "shield_vulnerable", ObservedAt: t0, Services: []eve.StructureService{{Name: "market", State: "online"}, {Name: "industry", State: "online"}}}
	current := eve.Structure{ID: 1, CorporationID: 42, State: "armor_reinforce", ObservedAt: t0.Add(time.Minute), Services: []eve.StructureService{{Name: "market", State: "offline"}}}
	got := Transitions([]eve.Structure{old}, []eve.Structure{current})
	if len(got) != 2 || got[0].Field != "service:market" || got[0].From != "online" || got[0].To != "offline" || got[1].Field != "state" {
		t.Fatalf("unexpected transitions: %+v", got)
	}
	if got := Transitions(nil, []eve.Structure{current}); len(got) != 0 {
		t.Fatalf("initial observation must be a baseline: %+v", got)
	}
	if got := Transitions([]eve.Structure{current}, []eve.Structure{old}); len(got) != 0 {
		t.Fatalf("older snapshot must not produce a transition: %+v", got)
	}
	current.CorporationID++
	if got := Transitions([]eve.Structure{old}, []eve.Structure{current}); len(got) != 0 {
		t.Fatalf("corporation changes must not produce a transition: %+v", got)
	}
}
