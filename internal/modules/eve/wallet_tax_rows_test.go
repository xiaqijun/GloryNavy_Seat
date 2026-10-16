package eve

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"testing"
	"time"
)

func TestWalletIncomeIncludesFullyWithheldRewards(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	c, e := store.New(s.pool).GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	at := time.Now().UTC()
	for i, payload := range []string{`{"amount":0,"tax":5}`, `{"amount":-7,"tax":2}`, `{"amount":0,"tax":0}`} {
		for _, kind := range []string{"character", "corporation"} {
			if e = store.SaveWalletObservation(ctx, s.pool, kind, ch.ID, 0, "journal", int64(i+1), ch.ID, c.OwnerHash, at, at, []byte(payload)); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, kind := range []string{"character", "corporation"} {
		rows, err := s.auth.WalletData(ctx, WalletFilter{Kind: kind, Owner: ch.ID, Part: "journal", Direction: "in"})
		if err != nil {
			t.Fatal(err)
		}
		if kind == "character" {
			if len(rows) != 1 || rows[0]["amount"] != "0" || rows[0]["tax"] != "5" {
				t.Fatal("fully withheld reward omitted or mutated", rows)
			}
		} else if len(rows) != 0 {
			t.Fatal("corporation journal incorrectly grossed up", rows)
		}
	}
}
