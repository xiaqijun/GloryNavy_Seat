package approval

import (
	"testing"

	"glorynavy.local/seat/internal/platform/reviewqueue"
)

func TestScopesKeepBindingLimitedSourcesOutOfAllScope(t *testing.T) {
	got := scopes(map[string]reviewqueue.Access{
		"exchange": {Allowed: true, Bindings: []reviewqueue.Binding{{Account: "a", Recipient: "10"}}, RestrictBindings: true},
	})
	if len(got) != 1 || got[0].All || len(got[0].Bindings) != 1 {
		t.Fatalf("scopes() = %+v, want one non-all binding scope", got)
	}
}

func TestScopesUseCorporationAccountRestrictions(t *testing.T) {
	got := scopes(map[string]reviewqueue.Access{
		"welfare": {Allowed: true, Corporations: []reviewqueue.Option{{ID: "7"}}, AccountsByCorporation: map[string][]string{"7": {"a"}}},
	})
	if len(got) != 1 || got[0].All || got[0].Corporation != "7" || !got[0].RestrictAccounts || len(got[0].Accounts) != 1 {
		t.Fatalf("scopes() = %+v, want corporation account restriction", got)
	}
}
