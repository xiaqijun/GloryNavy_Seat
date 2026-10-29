package eve

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

func TestCreditWalletExcludesOnlyOwnTransfersAndUsesJournalFreshness(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	credential, err := store.New(s.pool).GetCredential(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err = s.pool.Exec(ctx, `UPDATE eve_sync_targets SET last_success_at=$2 WHERE character_id=$1 AND resource='wallet_journal'`, ch.ID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		id, first, second int64
		ref, amount       string
	}{
		{1, 222, ch.ID, "player_donation", "1000"}, // another bound character
		{2, 333, ch.ID, "player_donation", "75"},   // external transfer stays
		{3, 222, ch.ID, "bounty_prizes", "25"},     // non-transfer remains
		{4, ch.ID, 222, "player_donation", "-200"},
		{5, ch.ID, 333, "player_donation", "-10"},
	} {
		raw, _ := json.Marshal(map[string]any{"ref_type": entry.ref, "first_party_id": entry.first, "second_party_id": entry.second, "amount": json.Number(entry.amount)})
		if err := store.SaveWalletObservation(ctx, s.pool, "character", ch.ID, 0, "journal", entry.id, ch.ID, credential.OwnerHash, now, now, raw); err != nil {
			t.Fatal(err)
		}
	}
	owners := []CreditWalletOwner{{ID: ch.ID, OwnerHash: credential.OwnerHash}, {ID: 222, OwnerHash: credential.OwnerHash}}
	rows, err := s.auth.CreditWalletSummaries(ctx, owners, now.Add(-24*time.Hour))
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	income, ok := new(big.Rat).SetString(rows[0].Income)
	if !ok || income.Cmp(big.NewRat(100, 1)) != 0 || rows[0].Expense != "10" {
		t.Fatal(rows[0])
	}
	if !rows[0].ObservedAt.Equal(now.Add(-time.Hour)) {
		t.Fatal("must use journal sync, not balance", rows[0].ObservedAt)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE eve_sync_targets SET generation=generation+1 WHERE character_id=$1 AND resource='wallet_journal'`, ch.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = s.auth.CreditWalletSummaries(ctx, owners, now.Add(-24*time.Hour))
	if err != nil || len(rows) != 0 {
		t.Fatal("old generation must not count", rows, err)
	}
}
