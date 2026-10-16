package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"glorynavy.local/seat/migrations"
)

type cleanupFixture struct {
	pool    *pgxpool.Pool
	users   *identity.Service
	acl     *access.Service
	cleanup *identity.CorporationCleanup
}

func newCleanupFixture(t *testing.T) cleanupFixture {
	pool := testutil.Database(t)
	acl := access.New(pool, nil, nil)
	return cleanupFixture{pool, identity.New(pool), acl, corporationCleanupService(pool, acl, nil)}
}
func (f cleanupFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}
func (f cleanupFixture) sign(t *testing.T, id int64) (string, *identity.Session) {
	t.Helper()
	token, err := f.users.SignIn(context.Background(), id, fmt.Sprint("Pilot ", id), "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := f.users.Session(context.Background(), token)
	if err != nil || s == nil {
		t.Fatal("session", err)
	}
	return token, s
}
func (f cleanupFixture) facts(t *testing.T, id, corp int64) {
	t.Helper()
	f.exec(t, `INSERT INTO eve_credentials(character_id,owner_hash,sealed,state,scopes) VALUES($1,$2,''::bytea,'ready',ARRAY['esi-characters.read_corporation_roles.v1'])`, id, httpapi.Hash("owner"))
	f.exec(t, `INSERT INTO eve_role_snapshots(character_id,owner_hash,corporation_id,corporation_name,ceo_id,roles,roles_at_hq,roles_at_base,roles_at_other,synced_at,valid_until) VALUES($1,$2,$3,'Corporation',999,'{}','{}','{}','{}',now(),now()+interval '1 hour')`, id, httpapi.Hash("owner"), corp)
	f.exec(t, `INSERT INTO eve_character_profiles(character_id,name,corporation_id,checked_at,valid_until) VALUES($1,'Pilot',$2,now(),now()+interval '1 hour')`, id, corp)
	f.exec(t, `INSERT INTO eve_sync_targets(character_id,resource,generation,last_success_at) SELECT character_id,resource,grant_generation,now() FROM eve_credentials CROSS JOIN(VALUES('profile'),('authorization')) r(resource) WHERE character_id=$1`, id)
}
func (f cleanupFixture) due(t *testing.T) {
	t.Helper()
	f.exec(t, `UPDATE identity_corporation_cleanup SET enabled=true,corporation_ids=ARRAY[10]::bigint[],next_run_at=now()-interval '1 minute'`)
}
func (f cleanupFixture) run(t *testing.T) {
	t.Helper()
	if err := f.cleanup.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func (f cleanupFixture) scalar(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCorporationCleanupStrictMainLastAndRecovery(t *testing.T) {
	ctx := context.Background()
	f := newCleanupFixture(t)
	token, s := f.sign(t, 101)
	for _, id := range []int64{102, 103} {
		if _, err := f.users.Complete(ctx, id, "Alt", "owner", token, identity.LoginIntent{Kind: "link", UserID: s.UserID}, nil); err != nil {
			t.Fatal(err)
		}
	}
	altToken, _ := f.sign(t, 102)
	keepToken, _ := f.sign(t, 103)
	lastToken, last := f.sign(t, 201)
	f.facts(t, 101, 20)
	f.facts(t, 102, 20)
	f.facts(t, 103, 10)
	f.facts(t, 201, 20)
	f.exec(t, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','cleanup-retain',gen_random_uuid(),1200,'history')`, last.UserID)
	f.exec(t, `INSERT INTO eve_esi_cache(cache_key,character_id,body,expires_at) VALUES('private',201,'cache',now()+interval '1 hour')`)
	f.exec(t, `INSERT INTO eve_contracts(owner_kind,owner_id,contract_id,source_character_id,source_generation,contract_type,status,payload) VALUES('character',201,1,201,1,'auction','outstanding','{}')`)
	// Default off and future schedules never remove characters.
	f.run(t)
	if f.scalar(t, `SELECT count(*) FROM identity_characters`) != 4 {
		t.Fatal("default enabled")
	}
	f.due(t)
	f.exec(t, `UPDATE identity_corporation_cleanup SET next_run_at=now()+interval '1 day'`)
	f.run(t)
	if f.scalar(t, `SELECT count(*) FROM identity_characters`) != 4 {
		t.Fatal("ran before due")
	}
	f.due(t)
	f.run(t)
	for _, token := range []string{token, altToken, lastToken} {
		if ss, err := f.users.Session(ctx, token); err != nil || ss != nil {
			t.Fatal("removed session survived", err)
		}
	}
	remaining, err := f.users.Session(ctx, keepToken)
	if err != nil || remaining == nil || remaining.MainCharacter.ID != "103" {
		t.Fatal("retained main/session", remaining, err)
	}
	for _, q := range []string{`SELECT count(*) FROM eve_credentials WHERE character_id<>103`, `SELECT count(*) FROM eve_esi_cache`, `SELECT count(*) FROM eve_contracts WHERE owner_id=201`, `SELECT count(*) FROM identity_sessions WHERE user_id='` + last.UserID + `'`} {
		if f.scalar(t, q) != 0 {
			t.Fatal("cached state survived", q)
		}
	}
	if f.scalar(t, `SELECT count(*) FROM identity_users`) != 2 || f.scalar(t, `SELECT sum(delta) FROM exchange_coin_ledger WHERE account_id=$1`, last.UserID) != 1200 || f.scalar(t, `SELECT count(*) FROM identity_cleanup_removed`) != 3 {
		t.Fatal("account/history lost")
	}
	if f.scalar(t, `SELECT last_removed_count FROM identity_corporation_cleanup`) != 3 {
		t.Fatal("incorrect run count")
	}
	if f.scalar(t, `SELECT count(*) FROM identity_corporation_cleanup WHERE run_started_at IS NULL AND next_run_at>now()+interval '6 days'`) != 1 {
		t.Fatal("schedule not advanced")
	}
	f.run(t)
	if f.scalar(t, `SELECT count(*) FROM identity_cleanup_audit WHERE action='completed'`) != 1 {
		t.Fatal("duplicate completion")
	}
	if _, err = f.users.SignIn(ctx, 201, "Pilot", "other-owner", ""); !errors.Is(err, identity.ErrOwnership) {
		t.Fatal("owner mismatch", err)
	}
	if _, err = f.users.Complete(ctx, 201, "Pilot", "owner", keepToken, identity.LoginIntent{Kind: "link", UserID: s.UserID}, nil); !errors.Is(err, identity.ErrConflict) {
		t.Fatal("cross-account recovery", err)
	}
	_, restored := f.sign(t, 201)
	if restored.UserID != last.UserID || restored.MainCharacter.ID != "201" {
		t.Fatal("last login lost ownership")
	}
	_, restoredAlt := f.sign(t, 101)
	if restoredAlt.UserID != s.UserID || restoredAlt.MainCharacter.ID != "103" {
		t.Fatal("restored alt replaced main")
	}
	if err = f.users.ChangeCharacter(ctx, keepToken, 103, true, eve.RemoveCharacterTx); !errors.Is(err, identity.ErrMain) {
		t.Fatal("manual main protection changed", err)
	}
	db := stdlib.OpenDB(*f.pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Down(ctx); err == nil || !strings.Contains(err.Error(), "preserved character ownership") {
		t.Fatal("unsafe rollback allowed", err)
	}
}

func TestCorporationCleanupUncertainFactsAndRollback(t *testing.T) {
	ctx := context.Background()
	f := newCleanupFixture(t)
	_, admin := f.sign(t, 1)
	if err := f.acl.SetAdministrator(ctx, admin.UserID, true); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`UPDATE eve_role_snapshots SET valid_until=now()-interval '1 second' WHERE character_id=$1`,
		`UPDATE eve_character_profiles SET valid_until=now()-interval '1 second' WHERE character_id=$1`,
		`UPDATE eve_character_profiles SET corporation_id=30 WHERE character_id=$1`,
		`UPDATE eve_sync_targets SET failures=1 WHERE character_id=$1`,
		`UPDATE eve_sync_targets SET generation=generation+1 WHERE character_id=$1`,
		`UPDATE eve_sync_targets SET state='blocked' WHERE character_id=$1`,
		`UPDATE eve_sync_targets SET last_success_at=NULL WHERE character_id=$1`,
		`UPDATE eve_credentials SET state='reauthorize' WHERE character_id=$1`,
		`UPDATE eve_credentials SET scopes='{}' WHERE character_id=$1`,
		`UPDATE eve_role_snapshots SET owner_hash=decode(repeat('42',32),'hex') WHERE character_id=$1`,
		`UPDATE identity_characters SET owner_hash=decode(repeat('42',32),'hex') WHERE character_id=$1`,
		`UPDATE identity_characters SET status='blocked' WHERE character_id=$1`,
		`DELETE FROM eve_credentials WHERE character_id=$1`,
	}
	for i, q := range cases {
		id := int64(100 + i)
		f.sign(t, id)
		f.facts(t, id, 20)
		f.exec(t, q, id)
	}
	token, removable := f.sign(t, 900)
	f.facts(t, 900, 20)
	p, err := f.cleanup.Preview(ctx, admin.UserID, []string{"10"})
	if err != nil || p.Remove != 1 || p.Unknown != len(cases)+1 || p.Items[0].ID != "900" {
		t.Fatalf("preview %+v %v", p, err)
	}
	f.due(t)
	f.cleanup.Remove = func(ctx context.Context, tx pgx.Tx, id int64, user string) error {
		if err := eve.RemoveCharacterTx(ctx, tx, id, user); err != nil {
			return err
		}
		return errors.New("rollback")
	}
	if err = f.cleanup.RunDue(ctx); err == nil {
		t.Fatal("failure ignored")
	}
	if f.scalar(t, `SELECT count(*) FROM eve_credentials WHERE character_id=900`) != 1 || f.scalar(t, `SELECT count(*) FROM identity_cleanup_removed`) != 0 {
		t.Fatal("partial delete")
	}
	if session, _ := f.users.Session(ctx, token); session == nil {
		t.Fatal("session lost on failure")
	}
	f.cleanup.Remove = eve.RemoveCharacterTx
	f.run(t)
	if f.scalar(t, `SELECT count(*) FROM identity_cleanup_removed WHERE user_id=$1`, removable.UserID) != 1 || f.scalar(t, `SELECT last_skipped_count FROM identity_corporation_cleanup`) != len(cases)+1 {
		t.Fatal("resume/skips failed")
	}
	if f.scalar(t, `SELECT count(*) FROM identity_users WHERE main_character_id=111`) != 1 {
		t.Fatal("uncertain-only account was modified")
	}
}

func TestCorporationCleanupSettingsAndHTTPPermissions(t *testing.T) {
	ctx := context.Background()
	f := newCleanupFixture(t)
	adminToken, admin := f.sign(t, 101)
	managerToken, manager := f.sign(t, 102)
	if err := f.acl.SetAdministrator(ctx, admin.UserID, true); err != nil {
		t.Fatal(err)
	}
	role, err := f.acl.SaveRole(ctx, "test", access.Role{ID: "01994763-4111-7000-8000-111111111111", Name: "Manager", Grants: []access.Grant{{Permission: "access.manage"}, {Permission: "eve.sync.manage"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.acl.Assign(ctx, "test", manager.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	handler, err := New(f.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(token, csrf, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/identity/corporation-cleanup"+path, strings.NewReader(body))
		r.Header.Set("Origin", "https://example.com")
		r.Header.Set("X-CSRF-Token", csrf)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body := `{"enabled":true,"corporation_ids":["10"],"version":"1"}`
	for _, route := range []struct{ method, path string }{{"GET", ""}, {"PUT", ""}, {"POST", "/preview"}} {
		if w := call("", "", route.method, route.path, body); w.Code != 401 {
			t.Fatal("anonymous", w.Code)
		}
		if w := call(managerToken, manager.CSRFToken, route.method, route.path, body); w.Code != 403 {
			t.Fatal("manager", w.Code, w.Body.String())
		}
	}
	if w := call(adminToken, "", "PUT", "", body); w.Code != 403 {
		t.Fatal("csrf", w.Code)
	}
	if w := call(adminToken, admin.CSRFToken, "PUT", "", `{"enabled":true,"corporation_ids":[],"version":"1"}`); w.Code != 400 {
		t.Fatal("empty allow list", w.Code)
	}
	if w := call(adminToken, admin.CSRFToken, "PUT", "", body); w.Code != 200 {
		t.Fatal("save", w.Code, w.Body.String())
	}
	p, err := f.cleanup.Settings(ctx, admin.UserID)
	if err != nil || !p.Enabled || p.NextRun == nil || time.Until(*p.NextRun) < 6*24*time.Hour {
		t.Fatal("schedule", p, err)
	}
	if w := call(adminToken, admin.CSRFToken, "PUT", "", body); w.Code != 409 {
		t.Fatal("stale version", w.Code)
	}
	f.exec(t, `UPDATE identity_corporation_cleanup SET last_run_at=now(),last_removed_count=3`)
	p, err = f.cleanup.Save(ctx, admin.UserID, identity.CleanupInput{Enabled: false, Version: p.Version})
	if err != nil || p.NextRun != nil || p.LastRemoved != 3 {
		t.Fatal("disable/last result", p, err)
	}
	if err = f.acl.SetAdministrator(ctx, admin.UserID, false); err != nil {
		t.Fatal(err)
	}
	if w := call(adminToken, admin.CSRFToken, "GET", "", ""); w.Code != 403 {
		t.Fatal("demoted admin", w.Code)
	}
}

func TestCorporationCleanupOwnershipFollowsExplicitMerge(t *testing.T) {
	ctx := context.Background()
	f := newCleanupFixture(t)
	targetToken, target := f.sign(t, 101)
	sourceToken, source := f.sign(t, 201)
	if _, err := f.users.Complete(ctx, 202, "Alt", "owner", sourceToken, identity.LoginIntent{Kind: "link", UserID: source.UserID}, nil); err != nil {
		t.Fatal(err)
	}
	f.facts(t, 101, 10)
	f.facts(t, 201, 10)
	f.facts(t, 202, 20)
	f.due(t)
	f.run(t)
	f.users.MergeParticipants = map[string]identity.MergeParticipant{"test": func(context.Context, pgx.Tx, string, string, bool) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}}
	id, err := f.users.ProveMerge(ctx, 201, "owner", targetToken, target.UserID, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.users.Merge(ctx, targetToken, id, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.users.Merge(ctx, targetToken, id, preview.Token, true); err != nil {
		t.Fatal(err)
	}
	_, restored := f.sign(t, 202)
	if restored.UserID != target.UserID || restored.MainCharacter.ID != "101" {
		t.Fatal("restored retired account")
	}
	if f.scalar(t, `SELECT count(*) FROM identity_cleanup_removed WHERE character_id=202 AND user_id=$1 AND original_user_id=$2`, target.UserID, source.UserID) != 1 {
		t.Fatal("original ownership lost")
	}
}

func TestCorporationCleanupResumesDurableCursor(t *testing.T) {
	f := newCleanupFixture(t)
	for id := int64(100); id < 126; id++ {
		f.sign(t, id)
		f.facts(t, id, 20)
	}
	f.due(t)
	f.run(t)
	if f.scalar(t, `SELECT count(*) FROM identity_corporation_cleanup WHERE run_started_at IS NOT NULL AND after_user_id IS NOT NULL AND removed_count=25`) != 1 {
		t.Fatal("batch cursor was not committed")
	}
	if f.scalar(t, `SELECT count(*) FROM identity_characters`) != 1 {
		t.Fatal("unexpected first batch size")
	}
	// A new service instance represents a worker restart; no process-local cursor.
	f.cleanup = corporationCleanupService(f.pool, f.acl, nil)
	f.run(t)
	if f.scalar(t, `SELECT last_removed_count FROM identity_corporation_cleanup`) != 26 || f.scalar(t, `SELECT count(*) FROM identity_cleanup_audit WHERE action='removed'`) != 26 {
		t.Fatal("resume lost or duplicated progress")
	}
}
