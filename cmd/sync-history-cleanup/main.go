// sync-history-cleanup drains expired ESI run history using module-owned SQL.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve"
)

func main() {
	apply := flag.Bool("apply", false, "Delete expired sync history in bounded batches")
	maxBatches := flag.Int("max-batches", 2000, "Maximum 500-row batches")
	pause := flag.Duration("pause", 50*time.Millisecond, "Delay between batches (minimum 10ms)")
	flag.Parse()
	if !*apply || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "explicit --apply is required; success 7d, deferred/progress 3d, other finished 30d")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	config, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil || os.Getenv("DATABASE_URL") == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required and must be valid")
		os.Exit(1)
	}
	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2000"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database unavailable")
		os.Exit(1)
	}
	defer pool.Close()
	total, err := eve.CleanSyncHistory(ctx, pool, *maxBatches, *pause, func(batch int, rows int64) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"batches": batch, "deleted_rows": rows})
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "history cleanup stopped after %d rows; inspect database availability and batch limits\n", total)
		os.Exit(1)
	}
}
