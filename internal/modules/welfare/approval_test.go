package welfare

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"testing"
	"time"
)

func TestApprovalWelfareScopeCountsAndHistory(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.Corporations = func(_ context.Context, u string) ([]Corporation, error) {
		return []Corporation{{ID: 10, Name: "test", Manage: u == adminID}}, nil
	}
	s.Members = func(context.Context, string, int64) ([]Character, error) {
		return []Character{{AccountID: userID}, {AccountID: adminID}}, nil
	}
	now := time.Now().UTC().Truncate(time.Second)
	var ids []int64
	for _, v := range []struct {
		owner, state, status string
		corp                 int64
	}{{userID, "submitted", "", 10}, {adminID, "submitted", "", 10}, {otherID, "submitted", "", 10}, {userID, "submitted", "", 20}, {userID, "approved", "waiting_items", 10}, {userID, "approved", "mismatch", 10}, {userID, "information", "", 10}, {userID, "completed", "finished", 10}, {userID, "approved", "awaiting_acceptance", 10}} {
		c, e := store.Save(ctx, s.Pool, Case{AccountID: v.owner, CorporationID: v.corp, Kind: "solo", State: v.state, Detail: raw(Detail{CharacterName: "Target", PaymentStatus: v.status}), Keys: []string{}})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, c.ID)
	}
	if _, e := s.Pool.Exec(ctx, `UPDATE welfare_cases SET created_at=$1,updated_at=$1`, now); e != nil {
		t.Fatal(e)
	}
	f := reviewqueue.Filter{View: "pending"}
	p, e := s.ApprovalQueue(ctx, adminID, f, reviewqueue.Position{}, 31)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].ID != ids[0] || p.Counts["pending"] != 1 || p.Counts["fulfillment"] != 1 || p.Counts["exceptions"] != 1 || p.Counts["information"] != 1 || p.Counts["history"] != 2 {
		t.Fatalf("wrong scope/counts: %+v", p)
	}
	if sorted, err := s.ApprovalQueue(ctx, adminID, reviewqueue.Filter{View: "pending", Sort: "id_asc"}, reviewqueue.Position{}, 31); err != nil || len(sorted.Items) != 1 {
		t.Fatalf("ID sort query: %+v %v", sorted, err)
	}
	if _, e = s.ApprovalQueue(ctx, userID, f, reviewqueue.Position{}, 31); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("member: %v", e)
	}
	f.ID = ids[2]
	p, e = s.ApprovalQueue(ctx, adminID, f, reviewqueue.Position{}, 1)
	if e != nil || len(p.Items) != 0 {
		t.Fatalf("hidden account detail: %+v %v", p, e)
	}
	f.ID = ids[1]
	p, e = s.ApprovalQueue(ctx, adminID, f, reviewqueue.Position{}, 1)
	if e != nil || len(p.Items) != 1 || len(p.Items[0].Actions) != 0 {
		t.Fatalf("self detail actions: %+v %v", p, e)
	}
	if e = store.Audit(ctx, s.Pool, adminID, key(400), "approval", "approve", "ok", ids[4], map[string]string{"state": "approved"}); e != nil {
		t.Fatal(e)
	}
	f = reviewqueue.Filter{View: "history", Mine: true}
	p, e = s.ApprovalQueue(ctx, adminID, f, reviewqueue.Position{}, 31)
	if e != nil || len(p.Items) != 1 || p.Items[0].State != "approved" {
		t.Fatalf("approved remains unfulfilled in history %+v %v", p, e)
	}
}
