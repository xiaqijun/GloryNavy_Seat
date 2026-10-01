package attendance

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange"
	"sync"
	"testing"
)

func papFixture(t *testing.T) (*Service, Event) {
	t.Helper()
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	captured, err := s.Change(ctx, manager, e.ID, "capture", Change{Version: e.Version, SourceID: 1, RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := s.Change(ctx, manager, e.ID, "close", Change{Version: captured.Event.Version, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc"})
	if err != nil {
		t.Fatal(err)
	}
	return s, closed.Event
}

func TestPendingPAPIsManagerScopedAndExcludesIssuedEvents(t *testing.T) {
	s, e := papFixture(t)
	ctx := context.Background()
	pending, err := s.PendingPAP(ctx, manager)
	if err != nil || len(pending.Events) != 1 || pending.Events[0].ID != e.ID || !pending.Events[0].CanManage {
		t.Fatal(pending, err)
	}
	memberPending, err := s.PendingPAP(ctx, member)
	if err != nil || len(memberPending.Events) != 0 {
		t.Fatal("member saw manager queue", memberPending, err)
	}
	if err = s.SetPAP(ctx, manager, e.ID, PAPChange{Version: e.Version, RequestKey: "11111111-1111-4111-8111-111111111101", Points: 1, Reason: "统一发放"}); err != nil {
		t.Fatal(err)
	}
	pending, err = s.PendingPAP(ctx, manager)
	if err != nil || len(pending.Events) != 0 {
		t.Fatal("issued event remained pending", pending, err)
	}
}

func TestPAPSupplementAwardsOnlyNewManualAttendance(t *testing.T) {
	s, e := papFixture(t)
	ctx := context.Background()
	if err := s.SetPAP(ctx, manager, e.ID, PAPChange{
		Version: e.Version, RequestKey: "11111111-1111-4111-8111-111111111201", Points: 2, Reason: "统一发放",
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(ctx, manager, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The new character is bound only when the post-event manual record is made.
	originalBindings := s.Bindings
	s.Bindings = func(ctx context.Context, tx pgx.Tx, ids []int64) ([]Binding, error) {
		rows, err := originalBindings(ctx, tx, ids)
		for _, id := range ids {
			if id == 6 {
				rows = append(rows, Binding{ID: 6, Name: "Late pilot", UserID: member})
			}
		}
		return rows, err
	}
	changed, err := s.Change(ctx, manager, e.ID, "manual", Change{
		Version: detail.Event.Version, CharacterID: 6, Present: true, Reason: "补录", RequestKey: "22222222-2222-4222-8222-222222222201",
	})
	if err != nil {
		t.Fatal("manual supplement", err)
	}
	if err := s.SetPAP(ctx, manager, e.ID, PAPChange{
		Version: changed.Event.Version, RequestKey: "33333333-3333-4333-8333-333333333201", Points: 2, Supplement: true, Reason: "补录后补发",
	}); err != nil {
		t.Fatal("PAP supplement", err)
	}
	detail, err = s.Detail(ctx, manager, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	var late Entry
	for _, row := range detail.Entries {
		if row.ID == 6 {
			late = row
		}
	}
	if late.PAPPoints != 2 {
		t.Fatal("late participant did not receive PAP", late)
	}
	report, err := s.PAPReport(ctx, manager, 10, "30d", 0)
	if err != nil || report.Points != 8 || report.Participations != 4 {
		t.Fatal("supplement changed existing awards", report, err)
	}
	version := detail.Event.Version + 1
	if err := s.SetPAP(ctx, manager, e.ID, PAPChange{
		Version: version, RequestKey: "44444444-4444-4444-8444-444444444201", Points: 2, Supplement: true, Reason: "重复补发",
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("empty PAP supplement accepted", err)
	}
}

func TestPAPMultiCharacterReplayCorrectionAndHistory(t *testing.T) {
	s, e := papFixture(t)
	ctx := context.Background()
	c := PAPChange{Version: e.Version, RequestKey: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", Points: 2, Reason: "集结"}
	if err := s.SetPAP(ctx, member, e.ID, c); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("member issued", err)
	}
	if err := s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal("replay", err)
	}
	own, err := s.PAPReport(ctx, manager, 0, "month", 0)
	if err != nil || own.Points != 4 || own.Participations != 2 || len(own.Rows) != 2 {
		t.Fatal("alts not added", own, err)
	}
	all, err := s.PAPReport(ctx, manager, 10, "30d", 0)
	if err != nil || all.Points != 6 || all.Participations != 3 {
		t.Fatal("external/unbound counted", all, err)
	}
	if _, err = s.PAPReport(ctx, member, 10, "month", 0); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("scope bypass", err)
	}
	detail, err := s.Detail(ctx, manager, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Event.PAPIssued || detail.Entries[0].PAPPoints != 2 {
		t.Fatal(detail)
	}
	if _, err = s.Change(ctx, manager, e.ID, "reopen", Change{Version: detail.Event.Version, RequestKey: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", Reason: "修订"}); !errors.Is(err, ErrConflict) {
		t.Fatal("reopened issued scores", err)
	}
	c.Version = detail.Event.Version
	c.Points = 3
	c.RequestKey = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	c.Reason = "更正分值"
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	history, err := s.PAPHistory(ctx, member, e.ID, 0)
	if err != nil || len(history.Items) != 2 || history.Items[1].Delta != 1 || history.Items[1].Balance != 3 {
		t.Fatal(history, err)
	}
	c.Version++
	c.Revoke = true
	c.RequestKey = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	c.Reason = "撤销"
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	own, err = s.PAPReport(ctx, manager, 0, "month", 0)
	if err != nil || own.Points != 0 || len(own.Rows) != 0 {
		t.Fatal(own, err)
	}
	history, err = s.PAPHistory(ctx, manager, e.ID, 0)
	if err != nil || len(history.Items) != 9 {
		t.Fatal("ledger deleted", history, err)
	}
	cancelled, err := s.Change(ctx, manager, e.ID, "manual", Change{Version: c.Version + 1, CharacterID: 2, Present: false, Reason: "误记", RequestKey: "acacacac-acac-4cac-8cac-acacacacacac"})
	if err != nil {
		t.Fatal(err)
	}
	c.Version = cancelled.Event.Version
	c.Revoke = false
	c.RequestKey = "aeaeaeae-aeae-4eae-8eae-aeaeaeaeaeae"
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	own, err = s.PAPReport(ctx, manager, 0, "month", 0)
	if err != nil || own.Points != 3 || len(own.Rows) != 1 {
		t.Fatal("cancelled still awarded", own, err)
	}
	// A changed current binding does not transfer the historical award.
	s.Own = func(context.Context, string) ([]Binding, error) { return []Binding{}, nil }
	own, err = s.PAPReport(ctx, manager, 0, "month", 0)
	if err != nil || own.Points != 3 {
		t.Fatal("history tied to current bindings", own, err)
	}
	s.Manage = func(context.Context, string, int64) (bool, error) { return false, nil }
	if err = s.SetPAP(ctx, manager, e.ID, c); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked manager replay", err)
	}
}

func TestPAPConcurrentIssuanceAndTransactionRollback(t *testing.T) {
	s, e := papFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- s.SetPAP(ctx, manager, e.ID, PAPChange{Version: e.Version, RequestKey: fmt.Sprintf("00000000-0000-4000-8000-%012d", i), Points: 2, Reason: "集结"})
		}(i)
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal(ok, conflict)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, "SELECT count(*) FROM attendance_pap_ledger").Scan(&count); err != nil || count != 3 {
		t.Fatal(count, err)
	}
	// Force the ledger insert to fail after a balance update; the entire publication rolls back.
	_, err := s.Pool.Exec(ctx, `CREATE FUNCTION reject_pap() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test failure'; END$$; CREATE TRIGGER reject_pap BEFORE INSERT ON attendance_pap_ledger FOR EACH ROW EXECUTE FUNCTION reject_pap()`)
	if err != nil {
		t.Fatal(err)
	}
	err = s.SetPAP(ctx, manager, e.ID, PAPChange{Version: e.Version + 1, RequestKey: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", Points: 3, Reason: "修正"})
	if err == nil {
		t.Fatal("failure swallowed")
	}
	report, err := s.PAPReport(ctx, manager, 10, "month", 0)
	if err != nil || report.Points != 6 {
		t.Fatal("partial balances committed", report, err)
	}
	detail, err := s.Detail(ctx, manager, e.ID)
	if err != nil || detail.Event.Version != e.Version+1 || detail.Event.PAPPoints != 2 {
		t.Fatal(detail, err)
	}
}

func TestPAPCoinHookAtomicityAndCorrection(t *testing.T) {
	s, e := papFixture(t)
	if _, err := s.Pool.Exec(context.Background(), "UPDATE exchange_source_rates SET conversion_mode='automatic'"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	coins := &exchange.Service{Pool: s.Pool, Sources: map[string]string{"pap": "PAP"}, AllowNew: true, Administrator: func(context.Context, string) (bool, error) { return true, nil }}
	s.CoinAwards = func(ctx context.Context, tx pgx.Tx, key, reason string, rows []PAPCoinAward) error {
		awards := []exchange.Award{}
		for _, r := range rows {
			awards = append(awards, exchange.Award{Reference: r.Reference, AccountID: r.AccountID, Previous: r.Previous, Units: r.Units})
		}
		return coins.ReconcileTx(ctx, tx, "pap", key, reason, awards)
	}
	c := PAPChange{Version: e.Version, RequestKey: "11111111-1111-4111-8111-111111111112", Points: 2, Reason: "集结"}
	if err := s.SetPAP(ctx, manager, e.ID, c); !errors.Is(err, exchange.ErrRateRequired) {
		t.Fatal("unconfigured minted", err)
	}
	d, err := s.Detail(ctx, manager, e.ID)
	if err != nil || d.Event.PAPIssued || d.Event.Version != e.Version {
		t.Fatal("partial PAP commit", d, err)
	}
	if err = coins.EditSource(ctx, manager, exchange.SourceEdit{ID: "pap", MinorPerUnit: 250, Version: 1, RequestKey: "11111111-1111-4111-8111-111111111113"}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal("replay", err)
	}
	var sum int64
	if err = s.Pool.QueryRow(ctx, "SELECT sum(delta) FROM exchange_coin_ledger WHERE kind='source'").Scan(&sum); err != nil || sum != 1500 {
		t.Fatal("alts credit", sum, err)
	}
	if err = coins.EditSource(ctx, manager, exchange.SourceEdit{ID: "pap", MinorPerUnit: 1000, Version: 2, RequestKey: "11111111-1111-4111-8111-111111111114"}); err != nil {
		t.Fatal(err)
	}
	c.Version++
	c.Points = 3
	c.RequestKey = "11111111-1111-4111-8111-111111111115"
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, "SELECT sum(delta) FROM exchange_coin_ledger WHERE kind='source'").Scan(&sum); err != nil || sum != 2250 {
		t.Fatal("correction used new rate", sum, err)
	}
	// Failure in the currency journal must roll back PAP and currency together.
	_, err = s.Pool.Exec(ctx, `CREATE FUNCTION reject_coin() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test'; END$$; CREATE TRIGGER reject_coin BEFORE INSERT ON exchange_coin_ledger FOR EACH ROW EXECUTE FUNCTION reject_coin()`)
	if err != nil {
		t.Fatal(err)
	}
	c.Version++
	c.Points = 4
	c.RequestKey = "11111111-1111-4111-8111-111111111116"
	if err = s.SetPAP(ctx, manager, e.ID, c); err == nil {
		t.Fatal("failure swallowed")
	}
	report, err := s.PAPReport(ctx, manager, 0, "month", 0)
	if err != nil || report.Points != 6 {
		t.Fatal(report, err)
	}
}
