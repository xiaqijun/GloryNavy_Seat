package welfare

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/migrations"
)

func TestShortReferenceUpgradePreservesLegacyAndGuardsRollback(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.DownTo(ctx, 93); err != nil {
		t.Fatal(err)
	}
	var oldCase, oldBatch string
	if err = s.Pool.QueryRow(ctx, `INSERT INTO welfare_cases(account_id,corporation_id,kind,state,detail,award_minor,claim_keys) VALUES($1,10,'srp','approved','{}',100,'{}') RETURNING settlement_reference`, userID).Scan(&oldCase); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, `INSERT INTO welfare_contract_settlement_batches(request_key,actor_id,total_count,settlement_reference) VALUES($1,$2,1,gn_settlement_reference('BATCH',now())) RETURNING settlement_reference`, key(900), adminID).Scan(&oldBatch); err != nil {
		t.Fatal(err)
	}
	if len(oldBatch) != 51 {
		t.Fatal(oldBatch)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var gotCase, gotBatch string
	if err = s.Pool.QueryRow(ctx, `SELECT settlement_reference FROM welfare_cases`).Scan(&gotCase); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT settlement_reference FROM welfare_contract_settlement_batches`).Scan(&gotBatch); err != nil {
		t.Fatal(err)
	}
	if oldCase != gotCase || oldBatch != gotBatch {
		t.Fatal("upgrade rewrote an existing contract reference")
	}
	if !settlementContractTitleMatches(oldBatch[:50], gotBatch) {
		t.Fatal("legacy game title stopped matching")
	}
	// Reverting before any new reference is saved remains safe, even with old rows.
	if _, err = p.DownTo(ctx, 93); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var short string
	if err = s.Pool.QueryRow(ctx, `INSERT INTO welfare_contract_settlement_batches(request_key,actor_id,total_count,settlement_reference) VALUES($1,$2,1,gn_settlement_reference('BATCH',now())) RETURNING settlement_reference`, key(901), adminID).Scan(&short); err != nil {
		t.Fatal(err)
	}
	if len(short) != 21 || !settlementContractTitleMatches(short, short) || settlementContractTitleMatches(short[:20], short) || settlementContractTitleMatches(short+"0", short) {
		t.Fatal("short title must match exactly", short)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_contract_settlement_batches SET settlement_reference=$1 WHERE request_key=$2`, short, key(900)); err == nil {
		t.Fatal("duplicate batch reference accepted")
	}
	if _, err = p.DownTo(ctx, 93); err == nil {
		t.Fatal("rollback accepted with saved short reference")
	}
}

func TestShortReferenceConcurrentUniquenessAndSequenceBoundaries(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan string, 160)
	errors := make(chan error, 8)
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				var ref string
				if err := s.Pool.QueryRow(ctx, `SELECT gn_settlement_reference($1,'2026-10-18T00:30:00+08:00'::timestamptz)`, []string{"EX", "SRP", "PVP", "WF", "BATCH"}[i%5]).Scan(&ref); err != nil {
					errors <- err
					return
				}
				results <- ref
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for ref := range results {
		parts := strings.Split(ref, "-")
		if len(parts) != 3 || parts[1] != "20261017" || len(ref) > 21 || seen[parts[2]] {
			t.Fatal("duplicate or invalid reference", ref)
		}
		seen[parts[2]] = true
	}
	if len(seen) != 160 {
		t.Fatal("missing concurrent results", len(seen))
	}
	for _, n := range []int64{999999, 1000000, 9223372036854775807} {
		if _, err := s.Pool.Exec(ctx, `SELECT setval('gn_settlement_reference_seq',$1,false)`, n); err != nil {
			t.Fatal(err)
		}
		var ref string
		if err := s.Pool.QueryRow(ctx, `SELECT gn_settlement_reference('BATCH','2026-10-18T12:00:00Z'::timestamptz)`).Scan(&ref); err != nil {
			t.Fatal(err)
		}
		if ref != fmt.Sprintf("BATCH-20261018-%d", n) || len(ref) > 50 {
			t.Fatal("sequence was truncated", ref)
		}
	}
	if _, err := s.Pool.Exec(ctx, `SELECT gn_settlement_reference('BATCH',now())`); err == nil {
		t.Fatal("exhausted sequence recycled")
	}
}
