package welfare

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/migrations"
)

func TestApprovalReversionPreventsMigrationDowngrade(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	setQuota(t, s, Config{})
	c := quotaPending(t, s, 32601)
	raw, err := s.Execute(ctx, adminID, quotaApprove(c, 12000, 32601))
	if err != nil {
		t.Fatal(err)
	}
	var approved Case
	if err = json.Unmarshal(raw, &approved); err != nil {
		t.Fatal(err)
	}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return nil, nil
	}
	if _, err = s.Execute(ctx, adminID, Command{Action: "revert_approval", ID: approved.ID, Version: approved.Version, RequestKey: key(32602), Note: "mistake", ConfirmedNotDelivered: true}); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 115); err == nil {
		t.Fatal("downgrade discarded approval history")
	}
}
