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

func TestRevertApprovedCancellationReclaimsEntitlement(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	setQuota(t, s, Config{})
	c := quotaPending(t, s, 32301)
	raw, err := s.Execute(ctx, adminID, quotaApprove(c, 12000, 32301))
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
	raw, err = s.Execute(ctx, userID, Command{Action: "request_cancel", ID: approved.ID, Version: approved.Version, RequestKey: key(32302), Note: "cancel"})
	if err != nil {
		t.Fatal(err)
	}
	var requested Case
	if err = json.Unmarshal(raw, &requested); err != nil {
		t.Fatal(err)
	}
	raw, err = s.Execute(ctx, adminID, Command{Action: "approve_cancel", ID: approved.ID, Version: requested.Version, RequestKey: key(32303), Note: "yes", ConfirmedNotDelivered: true})
	if err != nil {
		t.Fatal(err)
	}
	var cancelled Case
	if err = json.Unmarshal(raw, &cancelled); err != nil {
		t.Fatal(err)
	}
	input := Command{Action: "revert_cancel_review", ID: approved.ID, Version: cancelled.Version, RequestKey: key(32304), Note: "mistake", ConfirmedNotDelivered: true}
	other := quotaPending(t, s, 32305)
	if len(requested.Keys) == 0 {
		t.Fatal("missing original entitlement")
	}
	if err = store.Claim(ctx, s.Pool, requested.Keys[0], other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(ctx, adminID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("occupied entitlement: %v", err)
	}
	if err = store.ReleaseEntitlements(ctx, s.Pool, other.ID); err != nil {
		t.Fatal(err)
	}
	raw, err = s.Execute(ctx, adminID, input)
	if err != nil {
		t.Fatal(err)
	}
	var restored Case
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.State != "cancel_requested" || restored.Award != approved.Award {
		t.Fatalf("restored: %+v", restored)
	}
	claimed, err := store.Claimed(ctx, s.Pool, requested.Keys[0])
	if err != nil || !claimed {
		t.Fatalf("claim %v: %v", claimed, err)
	}
}
