package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

type rewardNames struct{}

func (rewardNames) TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return map[int64]eve.StaticTypeName{34: {Name: "三钛合金", Source: "sde"}}, nil
}
func (rewardNames) SolarSystemNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return nil, nil
}

func TestGrowthRewardValidation(t *testing.T) {
	for _, kind := range []string{"growth_fitting_0", "growth_fitting_-1", "growth_fitting_01", "growth_fitting_9223372036854775808", "growth_unknown"} {
		if validKind(kind) {
			t.Fatal(kind)
		}
	}
	for _, r := range []*GrowthRewards{nil, {}, {Coins: -1}, {Coins: 1000000000001}, {ISKMinor: 99}, {Fittings: []GrowthFitting{{ID: 1, Quantity: 0}}}, {Items: []GrowthItem{{ID: 34, Quantity: 1}, {ID: 34, Quantity: 1}}}} {
		if validateGrowthRewards(r) == nil {
			t.Fatalf("invalid rewards accepted: %+v", r)
		}
	}
	if validateGrowthRewards(&GrowthRewards{Coins: 1}) != nil {
		t.Fatal("coin-only reward rejected")
	}
	s := Service{Names: rewardNames{}, GrowthFitting: func(_ context.Context, _ string, corp, id int64) (GrowthFitting, error) {
		if corp != 10 || id != 1 {
			return GrowthFitting{}, pgx.ErrNoRows
		}
		return GrowthFitting{ID: 1, Name: "军团方案", ShipTypeID: 587, Version: 2, Fit: json.RawMessage(`{"name":"保存版本"}`)}, nil
	}}
	r := &GrowthRewards{Fittings: []GrowthFitting{{ID: 1, Quantity: 2, Name: "伪造名称", Fit: json.RawMessage(`{"fake":true}`)}}, Items: []GrowthItem{{ID: 34, Quantity: 20, Name: "伪造物品"}}, Coins: 10}
	out, err := s.growthRewards(context.Background(), "user", 10, r, true)
	if err != nil || out.Fittings[0].Name != "军团方案" || string(out.Fittings[0].Fit) != `{"name":"保存版本"}` || out.Items[0].Name != "三钛合金" {
		t.Fatal(out, err)
	}
	if _, err = s.growthRewards(context.Background(), "user", 20, r, true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross-corporation fitting accepted", err)
	}
	r.Items[0].ID = 999
	if _, err = s.growthRewards(context.Background(), "user", 10, r, true); !errors.Is(err, ErrRule) {
		t.Fatal("unknown item accepted", err)
	}
}

func TestGrowthLibrarySnapshotKeepsSeparateCoins(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.GrowthFitting = func(context.Context, string, int64, int64) (GrowthFitting, error) {
		return GrowthFitting{ID: 1, Name: "Growth ship", ShipTypeID: 587, Version: 1}, nil
	}
	s.LibraryReward = func(_ context.Context, _ string, corp, id, version int64) (*GrowthRewards, error) {
		if corp != 10 || id != 7 || version != 3 {
			return nil, ErrRule
		}
		return &GrowthRewards{Fittings: []GrowthFitting{{ID: 2, Name: "Frozen fit", Quantity: 2, ShipTypeID: 587, Version: 9, Fit: json.RawMessage(`{"name":"frozen"}`)}}, Items: []GrowthItem{{ID: 34, Quantity: 10}}}, nil
	}
	c := Command{Action: "configure", RequestKey: key(901), CorporationID: 10, Kind: "growth_fitting_1", Config: Config{Enabled: true, EffectiveAt: time.Now().Add(-time.Hour).Format(time.RFC3339), FittingID: 1, RewardID: 7, RewardVersion: 3, Rewards: &GrowthRewards{Coins: 125, Items: []GrowthItem{{ID: 999, Quantity: 999}}}}}
	if _, err := s.Execute(ctx, adminID, c); err != nil {
		t.Fatal(err)
	}
	s.LibraryReward = func(context.Context, string, int64, int64, int64) (*GrowthRewards, error) {
		t.Fatal("application unexpectedly refreshed library")
		return nil, ErrRule
	}
	raw, err := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(902), CorporationID: 10, Kind: c.Kind, Detail: Detail{CharacterID: 123, Description: "Growth", Evidence: "Proof"}})
	if err != nil {
		t.Fatal(err)
	}
	var row Case
	json.Unmarshal(raw, &row)
	var d Detail
	json.Unmarshal(row.Detail, &d)
	if d.Rewards == nil || d.Rewards.Coins != 125 || d.Rewards.Items[0].ID != 34 || d.Rewards.Fittings[0].Version != 9 || string(d.Rewards.Fittings[0].Fit) != `{"name": "frozen"}` && string(d.Rewards.Fittings[0].Fit) != `{"name":"frozen"}` {
		t.Fatal(d)
	}
}

func TestGrowthItemSearchChecksAdministratorAndCorporation(t *testing.T) {
	calls := 0
	s := &Service{Administrator: func(_ context.Context, user string) (bool, error) { return user == "admin", nil }, Scope: func(_ context.Context, user string, corp int64, manage bool) (bool, error) {
		return user == "admin" && corp == 10 && manage, nil
	}, SearchItems: func(_ context.Context, q string) ([]eve.StaticTypeName, error) {
		calls++
		return []eve.StaticTypeName{{ID: 34, Name: "三钛合金"}}, nil
	}}
	h := Handler{Service: s, User: func(r *http.Request) string { return r.Header.Get("Test-Actor") }}
	for _, c := range []struct {
		actor, query string
		status       int
	}{{"member", "corporation_id=10&q=34", 404}, {"admin", "corporation_id=20&q=34", 404}, {"admin", "corporation_id=10&q=x", 400}, {"admin", "corporation_id=10&q=34", 200}} {
		r := httptest.NewRequest("GET", "/items?"+c.query, nil)
		r.Header.Set("Test-Actor", c.actor)
		w := httptest.NewRecorder()
		h.items(w, r)
		if w.Code != c.status {
			t.Fatal(w.Code, w.Body.String())
		}
		if c.status != 200 && strings.Contains(w.Body.String(), "三钛合金") {
			t.Fatal("unauthorized item data")
		}
	}
	if calls != 1 {
		t.Fatal("search ran before authorization", calls)
	}
}

func TestGrowthProjectSnapshotAndAtomicCompletion(t *testing.T) {
	s, coins := fixture(t)
	ctx := context.Background()
	deleted := false
	s.Names = rewardNames{}
	s.GrowthFitting = func(_ context.Context, _ string, corp, id int64) (GrowthFitting, error) {
		if deleted || corp != 10 || id < 1 || id > 2 {
			return GrowthFitting{}, pgx.ErrNoRows
		}
		return GrowthFitting{ID: id, Name: "自定义军团方案", ShipTypeID: 587, Version: 3, Fit: json.RawMessage(`{"ship_type_id":"587","items":[]}`)}, nil
	}
	cfg := Config{Enabled: true, EffectiveAt: time.Now().Add(-time.Hour).Format(time.RFC3339), FittingID: 1, Rewards: &GrowthRewards{Fittings: []GrowthFitting{{ID: 2, Quantity: 2}}, Items: []GrowthItem{{ID: 34, Quantity: 100}}, Coins: 125}}
	configure := Command{Action: "configure", RequestKey: key(801), CorporationID: 10, Kind: "growth_fitting_1", Config: cfg}
	if _, err := s.Execute(ctx, userID, configure); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("non-admin configured rewards", err)
	}
	if _, err := s.Execute(ctx, adminID, configure); err != nil {
		t.Fatal(err)
	}
	applied, err := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(802), CorporationID: 10, Kind: configure.Kind, Detail: Detail{CharacterID: 123, Rewards: &GrowthRewards{Coins: 999999}}})
	if err != nil {
		t.Fatal(err)
	}
	var c Case
	if err = json.Unmarshal(applied, &c); err != nil {
		t.Fatal(err)
	}
	var d Detail
	json.Unmarshal(c.Detail, &d)
	if d.Rewards == nil || d.Rewards.Coins != 125 || d.Rewards.Fittings[0].Quantity != 2 || len(d.Rewards.Fittings[0].Fit) == 0 || d.Rule.ProjectName != "自定义军团方案" || d.ShipTypeID != 587 {
		t.Fatal("bad snapshot", d)
	}
	if d.Description != "" || d.Evidence != "" || d.ContractID != 0 {
		t.Fatal("growth fabricated manual evidence", d)
	}
	if _, err := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(811), CorporationID: 10, Kind: configure.Kind, Detail: Detail{CharacterID: 999}}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign recipient", err)
	}
	if _, err := s.Execute(ctx, adminID, Command{Action: "apply", RequestKey: key(812), CorporationID: 10, Kind: "solo", Detail: Detail{CharacterID: 123}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("loss accepted missing explanation", err)
	}
	// Future policy edits must not alter a pending case's promised rewards.
	configure.Version = 1
	configure.RequestKey = key(803)
	configure.Config.Rewards.Coins = 900
	if _, err = s.Execute(ctx, adminID, configure); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(ctx, adminID, Command{Action: "profile", RequestKey: key(804), CorporationID: 10, AccountID: userID, Note: "核实历史", Profile: Member{Verified: true, History: map[string]string{configure.Kind: "unused"}}}); err != nil {
		t.Fatal(err)
	}
	for n, action := range []string{"approve"} {
		result, e := s.Execute(ctx, adminID, Command{Action: action, RequestKey: key(805 + n), ID: c.ID, Version: c.Version, Note: "人工核验"})
		if e != nil {
			t.Fatal(e)
		}
		json.Unmarshal(result, &c)
	}

	payout := eve.DeliveryContract{ID: 990, Type: "item_exchange", Title: c.Reference, Status: "finished", IssuerID: 789, IssuerCorporationID: 10, AssigneeID: 123, AcceptorID: 123, Price: "0", Reward: "0", Issued: c.CreatedAt.Add(time.Second), Completed: c.CreatedAt.Add(2 * time.Second).Format(time.RFC3339), ItemsReady: true, ContentToken: "snapshot", Items: []eve.DeliveryItem{{TypeID: 587, Quantity: 2, Included: true}, {TypeID: 34, Quantity: 100, Included: true}}}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{payout}, nil
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 789: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	coins.AllowNew = false
	if err = s.CheckDelivery(ctx, c.ID); err == nil {
		t.Fatal("disabled currency service accepted new coins")
	}
	stored, _ := store.Read(ctx, s.Pool, c.ID)
	if stored.State != "approved" {
		t.Fatal("case committed despite currency failure")
	}
	coins.AllowNew = true
	for range 2 {
		if err = s.CheckDelivery(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	var total, count int64
	if err = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0),count(*) FROM exchange_coin_ledger`).Scan(&total, &count); err != nil || total != 125 || count != 1 {
		t.Fatal("duplicate/wrong award", total, count, err)
	}
	stored, _ = store.Read(ctx, s.Pool, c.ID)
	if stored.State != "completed" {
		t.Fatal(stored.State)
	}
	// Completed claims are rejected before another application can be created.
	if _, err := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(808), CorporationID: 10, Kind: configure.Kind, Detail: Detail{CharacterID: 123}}); !errors.Is(err, ErrGrowthClaimed) {
		t.Fatal("duplicate growth application", err)
	}
	// Deleting the source fit still allows administrators to close applications.
	deleted = true
	configure.Version = 2
	configure.RequestKey = key(810)
	configure.Config.Enabled = false
	if _, err = s.Execute(ctx, adminID, configure); err != nil {
		t.Fatal("cannot disable deleted fitting project", err)
	}
	if _, err := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(813), CorporationID: 10, Kind: configure.Kind, Detail: Detail{CharacterID: 123}}); !errors.Is(err, ErrRule) {
		t.Fatal("disabled growth project accepted", err)
	}
}
