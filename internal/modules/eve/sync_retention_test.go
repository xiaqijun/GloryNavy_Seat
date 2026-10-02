package eve

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/testutil"
	"glorynavy.local/seat/migrations"
)

func TestSyncRunRetentionBoundaries(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	target := authorizationTarget(t, s)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// A single transaction fixes now() for exact 14/30-day boundaries.
	_, err = tx.Exec(ctx, `INSERT INTO eve_sync_runs(target_id,job_id,fence,outcome,reason,started_at,finished_at)
 SELECT $1,1,row_number() OVER (),outcome,label,now()-interval '50 days',
 CASE WHEN age IS NULL THEN NULL ELSE now()-age END
 FROM (VALUES
 ('success','success_old',interval '31 days'),
 ('success','success_expired',interval '14 days 1 second'),
 ('success','success_boundary',interval '14 days'),
 ('success','success_recent',interval '13 days'),
 ('failed','failure_between_cutoffs',interval '20 days'),
 ('failed','failure_expired',interval '30 days 1 second'),
 ('failed','failure_boundary',interval '30 days'),
 ('interrupted','interrupted_expired',interval '31 days'),
 ('future_outcome','unknown_expired',interval '31 days'),
 ('running','unfinished',NULL::interval),
 ('success','unfinished_success',NULL::interval),
 ('success','recent_finish_old_start',interval '1 hour')
 ) v(outcome,label,age)`, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.New(tx).CleanupSyncRuns(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, "SELECT reason FROM eve_sync_runs ORDER BY reason")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]bool{"success_boundary": true, "success_recent": true,
		"failure_between_cutoffs": true, "failure_boundary": true, "unfinished": true,
		"unfinished_success": true, "recent_finish_old_start": true}
	for rows.Next() {
		var reason string
		if err := rows.Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if !want[reason] {
			t.Fatalf("expired history retained: %s", reason)
		}
		delete(want, reason)
	}
	if err := rows.Err(); err != nil || len(want) != 0 {
		t.Fatalf("retained history lost: %v, %v", want, err)
	}
}

func TestSyncRunRetentionMigrationRoundTrip(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	// This test targets migration 46. The schema starts at the current head,
	// so rolling back only the latest migration leaves these indexes in place.
	if _, err := provider.DownTo(ctx, 45); err != nil {
		t.Fatal(err)
	}
	var count int
	const indexes = `SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid
 WHERE c.relnamespace=current_schema()::regnamespace
 AND c.relname IN ('eve_sync_runs_success_expiry','eve_sync_runs_other_expiry') AND i.indisvalid`
	if err := pool.QueryRow(ctx, indexes).Scan(&count); err != nil || count != 0 {
		t.Fatalf("down did not remove indexes: %d %v", count, err)
	}
	// Simulate a partially completed migration: retry must rebuild the owned name.
	if _, err := pool.Exec(ctx, `CREATE INDEX eve_sync_runs_success_expiry ON eve_sync_runs(id)`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 46); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, indexes).Scan(&count); err != nil || count != 2 {
		t.Fatalf("up did not create valid indexes: %d %v", count, err)
	}
	var definition string
	if err := pool.QueryRow(ctx, `SELECT pg_get_indexdef('eve_sync_runs_success_expiry'::regclass)`).Scan(&definition); err != nil || !strings.Contains(definition, "finished_at, id") {
		t.Fatalf("partial migration index was not rebuilt: %s %v", definition, err)
	}
}

type retentionQueryCapture struct {
	store.DBTX
	query string
}

func (c *retentionQueryCapture) Exec(_ context.Context, query string, _ ...interface{}) (pgconn.CommandTag, error) {
	c.query = query
	return pgconn.CommandTag{}, nil
}

// Opt-in scale check against an isolated test schema, never a production table.
func TestSyncRunRetentionLargeHistoryPlan(t *testing.T) {
	if os.Getenv("TEST_SYNC_RETENTION_PLAN") != "1" {
		t.Skip("set TEST_SYNC_RETENTION_PLAN=1 for the 1.84M-row plan check")
	}
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	target := authorizationTarget(t, s)
	_, err := s.pool.Exec(ctx, `INSERT INTO eve_sync_runs(target_id,job_id,fence,outcome,finished_at)
 SELECT $1,1,n,CASE WHEN n%30=0 THEN 'failed' ELSE 'success' END,
 now()-interval '1 day'*(1+n%7) FROM generate_series(1,1840000) n`, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, "ANALYZE eve_sync_runs"); err != nil {
		t.Fatal(err)
	}
	capture := &retentionQueryCapture{}
	if err := store.New(capture).CleanupSyncRuns(ctx); err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`SELECT id FROM eve_sync_runs WHERE (finished_at<now()-interval '30 days' OR (outcome='success' AND finished_at<now()-interval '14 days')) ORDER BY id LIMIT 500`,
		capture.query,
	}
	for i, query := range queries {
		var raw []byte
		if err := s.pool.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,TIMING OFF,FORMAT JSON) "+query).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var plan []map[string]interface{}
		if err := json.Unmarshal(raw, &plan); err != nil {
			t.Fatal(err)
		}
		t.Logf("plan %d execution_ms=%v plan=%s", i, plan[0]["Execution Time"], raw)
		if i == 1 && (strings.Contains(string(raw), `"Node Type": "Seq Scan"`) || !strings.Contains(string(raw), "eve_sync_runs_success_expiry") || !strings.Contains(string(raw), "eve_sync_runs_other_expiry")) {
			t.Fatal("cleanup did not use both retention indexes")
		}
	}
}

func TestSyncRunRetentionBatchLimitAndOldestFirst(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	target := authorizationTarget(t, s)
	// Put the older failures after the success rows in ID order: cleanup should
	// select by finish time across both indexed branches, with one combined limit.
	_, err := s.pool.Exec(ctx, `INSERT INTO eve_sync_runs(target_id,job_id,fence,outcome,finished_at)
 SELECT $1,1,n,CASE WHEN n<=600 THEN 'success' ELSE 'failed' END,
 now()-CASE WHEN n<=600 THEN interval '20 days' ELSE interval '40 days' END
 FROM generate_series(1,1200) n`, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	q := store.New(s.pool)
	for _, remaining := range []int{700, 200, 0, 0} {
		if err := q.CleanupSyncRuns(ctx); err != nil {
			t.Fatal(err)
		}
		var total, successes int
		if err := s.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE outcome='success') FROM eve_sync_runs`).Scan(&total, &successes); err != nil {
			t.Fatal(err)
		}
		if total != remaining || (remaining == 700 && successes != 600) {
			t.Fatalf("unexpected batch: total=%d success=%d expected total=%d", total, successes, remaining)
		}
	}
}
