package eve

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"testing"
	"time"
)

func TestWalletSameReferenceVariantsAndCompositePagination(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	c, e := store.New(s.pool).GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	for i := 1; i <= 52; i++ {
		raw := []byte(fmt.Sprintf(`{"id":100,"date":"2026-09-01T00:00:00Z","description":"test","ref_type":"player_donation","amount":%d}`, i))
		for replay := 0; replay < 2; replay++ {
			if e = store.SaveWalletObservation(ctx, s.pool, "character", ch.ID, 0, "journal", 100, ch.ID, c.OwnerHash, time.Now(), time.Now(), raw); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e = store.SaveWalletObservation(ctx, s.pool, "character", ch.ID, 0, "journal", 99, ch.ID, c.OwnerHash, time.Now(), time.Now(), []byte(`{"id":99,"description":"older","amount":1}`)); e != nil {
		t.Fatal(e)
	}
	f := WalletFilter{Kind: "character", Owner: ch.ID, Part: "journal"}
	first, e := s.auth.WalletData(ctx, f)
	if e != nil || len(first) != 51 {
		t.Fatal(len(first), e)
	}
	f.Before = 100
	f.BeforeEntry = first[49]["entry_key"].(string)
	second, e := s.auth.WalletData(ctx, f)
	if e != nil || len(second) != 3 || second[2]["id"] != "99" {
		t.Fatal(second, e)
	}
	seen := map[string]bool{}
	for _, r := range append(first[:50], second...) {
		key := fmt.Sprint(r["id"], r["entry_key"])
		if seen[key] {
			t.Fatal("pagination duplicate")
		}
		seen[key] = true
	}
	if len(seen) != 53 {
		t.Fatal("variants lost")
	}
}
