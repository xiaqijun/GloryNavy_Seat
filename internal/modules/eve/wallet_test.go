package eve

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestWalletPrecisionAndValidation(t *testing.T) {
	for _, v := range []string{`123456789012345.67`, `-0.01`, `1.5e10`, `0`} {
		if !validWalletNumber([]byte(v)) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{`"12.50"`, `null`, `true`, `1e999`, `NaN`} {
		if validWalletNumber([]byte(v)) {
			t.Fatal(v)
		}
	}
	for _, raw := range []string{`{"id":1,"date":"2026-09-01T00:00:00Z","description":"test","ref_type":"future_official_type"}`, `{"id":9007199254740993,"date":"2026-09-01T00:00:00Z","description":"test","ref_type":"player_donation","amount":123456789012345.67}`} {
		if _, e := validateWalletRecord([]byte(raw), "journal"); e != nil {
			t.Fatal(e)
		}
	}
	for _, raw := range []string{`null`, `{"id":1}`, `{"id":1,"date":"2026-09-01T00:00:00Z","description":"test","ref_type":"player_donation","amount":"12.50"}`} {
		if _, e := validateWalletRecord([]byte(raw), "journal"); e == nil {
			t.Fatal(raw)
		}
	}
}
func walletFixture(t *testing.T) (*SyncService, Character) {
	s, ch := syncFixture(t)
	s.SetWalletEnabled(true)
	ch.authorization.Scopes = append(ch.authorization.Scopes, CharacterWalletScope, CorporationWalletScope, WalletDivisionsScope)
	if e := s.auth.Save(context.Background(), ch); e != nil {
		t.Fatal(e)
	}
	if e := s.dispatch(context.Background()); e != nil {
		t.Fatal(e)
	}
	return s, ch
}

func TestWalletMissingRoleIsDeferredWithoutRequest(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	setContractCorporation(t, s, ch, 10)
	contractTransport(t, s, func(*http.Request) *http.Response { t.Fatal("unqualified character requested wallet"); return nil })
	target := contractTarget(t, s, ch.ID, "corporation_wallet_journal")
	e := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, target.Resource)
	var snooze *river.JobSnoozeError
	if !errors.As(e, &snooze) {
		t.Fatal(e)
	}
	var outcome, reason string
	if e = s.pool.QueryRow(ctx, "SELECT outcome,reason FROM eve_sync_runs WHERE target_id=$1 ORDER BY id DESC LIMIT 1", target.ID).Scan(&outcome, &reason); e != nil || outcome != "deferred" || reason != "missing_role" {
		t.Fatal(outcome, reason, e)
	}
}
func TestWalletJournalPaginationCacheHistoryAndOwnerFence(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	q := store.New(s.pool)
	target := contractTarget(t, s, ch.ID, "wallet_journal")
	c, e := q.GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	contractTransport(t, s, func(r *http.Request) *http.Response {
		calls++
		if r.URL.Path != "/characters/123/wallet/journal/" {
			t.Fatal(r.URL.Path)
		}
		raw := json.RawMessage(`{"id":9007199254740993,"date":"2026-09-01T00:00:00Z","ref_type":"player_donation","description":"test","amount":123456789012345.67,"balance":123456789012345.67}`)
		if r.URL.Query().Get("page") == "2" {
			raw = json.RawMessage(`{"id":9007199254740992,"date":"2026-08-01T00:00:00Z","ref_type":"player_donation","description":"test","amount":-0.01}`)
		}
		return contractListResponse([]json.RawMessage{raw}, "2")
	})
	publish := func(v syncResult) {
		t.Helper()
		claim, e := q.ClaimSync(ctx, store.ClaimSyncParams{ID: target.ID, Generation: target.Generation, ActiveJobID: target.ActiveJobID})
		if e != nil {
			t.Fatal(e)
		}
		e = s.finish(ctx, claim, c, target.ActiveJobID.Int64, v, nil)
		var snooze *river.JobSnoozeError
		if e != nil && !errors.As(e, &snooze) {
			t.Fatal(e)
		}
	}
	collect := func() syncResult {
		t.Helper()
		v, e := s.collectWallet(ctx, target, c)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	first := collect()
	if first.wallet.Done {
		t.Fatal("partial marked complete")
	}
	publish(first)
	second := collect()
	if !second.wallet.Done || second.next.Before(time.Now().Add(8*time.Minute)) {
		t.Fatal("cache expiry ignored")
	}
	publish(second)
	f := WalletFilter{Kind: "character", Owner: ch.ID, Part: "journal"}
	rows, e := s.auth.WalletData(ctx, f)
	if e != nil || len(rows) != 2 || rows[0]["id"] != "9007199254740993" || rows[0]["amount"] != "123456789012345.67" {
		t.Fatal(rows, e)
	}
	// New scan reuses cache. Missing historical rows are never removed by a later scan.
	again := collect()
	if calls != 2 {
		t.Fatal("fresh cache bypassed", calls)
	}
	tx, _ := s.pool.Begin(ctx)
	if e = s.saveWalletBatch(ctx, tx, target, c, again.wallet); e != nil {
		t.Fatal(e)
	}
	_ = tx.Commit(ctx)
	f.Direction = "out"
	rows, e = s.auth.WalletData(ctx, f)
	if e != nil || len(rows) != 1 || rows[0]["amount"] != "-0.01" {
		t.Fatal(rows, e)
	}
	f.Direction = ""
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	f.From = &from
	rows, e = s.auth.WalletData(ctx, f)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	f.From = nil
	ch.authorization.Scopes = []string{CorporationRolesScope}
	if e = s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	rows, e = s.auth.WalletData(ctx, f)
	if e != nil || len(rows) != 0 {
		t.Fatal("removed scope exposed history", e)
	}
	// Stale grant cannot publish after reauthorization.
	stale := first
	stale.wallet.Records[0].ID = 111
	if e = s.finish(ctx, target, c, target.ActiveJobID.Int64, stale, nil); e != nil {
		t.Fatal(e)
	}
	var count int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_wallet_observations WHERE record_id=111").Scan(&count)
	if count != 0 {
		t.Fatal("stale writer published")
	}
	ch.authorization.Scopes = append(ch.authorization.Scopes, CharacterWalletScope)
	ch.Owner = "different owner"
	if e = s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	rows, e = s.auth.WalletData(ctx, f)
	if e != nil || len(rows) != 0 {
		t.Fatal("owner inherited history", e)
	}
}
func TestWalletTransactionsCursorAndCorporationSource(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	q := store.New(s.pool)
	c, e := q.GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	target := contractTarget(t, s, ch.ID, "wallet_transactions")
	seen := []string{}
	contractTransport(t, s, func(r *http.Request) *http.Response {
		seen = append(seen, r.URL.Query().Get("from_id"))
		data := json.RawMessage(`[{"transaction_id":9007199254740993,"date":"2026-09-01T00:00:00Z","location_id":60003760,"type_id":34,"quantity":5,"unit_price":0.01,"client_id":456,"journal_ref_id":-1,"is_buy":true,"is_personal":true}]`)
		// Live ESI includes the cursor itself at the last boundary.
		return esiResponse(data)
	})
	v, e := s.collectWallet(ctx, target, c)
	if e != nil || v.wallet.Done {
		t.Fatal(e)
	}
	tx, _ := s.pool.Begin(ctx)
	if e = s.saveWalletBatch(ctx, tx, target, c, v.wallet); e != nil {
		t.Fatal(e)
	}
	_ = tx.Commit(ctx)
	v, e = s.collectWallet(ctx, target, c)
	if e != nil || !v.wallet.Done || strings.Join(seen, ",") != ",9007199254740993" {
		t.Fatal(seen, e)
	}
	setContractCorporation(t, s, ch, 10)
	if _, e = walletCorporation(ctx, s.pool, c, "corporation_wallet_journal"); e == nil {
		t.Fatal("missing role allowed")
	}
	_, e = s.pool.Exec(ctx, "UPDATE eve_role_snapshots SET roles=ARRAY['Junior_Accountant'] WHERE character_id=$1", ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	if id, e := walletCorporation(ctx, s.pool, c, "corporation_wallet_journal"); e != nil || id != 10 {
		t.Fatal(id, e)
	}
	if _, e = walletCorporation(ctx, s.pool, c, "corporation_wallet_divisions"); e == nil {
		t.Fatal("non-director read division names")
	}
	// A second eligible character can take over once the first credential is blocked.
	other := ch
	other.ID = 456
	other.Owner = "other"
	if e = s.auth.Save(ctx, other); e != nil {
		t.Fatal(e)
	}
	if e = s.dispatch(ctx); e != nil {
		t.Fatal(e)
	}
	setContractCorporation(t, s, other, 10)
	_, e = s.pool.Exec(ctx, "UPDATE eve_role_snapshots SET roles=ARRAY['Accountant'] WHERE character_id=456")
	if e != nil {
		t.Fatal(e)
	}
	if id, e := store.WalletSource(ctx, s.pool, 10, "corporation_wallet_journal", CorporationWalletScope, false); e != nil || id != ch.ID {
		t.Fatal(id, e)
	}
	_, _ = s.pool.Exec(ctx, "UPDATE eve_sync_targets SET state='blocked' WHERE character_id=$1 AND resource='corporation_wallet_journal'", ch.ID)
	if id, e := store.WalletSource(ctx, s.pool, 10, "corporation_wallet_journal", CorporationWalletScope, false); e != nil || id != other.ID {
		t.Fatal(id, e)
	}
	if _, e = store.WalletCursor(ctx, s.pool, target.ID, c.GrantGeneration+1, ch.ID); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("old cursor reused", e)
	}
}
