package reviewqueue

import (
	"context"
	"testing"
)

func TestValidateSourcesRequiresRegisteredProjectionContract(t *testing.T) {
	base := Source{
		ID:           "loan",
		Capabilities: Capabilities{DetailKind: "loan"},
		Access:       func(context.Context, string) (Access, error) { return Access{Allowed: true}, nil },
		Query:        func(context.Context, string, Filter, Position, int) (Page, error) { return Page{}, nil },
		Snapshot:     func(context.Context) ([]Item, error) { return []Item{}, nil },
	}
	if err := ValidateSources([]Source{base}); err != nil {
		t.Fatal(err)
	}
	for _, broken := range []Source{
		{ID: "loan", Access: base.Access, Query: base.Query, Capabilities: base.Capabilities},
		{ID: "loan", Access: base.Access, Query: base.Query, Snapshot: base.Snapshot},
	} {
		if err := ValidateSources([]Source{broken}); err == nil {
			t.Fatalf("incomplete source accepted: %+v", broken)
		}
	}
	indexOnly := base
	indexOnly.Query = nil
	indexOnly.IndexOnly = true
	if err := ValidateSources([]Source{indexOnly}); err != nil {
		t.Fatalf("index-only source rejected: %v", err)
	}
	if err := ValidateSources([]Source{base, base}); err == nil {
		t.Fatal("duplicate source accepted")
	}
}
