package reminder

import (
	"context"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
	"glorynavy.local/seat/migrations"
	"testing"
)

func TestReminderMigrationKeepsApprovalReceiptsAndGuardsNewEvents(t *testing.T) {
	s, _, u := fixture(t)
	ctx := context.Background()
	if e := s.deliver(ctx, u); e != nil {
		t.Fatal(e)
	}
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	p, e := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.DownTo(ctx, 97); e != nil {
		t.Fatal("unused new types should rollback", e)
	}
	if _, e = p.Up(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM reminder_receipts WHERE event='approval_pending' AND attempted_generation=generation`).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.Reconcile(ctx, tx, u, []string{"loan_due:test"}, "loan_due"); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = p.DownTo(ctx, 97); e == nil {
		t.Fatal("used new receipt rollback allowed")
	}
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM reminder_receipts WHERE event='loan_due'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("rollback damaged receipts", n, e)
	}
}
