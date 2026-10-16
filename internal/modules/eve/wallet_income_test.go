package eve

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

func TestWalletSpeedsExcludeTransfersAfterLatestVariantSelection(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	c, err := store.New(s.pool).GetCredential(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	save := func(id int64, ref, amount string, observed, occurred time.Time) {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"id": id, "ref_type": ref, "amount": json.Number(amount), "first_party_id": 999999, "second_party_id": ch.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err = store.SaveWalletObservation(ctx, s.pool, "character", ch.ID, 0, "journal", id, ch.ID, c.OwnerHash, observed, occurred, raw); err != nil {
			t.Fatal(err)
		}
	}
	for i, ref := range []string{"player_donation", "corporation_account_withdrawal", "corporation_payment", "corporation_bulk_payment"} {
		save(int64(i+1), ref, "1000", now, now)
		save(int64(i+11), ref, "-500", now, now)
	}
	// Unknown counterpart ownership must not make direct cash transfers income.
	// Real earnings with words such as transfer/reward remain counted by type.
	for i, ref := range []string{"bounty_prizes", "ess_escrow_transfer", "agent_mission_reward", "market_transaction", "player_trading", "corporation_dividend_payment", "future_official_type", ""} {
		save(int64(i+21), ref, "10.25", now, now)
	}
	save(40, "transaction_tax", "-2.50", now, now)
	save(41, "market_transaction", "123456789012345.67", now, now)
	// Filter the latest variant, not an older version that happens to pass.
	save(50, "bounty_prizes", "700", now.Add(-time.Minute), now)
	save(50, "player_donation", "900", now, now)
	save(51, "player_donation", "800", now.Add(-time.Minute), now)
	save(51, "bounty_prizes", "20", now, now)
	save(60, "bounty_prizes", "9999", now, from.Add(-time.Second))
	if err = store.SaveWalletObservation(ctx, s.pool, "character", ch.ID, 0, "balance", 0, ch.ID, c.OwnerHash, now, now, []byte(`{"balance":123456789012345.67}`)); err != nil {
		t.Fatal(err)
	}
	rows, err := s.auth.WalletSummaries(ctx, []int64{ch.ID}, from)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	row := rows[0]
	for _, v := range []struct{ name, actual, expected string }{
		{"gross income", row.Income, "123456789017347.67"},
		{"gross expense", row.Expense, "2002.50"},
		{"earned income", row.EarnedIncome, "123456789012447.67"},
		{"spent expense", row.SpentExpense, "2.50"},
	} {
		actual, ok := new(big.Rat).SetString(v.actual)
		expected, _ := new(big.Rat).SetString(v.expected)
		if !ok || actual.Cmp(expected) != 0 {
			t.Fatalf("%s: got %q want %q", v.name, v.actual, v.expected)
		}
	}
	if row.Balance == nil || *row.Balance != "123456789012345.67" {
		t.Fatal("balance changed", row)
	}
	detail, err := s.auth.WalletData(ctx, WalletFilter{Kind: "character", Part: "journal", Owner: ch.ID, Ref: "player_donation"})
	if err != nil || len(detail) != 4 {
		t.Fatal("transfer history changed", len(detail), err)
	}
	// An empty period is a real zero, and revoked evidence contributes nothing.
	rows, err = s.auth.WalletSummaries(ctx, []int64{ch.ID}, now.Add(time.Second))
	if err != nil || rows[0].EarnedIncome != "0" || rows[0].SpentExpense != "0" {
		t.Fatal(rows, err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE eve_credentials SET scopes='{}' WHERE character_id=$1`, ch.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = s.auth.WalletSummaries(ctx, []int64{ch.ID}, from)
	if err != nil || len(rows) != 1 || rows[0].Balance != nil || rows[0].EarnedIncome != "0" || rows[0].SpentExpense != "0" {
		t.Fatal(fmt.Sprint(rows), err)
	}
}
