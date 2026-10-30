package welfare

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func TestReapprovalCountsOnlyLatestQuotaPeriod(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	setQuota(t, s, Config{})
	c := quotaPending(t, s, 32501)
	raw, err := s.Execute(ctx, adminID, quotaApprove(c, 12000, 32501))
	if err != nil {
		t.Fatal(err)
	}
	var approved Case
	if err = json.Unmarshal(raw, &approved); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	oldStart, oldEnd := now.AddDate(0, -2, 0), now.AddDate(0, -1, 0)
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_audit SET created_at=$2 WHERE case_id=$1 AND action='approve'`, approved.ID, oldStart.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return nil, nil
	}
	raw, err = s.Execute(ctx, adminID, Command{Action: "revert_approval", ID: approved.ID, Version: approved.Version, RequestKey: key(32502), Note: "mistake", ConfirmedNotDelivered: true})
	if err != nil {
		t.Fatal(err)
	}
	var pending Case
	if err = json.Unmarshal(raw, &pending); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(ctx, adminID, quotaApprove(pending, 12000, 32503)); err != nil {
		t.Fatal(err)
	}
	old, err := store.ApprovedLossTotal(ctx, s.Pool, userID, 10, approved.Kind, oldStart, oldEnd)
	if err != nil || old != 0 {
		t.Fatalf("old period %d: %v", old, err)
	}
	current, err := store.ApprovedLossTotal(ctx, s.Pool, userID, 10, approved.Kind, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || current != 12000 {
		t.Fatalf("current period %d: %v", current, err)
	}
}
