package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func deliveryFixture(t *testing.T) (*Service, Case, *eve.DeliveryContract) {
	t.Helper()
	s, _ := fixture(t)
	s.ClaimDelivery = eve.ClaimDeliveryTx
	ctx := context.Background()
	d := Detail{CharacterID: 123, ShipTypeID: 17715, KillmailID: 12, OccurredAt: "2026-09-01T00:00:00Z"}
	v, e := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "supercarrier", State: "approved", Detail: raw(d), Award: 100, Keys: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE welfare_cases SET created_at='2026-09-01T00:00:00Z' WHERE id=$1", v.ID); e != nil {
		t.Fatal(e)
	}
	v, _ = store.Read(ctx, s.Pool, v.ID)
	c := &eve.DeliveryContract{ID: 456, OwnerKind: "corporation", OwnerID: 10, Type: "item_exchange", Status: "outstanding", IssuerID: 789, IssuerCorporationID: 10, AssigneeID: 123, ForCorporation: true, ItemsReady: true, Price: "0", Reward: "1", Issued: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), Items: []eve.DeliveryItem{{TypeID: 17715, Quantity: 1, Included: true}}, ContentToken: "snapshot"}
	s.Contracts = func(context.Context, string, int64, int64, int64, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{*c}, nil
	}
	s.Contract = func(context.Context, pgx.Tx, string, string, int64, int64) (eve.DeliveryContract, error) {
		return *c, nil
	}
	return s, v, c
}
func linkCommand(v Case, c *eve.DeliveryContract, n int) Command {
	return Command{Action: "link_delivery", ID: v.ID, Version: v.Version, RequestKey: key(n), Note: "核对交付", Delivery: DeliverySelection{c.OwnerKind, c.OwnerID, c.ID, c.ContentToken}}
}
func TestDeliveryLegacyLossLinkStillCompletes(t *testing.T) {
	s, v, c := deliveryFixture(t)
	ctx := context.Background()
	if _, err := s.Execute(ctx, adminID, linkCommand(v, c, 590)); err != nil {
		t.Fatal(err)
	}
	// Reproduce a pre-upgrade manually linked reimbursement with a ship delivery.
	if _, err := s.Pool.Exec(ctx, "UPDATE welfare_cases SET kind='srp' WHERE id=$1", v.ID); err != nil {
		t.Fatal(err)
	}
	c.Reward = "0"
	c.Status = "finished"
	c.AcceptorID = 123
	c.Completed = "2026-09-03T00:00:00Z"
	if err := s.CheckDelivery(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	current, err := store.Read(ctx, s.Pool, v.ID)
	if err != nil || current.State != "completed" {
		t.Fatalf("legacy delivery: %s %v", current.State, err)
	}
}
func TestDeliveryRecommendationConfirmationAndCompletion(t *testing.T) {
	s, v, c := deliveryFixture(t)
	ctx := context.Background()
	rows, e := s.DeliveryCandidates(ctx, adminID, v.ID, 0)
	if e != nil || len(rows) != 1 || !rows[0].CanLink {
		t.Fatalf("recommendation: %+v %v", rows, e)
	}
	unchanged, _ := store.Read(ctx, s.Pool, v.ID)
	if unchanged.Version != v.Version || unchanged.State != "approved" {
		t.Fatal("read changed case")
	}
	if _, e = s.DeliveryCandidates(ctx, userID, v.ID, 0); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("member accessed recommendation: %v", e)
	}
	command := linkCommand(v, c, 500)
	bad := command
	bad.RequestKey = key(501)
	bad.Delivery.ContentToken = "stale"
	if _, e = s.Execute(ctx, adminID, bad); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale preview accepted: %v", e)
	}
	for range 2 {
		if _, e = s.Execute(ctx, adminID, command); e != nil {
			t.Fatal(e)
		}
	}
	linked, _ := store.Read(ctx, s.Pool, v.ID)
	if linked.State != "executing" || linked.Version != 2 {
		t.Fatalf("link/replay: %+v", linked)
	}
	if _, e = s.Execute(ctx, adminID, Command{Action: "complete", ID: v.ID, Version: 2, RequestKey: key(502), Note: "手填", Detail: Detail{Receipt: "received"}}); !errors.Is(e, ErrDelivery) {
		t.Fatalf("manual bypass: %v", e)
	}
	for _, state := range []string{"outstanding", "cancelled", "finished_issuer", "finished_contractor", "failed", "reversed"} {
		c.Status = state
		c.AcceptorID = 123
		c.Completed = "2026-09-03T00:00:00Z"
		if e = s.CheckDelivery(ctx, v.ID); e != nil {
			t.Fatal(e)
		}
		current, _ := store.Read(ctx, s.Pool, v.ID)
		if current.State != "executing" {
			t.Fatalf("%s completed case", state)
		}
	}
	c.Status = "finished"
	c.AcceptorID = 456
	if e = s.CheckDelivery(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
	current, _ := store.Read(ctx, s.Pool, v.ID)
	if current.State != "executing" {
		t.Fatal("wrong recipient completed")
	}
	c.AcceptorID = 123
	c.ContentToken = "changed"
	if e = s.CheckDelivery(ctx, v.ID); !errors.Is(e, ErrDelivery) {
		t.Fatalf("changed terms accepted: %v", e)
	}
	c.ContentToken = "snapshot"
	admin := s.Administrator
	s.Administrator = func(context.Context, string) (bool, error) { return false, nil }
	if e = s.CheckDelivery(ctx, v.ID); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("revoked admin: %v", e)
	}
	s.Administrator = admin
	if e = s.CheckDelivery(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
	current, _ = store.Read(ctx, s.Pool, v.ID)
	if current.State != "completed" {
		t.Fatal("finished not completed")
	}
	var d Detail
	_ = json.Unmarshal(current.Detail, &d)
	if d.Delivery.VerifiedAt == nil || d.Receipt == "" {
		t.Fatal("missing evidence")
	}
	history, _ := store.History(ctx, s.Pool, v.ID)
	if e = s.CheckDelivery(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
	again, _ := store.History(ctx, s.Pool, v.ID)
	if len(history) != len(again) {
		t.Fatal("duplicate completion audit")
	}
}
func TestDeliveryUniqueAcrossCasesAndSelfDenied(t *testing.T) {
	s, v, c := deliveryFixture(t)
	ctx := context.Background()
	second, e := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "supercarrier", State: "approved", Award: 100, Detail: v.Detail, Keys: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE welfare_cases SET created_at='2026-09-01T00:00:00Z' WHERE id=$1", second.ID); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for n, one := range []Case{v, second} {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Execute(ctx, adminID, linkCommand(one, c, 510+n)); results <- e }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for e := range results {
		if e == nil {
			successes++
		} else if !errors.Is(e, ErrConflict) {
			t.Fatal(e)
		}
	}
	if successes != 1 {
		t.Fatalf("%d linked one contract", successes)
	}
	s.Administrator = func(context.Context, string) (bool, error) { return true, nil }
	s.Scope = func(context.Context, string, int64, bool) (bool, error) { return true, nil }
	fresh, e := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "supercarrier", State: "approved", Award: 100, Detail: v.Detail, Keys: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Execute(ctx, userID, linkCommand(fresh, c, 519)); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("self linked: %v", e)
	}
}
func TestDeliveryMatchingGuards(t *testing.T) {
	d := Detail{CharacterID: 123, ShipTypeID: 17715, OccurredAt: "2026-09-01T00:00:00Z"}
	v := Case{CorporationID: 10}
	c := eve.DeliveryContract{Type: "item_exchange", Status: "outstanding", AssigneeID: 123, IssuerID: 456, IssuerCorporationID: 10, ItemsReady: true, Issued: time.Now(), Items: []eve.DeliveryItem{{TypeID: 17715, Quantity: 1, Included: true}}}
	for _, modify := range []func(*eve.DeliveryContract){func(c *eve.DeliveryContract) { c.AssigneeID = 99 }, func(c *eve.DeliveryContract) { c.IssuerCorporationID = 99 }, func(c *eve.DeliveryContract) { c.Type = "courier" }, func(c *eve.DeliveryContract) { c.ItemsReady = false }, func(c *eve.DeliveryContract) { c.Items = nil }, func(c *eve.DeliveryContract) { c.Status = "deleted" }, func(c *eve.DeliveryContract) { c.Issued = time.Time{} }} {
		changed := c
		modify(&changed)
		if deliveryReason(v, d, changed) == "" {
			t.Fatalf("invalid contract matched: %+v", changed)
		}
	}
	c.Status = "finished"
	c.AcceptorID = 123
	c.Completed = "invalid"
	if deliveryFinished(c, 123) {
		t.Fatal("malformed completion accepted")
	}
}
