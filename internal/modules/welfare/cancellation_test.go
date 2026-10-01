package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/migrations"
)

func TestLossCancellationGuardsReplayAndPayment(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	d := Detail{CharacterID: 123, Reviewer: adminID, KillmailID: 900}
	v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "srp", State: "approved", Detail: raw(d), Award: 12345, Keys: []string{"km:900"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Claim(ctx, s.Pool, "km:900", v.ID); err != nil {
		t.Fatal(err)
	}
	var candidates []eve.DeliveryContract
	s.PaymentContracts = func(_ context.Context, _ pgx.Tx, owner string, character int64, ref string, _ time.Time) ([]eve.DeliveryContract, error) {
		if owner != userID || character != 123 || ref != v.Reference {
			t.Fatal("unscoped cancellation lookup")
		}
		return candidates, nil
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 456: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	n := 7100
	command := func(action string) Command {
		n++
		return Command{Action: action, ID: v.ID, Version: v.Version, RequestKey: key(n), Note: "test reason", ConfirmedNotDelivered: true}
	}
	run := func(actor string, c Command, want error) {
		t.Helper()
		result, err := s.Execute(ctx, actor, c)
		if !errors.Is(err, want) {
			t.Fatalf("%s: got %v want %v", c.Action, err, want)
		}
		if err == nil {
			if err = json.Unmarshal(result, &v); err != nil {
				t.Fatal(err)
			}
		}
	}
	run(otherID, command("request_cancel"), pgx.ErrNoRows)
	empty := command("request_cancel")
	empty.Note = ""
	run(userID, empty, ErrInvalid)
	request := command("request_cancel")
	run(userID, request, nil)
	if v.State != "cancel_requested" {
		t.Fatal(v.State)
	}
	run(userID, request, nil) // exact replay retains one transition
	run(userID, command("approve_cancel"), pgx.ErrNoRows)
	if yes, _ := store.Claimed(ctx, s.Pool, "km:900"); !yes {
		t.Fatal("request freed entitlement")
	}
	ids, err := store.DueDeliveries(ctx, s.Pool)
	if err != nil || len(ids) != 1 {
		t.Fatal("cancellation lost delivery scheduling", ids, err)
	}
	noConfirm := command("approve_cancel")
	noConfirm.ConfirmedNotDelivered = false
	run(adminID, noConfirm, ErrInvalid)
	c := eve.DeliveryContract{ID: 567, OwnerKind: "character", OwnerID: 123, Title: v.Reference, Type: "item_exchange", Status: "outstanding", IssuerID: 456, IssuerCorporationID: 10, AssigneeID: 123, Issued: v.CreatedAt.Add(time.Second), ItemsReady: true, Price: "0", Reward: "123.45", ContentToken: "fixed"}
	for _, status := range []string{"outstanding", "in_progress", "finished", "finished_issuer", "finished_contractor", "failed", "reversed", "unknown"} {
		c.Status = status
		candidates = []eve.DeliveryContract{c}
		run(adminID, command("approve_cancel"), ErrCancellationPayment)
	}
	c.Status = "outstanding"
	candidates = []eve.DeliveryContract{c}
	if err = s.CheckDelivery(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	v, _ = store.Read(ctx, s.Pool, v.ID)
	if v.State != "cancel_requested" {
		t.Fatal("outstanding payment overwrote cancellation", v.State)
	}
	candidates = nil
	run(adminID, command("approve_cancel"), ErrCancellationPayment) // linked evidence cannot disappear
	run(adminID, command("reject_cancel"), nil)
	if v.State != "executing" {
		t.Fatal("rejection lost linked delivery", v.State)
	}
	run(userID, command("request_cancel"), nil)
	c.Status = "cancelled"
	candidates = []eve.DeliveryContract{c}
	approved := command("approve_cancel")
	run(adminID, approved, nil)
	if v.State != "cancelled" {
		t.Fatal(v.State)
	}
	run(adminID, approved, nil)
	if yes, _ := store.Claimed(ctx, s.Pool, "km:900"); yes {
		t.Fatal("cancel did not free loss entitlement")
	}
	if yes, _ := store.Claimed(ctx, s.Pool, "delivery:567"); !yes {
		t.Fatal("cancel freed contract claim")
	}
	tx, _ := s.Pool.Begin(ctx)
	err = eve.ClaimDeliveryTx(ctx, tx, 567, "exchange", 99)
	tx.Rollback(ctx)
	if !errors.Is(err, eve.ErrDeliveryClaimed) {
		t.Fatal("global claim freed", err)
	}
	if err = s.CheckDelivery(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	// The next claim remains eligible until a completed contract settles it, even during review.
	v, err = store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "approved", Detail: raw(d), Award: 12345, Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	candidates = nil
	run(userID, command("request_cancel"), nil)
	stale := command("approve_cancel")
	c.ID = 568
	c.Title = v.Reference
	c.Status = "finished"
	c.AcceptorID = 123
	c.Issued = v.CreatedAt
	c.Completed = time.Now().UTC().Add(time.Second).Format(time.RFC3339)
	candidates = []eve.DeliveryContract{c}
	if err = s.CheckDelivery(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	run(adminID, stale, ErrConflict)
	v, _ = store.Read(ctx, s.Pool, v.ID)
	if v.State != "completed" {
		t.Fatal("payment did not win", v.State)
	}
	run(userID, command("request_cancel"), ErrConflict)
	run(adminID, command("approve_cancel"), ErrConflict)
}

func TestLossCancellationNoContractRequiresAttestationAndRevokedAccess(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "cancel_requested", Detail: raw(Detail{CharacterID: 123, Cancellation: &Cancellation{Reason: "mistake", RequestedAt: time.Now()}}), Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 41); err == nil {
		t.Fatal("pending cancellation allowed unsafe downgrade")
	}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return nil, nil
	}
	c := Command{Action: "approve_cancel", ID: v.ID, Version: v.Version, RequestKey: key(7201), Note: "verified in game"}
	if _, err = s.Execute(ctx, adminID, c); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing cache automatically cancelled", err)
	}
	c.ConfirmedNotDelivered = true
	scope := s.Scope
	s.Scope = func(context.Context, string, int64, bool) (bool, error) { return false, nil }
	if _, err = s.Execute(ctx, adminID, c); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked manager", err)
	}
	s.Scope = scope
	if _, err = s.Execute(ctx, adminID, c); err != nil {
		t.Fatal(err)
	}
}

func TestLossCancellationConcurrentFinishedPayment(t *testing.T) {
	s, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d := Detail{CharacterID: 123, Reviewer: adminID, Cancellation: &Cancellation{Reason: "mistake", RequestedAt: time.Now()}}
	v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "cancel_requested", Detail: raw(d), Award: 100, Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	c := eve.DeliveryContract{ID: 700, OwnerKind: "character", OwnerID: 123, Title: v.Reference, Type: "item_exchange", Status: "finished", IssuerID: 456, IssuerCorporationID: 10, AssigneeID: 123, AcceptorID: 123, Issued: v.CreatedAt, Completed: time.Now().Add(time.Second).UTC().Format(time.RFC3339), ItemsReady: true, Price: "0", Reward: "1", ContentToken: "fixed"}
	fenced, release := make(chan struct{}), make(chan struct{})
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 456: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	s.PaymentContracts = func(ctx context.Context, tx pgx.Tx, _ string, _ int64, _ string, _ time.Time) ([]eve.DeliveryContract, error) {
		if tx != nil {
			close(fenced)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return []eve.DeliveryContract{c}, nil
	}
	payment := make(chan error, 1)
	go func() { payment <- s.CheckDelivery(ctx, v.ID) }()
	select {
	case <-fenced:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	review := make(chan error, 1)
	go func() {
		_, err := s.Execute(ctx, adminID, Command{Action: "approve_cancel", ID: v.ID, Version: v.Version, RequestKey: key(7301), Note: "checked", ConfirmedNotDelivered: true})
		review <- err
	}()
	close(release)
	if err := <-payment; err != nil {
		t.Fatal(err)
	}
	if err := <-review; !errors.Is(err, ErrConflict) {
		t.Fatal("stale concurrent cancellation", err)
	}
	got, err := store.Read(ctx, s.Pool, v.ID)
	if err != nil || got.State != "completed" {
		t.Fatal(got.State, err)
	}
}
