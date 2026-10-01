package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/platform/jobs"
	"log/slog"
	"os"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		slog.Error("database configuration invalid")
		os.Exit(1)
	}
	defer pool.Close()
	if err = jobs.Migrate(ctx, pool); err != nil {
		slog.Error("River migration failed; check database and migration compatibility")
		os.Exit(1)
	}
	slog.Info("River migrations applied")
}
