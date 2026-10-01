package attendance

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange"
)

func conversionFixture(t *testing.T) (*Service, *exchange.Service, Event) {
	s, e := papFixture(t)
	s.Administrator = func(_ context.Context, u string) (bool, error) { return u == manager, nil }
	coins := &exchange.Service{Pool: s.Pool, Sources: map[string]string{"pap": "PAP", "alliance_pap": "联盟 PAP"}, SourceScales: map[string]int64{"alliance_pap": 100}, AllowNew: true, Administrator: s.Administrator}
	rowsToAwards := func(rows []PAPCoinAward) []exchange.Award {
		out := []exchange.Award{}
		for _, r := range rows {
			out = append(out, exchange.Award{Reference: r.Reference, AccountID: r.AccountID, Previous: r.Previous, Units: r.Units})
		}
		return out
	}
	s.CoinAwards = func(ctx context.Context, tx pgx.Tx, key, reason string, rows []PAPCoinAward) error {
		return coins.ReconcileTx(ctx, tx, "pap", key, reason, rowsToAwards(rows))
	}
	s.CoinConversion = func(ctx context.Context, tx pgx.Tx, user, key, reason, token string, rows []PAPCoinAward) (PAPCoinQuote, error) {
		q, err := coins.ConversionTx(ctx, tx, user, "pap", key, reason, token, rowsToAwards(rows))
		return PAPCoinQuote{Token: q.Token, Mode: q.Mode, Points: q.Points, Converted: q.Converted, Pending: q.Pending, CoinsMinor: q.CoinsMinor, Characters: q.Characters}, err
	}
	return s, coins, e
}
func coinTotal(t *testing.T, s *Service, want int64) {
	t.Helper()
	var got int64
	if err := s.Pool.QueryRow(context.Background(), "SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE kind='source'").Scan(&got); err != nil || got != want {
		t.Fatalf("coins %d != %d: %v", got, want, err)
	}
}

func TestManualConversionModesReplayAndCorrections(t *testing.T) {
	s, coins, e := conversionFixture(t)
	ctx := context.Background()
	if err := coins.EditSource(ctx, manager, exchange.SourceEdit{ID: "alliance_pap", Mode: "automatic", MinorPerUnit: 250, Version: 1, RequestKey: "11111111-1111-4111-8111-111111111200"}); err != nil {
		t.Fatalf("alliance PAP automatic mode rejected: %v", err)
	}
	c := PAPChange{Version: e.Version, RequestKey: "11111111-1111-4111-8111-111111111201", Points: 2, Reason: "PAP"}
	if err := s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal("manual PAP needs no rate", err)
	}
	coinTotal(t, s, 0)
	if _, err := s.ConvertPAP(ctx, member, e.ID, nil); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("member preview", err)
	}
	if err := coins.EditSource(ctx, manager, exchange.SourceEdit{ID: "pap", Mode: "manual", MinorPerUnit: 250, Version: 1, RequestKey: "11111111-1111-4111-8111-111111111202"}); err != nil {
		t.Fatal(err)
	}
	q, err := s.ConvertPAP(ctx, manager, e.ID, nil)
	if err != nil || q.Pending != 6 || q.CoinsMinor != 1500 || q.Converted != 0 {
		t.Fatal(q, err)
	}
	d := PAPConversion{Version: c.Version + 1, RequestKey: "11111111-1111-4111-8111-111111111203", Token: q.Token, Reason: "统一兑换"}
	if _, err = s.ConvertPAP(ctx, manager, e.ID, &d); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConvertPAP(ctx, manager, e.ID, &d); err != nil {
		t.Fatal("replay", err)
	}
	coinTotal(t, s, 1500)
	s.Administrator = func(context.Context, string) (bool, error) { return false, nil }
	if _, err = s.ConvertPAP(ctx, manager, e.ID, &d); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked admin replay", err)
	}
	s.Administrator = coins.Administrator
	c.Version++
	c.Points = 4
	c.RequestKey = "11111111-1111-4111-8111-111111111204"
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	coinTotal(t, s, 1500)
	if err = coins.EditSource(ctx, manager, exchange.SourceEdit{ID: "pap", Mode: "automatic", MinorPerUnit: 1000, Version: 2, RequestKey: "11111111-1111-4111-8111-111111111205"}); err != nil {
		t.Fatal(err)
	}
	coinTotal(t, s, 1500)
	c.Version++
	c.Points = 5
	c.RequestKey = "11111111-1111-4111-8111-111111111206"
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	coinTotal(t, s, 2250) // Only +1 each; original rate retained.
	q, err = s.ConvertPAP(ctx, manager, e.ID, nil)
	if err != nil || q.Converted != 9 || q.Pending != 6 || q.CoinsMinor != 1500 {
		t.Fatal(q, err)
	}
	d = PAPConversion{Version: c.Version + 1, RequestKey: "11111111-1111-4111-8111-111111111207", Token: q.Token, Reason: "补兑"}
	if _, err = s.ConvertPAP(ctx, manager, e.ID, &d); err != nil {
		t.Fatal(err)
	}
	coinTotal(t, s, 3750)
	q, err = s.ConvertPAP(ctx, manager, e.ID, nil)
	if err != nil || q.Pending != 0 || q.Converted != 15 {
		t.Fatal(q, err)
	}
	c.Version++
	c.Revoke = true
	c.RequestKey = "11111111-1111-4111-8111-111111111208"
	if err = s.SetPAP(ctx, manager, e.ID, c); err != nil {
		t.Fatal(err)
	}
	coinTotal(t, s, 0)
}

func TestConversionConcurrentPreviewAndRollback(t *testing.T) {
	s, coins, e := conversionFixture(t)
	ctx := context.Background()
	if err := coins.EditSource(ctx, manager, exchange.SourceEdit{ID: "pap", Mode: "manual", MinorPerUnit: 100, Version: 1, RequestKey: "11111111-1111-4111-8111-111111111301"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPAP(ctx, manager, e.ID, PAPChange{Version: e.Version, Points: 2, Reason: "PAP", RequestKey: "11111111-1111-4111-8111-111111111302"}); err != nil {
		t.Fatal(err)
	}
	q, err := s.ConvertPAP(ctx, manager, e.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = coins.EditSource(ctx, manager, exchange.SourceEdit{ID: "pap", Mode: "manual", MinorPerUnit: 200, Version: 2, RequestKey: "11111111-1111-4111-8111-111111111303"}); err != nil {
		t.Fatal(err)
	}
	d := PAPConversion{Version: e.Version + 1, Token: q.Token, Reason: "兑换", RequestKey: "11111111-1111-4111-8111-111111111304"}
	if _, err = s.ConvertPAP(ctx, manager, e.ID, &d); !errors.Is(err, exchange.ErrConflict) {
		t.Fatal("stale price", err)
	}
	coinTotal(t, s, 0)
	q, err = s.ConvertPAP(ctx, manager, e.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	d.Token = q.Token
	if _, err = s.Pool.Exec(ctx, `CREATE FUNCTION reject_conversion() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test'; END$$; CREATE TRIGGER reject_conversion BEFORE INSERT ON exchange_shop_audit FOR EACH ROW WHEN (NEW.kind='conversion') EXECUTE FUNCTION reject_conversion()`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConvertPAP(ctx, manager, e.ID, &d); err == nil {
		t.Fatal("audit failure ignored")
	}
	coinTotal(t, s, 0)
	if _, err = s.Pool.Exec(ctx, "DROP TRIGGER reject_conversion ON exchange_shop_audit"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"11111111-1111-4111-8111-111111111305", "11111111-1111-4111-8111-111111111306"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			c := d
			c.RequestKey = key
			_, err := s.ConvertPAP(ctx, manager, e.ID, &c)
			results <- err
		}(key)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, exchange.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	coinTotal(t, s, 1200)
	report, err := s.PAPReport(ctx, manager, 10, "month", 0)
	if err != nil || report.Points != 6 {
		t.Fatal("conversion consumed attendance", report, err)
	}
}
