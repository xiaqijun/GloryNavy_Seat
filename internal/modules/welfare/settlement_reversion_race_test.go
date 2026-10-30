package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
)

func TestSettlementRejectsConcurrentApprovalReversion(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	setQuota(t, s, Config{})
	c := quotaPending(t, s, 32401)
	raw, err := s.Execute(ctx, adminID, quotaApprove(c, 12000, 32401))
	if err != nil {
		t.Fatal(err)
	}
	var approved Case
	if err = json.Unmarshal(raw, &approved); err != nil {
		t.Fatal(err)
	}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return nil, nil
	}
	started, resume := make(chan struct{}), make(chan struct{})
	s.MainCharacterID = func(context.Context, string) (int64, error) { close(started); <-resume; return 123, nil }
	result := make(chan error, 1)
	go func() {
		_, e := s.CreateSettlementBatch(ctx, adminID, SettlementInput{RequestKey: key(32402), Items: []SettlementInputItem{{Source: "welfare", ID: approved.ID}}})
		result <- e
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("batch did not reach summary")
	}
	_, err = s.Execute(ctx, adminID, Command{Action: "revert_approval", ID: approved.ID, Version: approved.Version, RequestKey: key(32403), Note: "mistake", ConfirmedNotDelivered: true})
	close(resume)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, ErrConflict) {
		t.Fatalf("stale batch: %v", err)
	}
	var n int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_contract_settlement_items WHERE source='welfare' AND source_id=$1`, approved.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("published batch items %d: %v", n, err)
	}
}
