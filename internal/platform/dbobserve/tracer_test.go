package dbobserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/testutil"
)

func TestSlowQueryPGXHook(t *testing.T) {
	base := testutil.Database(t)
	var out bytes.Buffer
	cfg := base.Config().Copy()
	cfg.ConnConfig.Tracer = New(slog.New(slog.NewJSONHandler(&out, nil)), time.Millisecond)
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), "SELECT pg_sleep(0.03), $1::text", "private-binding"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "slow database query") || strings.Contains(out.String(), "private-binding") || strings.Contains(out.String(), "pg_sleep") {
		t.Fatalf("pgx hook missing or unsafe: %s", out.String())
	}
}

func TestSlowQueryThresholdAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name               string
		threshold, elapsed time.Duration
		err                error
		result             string
	}{
		{"fast", 500 * time.Millisecond, 499 * time.Millisecond, nil, ""},
		{"disabled", 0, time.Second, nil, ""},
		{"boundary", 500 * time.Millisecond, 500 * time.Millisecond, nil, "ok"},
		{"failed", 500 * time.Millisecond, time.Second, errors.New("private-error-value"), "error"},
		{"pg-error", 500 * time.Millisecond, time.Second, &pgconn.PgError{Code: "23505", Message: "private-error-value", Detail: "private-param"}, "database_error"},
		{"canceled", 500 * time.Millisecond, time.Second, context.Canceled, "canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			tracer := New(slog.New(slog.NewJSONHandler(&out, nil)), tc.threshold)
			now := time.Now()
			tracer.now = func() time.Time { return now }
			ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "-- name: SaveCredential :exec\nSELECT 'private-sql-literal', $1", Args: []any{"private-param"}})
			now = now.Add(tc.elapsed)
			tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: tc.err, CommandTag: pgconn.NewCommandTag("SELECT 1")})
			if tc.result == "" {
				if out.Len() != 0 {
					t.Fatal("logged fast/disabled query")
				}
				return
			}
			if strings.Contains(out.String(), "private-") || strings.Contains(out.String(), "SELECT") {
				t.Fatalf("sensitive query data logged: %s", out.String())
			}
			var entry map[string]any
			if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["result"] != tc.result || entry["query_name"] != "SaveCredential" || entry["duration_ms"] != float64(tc.elapsed.Milliseconds()) || len(entry["sql_hash"].(string)) != 16 {
				t.Fatalf("unexpected log: %v", entry)
			}
		})
	}
}
