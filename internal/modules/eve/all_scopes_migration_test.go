package eve

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/testutil"
	"glorynavy.local/seat/migrations"
)

func TestAllScopesMigrationPreservesPendingAndGuardsRollback(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.DownTo(ctx, 113); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO eve_login_flows(state_hash,browser_hash,session_hash,verifier,requested_scopes,intent,target_user_id,expected_character_id)
VALUES (decode(repeat('01',32),'hex'),decode(repeat('02',32),'hex'),decode(repeat('03',32),'hex'),'verifier',ARRAY[$1], 'reauthorize',gen_random_uuid(),123),
       (decode(repeat('04',32),'hex'),decode(repeat('05',32),'hex'),decode(repeat('06',32),'hex'),'verifier',ARRAY[$2], 'reauthorize',gen_random_uuid(),124)`, MailSendScope, FleetWriteScope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var mail, fleet string
	if err = pool.QueryRow(ctx, `SELECT purpose FROM eve_login_flows WHERE expected_character_id=123`).Scan(&mail); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT purpose FROM eve_login_flows WHERE expected_character_id=124`).Scan(&fleet); err != nil {
		t.Fatal(err)
	}
	if mail != "reminder-mail" || fleet != "fleet-write" {
		t.Fatalf("pending purposes: %q %q", mail, fleet)
	}
	if _, err = p.DownTo(ctx, 113); err == nil {
		t.Fatal("rollback allowed with pending special flows")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM eve_login_flows; UPDATE eve_full_scope_grant_state SET full_grants_seen=true`); err != nil {
		t.Fatal(err)
	}
	if _, err = p.DownTo(ctx, 113); err == nil {
		t.Fatal("rollback allowed after full grant")
	}
}
