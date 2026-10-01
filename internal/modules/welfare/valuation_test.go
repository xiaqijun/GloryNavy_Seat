package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/market"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func sameJSON(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func valuationLoss() Loss {
	return Loss{ID: 987, CharacterID: 123, CorporationID: 10, ShipTypeID: 17715, ShipName: "毒蜥级", At: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Items: []eve.BattleItem{
		{TypeID: 34, Name: "三钛合金", Slot: "5", Quantity: 5, Destroyed: 2, Dropped: 3},
		{TypeID: 35, Name: "嵌套货物", Slot: "5/5", Quantity: 7, Destroyed: 7},
	}}
}
func TestValuationPurchaseValidationAndNoFallback(t *testing.T) {
	loss := valuationLoss()
	d := Detail{CharacterID: 123, ShipTypeID: 17715, KillmailID: 987, ContractID: 456, OccurredAt: loss.At.Format(time.RFC3339), SyncedLoss: true, LossEvidence: &loss}
	contract := eve.DeliveryContract{ID: 456, OwnerKind: "character", OwnerID: 123, Type: "item_exchange", Status: "finished", ItemsReady: true, AcceptorID: 123, IssuerID: 456, Price: "12345678.91", Reward: "0", Issued: loss.At.Add(-time.Hour), Completed: loss.At.Add(-time.Minute).Format(time.RFC3339), Items: []eve.DeliveryItem{{TypeID: 17715, Quantity: 1, Included: true}}}
	s := &Service{PurchaseContract: func(context.Context, string, int64, int64) (eve.DeliveryContract, error) { return contract, nil }, EstimateLoss: func(context.Context, []market.Item) (market.Appraisal, int64, error) {
		t.Fatal("invalid contract silently fell back to market")
		return market.Appraisal{}, 0, nil
	}}
	v := s.valuate(context.Background(), userID, d)
	if v.State != "ready" || v.AmountMinor != 1234567891 || v.Contract == nil {
		t.Fatalf("quote %+v", v)
	}
	for _, mutate := range []func(*eve.DeliveryContract){
		func(c *eve.DeliveryContract) { c.Status = "outstanding" },
		func(c *eve.DeliveryContract) { c.AcceptorID = 999 },
		func(c *eve.DeliveryContract) { c.ItemsReady = false },
		func(c *eve.DeliveryContract) { c.Completed = loss.At.Add(time.Minute).Format(time.RFC3339) },
		func(c *eve.DeliveryContract) { c.Price = "-1" },
		func(c *eve.DeliveryContract) { c.Reward = "100" },
		func(c *eve.DeliveryContract) {
			c.Items = []eve.DeliveryItem{{TypeID: 17715, Quantity: 2, Included: true}}
		},
		func(c *eve.DeliveryContract) {
			c.Items = []eve.DeliveryItem{{TypeID: 17715, Quantity: 1, Included: false}}
		},
	} {
		copy := contract
		mutate(&copy)
		if purchaseReason(copy, d, nil) == "" {
			t.Fatalf("accepted %+v", copy)
		}
	}
	contract.Status = "reversed"
	if v := s.valuate(context.Background(), userID, d); v.State == "ready" || v.AmountMinor != 0 {
		t.Fatal(v)
	}
}

func TestValuationApplyRefreshApproveAuditAndReplay(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	loss := valuationLoss()
	s.Losses = func(context.Context, string, int64, int64, int64, int64) ([]Loss, error) { return []Loss{loss}, nil }
	complete := false
	quoteCalls := 0
	queuedID := int64(0)
	s.EnqueueValuationTx = func(_ context.Context, _ pgx.Tx, id int64) error {
		queuedID = id
		return nil
	}
	s.EstimateLoss = func(quoteCtx context.Context, items []market.Item) (market.Appraisal, int64, error) {
		quoteCalls++
		if deadline, ok := quoteCtx.Deadline(); !ok || time.Until(deadline) > 8*time.Second {
			t.Fatal("quote exceeds HTTP publication budget")
		}
		if len(items) != 3 || items[0].TypeID != loss.ShipTypeID || items[0].Quantity != 1 || items[1].Quantity != 5 || items[2].Quantity != 7 {
			t.Fatalf("lost cargo/dropped/hull: %+v", items)
		}
		return market.Appraisal{Complete: complete, RatioBPS: 8000, Adjusted: market.Amounts{Mid: "1000.01"}}, 2, nil
	}
	apply := Command{Action: "apply", CorporationID: 10, Kind: "solo", RequestKey: key(501), Detail: Detail{CharacterID: 123, KillmailID: 987, ShipTypeID: 17715, SyncedLoss: true, Description: "PVP 损失", Evidence: "同步击毁报告", Valuation: &Valuation{State: "ready", AmountMinor: 1}}}
	b, err := s.Execute(ctx, userID, apply)
	if err != nil {
		t.Fatal(err)
	}
	var c Case
	_ = json.Unmarshal(b, &c)
	var d Detail
	_ = json.Unmarshal(c.Detail, &d)
	if d.Valuation == nil || d.Valuation.State != "pending" || d.Valuation.AmountMinor != 0 || quoteCalls != 0 || queuedID != c.ID {
		t.Fatal("application waited for market quote or did not enqueue", d.Valuation, quoteCalls, queuedID)
	}
	ids, err := store.PendingValuations(ctx, s.Pool)
	if err != nil || len(ids) != 1 || ids[0] != c.ID {
		t.Fatal("pending quote was not recoverable", ids, err)
	}
	if err = s.AutoAppraise(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.AutoAppraise(ctx, c.ID); err != nil || quoteCalls != 1 {
		t.Fatal("automatic quote repeated", err, quoteCalls)
	}
	c, err = s.Read(ctx, userID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(c.Detail, &d); err != nil || d.Valuation == nil || d.Valuation.State != "incomplete" {
		t.Fatal("automatic quote did not persist", err, d.Valuation)
	}
	ids, err = store.PendingValuations(ctx, s.Pool)
	if err != nil || len(ids) != 0 {
		t.Fatal("finished quote remained pending", ids, err)
	}
	approve := Command{Action: "approve", ID: c.ID, Version: c.Version, RequestKey: key(502), Note: "核对报告", Detail: Detail{BaseMinor: 100001}}
	if _, err = s.Execute(ctx, adminID, approve); !errors.Is(err, ErrValuation) {
		t.Fatalf("partial approved %v", err)
	}
	complete = true
	refresh := Command{Action: "appraise", ID: c.ID, Version: c.Version, RequestKey: key(503), Note: "重新核价"}
	if _, err = s.Execute(ctx, otherID, refresh); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign refresh %v", err)
	}
	b, err = s.Execute(ctx, userID, refresh)
	if err != nil {
		t.Fatal(err)
	}
	if replay, e := s.Execute(ctx, userID, refresh); e != nil || !sameJSON(replay, b) {
		t.Fatal("refresh replay", e)
	}
	if _, err = s.Execute(ctx, adminID, approve); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale approval %v", err)
	}
	_ = json.Unmarshal(b, &c)
	_ = json.Unmarshal(c.Detail, &d)
	if d.Valuation.State != "ready" || d.Valuation.AmountMinor != 100001 || d.Valuation.SettingsVersion != 2 {
		t.Fatal(d.Valuation)
	}
	approve.Version = c.Version
	approve.Detail.BaseMinor++
	if _, err = s.Execute(ctx, adminID, approve); !errors.Is(err, ErrValuation) {
		t.Fatalf("tampered amount %v", err)
	}
	approve.Detail.BaseMinor--
	b, err = s.Execute(ctx, adminID, approve)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(b, &c)
	_ = json.Unmarshal(c.Detail, &d)
	if c.Award != 100001 || d.PricingMode != "automatic" {
		t.Fatal(c, d)
	}
	again, err := s.Execute(ctx, adminID, approve)
	if err != nil || !sameJSON(again, b) {
		t.Fatal("approval replay", err)
	}
	refresh.Version = c.Version
	refresh.RequestKey = key(504)
	if _, err = s.Execute(ctx, userID, refresh); !errors.Is(err, ErrConflict) {
		t.Fatal("approved repriced", err)
	}
}

func TestValuationManualOverrideAndIncompleteEvidence(t *testing.T) {
	d := Detail{}
	c := Command{Detail: Detail{BaseMinor: 100}}
	if approveValuation(&d, c) == nil {
		t.Fatal("missing quote approved")
	}
	c.ManualPricing = true
	if approveValuation(&d, c) == nil {
		t.Fatal("missing reason")
	}
	c.Note = "变异装备缺报价，人工核对合同"
	if approveValuation(&d, c) != nil || d.PricingMode != "manual" {
		t.Fatal(d)
	}
	loss := valuationLoss()
	loss.Items[0].Quantity = 4
	if _, err := lossMarketItems(Detail{SyncedLoss: true, LossEvidence: &loss, CharacterID: 123, ShipTypeID: 17715, KillmailID: 987}); err == nil {
		t.Fatal("inconsistent quantity accepted")
	}
}
