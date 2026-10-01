package access

import (
	"testing"
	"time"
)

func TestSeATCorporationPolicy(t *testing.T) {
	now := time.Now()
	target := Corporation{ID: 10, AllianceID: 100, CEOID: 7}
	base := Fact{CharacterID: 8, State: "ready", Corporation: target, SyncedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Hour)}
	tests := []struct {
		name, permission string
		admin            bool
		change           func(*Fact)
		grants           []Grant
		want             bool
	}{
		{name: "ordinary member", permission: "corporation.summary"},
		{name: "CEO only target corporation", permission: "corporation.summary", change: func(f *Fact) { f.CharacterID = 7 }, want: true},
		{name: "CEO in another corporation", permission: "corporation.summary", change: func(f *Fact) { f.CharacterID = 7; f.Corporation.ID = 20 }},
		{name: "Director all corporation abilities", permission: "corporation.projects", change: func(f *Fact) { f.Roles = []string{"Director"} }, want: true},
		{name: "Director cannot manage website", permission: "access.manage", change: func(f *Fact) { f.Roles = []string{"Director"} }},
		{name: "Director cannot approve site welfare automatically", permission: "corporation.welfare", change: func(f *Fact) { f.Roles = []string{"Director"} }},
		{name: "explicit welfare grant", permission: "corporation.welfare", grants: []Grant{{Permission: "corporation.welfare", Corporations: []int64{10}}}, want: true},
		{name: "location Director is not global", permission: "corporation.summary", change: func(f *Fact) { f.RolesAtHQ = []string{"Director"} }},
		{name: "stale Director", permission: "corporation.summary", change: func(f *Fact) { f.Roles = []string{"Director"}; f.ValidUntil = now.Add(-time.Second) }},
		{name: "revoked Director", permission: "corporation.summary", change: func(f *Fact) { f.Roles = []string{"Director"}; f.State = "reauthorize" }},
		{name: "accountant journal", permission: "corporation.journal", change: func(f *Fact) { f.Roles = []string{"Accountant"} }, want: true},
		{name: "accountant cannot see assets", permission: "corporation.asset", change: func(f *Fact) { f.Roles = []string{"Accountant"} }},
		{name: "junior accountant cannot see journal", permission: "corporation.journal", change: func(f *Fact) { f.Roles = []string{"Junior_Accountant"} }},
		{name: "contract enum correction", permission: "corporation.contract", change: func(f *Fact) { f.Roles = []string{"Contract_Manager"} }, want: true},
		{name: "project enum correction", permission: "corporation.projects", change: func(f *Fact) { f.Roles = []string{"Project_Manager"} }, want: true},
		{name: "second container correction", permission: "corporation.asset_second_division", change: func(f *Fact) { f.Roles = []string{"Container_Take_2"} }, want: true},
		{name: "second container cannot access first", permission: "corporation.asset_first_division", change: func(f *Fact) { f.Roles = []string{"Container_Take_2"} }},
		{name: "manual corporation filter", permission: "corporation.journal", grants: []Grant{{Permission: "corporation.journal", Corporations: []int64{10}}}, want: true},
		{name: "manual wrong corporation", permission: "corporation.journal", grants: []Grant{{Permission: "corporation.journal", Corporations: []int64{20}}}},
		{name: "manual alliance filter", permission: "corporation.journal", grants: []Grant{{Permission: "corporation.journal", Alliances: []int64{100}}}, want: true},
		{name: "empty filters mean unrestricted", permission: "corporation.journal", grants: []Grant{{Permission: "corporation.journal"}}, want: true},
		{name: "site administrator", permission: "access.manage", admin: true, want: true},
		{name: "unknown denied even administrator", permission: "corporation.root", admin: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := base
			if tc.change != nil {
				tc.change(&f)
			}
			if got := Evaluate(tc.admin, []Fact{f}, tc.grants, tc.permission, target, now); got != tc.want {
				t.Fatalf("allowed=%v want=%v", got, tc.want)
			}
		})
	}
	if Evaluate(true, nil, nil, "corporation.summary", Corporation{}, now) {
		t.Fatal("corporation operation without a target allowed")
	}
}
func TestGrantValidation(t *testing.T) {
	for _, g := range [][]Grant{{{Permission: "unknown"}}, {{Permission: "access.manage", Corporations: []int64{1}}}, {{Permission: "corporation.summary", Corporations: []int64{0}}}, {{Permission: "corporation.summary"}, {Permission: "corporation.summary"}}} {
		if ValidateGrants(g) == nil {
			t.Fatal("invalid grants accepted")
		}
	}
	for _, p := range Catalog() {
		if p.Scope == "corporation" && p.ID != "corporation.welfare" && !Evaluate(false, []Fact{{State: "ready", CharacterID: 1, Corporation: Corporation{ID: 10}, Roles: []string{"Director"}, SyncedAt: time.Now().Add(-time.Second), ValidUntil: time.Now().Add(time.Hour)}}, nil, p.ID, Corporation{ID: 10, CEOID: 2}, time.Now()) {
			t.Errorf("Director missing %s", p.ID)
		}
	}
}

func TestMemberReadIsAdministratorOnly(t *testing.T) {
	for _, admin := range []bool{false, true} {
		if got := Evaluate(admin, nil, []Grant{{Permission: "access.members.read"}}, "access.members.read", Corporation{}, time.Now()); got != admin {
			t.Fatal("member read must use admin flag")
		}
	}
	if ValidateGrants([]Grant{{Permission: "access.members.read"}}) == nil {
		t.Fatal("reserved ability assignable")
	}
	for _, p := range ManageableCatalog() {
		if p.ID == "access.members.read" {
			t.Fatal("admin-only ability exposed as a role grant")
		}
	}
}
