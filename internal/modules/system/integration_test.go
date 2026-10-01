package system

import (
	"context"
	"crypto/rand"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/system/internal/store"
	"glorynavy.local/seat/migrations"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMigrationLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("connect test database:", err)
	}
	defer admin.Close(context.Background())
	schema := "test_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*cfg)
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	service := Service{Reader: store.New(conn), Version: "test"}
	if _, err = service.Status(ctx); err == nil {
		t.Fatal("unmigrated database must not be ready")
	}
	for range 2 {
		if _, err = provider.Up(ctx); err != nil {
			t.Fatal(err)
		}
	}
	status, err := service.Status(ctx)
	if err != nil || status.SchemaVersion != 1 {
		t.Fatalf("migrated database status: %+v %v", status, err)
	}
	if _, err = provider.DownTo(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Status(ctx); err == nil {
		t.Fatal("rolled-back database must not be ready")
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, "UPDATE platform_metadata SET schema_version=2"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Status(ctx); err == nil {
		t.Fatal("unsupported schema must not be ready")
	}
}
