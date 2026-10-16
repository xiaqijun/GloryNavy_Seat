package eve

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"strconv"
	"testing"
	"time"
)

func TestWalletWithheldTaxesInExpenseAndTaxFilters(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	c, e := store.New(s.pool).GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	at := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= 53; i++ {
		tax := `0.12345`
		if i == 53 {
			tax = `9007199254740993.012345`
		}
		raw := []byte(fmt.Sprintf(`{"id":%d,"date":%q,"ref_type":"bounty_prizes","description":"reward","amount":85,"tax":%s,"tax_receiver_id":10}`, i, at.Format(time.RFC3339), tax))
		for replay := 0; replay < 2; replay++ {
			if e = store.SaveWalletObservation(ctx, s.pool, "character", ch.ID, 0, "journal", int64(i), ch.ID, c.OwnerHash, at, at, raw); e != nil {
				t.Fatal(e)
			}
		}
	}
	for i, payload := range []string{
		`{"amount":-7,"ref_type":"transaction_tax"}`,
		`{"amount":80,"tax":0}`, `{"amount":80}`, `{"amount":80,"tax":null}`, `{"amount":80,"tax":-5}`,
	} {
		if e = store.SaveWalletObservation(ctx, s.pool, "character", ch.ID, 0, "journal", int64(100+i), ch.ID, c.OwnerHash, at, at, []byte(payload)); e != nil {
			t.Fatal(e)
		}
	}
	readAll := func(f WalletFilter) []map[string]any {
		t.Helper()
		out := []map[string]any{}
		seen := map[string]bool{}
		for {
			rows, err := s.auth.WalletData(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			more := len(rows) > 50
			if more {
				rows = rows[:50]
			}
			for _, row := range rows {
				key := fmt.Sprint(row["id"], row["entry_key"])
				if seen[key] {
					t.Fatal("duplicate", key)
				}
				seen[key] = true
			}
			out = append(out, rows...)
			if !more {
				return out
			}
			last := rows[len(rows)-1]
			f.Before, _ = strconv.ParseInt(last["id"].(string), 10, 64)
			f.BeforeEntry = last["entry_key"].(string)
		}
	}
	f := WalletFilter{Kind: "character", Owner: ch.ID, Part: "journal", Direction: "out"}
	if rows := readAll(f); len(rows) != 54 {
		t.Fatal("expense filter omitted tax", len(rows))
	}
	f.Direction = "tax"
	f.Party = 10
	rows := readAll(f)
	if len(rows) != 53 || rows[0]["amount"] != "85" || rows[0]["tax"] != "9007199254740993.012345" {
		t.Fatal("tax precision or recipient filter", len(rows), rows[0])
	}
	f.Party = 11
	if len(readAll(f)) != 0 {
		t.Fatal("wrong recipient matched")
	}
	f.Party = 0
	from := at.Add(time.Second)
	f.From = &from
	if len(readAll(f)) != 0 {
		t.Fatal("date filter ignored")
	}
	f.From = nil
	f.Ref = "transaction_tax"
	if len(readAll(f)) != 0 {
		t.Fatal("ordinary tax debit duplicated as withholding")
	}
	f.Ref = ""
	f.Direction = "in"
	if len(readAll(f)) != 57 {
		t.Fatal("net income rows changed")
	}
	sums, err := s.auth.WalletSummaries(ctx, []int64{ch.ID}, at.Add(-time.Second))
	if err != nil || len(sums) != 1 || sums[0].Expense != "7" {
		t.Fatal("withholding subtracted twice", sums, err)
	}
	if _, e = s.pool.Exec(ctx, `UPDATE eve_credentials SET scopes=ARRAY[]::text[] WHERE character_id=$1`, ch.ID); e != nil {
		t.Fatal(e)
	}
	f.Direction = "tax"
	if len(readAll(f)) != 0 {
		t.Fatal("revoked scope exposed tax")
	}
}
