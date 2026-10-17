package community

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/migrations"
)

func TestOneBotConfiguredMigrationCannotBeDropped(t *testing.T) {
	_, pool, _, _ := oneBotFixture(t)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(context.Background(), 90); err == nil || !strings.Contains(err.Error(), "OneBot configuration or bindings exist") {
		t.Fatal("unsafe OneBot rollback", err)
	}
}
