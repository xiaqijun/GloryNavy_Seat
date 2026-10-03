package sentry

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

type pricingFixture struct {
	pool    *pgxpool.Pool
	service *Service
	admin   string
	member  string
}

func newPricingFixture(t *testing.T) pricingFixture {
	t.Helper()
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	adminToken, err := accounts.SignIn(ctx, 91101, "Pricing Admin", "pricing-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := accounts.Session(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	memberToken, err := accounts.SignIn(ctx, 91102, "Pricing Member", "pricing-member", "")
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := accounts.Session(ctx, memberToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO access_administrators(user_id) VALUES($1)`, adminSession.UserID); err != nil {
		t.Fatal(err)
	}
	service := New(pool, nil)
	service.AlertEnabled = true
	service.AlertPolicy = AlertGrantPolicy{PriceVersion: "env-default", UnitSeconds: 60, UnitPriceMinor: 10, MaxGrantSeconds: 3600, GrantTTL: time.Hour}
	service.Administrator = func(_ context.Context, user string) (bool, error) {
		return user == adminSession.UserID, nil
	}
	return pricingFixture{pool: pool, service: service, admin: adminSession.UserID, member: memberSession.UserID}
}

func testPricingEdit() AlertPricingEdit {
	return AlertPricingEdit{
		PriceVersion:    "prod-2026-10",
		UnitSeconds:     60,
		UnitPriceMinor:  25,
		MaxGrantSeconds: 7200,
		GrantTTLSeconds: 7200,
		Version:         0,
	}
}

func TestAlertPricingAdminPersistsAndAudits(t *testing.T) {
	f := newPricingFixture(t)
	ctx := context.Background()
	saved, err := f.service.EditAlertPricing(ctx, f.admin, testPricingEdit())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != 1 || !saved.Configured || !saved.CanEdit || saved.UnitPriceMinor != 25 || saved.GrantTTLSeconds != 7200 {
		t.Fatalf("saved pricing = %#v", saved)
	}
	read, err := f.service.ReadAlertPricing(ctx, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Configured || read.CanEdit || read.PriceVersion != saved.PriceVersion {
		t.Fatalf("member read = %#v", read)
	}

	reloaded := New(f.pool, nil)
	reloaded.AlertEnabled = true
	fallback := AlertGrantPolicy{PriceVersion: "wrong-fallback", UnitSeconds: 1, UnitPriceMinor: 1, MaxGrantSeconds: 1, GrantTTL: time.Minute}
	if err := reloaded.LoadAlertPricing(ctx, fallback); err != nil {
		t.Fatal(err)
	}
	loaded, err := reloaded.ReadAlertPricing(ctx, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PriceVersion != "prod-2026-10" || loaded.UnitPriceMinor != 25 || loaded.Version != 1 {
		t.Fatalf("reloaded pricing = %#v", loaded)
	}

	var previous, version int64
	var beforeJSON, afterJSON []byte
	if err := f.pool.QueryRow(ctx, `SELECT previous_version,version,before_snapshot,after_snapshot FROM sentry_alert_pricing_audit ORDER BY id DESC LIMIT 1`).Scan(&previous, &version, &beforeJSON, &afterJSON); err != nil {
		t.Fatal(err)
	}
	if previous != 0 || version != 1 {
		t.Fatalf("audit versions = %d -> %d", previous, version)
	}
	var before, after map[string]any
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatal(err)
	}
	if before["version"] != float64(0) || after["version"] != float64(1) || after["unit_price_minor"] != float64(25) {
		t.Fatalf("audit snapshots before=%v after=%v", before, after)
	}
}

func TestAlertPricingRejectsNonAdminAndStaleVersions(t *testing.T) {
	f := newPricingFixture(t)
	ctx := context.Background()
	if _, err := f.service.EditAlertPricing(ctx, f.member, testPricingEdit()); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("member edit error = %v, want pgx.ErrNoRows", err)
	}
	first, err := f.service.EditAlertPricing(ctx, f.admin, testPricingEdit())
	if err != nil {
		t.Fatal(err)
	}
	stale := testPricingEdit()
	stale.Version = first.Version - 1
	if _, err := f.service.EditAlertPricing(ctx, f.admin, stale); !errors.Is(err, ErrAlertPricingConflict) {
		t.Fatalf("stale edit error = %v, want ErrAlertPricingConflict", err)
	}
}

func TestTimePricingAdminControlsChargingSwitch(t *testing.T) {
	f := newPricingFixture(t)
	ctx := context.Background()
	enabled := true
	saved, err := f.service.EditTimePricing(ctx, f.admin, TimePricingEdit{
		AlertHourlyPriceMinor:    125,
		MonitorHourlyRewardMinor: 75,
		Version:                  0,
		ChargingEnabled:          &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.ChargingEnabled || saved.Version != 1 {
		t.Fatalf("saved time pricing = %#v", saved)
	}
	read, err := f.service.ReadTimePricing(ctx, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if !read.ChargingEnabled || read.AlertHourlyPriceMinor != 125 || read.MonitorHourlyRewardMinor != 75 || read.CanEdit {
		t.Fatalf("read time pricing = %#v", read)
	}

	disabled := false
	saved, err = f.service.EditTimePricing(ctx, f.admin, TimePricingEdit{
		AlertHourlyPriceMinor:    125,
		MonitorHourlyRewardMinor: 75,
		Version:                  1,
		ChargingEnabled:          &disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ChargingEnabled {
		t.Fatalf("switch remained enabled after disable: %#v", saved)
	}
}

func TestAlertPricingConcurrentFirstSaveHasOneConflict(t *testing.T) {
	f := newPricingFixture(t)
	ctx := context.Background()
	edit := testPricingEdit()
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.service.EditAlertPricing(ctx, f.admin, edit)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var success, conflict int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrAlertPricingConflict):
			conflict++
		default:
			t.Fatalf("concurrent edit error = %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent first save: success=%d conflict=%d", success, conflict)
	}
}

func TestAlertPricingRejectsInvalidRangesAndInt64Overflow(t *testing.T) {
	f := newPricingFixture(t)
	ctx := context.Background()
	cases := []AlertPricingEdit{
		func() AlertPricingEdit { e := testPricingEdit(); e.UnitSeconds = 0; return e }(),
		func() AlertPricingEdit { e := testPricingEdit(); e.UnitSeconds = math.MaxInt64; return e }(),
		func() AlertPricingEdit { e := testPricingEdit(); e.UnitPriceMinor = math.MaxInt64; return e }(),
		func() AlertPricingEdit { e := testPricingEdit(); e.MaxGrantSeconds = math.MaxInt64; return e }(),
		func() AlertPricingEdit { e := testPricingEdit(); e.GrantTTLSeconds = math.MaxInt64; return e }(),
		func() AlertPricingEdit { e := testPricingEdit(); e.GrantTTLSeconds = math.MinInt64; return e }(),
	}
	for i, edit := range cases {
		edit.Version = 0
		if _, err := f.service.EditAlertPricing(ctx, f.admin, edit); !errors.Is(err, ErrAlertPricingInvalid) {
			t.Errorf("case %d error = %v, want ErrAlertPricingInvalid", i, err)
		}
	}
}
