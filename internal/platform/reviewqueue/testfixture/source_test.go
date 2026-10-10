package testfixture

import (
	"context"
	"testing"

	"glorynavy.local/seat/internal/platform/reviewqueue"
)

func TestAdapterIsValidAndDeterministic(t *testing.T) {
	fixture := New("example", []reviewqueue.Item{{Source: "example", ID: 1, Status: "pending"}})
	adapter := fixture.Adapter()
	if err := reviewqueue.ValidateSources([]reviewqueue.Source{adapter}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Query(context.Background(), "manager", reviewqueue.Filter{View: "pending"}, reviewqueue.Position{}, 30); err != nil {
		t.Fatal(err)
	}
	if err := reviewqueue.ValidateSources([]reviewqueue.Source{fixture.IndexOnlyAdapter()}); err != nil {
		t.Fatalf("index-only fixture: %v", err)
	}
	items, err := adapter.Snapshot(context.Background())
	if err != nil || len(items) != 1 || fixture.QueryCalls() != 1 || fixture.SnapshotCalls() != 1 {
		t.Fatalf("fixture calls/items = %d/%d/%+v, err=%v", fixture.QueryCalls(), fixture.SnapshotCalls(), items, err)
	}
}
