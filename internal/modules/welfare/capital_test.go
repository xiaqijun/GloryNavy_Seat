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

func capitalFixture(t *testing.T) (*Service, *eve.DeliveryContract) {
	s, _ := fixture(t)
	s.IsShip = func(id int64) bool { return id == 23919 || id == 671 }
	s.ShipGroup = func(id int64) string {
		if id == 23919 {
			return "超级航母"
		}
		if id == 671 {
			return "泰坦"
		}
		return ""
	}
	c := &eve.DeliveryContract{ID: 901, OwnerKind: "character", OwnerID: 123, Type: "item_exchange", Status: "finished", IssuerID: 789, AcceptorID: 123, AssigneeID: 123, Price: "10000000000", Reward: "0", Issued: time.Now().Add(-2 * time.Hour), Completed: time.Now().Add(-time.Hour).Format(time.RFC3339), ItemsReady: true, Items: []eve.DeliveryItem{{TypeID: 23919, Quantity: 1, Included: true}}, ContentToken: "purchase-v1"}
	s.PurchaseContract = func(_ context.Context, _ string, id, contract int64) (eve.DeliveryContract, error) {
		if id != 123 || contract != c.ID {
			return eve.DeliveryContract{}, pgx.ErrNoRows
		}
		return *c, nil
	}
	s.Contract = func(_ context.Context, _ pgx.Tx, _ string, _ string, _, _ int64) (eve.DeliveryContract, error) {
		return *c, nil
	}
	return s, c
}

func TestCapitalPurchaseValidation(t *testing.T) {
	s := &Service{IsShip: func(id int64) bool { return id == 23919 || id == 671 }, ShipGroup: func(id int64) string {
		if id == 23919 {
			return "超级航母"
		}
		return "泰坦"
	}}
	base := eve.DeliveryContract{ID: 901, OwnerKind: "character", OwnerID: 123, Type: "item_exchange", Status: "finished", IssuerID: 789, AcceptorID: 123, AssigneeID: 123, Price: "123.45", Reward: "0", Issued: time.Now().Add(-2 * time.Hour), Completed: time.Now().Add(-time.Hour).Format(time.RFC3339), ItemsReady: true, Items: []eve.DeliveryItem{{TypeID: 23919, Quantity: 1, Included: true}}}
	hull, amount, e := s.capitalPurchase("supercarrier", 123, base)
	if e != nil || hull != 23919 || amount != 12345 {
		t.Fatal(hull, amount, e)
	}
	for _, change := range []func(*eve.DeliveryContract){
		func(c *eve.DeliveryContract) { c.Status = "outstanding" }, func(c *eve.DeliveryContract) { c.AcceptorID = 456 }, func(c *eve.DeliveryContract) { c.ItemsReady = false }, func(c *eve.DeliveryContract) { c.Type = "courier" }, func(c *eve.DeliveryContract) { c.Price = "-123.45" }, func(c *eve.DeliveryContract) { c.Reward = "1" }, func(c *eve.DeliveryContract) {
			c.Items = []eve.DeliveryItem{{TypeID: 23919, Quantity: 2, Included: true}}
		}, func(c *eve.DeliveryContract) {
			c.Items = append(c.Items, eve.DeliveryItem{TypeID: 671, Quantity: 1, Included: true})
		}, func(c *eve.DeliveryContract) {
			c.Items = append(c.Items, eve.DeliveryItem{TypeID: 34, Quantity: 1, Included: false})
		},
	} {
		c := base
		c.Items = append([]eve.DeliveryItem{}, base.Items...)
		change(&c)
		if _, _, e := s.capitalPurchase("supercarrier", 123, c); !errors.Is(e, ErrCapitalPurchase) {
			t.Fatal("accepted invalid purchase", c, e)
		}
	}
	if _, _, e = s.capitalPurchase("titan", 123, base); !errors.Is(e, ErrCapitalPurchase) {
		t.Fatal("accepted wrong class", e)
	}
}

func TestCapitalConfiguredRates(t *testing.T) {
	for _, kind := range []string{"supercarrier", "titan"} {
		legacy := int64(1000)
		if kind == "titan" {
			legacy = 500
		}
		if got, err := capitalAward(kind, 10000, Config{}); err != nil || got != legacy {
			t.Fatal(kind, got, err)
		}
		for _, rate := range []int64{-1, 0, 1, 1250, 10000, 10001} {
			c := Config{SubsidyRateBPS: &rate}
			err := validateConfig(kind, c)
			got, calcErr := capitalAward(kind, 12345, c)
			if rate < 1 || rate > 10000 {
				if !errors.Is(err, ErrInvalid) || !errors.Is(calcErr, ErrInvalid) {
					t.Fatal("invalid rate accepted", rate, err, calcErr)
				}
			} else if 12345*rate/10000 < 100 {
				if err != nil || !errors.Is(calcErr, ErrInvalid) {
					t.Fatal("subsidy below one ISK accepted", kind, rate, err, calcErr)
				}
			} else if err != nil || calcErr != nil || got != 12345*rate/10000 {
				t.Fatal(kind, rate, got, err, calcErr)
			}
		}
	}
	rate := int64(10000)
	if got, err := capitalAward("titan", 100000000000000, Config{SubsidyRateBPS: &rate}); err != nil || got != 100000000000000 {
		t.Fatal(got, err)
	}
}

func TestCapitalISKApplicationAndContractSettlement(t *testing.T) {
	s, purchase := capitalFixture(t)
	ctx := context.Background()
	rate := int64(1250)
	_, e := s.Execute(ctx, adminID, Command{Action: "configure", RequestKey: key(2100), CorporationID: 10, Kind: "supercarrier", Config: Config{Enabled: true, SubsidyRateBPS: &rate}})
	if e != nil {
		t.Fatal(e)
	}
	if e = store.SaveProfile(ctx, s.Pool, Member{AccountID: userID, Verified: false, History: map[string]string{"supercarrier": "unknown"}, Months: []string{}}); e != nil {
		t.Fatal(e)
	}
	cmd := Command{Action: "apply", RequestKey: key(2101), CorporationID: 10, Kind: "supercarrier", Detail: Detail{ContractID: 901, CharacterID: 999, ShipTypeID: 671, BaseMinor: 1}}
	data, e := s.Execute(ctx, userID, cmd)
	if e != nil {
		t.Fatal(e)
	}
	var v Case
	json.Unmarshal(data, &v)
	var d Detail
	json.Unmarshal(v.Detail, &d)
	if d.CharacterID != 123 || d.ShipTypeID != 23919 || d.BaseMinor != 1000000000000 || d.Purchase == nil {
		t.Fatal("client controlled evidence", d)
	}
	if _, e = s.Execute(ctx, userID, cmd); e != nil {
		t.Fatal("replay", e)
	}
	cmd.RequestKey = key(2102)
	if _, e = s.Execute(ctx, userID, cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate allowed", e)
	}
	approve := Command{Action: "approve", RequestKey: key(2103), ID: v.ID, Version: v.Version, Note: "reviewed", Detail: Detail{BaseMinor: 1}}
	policy, _, e := s.rule(ctx, 10, "supercarrier")
	if e != nil {
		t.Fatal(e)
	}
	rate = 2500
	if _, e = s.Execute(ctx, adminID, Command{Action: "configure", RequestKey: key(2106), CorporationID: 10, Kind: "supercarrier", Version: policy.Version, Config: Config{Enabled: true, SubsidyRateBPS: &rate}}); e != nil {
		t.Fatal(e)
	}
	purchase.ContentToken = "changed"
	if _, e = s.Execute(ctx, adminID, approve); !errors.Is(e, ErrCapitalPurchase) {
		t.Fatal("stale evidence accepted", e)
	}
	purchase.ContentToken = "purchase-v1"
	if _, e = s.Execute(ctx, adminID, approve); e != nil {
		t.Fatal(e)
	}
	v, _ = store.Read(ctx, s.Pool, v.ID)
	if v.Award != 125000000000 {
		t.Fatal("wrong amount", v.Award)
	}
	payout := eve.DeliveryContract{ID: 902, OwnerKind: "corporation", OwnerID: 10, Type: "item_exchange", Status: "outstanding", IssuerID: 789, IssuerCorporationID: 10, AssigneeID: 123, ForCorporation: true, Price: "0", Reward: "1250000000.00", Issued: time.Now().Add(time.Second), ItemsReady: true, ContentToken: "payout-v1", Items: []eve.DeliveryItem{}}
	s.Contract = func(context.Context, pgx.Tx, string, string, int64, int64) (eve.DeliveryContract, error) {
		return payout, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	s.Contracts = func(context.Context, string, int64, int64, int64, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{payout}, nil
	}

	payout.Title = v.Reference
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{payout}, nil
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 789: adminID}, nil
	}
	if _, e = s.Execute(ctx, adminID, linkCommand(v, &payout, 2104)); !errors.Is(e, ErrDelivery) {
		t.Fatal("manual link accepted", e)
	}
	if e = s.CheckDelivery(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
	v, _ = store.Read(ctx, s.Pool, v.ID)
	if v.State != "executing" {
		t.Fatal(v.State)
	}
	if _, e = s.Execute(ctx, adminID, Command{Action: "complete", ID: v.ID, Version: v.Version, RequestKey: key(2105), Note: "manual attempt", Detail: Detail{Receipt: "manual"}}); !errors.Is(e, ErrDelivery) {
		t.Fatal("manual bypass", e)
	}
	payout.Status = "finished"
	payout.AcceptorID = 123
	payout.Completed = payout.Issued.Add(time.Second).Format(time.RFC3339)
	if e = s.CheckDelivery(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
	v, _ = store.Read(ctx, s.Pool, v.ID)
	if v.State != "completed" {
		t.Fatal(v.State)
	}
	s.Contract = func(context.Context, pgx.Tx, string, string, int64, int64) (eve.DeliveryContract, error) {
		return *purchase, nil
	}
	if _, e = s.Execute(ctx, userID, cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("repeat claim", e)
	}
}

func TestCapitalBoundMemberQualification(t *testing.T) {
	s, _ := capitalFixture(t)
	ctx := context.Background()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, kind := range []string{"supercarrier", "titan"} {
		for _, history := range []string{"", "unknown", "unused", "used"} {
			if err = store.SaveProfile(ctx, tx, Member{AccountID: userID, Verified: false, History: map[string]string{kind: history}, Months: []string{}}); err != nil {
				t.Fatal(err)
			}
			_, err = s.qualify(ctx, tx, Case{AccountID: userID, Kind: kind}, Detail{ContractID: 901, ShipTypeID: 23919})
			if history == "used" {
				if !errors.Is(err, ErrRule) {
					t.Fatalf("%s claimed history accepted: %v", kind, err)
				}
			} else if err != nil {
				t.Fatalf("%s %q blocked without manual verification: %v", kind, history, err)
			}
		}
	}
}

func TestCapitalMissingBindingDenied(t *testing.T) {
	s, _ := capitalFixture(t)
	s.Characters = func(context.Context, string) ([]Character, error) { return nil, nil }
	_, err := s.Execute(context.Background(), userID, Command{Action: "apply", RequestKey: key(2180), CorporationID: 10, Kind: "supercarrier", Detail: Detail{ContractID: 901}})
	if err == nil {
		t.Fatal("unbound applicant accepted")
	}
}

func TestCapitalPaymentDirectionAndExactAmount(t *testing.T) {
	for _, v := range []struct {
		price, reward string
		want          bool
	}{{"0", "123", true}, {"0", "123.45", true}, {"0", "123.450", true}, {"123.45", "0", false}, {"-123.45", "0", false}, {"0", "123.451", false}, {"0", "123.44", false}, {"0", "124", false}, {"1", "124.45", false}, {"0", "NaN", false}} {
		c := eve.DeliveryContract{Price: v.price, Reward: v.reward}
		if capitalPaymentMatches(c, 12345) != v.want {
			t.Fatal(v)
		}
	}
	c := eve.DeliveryContract{Price: "0", Reward: "123.45", Items: []eve.DeliveryItem{{TypeID: 34, Quantity: 1, Included: false}}}
	if capitalPaymentMatches(c, 12345) {
		t.Fatal("required goods accepted")
	}
	v := Case{Kind: "titan", CorporationID: 10, Award: 12345, CreatedAt: time.Now().Add(-time.Hour)}
	d := Detail{CharacterID: 123, ContractID: 901}
	c = eve.DeliveryContract{ID: 902, Type: "item_exchange", Status: "outstanding", Price: "0", Reward: "123.45", IssuerID: 789, IssuerCorporationID: 10, AssigneeID: 123, ItemsReady: true, Issued: time.Now()}
	if reason := deliveryReason(v, d, c); reason != "" {
		t.Fatal(reason)
	}
	for _, change := range []func(*eve.DeliveryContract){func(c *eve.DeliveryContract) { c.ID = 901 }, func(c *eve.DeliveryContract) { c.AssigneeID = 456 }, func(c *eve.DeliveryContract) { c.IssuerCorporationID = 11 }, func(c *eve.DeliveryContract) { c.Issued = v.CreatedAt.Add(-time.Second) }, func(c *eve.DeliveryContract) { c.Status = "cancelled" }, func(c *eve.DeliveryContract) { c.ItemsReady = false }} {
		bad := c
		change(&bad)
		if deliveryReason(v, d, bad) == "" {
			t.Fatal("invalid payout", bad)
		}
	}
}

func TestCapitalMergePendingAllowance(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	for n, account := range []string{userID, otherID} {
		_, e := store.Save(ctx, s.Pool, Case{AccountID: account, CorporationID: 10, Kind: "supercarrier", State: "submitted", Detail: raw(Detail{ContractID: int64(n + 1)}), Keys: []string{}})
		if e != nil {
			t.Fatal(e)
		}
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.MergeAccountTx(ctx, tx, userID, otherID, false)
	tx.Rollback(ctx)
	if e == nil {
		t.Fatal("merged pending capital entitlements")
	}
	if _, e = s.Pool.Exec(ctx, `UPDATE welfare_cases SET state='completed'`); e != nil {
		t.Fatal(e)
	}
	tx, e = s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = s.MergeAccountTx(ctx, tx, userID, otherID, false); e != nil {
		t.Fatal("completed history should remain mergeable", e)
	}
}

func TestCapitalOpeningIgnoresLegacyEffectiveTime(t *testing.T) {
	s, _ := capitalFixture(t)
	ctx := context.Background()
	for n, at := range []string{"", "2099-01-01T00:00:00Z", "legacy invalid date"} {
		if e := validateConfig("supercarrier", Config{Enabled: true, EffectiveAt: at}); e != nil {
			t.Fatal(e)
		}
		if e := validateConfig("titan", Config{Enabled: true, EffectiveAt: at}); e != nil {
			t.Fatal(e)
		}
		_, e := s.Pool.Exec(ctx, `INSERT INTO welfare_policies(corporation_id,kind,config) VALUES(10,'supercarrier',$1) ON CONFLICT(corporation_id,kind) DO UPDATE SET config=excluded.config,version=welfare_policies.version+1`, raw(Config{Enabled: true, EffectiveAt: at}))
		if e != nil {
			t.Fatal(e)
		}
		body, e := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(2200 + n*2), CorporationID: 10, Kind: "supercarrier", Detail: Detail{ContractID: 901}})
		if e != nil {
			t.Fatal(at, e)
		}
		var v Case
		if e = json.Unmarshal(body, &v); e != nil {
			t.Fatal(e)
		}
		if _, e = s.Execute(ctx, userID, Command{Action: "cancel", RequestKey: key(2201 + n*2), ID: v.ID, Version: v.Version, Note: "withdraw fixture"}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.Pool.Exec(ctx, `UPDATE welfare_policies SET config=jsonb_set(config,'{enabled}','false')`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(2210), CorporationID: 10, Kind: "supercarrier", Detail: Detail{ContractID: 901}}); !errors.Is(e, ErrRule) {
		t.Fatal("closed program allowed application", e)
	}
	if e := validateConfig("growth_gila", Config{Enabled: true}); !errors.Is(e, ErrInvalid) {
		t.Fatal("changed growth time validation", e)
	}
}
