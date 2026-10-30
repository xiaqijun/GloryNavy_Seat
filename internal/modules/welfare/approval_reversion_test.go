package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func TestRevertApprovalPreservesHistoryAndRechecksPayment(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	setQuota(t, s, Config{})
	c := quotaPending(t, s, 32101)
	first, err := s.Execute(ctx, adminID, quotaApprove(c, 12000, 32101))
	if err != nil {
		t.Fatal(err)
	}
	var approved Case
	if err = json.Unmarshal(first, &approved); err != nil {
		t.Fatal(err)
	}
	var candidates []eve.DeliveryContract
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return candidates, nil
	}
	revert := Command{Action: "revert_approval", ID: approved.ID, Version: approved.Version, RequestKey: key(32102), Note: "wrong approval", ConfirmedNotDelivered: true}
	if _, err = s.Execute(ctx, userID, revert); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("self rollback: %v", err)
	}
	missing := revert
	missing.RequestKey = key(32103)
	missing.ConfirmedNotDelivered = false
	if _, err = s.Execute(ctx, adminID, missing); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing confirmation: %v", err)
	}
	candidates = []eve.DeliveryContract{{ID: 7, Status: "outstanding"}}
	if _, err = s.Execute(ctx, adminID, revert); !errors.Is(err, ErrCancellationPayment) {
		t.Fatalf("active contract: %v", err)
	}
	candidates = nil
	out, err := s.Execute(ctx, adminID, revert)
	if err != nil {
		t.Fatal(err)
	}
	var pending Case
	if err = json.Unmarshal(out, &pending); err != nil {
		t.Fatal(err)
	}
	if pending.State != "submitted" || pending.Award != 0 || pending.Reference != approved.Reference {
		t.Fatalf("rollback: %+v", pending)
	}
	if _, err = s.Execute(ctx, adminID, revert); err != nil {
		t.Fatalf("replay: %v", err)
	}
	var old, spans int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_approval_reversions WHERE case_id=$1 AND previous_state='approved' AND (snapshot->>'award_minor')::bigint=12000 AND jsonb_array_length(quota_spans)=1`, pending.ID).Scan(&old); err != nil || old != 1 {
		t.Fatalf("history %d: %v", old, err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_loss_quota_days WHERE case_id=$1`, pending.ID).Scan(&spans); err != nil || spans != 0 {
		t.Fatalf("quota %d: %v", spans, err)
	}
	candidates = []eve.DeliveryContract{{ID: 8, Status: "finished"}}
	if _, err = s.Execute(ctx, adminID, quotaApprove(pending, 12000, 32104)); !errors.Is(err, ErrCancellationPayment) {
		t.Fatalf("late payment: %v", err)
	}
	candidates = nil
	again, err := s.Execute(ctx, adminID, quotaApprove(pending, 12000, 32105))
	if err != nil {
		t.Fatal(err)
	}
	var final Case
	if err = json.Unmarshal(again, &final); err != nil || final.State != "approved" {
		t.Fatalf("reapproval %+v: %v", final, err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_loss_quota_days WHERE case_id=$1`, final.ID).Scan(&spans); err != nil || spans != 1 {
		t.Fatalf("reapproval quota %d: %v", spans, err)
	}
	if _, err = store.Read(ctx, s.Pool, final.ID); err != nil {
		t.Fatal(err)
	}
}
