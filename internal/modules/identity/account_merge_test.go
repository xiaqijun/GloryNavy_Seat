package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/attendance"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/fittings"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"glorynavy.local/seat/migrations"
	"sync"
	"testing"
	"time"
)

func TestAccountMergeProofLedgerRefundAndReplay(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := identity.New(pool)
	att := &attendance.Service{Pool: pool}
	ex := &exchange.Service{Pool: pool, Sources: map[string]string{"pap": "PAP"}, AllowNew: true}
	fit := &fittings.Service{Pool: pool}
	s.MergeParticipants = map[string]identity.MergeParticipant{"attendance": att.MergeAccountTx, "exchange": ex.MergeAccountTx, "fittings": fit.MergeAccountTx, "eve": eve.MergeAccountTx}
	targetToken, err := s.SignIn(ctx, 101, "Target", "a", "")
	if err != nil {
		t.Fatal(err)
	}
	target, _ := s.Session(ctx, targetToken)
	sourceToken, err := s.SignIn(ctx, 201, "Source", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	source, _ := s.Session(ctx, sourceToken)
	if _, err = s.Complete(ctx, 202, "Alt", "c", sourceToken, identity.LoginIntent{Kind: "link", UserID: source.UserID}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProveMerge(ctx, 201, "wrong", targetToken, target.UserID, nil); !errors.Is(err, identity.ErrMerge) {
		t.Fatalf("wrong owner accepted %v", err)
	}
	if _, err = s.ProveMerge(ctx, 101, "a", targetToken, target.UserID, nil); !errors.Is(err, identity.ErrMerge) {
		t.Fatalf("self merge accepted %v", err)
	}
	var event, order int64
	if err = pool.QueryRow(ctx, `INSERT INTO attendance_events(corporation_id,title,starts_at,created_by,request_key,state,pap_issued,pap_points) VALUES(10,'Event',now(),$1,gen_random_uuid(),'closed',true,2) RETURNING id`, source.UserID).Scan(&event); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO attendance_entries(event_id,character_id,account_id,character_name,source,present,recorded_at) VALUES($1,201,$2,'Source','fleet',true,now())`, event, source.UserID)
	exec(`INSERT INTO attendance_pap_awards(event_id,character_id,account_id,character_name,points) VALUES($1,201,$2,'Source',2)`, event, source.UserID)
	exec(`INSERT INTO attendance_pap_ledger(event_id,character_id,account_id,actor_id,request_key,delta,balance,reason) VALUES($1,201,$2,$2,gen_random_uuid(),2,2,'original')`, event, source.UserID)
	exec(`INSERT INTO exchange_source_awards(source_id,reference,account_id,units,minor_per_unit,coins) VALUES('pap','test/201',$1,2,500,1000)`, source.UserID)
	exec(`INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','source-original',gen_random_uuid(),1000,'original'),($2,'source','target-original',gen_random_uuid(),2000,'original')`, source.UserID, target.UserID)
	exec(`INSERT INTO exchange_rewards(type_id,quantity,isk_value,stock) VALUES(34,1,300,0)`)
	if err = pool.QueryRow(ctx, `INSERT INTO exchange_redemptions(account_id,request_key,fingerprint,reward_id,type_id,quantity,recipient_id,recipient_name,isk_per_coin,isk_value,coins_minor) VALUES($1,gen_random_uuid(),'original',1,34,1,201,'Source',100,300,300) RETURNING id`, source.UserID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'reserve',$2,gen_random_uuid(),-300,'original')`, source.UserID, "1")
	exec(`INSERT INTO access_administrators(user_id) VALUES($1)`, source.UserID)
	id, err := s.ProveMerge(ctx, 201, "b", targetToken, target.UserID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Merge(ctx, sourceToken, id, "", false); !errors.Is(err, identity.ErrMerge) {
		t.Fatalf("foreign session preview: %v", err)
	}
	p, err := s.Merge(ctx, targetToken, id, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Characters) != 2 || p.TargetMain.ID != "101" {
		t.Fatalf("preview %+v", p)
	}
	// A changed amount invalidates the displayed confirmation; no partial ownership move.
	exec(`INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','extra',gen_random_uuid(),100,'extra')`, source.UserID)
	if _, err = s.Merge(ctx, targetToken, id, p.Token, true); !errors.Is(err, identity.ErrMerge) {
		t.Fatalf("stale preview accepted %v", err)
	}
	p, err = s.Merge(ctx, targetToken, id, "", false)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Merge(ctx, targetToken, id, p.Token, true)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Completed {
		t.Fatal("not complete")
	}
	if _, err = s.Merge(ctx, targetToken, id, p.Token, true); err != nil {
		t.Fatal("replay", err)
	}
	if old, _ := s.Session(ctx, sourceToken); old != nil {
		t.Fatal("old session survived")
	}
	session, _ := s.Session(ctx, targetToken)
	if session == nil || session.MainCharacter.ID != "101" {
		t.Fatal("target session/main changed")
	}
	var balance, original, pap int64
	var admin bool
	pool.QueryRow(ctx, `SELECT sum(delta),count(*) FILTER(WHERE original_account_id=$2) FROM exchange_coin_ledger WHERE account_id=$1`, target.UserID, source.UserID).Scan(&balance, &original)
	if balance != 2800 || original != 3 {
		t.Fatalf("balance/provenance %d %d", balance, original)
	}
	pool.QueryRow(ctx, `SELECT sum(points) FROM attendance_pap_awards WHERE account_id=$1`, target.UserID).Scan(&pap)
	if pap != 2 {
		t.Fatal("PAP not moved")
	}
	pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM access_administrators WHERE user_id=$1)`, target.UserID).Scan(&admin)
	if admin {
		t.Fatal("source admin transferred")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','late',gen_random_uuid(),1,'late')`, source.UserID); err == nil {
		t.Fatal("retired account accepts late write")
	}
	// The resulting owner requests cancellation; an administrator confirms the refund.
	ex.LockAccounts = s.LockActiveAccounts
	ex.Bindings = func(ctx context.Context, tx pgx.Tx, ids []int64) ([]exchange.Binding, error) {
		rows, err := s.Bindings(ctx, tx, ids)
		var out []exchange.Binding
		for _, row := range rows {
			out = append(out, exchange.Binding{ID: row.ID, UserID: row.UserID, Name: row.Name})
		}
		return out, err
	}
	ex.Contracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{}, nil
	}
	exec(`INSERT INTO exchange_deliveries(order_id) VALUES($1)`, order)
	managerToken, err := s.SignIn(ctx, 301, "Manager", "manager", "")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := s.Session(ctx, managerToken)
	if err != nil || manager == nil {
		t.Fatal("manager session", err)
	}
	ex.Administrator = func(_ context.Context, user string) (bool, error) { return user == manager.UserID, nil }
	if err = ex.DecideOrder(ctx, target.UserID, order, exchange.OrderDecision{Version: 1, RequestKey: "11111111-1111-4111-8111-111111111110", State: "cancel_requested", Note: "cancel after merge"}); err != nil {
		t.Fatal(err)
	}
	decision := exchange.OrderDecision{Version: 2, RequestKey: "11111111-1111-4111-8111-111111111111", State: "cancelled", Note: "refund after merge", UndeliveredConfirmed: true}
	if err = ex.DecideOrder(ctx, target.UserID, order, decision); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("owner bypassed cancellation review", err)
	}
	if err = ex.DecideOrder(ctx, manager.UserID, order, decision); err != nil {
		t.Fatal(err)
	}
	tx, _ := pool.Begin(ctx)
	if err = ex.ReconcileTx(ctx, tx, "pap", "22222222-2222-4222-8222-222222222222", "correction", []exchange.Award{{Reference: "test/201", AccountID: target.UserID, Previous: 2, Units: 1}}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	pool.QueryRow(ctx, `SELECT sum(delta) FROM exchange_coin_ledger WHERE account_id=$1`, target.UserID).Scan(&balance)
	if balance != 2600 {
		t.Fatalf("refund/correction balance %d", balance)
	}
	// A bound alt now signs in to the surviving account.
	login, err := s.SignIn(ctx, 202, "Alt", "c", "")
	if err != nil {
		t.Fatal(err)
	}
	signed, _ := s.Session(ctx, login)
	if signed.UserID != target.UserID {
		t.Fatal("alt login wrong account")
	}
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 29); err == nil {
		t.Fatal("Down erased completed merge provenance")
	}
}

func TestAccountMergeExpiryCancelRollbackAndConcurrency(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := identity.New(pool)
	a := &attendance.Service{Pool: pool}
	e := &exchange.Service{Pool: pool}
	f := &fittings.Service{Pool: pool}
	s.MergeParticipants = map[string]identity.MergeParticipant{"attendance": a.MergeAccountTx, "exchange": e.MergeAccountTx, "fittings": f.MergeAccountTx}
	tkn, err := s.SignIn(ctx, 301, "Target", "a", "")
	if err != nil {
		t.Fatal(err)
	}
	target, _ := s.Session(ctx, tkn)
	other, err := s.SignIn(ctx, 401, "Source", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	source, _ := s.Session(ctx, other)
	// Empty-account upgrade can roll down/up; completed merge provenance cannot.
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	prove := func() string {
		t.Helper()
		id, e := s.ProveMerge(ctx, 401, "b", tkn, target.UserID, nil)
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	id := prove()
	pool.Exec(ctx, `UPDATE identity_merge_requests SET expires_at=now()-interval '1 second' WHERE id=$1`, id)
	if _, err = s.Merge(ctx, tkn, id, "", false); err == nil {
		t.Fatal("expired accepted")
	}
	id = prove()
	if err = s.CancelMerge(ctx, tkn, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Merge(ctx, tkn, id, "", false); err == nil {
		t.Fatal("cancelled accepted")
	}
	id = prove()
	p, err := s.Merge(ctx, tkn, id, "", false)
	if err != nil {
		t.Fatal(err)
	}
	original := s.MergeParticipants["fittings"]
	s.MergeParticipants["fittings"] = func(ctx context.Context, tx pgx.Tx, a, b string, apply bool) (json.RawMessage, error) {
		if apply {
			return nil, errors.New("injected failure")
		}
		return original(ctx, tx, a, b, false)
	}
	if _, err = s.Merge(ctx, tkn, id, p.Token, true); err == nil {
		t.Fatal("injected failure accepted")
	}
	owner, _ := s.UserForCharacter(ctx, 401)
	if owner != source.UserID {
		t.Fatal("partial merge after failure")
	}
	s.MergeParticipants["fittings"] = original
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Merge(ctx, tkn, id, p.Token, true); results <- e }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success < 1 {
		t.Fatal("no concurrent request completed")
	}
	var count int
	pool.QueryRow(ctx, `SELECT count(*) FROM identity_account_merges WHERE source_id=$1`, source.UserID).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate merge")
	}
}
