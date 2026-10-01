package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/migrations"
)

func main() {
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "up" && command != "status" && command != "down" {
		slog.Error("usage: migrate [up|status|down]")
		os.Exit(1)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error("database configuration invalid", "error", err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = conn.PingContext(ctx); err != nil {
		slog.Error("database connectivity failed", "error", err)
		os.Exit(1)
	}
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("migration dialect failed")
		os.Exit(1)
	}
	if err = goose.RunContext(ctx, command, conn, "."); err != nil {
		slog.Error("migration failed; check database connectivity and reviewed schema", "error", err)
		os.Exit(1)
	}
}
