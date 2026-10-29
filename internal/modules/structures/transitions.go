package structures

import (
	"sort"
	"strings"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

// Transition describes a change between two successful observations. It is
// only an in-memory projection; delivery and deduplication belong to the
// notification channel, not to the underlying structure snapshot.
type Transition struct {
	StructureID int64
	Field       string
	From        string
	To          string
	ObservedAt  time.Time
}

// Transitions compares observations for existing structures. The first sample
// is a baseline; missing structures or services are not proof of destruction
// or an outage. Older or repeated samples cannot produce transitions.
func Transitions(previous, current []eve.Structure) []Transition {
	before := make(map[int64]eve.Structure, len(previous))
	for _, item := range previous {
		before[item.ID] = item
	}
	out := make([]Transition, 0)
	for _, item := range current {
		old, ok := before[item.ID]
		if !ok || item.ID <= 0 || item.CorporationID != old.CorporationID || item.ObservedAt.IsZero() || !item.ObservedAt.After(old.ObservedAt) {
			continue
		}
		add := func(field, from, to string) {
			if from != "" && to != "" && from != to {
				out = append(out, Transition{item.ID, field, from, to, item.ObservedAt})
			}
		}
		add("state", strings.TrimSpace(old.State), strings.TrimSpace(item.State))
		oldServices := make(map[string]string, len(old.Services))
		for _, service := range old.Services {
			oldServices[service.Name] = strings.TrimSpace(service.State)
		}
		for _, service := range item.Services {
			if name := strings.TrimSpace(service.Name); name != "" {
				add("service:"+name, oldServices[service.Name], strings.TrimSpace(service.State))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StructureID != out[j].StructureID {
			return out[i].StructureID < out[j].StructureID
		}
		return out[i].Field < out[j].Field
	})
	return out
}
