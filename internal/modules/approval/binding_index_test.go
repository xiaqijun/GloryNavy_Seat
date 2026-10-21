package approval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"glorynavy.local/seat/internal/testutil"
)

// Exercise the serialized authorization scope against real SQL: a scope with
// no corporation must still match an unscoped redemption's account/recipient.
func TestBindingLimitedIndexListsRedemptionsWithoutCorporation(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	const member = "11111111-1111-4111-8111-111111111111"
	const other = "22222222-2222-4222-8222-222222222222"
	const admin = "33333333-3333-4333-8333-333333333333"
	items := []reviewqueue.Item{
		{ID: 1, Account: member, Recipient: "101", State: "pending"},
		{ID: 2, Account: member, Recipient: "101", State: "cancel_requested"},
		{ID: 3, Account: member, Recipient: "101", State: "cancelled", History: true},
		{ID: 4, Account: other, Recipient: "101", State: "pending"},
		{ID: 5, Account: member, Recipient: "102", State: "pending"},
		{ID: 6, Account: admin, Recipient: "201", State: "pending"},
		{ID: 7, Account: member, Recipient: "101", Corporation: "7", State: "pending"},
	}
	for i := range items {
		items[i].Source, items[i].Kind = "exchange", "exchange"
		items[i].Version = 1
		items[i].Status = "waiting_contract"
		items[i].Time = time.Date(2026, 10, 21, 0, 0, i, 0, time.UTC)
	}
	s := NewService(nil, pool)
	s.UseIndex, s.AccessCacheTTL = true, 0
	if err := s.Index.ReplaceSource(ctx, "exchange", items); err != nil {
		t.Fatal(err)
	}
	allowed := true
	bindings := []reviewqueue.Binding{{Account: member, Recipient: "101"}, {Account: admin, Recipient: "201"}}
	s.Sources = []reviewqueue.Source{{ID: "exchange", IndexOnly: true,
		Access: func(_ context.Context, actor string) (reviewqueue.Access, error) {
			return reviewqueue.Access{Allowed: allowed && actor == admin, RestrictBindings: true, Bindings: bindings}, nil
		},
	}}
	for _, tc := range []struct {
		view string
		id   int64
	}{{"fulfillment", 1}, {"pending", 2}, {"history", 3}} {
		t.Run(tc.view, func(t *testing.T) {
			page, err := s.List(ctx, admin, reviewqueue.Filter{View: tc.view}, "")
			if err != nil || len(page.Items) != 1 || page.Items[0].ID != tc.id {
				t.Fatalf("%s list: items=%+v err=%v", tc.view, page.Items, err)
			}
			for _, view := range []string{"fulfillment", "pending", "history"} {
				if page.Counts[view] != 1 {
					t.Errorf("%s count=%d, want 1", view, page.Counts[view])
				}
			}
		})
	}
	t.Run("corporation filter", func(t *testing.T) {
		page, err := s.List(ctx, admin, reviewqueue.Filter{View: "fulfillment", Corporation: "7"}, "")
		if err != nil || len(page.Items) != 0 || page.Counts["fulfillment"] != 0 {
			t.Fatalf("unscoped redemption passed corporation filter: %+v %v", page, err)
		}
	})
	t.Run("binding revoked", func(t *testing.T) {
		bindings = nil
		page, err := s.List(ctx, admin, reviewqueue.Filter{View: "fulfillment"}, "")
		if err != nil || len(page.Items) != 0 || page.Counts["fulfillment"] != 0 {
			t.Fatalf("revoked binding remained visible: %+v %v", page, err)
		}
	})
	t.Run("administrator revoked", func(t *testing.T) {
		allowed = false
		if _, err := s.List(ctx, admin, reviewqueue.Filter{View: "fulfillment"}, ""); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("revoked list", err)
		}
	})
}
