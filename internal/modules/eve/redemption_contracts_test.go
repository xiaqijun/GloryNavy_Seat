package eve

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/testutil"
	"testing"
	"time"
)

func TestRedemptionContractExactScopedLookup(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	q := store.New(pool)
	for i := 1; i <= 62; i++ {
		title := "unrelated"
		if i == 1 {
			title = "GNV-EX-7"
		}
		if i == 2 {
			title = "GNV-EX-70"
		}
		payload := []byte(fmt.Sprintf(`{"title":%q,"issuer_id":456,"assignee_id":123,"price":0,"reward":0,"date_issued":"2026-09-20T00:00:00Z"}`, title))
		if _, e := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: "character", OwnerID: 123, ContractID: int64(i), ContractType: "item_exchange", Status: "outstanding", Payload: payload}); e != nil {
			t.Fatal(e)
		}
	}
	h := NewContractHTTP(pool, nil, nil)
	allowed := true
	h.LookupOwner = func(_ context.Context, _ string, kind string, id int64) (ContractOwner, error) {
		if !allowed || kind != "character" || id != 123 {
			return ContractOwner{}, pgx.ErrNoRows
		}
		return ContractOwner{Kind: kind, ID: id}, nil
	}
	since := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	rows, e := h.RedemptionContracts(ctx, tx, "member", 123, "GNV-EX-7", since)
	if e != nil || len(rows) != 1 || rows[0].ID != 1 {
		t.Fatal("exact lookup behind more than fifty contracts", rows, e)
	}
	allowed = false
	if _, e = h.RedemptionContracts(ctx, tx, "member", 123, "GNV-EX-7", since); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("revoked binding read", e)
	}
	if e = ClaimDeliveryTx(ctx, tx, 1, "welfare", 8); e != nil {
		t.Fatal(e)
	}
	if e = ClaimDeliveryTx(ctx, tx, 1, "exchange", 8); !errors.Is(e, ErrDeliveryClaimed) {
		t.Fatal("cross-module reuse", e)
	}
	if e = ClaimDeliveryTx(ctx, tx, 1, "welfare", 8); e != nil {
		t.Fatal("claim replay", e)
	}
}
