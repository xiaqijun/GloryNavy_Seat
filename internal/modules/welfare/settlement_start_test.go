package welfare

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestSettlementStartsExchangeAtomically(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.MainCharacterID = func(context.Context, string) (int64, error) { return 123, nil }
	s.ExchangeDelivery = func(context.Context, int64) error { return nil }
	s.ExchangeSettlementEligible = func(context.Context, int64) error { return nil }
	s.ExchangeSettlementAccount = func(context.Context, int64) (string, error) { return userID, nil }
	s.ExchangeSettlementReward = func(context.Context, int64) (SettlementReward, error) {
		return SettlementReward{AccountID: userID, RecipientID: 123, Items: []SettlementItemSummary{{TypeID: 34, Quantity: 1}}}, nil
	}
	input := SettlementInput{RequestKey: key(9501), Items: []SettlementInputItem{{Source: "exchange", ID: 1}, {Source: "exchange", ID: 2}}}
	if _, e := s.CreateSettlementBatch(ctx, adminID, input); !errors.Is(e, ErrSettlement) {
		t.Fatal("missing dispatch boundary allowed", e)
	}
	if _, e := s.Pool.Exec(ctx, `CREATE TABLE test_dispatch(id bigint PRIMARY KEY)`); e != nil {
		t.Fatal(e)
	}
	fail := true
	s.ExchangeSettlementStartTx = func(ctx context.Context, tx pgx.Tx, actor, account string, id int64) error {
		if actor != adminID || account != userID {
			t.Fatal("incorrect scope", actor, account)
		}
		if fail && id == 2 {
			return ErrConflict
		}
		_, e := tx.Exec(ctx, `INSERT INTO test_dispatch VALUES($1)`, id)
		return e
	}
	if _, e := s.CreateSettlementBatch(ctx, adminID, input); !errors.Is(e, ErrConflict) {
		t.Fatal("failed dispatch accepted", e)
	}
	var n int
	if e := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM test_dispatch)+(SELECT count(*) FROM welfare_contract_settlement_batches)`).Scan(&n); e != nil || n != 0 {
		t.Fatal("partial batch committed", n, e)
	}
	fail = false
	first, e := s.CreateSettlementBatch(ctx, adminID, input)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.CreateSettlementBatch(ctx, adminID, input)
	if e != nil || again.Batch.ID != first.Batch.ID {
		t.Fatal("idempotent batch", again, e)
	}
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM test_dispatch`).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
}
