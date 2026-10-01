package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"sync"
	"testing"
	"time"
)

func TestGrowthFulfillmentSnapshotGuards(t *testing.T) {
	s := &Service{MatchReward: exchange.MatchRewardDelivery}
	d := Detail{CharacterID: 123, ShipTypeID: 587, Rewards: &GrowthRewards{Fittings: []GrowthFitting{{ID: 1, Quantity: 2, ShipTypeID: 587, Fit: json.RawMessage(`{"ship_type_id":"587","items":[{"type_id":"34","quantity":10}]}`)}}, Items: []GrowthItem{{ID: 34, Quantity: 5}}, Coins: 100}}
	v := Case{Kind: "growth_fitting_1", Reference: "WF-test", CorporationID: 10, CreatedAt: time.Now().Add(-time.Hour)}
	c := eve.DeliveryContract{ID: 1, ContentToken: "terms", Type: "item_exchange", Title: v.Reference, IssuerID: 456, IssuerCorporationID: 10, AssigneeID: 123, Issued: time.Now().Add(-time.Minute), Status: "outstanding", ItemsReady: true, Price: "0", Reward: "0", Items: []eve.DeliveryItem{{TypeID: 587, Quantity: 2, Included: true}, {TypeID: 34, Quantity: 25, Included: true}}}
	if err := s.prepareFulfillment(v.Kind, &d); err != nil {
		t.Fatal(err)
	}
	if got := fulfillmentState(v, d, c, s.MatchReward); got != "awaiting_acceptance" {
		t.Fatal(got)
	}
	for _, change := range []func(*eve.DeliveryContract){
		func(c *eve.DeliveryContract) { c.Items = c.Items[:1] },
		func(c *eve.DeliveryContract) { c.Items[1].Quantity-- },
		func(c *eve.DeliveryContract) { c.Items[1].Included = false },
		func(c *eve.DeliveryContract) { n := int64(-2); c.Items[1].RawQuantity = &n },
		func(c *eve.DeliveryContract) { c.Price = "1" },
		func(c *eve.DeliveryContract) { c.Reward = "1" },
		func(c *eve.DeliveryContract) { c.Title = "other" },
	} {
		bad := c
		bad.Items = append([]eve.DeliveryItem{}, c.Items...)
		change(&bad)
		if got := fulfillmentState(v, d, bad, s.MatchReward); got != "mismatch" {
			t.Fatal("unsafe delivery", got)
		}
	}
	d.Rewards = nil
	if s.prepareFulfillment(v.Kind, &d) == nil {
		t.Fatal("missing snapshot accepted")
	}
	d.Rule.FittingID = 1
	d.FittingEvidence = json.RawMessage(`{"name":"old","fit":{"ship_type_id":"587","items":[]}}`)
	if err := s.prepareFulfillment(v.Kind, &d); err != nil || d.Rewards.Fittings[0].Quantity != 1 {
		t.Fatal("frozen legacy snapshot", err)
	}
}

func TestUnifiedCancellationPreservesDeliveryReservation(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return nil, nil
	}
	for n, kind := range []string{"growth_fitting_1", "supercarrier", "titan"} {
		claim := "once:" + userID + ":" + kind
		v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: kind, State: "approved", Detail: raw(Detail{CharacterID: 123, Reviewer: adminID}), Keys: []string{claim, "delivery:998"}})
		if err != nil {
			t.Fatal(err)
		}
		if err = store.Claim(ctx, s.Pool, claim, v.ID); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			if err = store.Claim(ctx, s.Pool, "delivery:998", v.ID); err != nil {
				t.Fatal(err)
			}
		}
		for i, a := range []string{"request_cancel", "approve_cancel"} {
			actor := userID
			if i > 0 {
				actor = adminID
			}
			out, err := s.Execute(ctx, actor, Command{Action: a, ID: v.ID, Version: v.Version, RequestKey: key(8200 + n*10 + i), Note: "not delivered", ConfirmedNotDelivered: true})
			if err != nil {
				t.Fatal(kind, a, err)
			}
			json.Unmarshal(out, &v)
		}
		if v.State != "cancelled" {
			t.Fatal(v.State)
		}
		if yes, _ := store.Claimed(ctx, s.Pool, claim); yes {
			t.Fatal("entitlement retained")
		}
		if yes, _ := store.Claimed(ctx, s.Pool, "delivery:998"); !yes {
			t.Fatal("delivery reservation lost")
		}
	}
}

func TestLegacyCoinReleaseRequiresExplicitActionAndIsAtomic(t *testing.T) {
	s, coins := fixture(t)
	ctx := context.Background()
	d := Detail{CharacterID: 123, ShipTypeID: 587, Reviewer: adminID, Rewards: &GrowthRewards{Coins: 125}}
	v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "growth_fitting_1", State: "approved", Detail: raw(d), Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID}, nil
	}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return nil, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	if err = s.CheckDelivery(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	v, _ = store.Read(ctx, s.Pool, v.ID)
	if v.State != "approved" {
		t.Fatal("historical coins backfilled")
	}
	cmd := Command{Action: "release_coins", ID: v.ID, Version: v.Version, RequestKey: key(8300), Note: "confirm legacy reward"}
	if _, err = s.Execute(ctx, userID, cmd); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("self release", err)
	}
	coins.AllowNew = false
	if _, err = s.Execute(ctx, adminID, cmd); err == nil {
		t.Fatal("coin service failure ignored")
	}
	coins.AllowNew = true
	for range 2 {
		if _, err = s.Execute(ctx, adminID, cmd); err != nil {
			t.Fatal(err)
		}
	}
	var total, count int64
	if err = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0),count(*) FROM exchange_coin_ledger`).Scan(&total, &count); err != nil || total != 125 || count != 1 {
		t.Fatal(total, count, err)
	}
}

func TestGrowthCancellationAndDeliveryRace(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	d := Detail{CharacterID: 123, ShipTypeID: 587, Reviewer: adminID, Rewards: &GrowthRewards{Items: []GrowthItem{{ID: 34, Quantity: 10}}, Coins: 125}, Cancellation: &Cancellation{Reason: "cancel", RequestedAt: time.Now()}}
	v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "growth_fitting_1", State: "cancel_requested", Detail: raw(d), Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	c := eve.DeliveryContract{ID: 990, Title: v.Reference, Type: "item_exchange", Status: "finished", IssuerID: 789, IssuerCorporationID: 10, AssigneeID: 123, AcceptorID: 123, Price: "0", Reward: "0", Issued: v.CreatedAt.Add(time.Second), Completed: v.CreatedAt.Add(2 * time.Second).Format(time.RFC3339), ItemsReady: true, ContentToken: "terms", Items: []eve.DeliveryItem{{TypeID: 34, Quantity: 10, Included: true}}}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{c}, nil
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 789: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.CheckDelivery(ctx, v.ID) }()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, e := s.Execute(ctx, adminID, Command{Action: "approve_cancel", ID: v.ID, Version: v.Version, RequestKey: key(8400), Note: "checked", ConfirmedNotDelivered: true})
		if e == nil {
			errs <- errors.New("paid reward cancellation succeeded")
		} else if !errors.Is(e, ErrConflict) && !errors.Is(e, ErrCancellationPayment) {
			errs <- e
		}
	}()
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil && !errors.Is(e, ErrConflict) {
			t.Fatal(e)
		}
	}
	current, err := store.Read(ctx, s.Pool, v.ID)
	if err != nil || current.State != "completed" {
		t.Fatal(current.State, err)
	}
	var sum, count int64
	if err = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0),count(*) FROM exchange_coin_ledger`).Scan(&sum, &count); err != nil || sum != 125 || count != 1 {
		t.Fatal(sum, count, err)
	}
}

func TestGrowthCashRewardRequiresContract(t *testing.T) {
	s := &Service{MatchReward: exchange.MatchRewardDelivery}
	d := Detail{Rewards: &GrowthRewards{ISKMinor: 12345, Coins: 100, Fittings: []GrowthFitting{}, Items: []GrowthItem{}}}
	if coinOnly(d) {
		t.Fatal("cash bypassed contract fulfillment")
	}
	if err := s.prepareFulfillment("growth_fitting_1", &d); err != nil {
		t.Fatal(err)
	}
	c := eve.DeliveryContract{Price: "0", Reward: "123.45", ItemsReady: true}
	if ok, err := s.MatchReward(raw(d.Rewards), c); err != nil || !ok {
		t.Fatal(ok, err)
	}
	c.Reward = "0"
	if ok, _ := s.MatchReward(raw(d.Rewards), c); ok {
		t.Fatal("cash missing")
	}
}
