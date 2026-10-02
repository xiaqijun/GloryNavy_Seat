package attendance

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/testutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestAllianceAutomaticBaselineDeltaAndRollback(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	const user = "00000000-0000-4000-8000-000000000001"
	if _, err := pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1);`, user); err != nil {
		t.Fatal(err)
	}
	// One hundredths of a coin per PAP deliberately exercises sub-cent accumulation.
	if _, err := pool.Exec(ctx, `UPDATE exchange_source_rates SET minor_per_unit=1,conversion_mode='automatic' WHERE source_id='alliance_pap'`); err != nil {
		t.Fatal(err)
	}
	pap := "3.00"
	complete := true
	month := time.Now().UTC()
	bound := true
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"ok":true,"source":"winterco","year":%d,"month":%d,"complete":%t,"records_total":1,"rows":[{"character_id":"101","character_name":"Pilot","pap":"%s"}]}`, month.Year(), month.Month(), complete, pap)
	}))
	defer server.Close()
	auth := t.TempDir() + "/auth.json"
	if err := os.WriteFile(auth, []byte(`{"api_token":"test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	coins := &exchange.Service{Pool: pool, Sources: map[string]string{"alliance_pap": "Alliance PAP"}, SourceScales: map[string]int64{"alliance_pap": 100}, AllowNew: true}
	s := &Service{Pool: pool, AlliancePAP: AlliancePAPConfig{URL: server.URL, AuthFile: auth}, Bindings: func(context.Context, pgx.Tx, []int64) ([]Binding, error) {
		if !bound {
			return nil, nil
		}
		return []Binding{{ID: 101, UserID: user, Name: "Pilot"}}, nil
	}}
	s.AllianceCoinAwards = func(ctx context.Context, tx pgx.Tx, key, reason string, rows []PAPCoinAward) error {
		awards := []exchange.Award{}
		for _, r := range rows {
			awards = append(awards, exchange.Award{AccountID: r.AccountID, Reference: r.Reference, Units: r.Units, Previous: r.Previous})
		}
		if err := coins.ReconcileTx(ctx, tx, "alliance_pap", key, reason, awards); err != nil {
			return err
		}
		if fail {
			return fmt.Errorf("publish failure")
		}
		return nil
	}
	check := func(want int64) {
		t.Helper()
		var got int64
		if err := pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger`).Scan(&got); err != nil || got != want {
			t.Fatal(got, want, err)
		}
	}
	sync := func() {
		t.Helper()
		if err := s.SyncAlliancePAP(ctx); err != nil {
			t.Fatal(err)
		}
	}
	sync()
	check(0) // No history backfill.
	pap = "3.50"
	sync()
	check(0)
	pap = "4.00"
	sync()
	check(1)
	sync()
	check(1) // Fractional progress persisted, replay no-op.
	pap = "3.50"
	sync()
	check(0)
	pap = "4.00"
	sync()
	check(1) // Correction/rebound cannot mint twice.
	pap = "5.00"
	complete = false
	if err := s.SyncAlliancePAP(ctx); err == nil {
		t.Fatal("partial snapshot accepted")
	}
	check(1)
	complete = true
	fail = true
	if err := s.SyncAlliancePAP(ctx); err == nil {
		t.Fatal("partial publication committed")
	}
	check(1)
	fail = false
	sync()
	check(2)
	bound = false
	pap = "6.00"
	sync()
	check(2)
	bound = true
	sync()
	check(2) // Rebinding does not backfill unbound increases.
	month = month.AddDate(0, -1, 0)
	if err := s.SyncAlliancePAP(ctx); err == nil {
		t.Fatal("past month accepted")
	}
	check(2)
}

func TestAllianceSnapshotAccountMerge(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := &Service{Pool: pool}
	const source = "00000000-0000-4000-8000-000000000001"
	const target = "00000000-0000-4000-8000-000000000002"
	if _, err := pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1),($2)`, source, target); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO attendance_alliance_pap_snapshot(month,character_id,character_name,pap,account_id) VALUES(date_trunc('month',now())::date,101,'Pilot',3,$1)`, source); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	before, err := s.MergeAccountTx(ctx, tx, source, target, false)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err = json.Unmarshal(before, &summary); err != nil || summary["fingerprint"] == nil || summary["points"] == nil {
		t.Fatal(summary, err)
	}
	if _, err = tx.Exec(ctx, `UPDATE attendance_alliance_pap_snapshot SET pap=4 WHERE character_id=101`); err != nil {
		t.Fatal(err)
	}
	after, err := s.MergeAccountTx(ctx, tx, source, target, false)
	if err != nil || string(after) == string(before) {
		t.Fatal("preview did not reflect PAP change", err)
	}
	if _, err = s.MergeAccountTx(ctx, tx, source, target, true); err != nil {
		t.Fatal(err)
	}
	var owner, original string
	if err = tx.QueryRow(ctx, `SELECT account_id::text,original_account_id::text FROM attendance_alliance_pap_snapshot WHERE character_id=101`).Scan(&owner, &original); err != nil || owner != target || original != source {
		t.Fatal(owner, original, err)
	}
}

func TestAllianceHistoricalMonthManualConversion(t *testing.T) {
	ctx := context.Background()
	p := testutil.Database(t)
	const user = "00000000-0000-4000-8000-000000000011"
	if _, err := p.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE exchange_source_rates SET minor_per_unit=100, conversion_mode='manual' WHERE source_id='alliance_pap'`); err != nil {
		t.Fatal(err)
	}
	month := allianceMonth(time.Now().UTC().AddDate(0, -1, 0))
	if _, err := p.Exec(ctx, `
		INSERT INTO attendance_alliance_pap_snapshot(month,character_id,character_name,pap,account_id)
		VALUES($1,101,'Pilot',3.00,$2)`, month, user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `
		INSERT INTO attendance_alliance_pap_sync_history(month,state,complete,records_total,last_synced_at,version)
		VALUES($1,'ready',true,1,now(),7)`, month); err != nil {
		t.Fatal(err)
	}
	coins := &exchange.Service{
		Pool: p, Administrator: func(context.Context, string) (bool, error) { return true, nil },
		Sources: map[string]string{"alliance_pap": "Alliance PAP"}, SourceScales: map[string]int64{"alliance_pap": 100}, AllowNew: true,
	}
	s := &Service{
		Pool:          p,
		Administrator: func(_ context.Context, actor string) (bool, error) { return actor == user, nil },
		Bindings: func(context.Context, pgx.Tx, []int64) ([]Binding, error) {
			return []Binding{{ID: 101, UserID: user, Name: "Pilot"}}, nil
		},
	}
	s.AllianceCoinConversion = func(ctx context.Context, tx pgx.Tx, actor, key, reason, token string, rows []PAPCoinAward) (PAPCoinQuote, error) {
		awards := make([]exchange.Award, 0, len(rows))
		for _, row := range rows {
			awards = append(awards, exchange.Award{Reference: row.Reference, AccountID: row.AccountID, Previous: row.Previous, Units: row.Units})
		}
		q, err := coins.ConversionTx(ctx, tx, actor, "alliance_pap", key, reason, token, awards)
		return PAPCoinQuote{Token: q.Token, Mode: q.Mode, Points: q.Points, Converted: q.Converted, Pending: q.Pending, CoinsMinor: q.CoinsMinor, Characters: q.Characters, UnitScale: q.UnitScale}, err
	}
	months, err := s.AlliancePAPConversionMonths(ctx, user)
	if err != nil || len(months) != 1 || months[0].Month != month.Format("2006-01") || months[0].Pending != 300 {
		t.Fatalf("historical quote: %+v, %v", months, err)
	}
	conversion := &PAPConversion{Version: months[0].Version, RequestKey: "11111111-1111-4111-8111-111111111112", Token: months[0].Token, Reason: "九月联盟 PAP 补兑", Month: months[0].Month}
	if _, err = s.ConvertAlliancePAP(ctx, user, conversion); err != nil {
		t.Fatal(err)
	}
	var total int64
	if err = p.QueryRow(ctx, `SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger`).Scan(&total); err != nil || total != 300 {
		t.Fatalf("coin total=%d err=%v", total, err)
	}
	quote, err := s.ConvertAlliancePAP(ctx, user, nil, month.Format("2006-01"))
	if err != nil || quote.Pending != 0 {
		t.Fatalf("historical replay quote=%+v err=%v", quote, err)
	}
}
