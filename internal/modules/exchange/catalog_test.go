package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogPhysicalValidation(t *testing.T) {
	for _, p := range []PhysicalReward{{}, {ISKMinor: 99}, {ISKMinor: 99, Items: []PhysicalItem{{ID: 34, Quantity: 1}}}, {Items: []PhysicalItem{{ID: 34, Quantity: 0}}}, {Items: []PhysicalItem{{ID: 34, Quantity: 1}, {ID: 34, Quantity: 1}}}, {Fittings: []PhysicalFitting{{ID: 1, Quantity: 101}}}} {
		if physicalValid(p) {
			t.Fatal(p)
		}
	}
	req := httptest.NewRequest("POST", "/catalog", strings.NewReader(`{"name":"coins","content":{"fittings":[],"items":[],"coins_minor":100}}`))
	w := httptest.NewRecorder()
	var c CatalogEdit
	if readBody(w, req, &c) || w.Code != 400 {
		t.Fatal("currency accepted", w.Code)
	}
}
func TestCatalogSharedSnapshotAndCancellation(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	s.RewardFitting = func(_ context.Context, user string, id int64) (PhysicalFitting, error) {
		if user != manager || id != 1 {
			return PhysicalFitting{}, pgx.ErrNoRows
		}
		return PhysicalFitting{ID: 1, ShipTypeID: 34, CorporationID: 10, Name: "Saved fit", Version: 7, Fit: json.RawMessage(`{"ship_type_id":"34","items":[]}`)}, nil
	}
	edit := CatalogEdit{CatalogEntry: CatalogEntry{Name: "Starter pack", Content: PhysicalReward{Fittings: []PhysicalFitting{{ID: 1, Quantity: 2, Name: "spoofed"}}, Items: []PhysicalItem{{ID: 34, Quantity: 100}}}}, RequestKey: rewardKey(900)}
	if _, err := s.SaveCatalog(ctx, rewardKey(777), edit); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("non-admin saved", err)
	}
	id, err := s.SaveCatalog(ctx, manager, edit)
	if err != nil {
		t.Fatal(err)
	}
	if again, e := s.SaveCatalog(ctx, manager, edit); e != nil || again != id {
		t.Fatal("replay", again, e)
	}
	lib, err := s.Catalog(ctx, manager, 0)
	if err != nil || len(lib.Items) != 2 {
		t.Fatal(lib, err)
	}
	for _, listedOnly := range []bool{false, true} {
		shop, e := s.shopFor(ctx, manager, manager, 0, listedOnly)
		want := 2
		if listedOnly {
			want = 1
		}
		if e != nil || len(shop.Rewards) != want {
			t.Fatalf("listed=%v: %+v %v", listedOnly, shop, e)
		}
	}
	s.Administrator = func(context.Context, string) (bool, error) { return false, nil }
	memberShop, e := s.shopFor(ctx, manager, manager, 0, false)
	if e != nil || len(memberShop.Rewards) != 1 {
		t.Fatalf("member all scope: %+v %v", memberShop, e)
	}
	s.Administrator = func(_ context.Context, user string) (bool, error) { return user == manager || user == reviewer, nil }
	r := lib.Items[1]
	if r.Content.Fittings[0].Name != "Saved fit" || r.Content.Items[0].Name != "三钛合金" {
		t.Fatal(r)
	}
	if _, err = s.WelfareReward(ctx, manager, 11, id, 1); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross corp", err)
	}
	if _, err = s.WelfareReward(ctx, manager, 10, id, 2); !errors.Is(err, ErrConflict) {
		t.Fatal("version", err)
	}
	if _, err = s.WelfareReward(ctx, manager, 10, id, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.EditShop(ctx, manager, "reward", ShopEdit{ID: id, Version: 1, RequestKey: rewardKey(901), TypeID: 34, Quantity: 1, Value: 100, Stock: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	order, err := s.ClaimReward(ctx, manager, ClaimReward{RewardID: id, RewardVersion: 2, RateVersion: sh.Version, RecipientID: 1, RequestKey: rewardKey(902)})
	if err != nil {
		t.Fatal(err)
	}
	edit.ID = id
	edit.Version = 1
	edit.RequestKey = rewardKey(903)
	edit.Content.Items[0].Quantity = 999
	if _, err = s.SaveCatalog(ctx, manager, edit); err != nil {
		t.Fatal(err)
	}
	orders, err := s.RewardOrders(ctx, manager, false, 0)
	if err != nil || orders.Items[0].Content.Items[0].Quantity != 100 || orders.Items[0].Name != "Starter pack" {
		t.Fatal(orders, err)
	}
	current, err := s.Shop(ctx, manager, 0)
	if err != nil || current.Rewards[1].Enabled || current.Rewards[1].Stock != 0 {
		t.Fatal(current, err)
	}
	requestCancel(t, s, manager, order, 9004)
	if err = s.DecideOrder(ctx, reviewer, order, OrderDecision{Version: 2, RequestKey: rewardKey(904), State: "cancelled", UndeliveredConfirmed: true, Note: "cancel old pack"}); err != nil {
		t.Fatal(err)
	}
	current, err = s.Shop(ctx, manager, 0)
	if err != nil || current.Rewards[1].Stock != 0 || current.Reserved != 0 {
		t.Fatal("old stock returned to new pack", current, err)
	}
	edit.Version = 2
	edit.Archived = true
	edit.RequestKey = rewardKey(905)
	if _, err = s.SaveCatalog(ctx, manager, edit); err != nil {
		t.Fatal(err)
	}
	if _, err = s.WelfareReward(ctx, manager, 10, id, 3); !errors.Is(err, ErrConflict) {
		t.Fatal("archived selected", err)
	}
	current, err = s.Shop(ctx, manager, 0)
	if err != nil || len(current.Rewards) != 1 {
		t.Fatal(current, err)
	}
}
