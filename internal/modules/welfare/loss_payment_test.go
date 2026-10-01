package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"testing"
	"time"
)

func TestLossPaymentAutomaticGuardsAndCompletion(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	d := Detail{CharacterID: 123, Reviewer: adminID}
	v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "approved", Detail: raw(d), Award: 12345, Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE welfare_cases SET created_at=now()-interval '1 day' WHERE id=$1", v.ID); err != nil {
		t.Fatal(err)
	}
	v, _ = store.Read(ctx, s.Pool, v.ID)
	c := eve.DeliveryContract{ID: 567, OwnerKind: "character", OwnerID: 123, Title: lossReference(v), Type: "item_exchange", Status: "outstanding", IssuerID: 456, IssuerCorporationID: 10, AssigneeID: 123, Issued: time.Now().Add(-time.Hour), ItemsReady: true, Price: "0", Reward: "123", ContentToken: "fixed"}
	candidates := []eve.DeliveryContract{}
	s.PaymentContracts = func(_ context.Context, _ pgx.Tx, owner string, char int64, ref string, _ time.Time) ([]eve.DeliveryContract, error) {
		if owner != userID || char != 123 || ref != lossReference(v) {
			t.Fatal("wrong scoped lookup")
		}
		return candidates, nil
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 456: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	check := func(want string) {
		t.Helper()
		if err := s.CheckDelivery(ctx, v.ID); err != nil {
			t.Fatal(err)
		}
		got, _ := store.Read(ctx, s.Pool, v.ID)
		var d Detail
		_ = json.Unmarshal(got.Detail, &d)
		if d.PaymentStatus != want {
			t.Fatalf("want %s got %s", want, d.PaymentStatus)
		}
	}
	check("waiting_contract")
	candidates = []eve.DeliveryContract{c, c}
	check("multiple_contracts")
	for _, mutate := range []func(*eve.DeliveryContract){
		func(c *eve.DeliveryContract) { c.Title += "9" }, func(c *eve.DeliveryContract) { c.Reward = "123.44" }, func(c *eve.DeliveryContract) { c.Price = "1" }, func(c *eve.DeliveryContract) { c.AssigneeID = 999 }, func(c *eve.DeliveryContract) { c.IssuerCorporationID = 20 }, func(c *eve.DeliveryContract) {
			c.Items = []eve.DeliveryItem{{TypeID: 34, Quantity: 1, Included: false}}
		},
	} {
		bad := c
		mutate(&bad)
		candidates = []eve.DeliveryContract{bad}
		check("mismatch")
	}
	bad := c
	bad.ItemsReady = false
	candidates = []eve.DeliveryContract{bad}
	check("waiting_items")
	bad = c
	bad.IssuerID = 999
	candidates = []eve.DeliveryContract{bad}
	check("issuer_unverified")
	candidates = []eve.DeliveryContract{c}
	claim := s.ClaimDelivery
	s.ClaimDelivery = func(context.Context, pgx.Tx, int64, string, int64) error { return eve.ErrDeliveryClaimed }
	check("contract_claimed")
	s.ClaimDelivery = claim
	scope := s.Scope
	s.Scope = func(context.Context, string, int64, bool) (bool, error) { return false, nil }
	if err := s.CheckDelivery(ctx, v.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked reviewer accepted", err)
	}
	s.Scope = scope
	bindings := s.PaymentBindings
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: otherID, 456: adminID}, nil
	}
	if err := s.CheckDelivery(ctx, v.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rebound recipient accepted", err)
	}
	s.PaymentBindings = bindings
	check("awaiting_acceptance")
	got, _ := store.Read(ctx, s.Pool, v.ID)
	if got.State != "executing" {
		t.Fatal(got.State)
	}
	priorVersion := got.Version
	check("awaiting_acceptance")
	got, _ = store.Read(ctx, s.Pool, v.ID)
	if got.Version != priorVersion {
		t.Fatal("unchanged evidence creates duplicate audit")
	}
	candidates = nil
	check("contract_unavailable")
	changed := c
	changed.ContentToken = "changed"
	candidates = []eve.DeliveryContract{changed}
	check("mismatch")
	c.Status = "finished"
	c.AcceptorID = 123
	c.Completed = time.Now().Add(-time.Minute).Format(time.RFC3339)
	candidates = []eve.DeliveryContract{c}
	check("finished")
	got, _ = store.Read(ctx, s.Pool, v.ID)
	if got.State != "completed" {
		t.Fatal(got.State)
	}
	var done Detail
	_ = json.Unmarshal(got.Detail, &done)
	if done.Delivery == nil || !done.Delivery.Automatic || done.Delivery.VerifiedAt == nil {
		t.Fatal("missing verified evidence")
	}
	version := got.Version
	if err = s.CheckDelivery(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Read(ctx, s.Pool, v.ID)
	if got.Version != version {
		t.Fatal("repeated completion")
	}
	ids, err := store.DueDeliveries(ctx, s.Pool)
	if err != nil || len(ids) != 0 {
		t.Fatal("completed case remained scheduled", ids, err)
	}
}

func TestLossPolicyApprovalSnapshot(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	rate, cap := int64(5000), int64(40000)
	cfg := Config{LossRateBPS: &rate, LossCapMinor: &cap}
	if _, e := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "srp", Config: cfg, RequestKey: key(6001)}); e != nil {
		t.Fatal(e)
	}
	d := Detail{CharacterID: 123, ShipTypeID: 34, KillmailID: 900, EventID: 1, OccurredAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}
	v, e := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "srp", State: "submitted", Detail: raw(d), Keys: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	version := int64(0)
	cmd := Command{Action: "approve", ID: v.ID, Version: 1, RequestKey: key(6002), Note: "审核", Detail: Detail{BaseMinor: 100001}, LossPolicyVersion: &version}
	if _, e = s.Execute(ctx, adminID, cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("stale policy accepted", e)
	}
	version = 1
	result, e := s.Execute(ctx, adminID, cmd)
	if e != nil {
		t.Fatal(e)
	}
	_ = json.Unmarshal(result, &v)
	_ = json.Unmarshal(v.Detail, &d)
	if v.Award != 40000 || d.BaseMinor != 100001 || d.Rule.LossRateBPS == nil || *d.Rule.LossRateBPS != 5000 {
		t.Fatal("wrong approval snapshot", v, d)
	}
	rate = 8000
	cfg.LossRateBPS = &rate
	if _, e = s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "srp", Version: 1, Config: cfg, RequestKey: key(6003)}); e != nil {
		t.Fatal(e)
	}
	result, e = s.Execute(ctx, adminID, cmd)
	if e != nil {
		t.Fatal("replay after policy change", e)
	}
	_ = json.Unmarshal(result, &v)
	if v.Award != 40000 {
		t.Fatal("old approval repriced")
	}
}

func TestLossAwardBounds(t *testing.T) {
	if _, e := lossAward(99, Config{}); !errors.Is(e, ErrInvalid) {
		t.Fatal("sub-ISK reimbursement accepted", e)
	}
	rate, cap := int64(3333), int64(100)
	n, e := lossAward(10001, Config{LossRateBPS: &rate})
	if e != nil || n != 3333 {
		t.Fatal(n, e)
	}
	n, e = lossAward(10001, Config{LossRateBPS: &rate, LossCapMinor: &cap})
	if e != nil || n != 100 {
		t.Fatal(n, e)
	}
	n, e = lossAward(100000000000000, Config{})
	if e != nil || n != 100000000000000 {
		t.Fatal(n, e)
	}
	for _, bad := range []int64{-1, 0, 10001} {
		if validateConfig("solo", Config{LossRateBPS: &bad}) == nil {
			t.Fatal("invalid rate accepted", bad)
		}
	}
	if validateConfig("growth_gila", Config{LossRateBPS: &rate}) == nil {
		t.Fatal("rate escaped loss scope")
	}
	tooSmall := int64(99)
	if validateConfig("solo", Config{LossDailyCapMinor: &tooSmall}) == nil {
		t.Fatal("sub-ISK period cap accepted")
	}
}

func TestLossPeriodQuotaUsesApprovalDateAndActiveAwards(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	d := Detail{CharacterID: 123, KillmailID: 901}
	prior, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "srp", State: "approved", Detail: raw(d), Award: 20000, Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO welfare_audit(actor_id,request_key,fingerprint,case_id,action,note,result)
		VALUES($1,$2,'quota-test',$3,'approve','test','{}'::jsonb)`, adminID, key(6099), prior.ID)
	if err != nil {
		t.Fatal(err)
	}
	proposed := Case{AccountID: userID, CorporationID: 10, Kind: "srp", Award: 10000}
	cap := int64(30000)
	for _, cfg := range []Config{{LossDailyCapMinor: &cap}, {LossWeeklyCapMinor: &cap}, {LossMonthlyCapMinor: &cap}} {
		if err = s.checkLossQuota(ctx, s.Pool, proposed, cfg); err != nil {
			t.Fatal("exact remaining quota rejected", err)
		}
		proposed.Award++
		if err = s.checkLossQuota(ctx, s.Pool, proposed, cfg); !errors.Is(err, ErrLossQuota) {
			t.Fatal("over-quota award accepted", err)
		}
		proposed.Award--
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_audit SET created_at=created_at-interval '1 day' WHERE case_id=$1 AND action='approve'`, prior.ID); err != nil {
		t.Fatal(err)
	}
	proposed.Award = 30000
	if err = s.checkLossQuota(ctx, s.Pool, proposed, Config{LossDailyCapMinor: &cap}); err != nil {
		t.Fatal("previous-day approval counted today", err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_audit SET created_at=now() WHERE case_id=$1 AND action='approve'`, prior.ID); err != nil {
		t.Fatal(err)
	}
	policy, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "srp", Config: Config{LossDailyCapMinor: &cap}, RequestKey: key(6098)})
	if err != nil || len(policy) == 0 {
		t.Fatal("cannot configure quota", err)
	}
	pending := Detail{CharacterID: 123, ShipTypeID: 34, KillmailID: 902, EventID: 1, OccurredAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}
	caseToApprove, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "srp", State: "submitted", Detail: raw(pending), Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	version := int64(1)
	_, err = s.Execute(ctx, adminID, Command{Action: "approve", ID: caseToApprove.ID, Version: caseToApprove.Version, RequestKey: key(6097), Note: "审核", Detail: Detail{BaseMinor: 10001}, LossPolicyVersion: &version})
	if !errors.Is(err, ErrLossQuota) {
		t.Fatal("approval did not enforce quota", err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_cases SET state='cancelled' WHERE id=$1`, prior.ID); err != nil {
		t.Fatal(err)
	}
	proposed.Award = 30000
	if err = s.checkLossQuota(ctx, s.Pool, proposed, Config{LossMonthlyCapMinor: &cap}); err != nil {
		t.Fatal("cancelled award still consumes quota", err)
	}
	if validateConfig("growth_gila", Config{LossDailyCapMinor: &cap}) == nil {
		t.Fatal("loss quota escaped loss policy scope")
	}
}
