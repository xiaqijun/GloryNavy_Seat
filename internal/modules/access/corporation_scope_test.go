package access

import (
	"testing"
	"time"
)

func TestConfiguredCorporationScopeOverridesGrants(t *testing.T) {
	now := time.Now()
	fresh := Fact{CharacterID: 1, Corporation: Corporation{ID: 10}, State: "ready", SyncedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Hour), Roles: []string{"Director"}}
	a := Account{restrictCorporations: true, corporations: []int64{10, 20}, Characters: []Fact{fresh}, Roles: []Role{{Grants: []Grant{{Permission: "corporation.welfare"}, {Permission: "access.manage"}, {Permission: "corporation.journal", Corporations: EntityIDs{20}}}}}}
	for _, permission := range []string{"corporation.welfare", "corporation.journal", "corporation.summary"} {
		for _, corp := range []int64{20, 30} {
			if a.Can(permission, Corporation{ID: corp}) {
				t.Fatalf("cross-corporation grant: %s %d", permission, corp)
			}
		}
	}
	if !a.Can("access.manage", Corporation{}) || !a.Can("corporation.welfare", Corporation{ID: 10}) || !a.Can("corporation.summary", Corporation{ID: 10}) {
		t.Fatal("own/global abilities lost")
	}
	alt := fresh
	alt.CharacterID = 2
	alt.Corporation.ID = 20
	a.Characters = append(a.Characters, alt)
	if !a.Can("corporation.journal", Corporation{ID: 20}) {
		t.Fatal("valid bound alt not included")
	}
	a.Characters[1].ValidUntil = now.Add(-time.Minute)
	if a.Can("corporation.journal", Corporation{ID: 20}) {
		t.Fatal("stale alt expanded scope")
	}
	a.Administrator = true
	if !a.Can("corporation.summary", Corporation{ID: 20}) || a.Can("corporation.summary", Corporation{ID: 30}) || a.Can("unknown", Corporation{ID: 10}) {
		t.Fatal("administrator/unknown scope")
	}
	a.corporations = nil
	if a.Can("corporation.summary", Corporation{ID: 10}) {
		t.Fatal("empty configuration must deny")
	}
}
