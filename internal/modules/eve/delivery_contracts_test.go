package eve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/testutil"
)

func TestDeliveryContractScopedSnapshotAndPrecision(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	q := store.New(pool)
	for _, scope := range []struct {
		kind string
		id   int64
	}{{"corporation", 10}, {"character", 123}, {"corporation", 20}} {
		payload := []byte(`{"issuer_id":456,"issuer_corporation_id":10,"for_corporation":true,"assignee_id":123,"acceptor_id":0,"price":123456789012345.67,"reward":0,"title":"测试交付","date_issued":"2026-09-02T00:00:00Z"}`)
		if _, e := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: scope.kind, OwnerID: scope.id, ContractID: 987, ContractType: "item_exchange", Status: "outstanding", Payload: payload}); e != nil {
			t.Fatal(e)
		}
		if e := q.InsertContractItems(ctx, store.InsertContractItemsParams{OwnerKind: scope.kind, OwnerID: scope.id, ContractID: 987, Items: []byte(`[{"record_id":1,"type_id":17715,"quantity":9007199254740993,"is_included":true,"is_singleton":true}]`)}); e != nil {
			t.Fatal(e)
		}
	}
	h := NewContractHTTP(pool, nil, nil)
	h.StaticData = nil
	allowed := true
	h.Owners = func(context.Context, string) ([]ContractOwner, error) {
		return []ContractOwner{{Kind: "corporation", ID: 10}}, nil
	}
	h.LookupOwner = func(_ context.Context, _ string, kind string, id int64) (ContractOwner, error) {
		if !allowed || kind != "corporation" || id != 10 {
			return ContractOwner{}, pgx.ErrNoRows
		}
		return ContractOwner{Kind: kind, ID: id}, nil
	}
	if _, e := h.PurchaseContract(ctx, "admin", 123, 987); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("purchase guard bypass", e)
	}
	originalLookup := h.LookupOwner
	h.LookupOwner = func(_ context.Context, _ string, kind string, id int64) (ContractOwner, error) {
		if kind != "character" || id != 123 {
			return ContractOwner{}, pgx.ErrNoRows
		}
		return ContractOwner{Kind: kind, ID: id}, nil
	}
	if c, e := h.PurchaseContract(ctx, "member", 123, 987); e != nil || c.ID != 987 || c.OwnerKind != "character" {
		t.Fatal("purchase scope", c, e)
	}
	h.LookupOwner = originalLookup
	if c, err := h.ReadContract(ctx, "admin", "corporation", 10, 987); err != nil || c.OwnerKind != "corporation" || len(c.Items) != 1 {
		t.Fatal(c, err)
	}
	if _, err := h.ReadContract(ctx, "admin", "corporation", 20, 987); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross-corporation appraisal", err)
	}
	list, e := h.DeliveryContracts(ctx, "admin", 10, 123, 0, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if e != nil || len(list) != 1 {
		t.Fatalf("read: %+v %v", list, e)
	}
	c := list[0]
	if c.Price != "123456789012345.67" || c.Items[0].Quantity != 9007199254740993 || c.ItemsReady {
		t.Fatalf("precision/state: %+v", c)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	one, e := h.DeliveryContractTx(ctx, tx, "admin", "corporation", 10, 987)
	if e != nil || one.ContentToken != c.ContentToken {
		t.Fatalf("snapshot: %+v %v", one, e)
	}
	if _, e = h.DeliveryContractTx(ctx, tx, "admin", "corporation", 20, 987); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign corp: %v", e)
	}
	allowed = false
	if _, e = h.DeliveryContractTx(ctx, tx, "admin", "corporation", 10, 987); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("revoked access: %v", e)
	}
}
func TestDeliveryContractContentFingerprint(t *testing.T) {
	makeRow := func(status, price string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"id":"9","owner_kind":"corporation","owner_id":"10","type":"item_exchange","status":%q,"payload":{"issuer_id":1,"issuer_corporation_id":10,"assignee_id":2,"price":%s,"date_issued":"2026-09-02T00:00:00Z"},"items":[{"type_id":"17715","quantity":"1","included":true}]}`, status, price))
	}
	a, e := decodeDelivery(makeRow("outstanding", "0"))
	if e != nil {
		t.Fatal(e)
	}
	b, _ := decodeDelivery(makeRow("finished", "0"))
	c, _ := decodeDelivery(makeRow("finished", "1"))
	if a.ContentToken == "" || a.ContentToken != b.ContentToken || a.ContentToken == c.ContentToken {
		t.Fatal("status/terms fingerprint boundary")
	}
	missing, e := decodeDelivery(json.RawMessage(`{"id":"1","payload":{},"items":[]}`))
	if e != nil || missing.Price != "" || missing.Reward != "" {
		t.Fatal("missing monetary fields inferred as zero", e)
	}
}
