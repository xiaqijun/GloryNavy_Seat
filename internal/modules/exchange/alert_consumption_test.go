package exchange

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAlertConsumptionsEmptyPageUsesArray(t *testing.T) {
	s, _, _ := rewardFixture(t)
	page, err := s.AlertConsumptions(context.Background(), manager, 0, "", nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Items == nil {
		t.Fatal("empty page must keep items as an initialized array")
	}
}

func TestAlertTimeReservationAndSettlement(t *testing.T) {
	s, _, _ := rewardFixture(t)
	s.AllowAlertConsumption = true
	ctx := context.Background()
	grantID := rewardKey(70)
	if err := s.ReserveAlertTime(ctx, AlertTimeGrantRequest{
		ID: grantID, AccountID: manager, RequestKey: rewardKey(71), UnitSeconds: 1, UnitPriceMinor: 1,
		ReservedSeconds: 5, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(time.Second)
	if err := s.ReserveAlertInterval(ctx, AlertTimeIntervalRequest{GrantID: grantID, IntervalID: "interval-1", RequestKey: rewardKey(72), StartedAt: start, EndedAt: start.Add(2 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SettleAlertTime(ctx, grantID, "interval-1", rewardKey(73)); err != nil {
		t.Fatal(err)
	}
	sh, err := s.Shop(ctx, manager, 0)
	if err != nil || sh.Reserved != 3 || sh.Spent != 2 {
		t.Fatalf("after settlement: %+v, %v", sh, err)
	}
	if err := s.ReserveAlertTime(ctx, AlertTimeGrantRequest{ID: rewardKey(76), AccountID: manager, RequestKey: rewardKey(77), UnitSeconds: 1, UnitPriceMinor: 999999999999, ReservedSeconds: 5, ExpiresAt: time.Now().UTC().Add(time.Hour)}); !errors.Is(err, ErrInsufficientCoinsMinor) {
		t.Fatalf("expected insufficient coins, got %v", err)
	}
}

func TestAlertUsageReadModelKeepsCoinStatesSeparate(t *testing.T) {
	s, _, _ := rewardFixture(t)
	s.AllowAlertConsumption = true
	ctx := context.Background()
	grantID := rewardKey(90)
	start := time.Now().UTC().Truncate(time.Second)
	if err := s.ReserveAlertTime(ctx, AlertTimeGrantRequest{
		ID: grantID, AccountID: manager, RequestKey: rewardKey(91), PriceVersion: "price-v1",
		UnitSeconds: 1, UnitPriceMinor: 1, ReservedSeconds: 5, ExpiresAt: start.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	usage, err := s.AlertUsage(ctx, manager)
	if err != nil || usage.AlertReservedMinor != 5 || usage.AlertSettledMinor != 0 || usage.AlertReleasedMinor != 0 || usage.AlertRefundedMinor != 0 {
		t.Fatalf("after reserve: %+v, %v", usage, err)
	}
	if err := s.ReserveAlertInterval(ctx, AlertTimeIntervalRequest{GrantID: grantID, IntervalID: "usage-interval", RequestKey: rewardKey(92), StartedAt: start, EndedAt: start.Add(2 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SettleAlertTime(ctx, grantID, "usage-interval", rewardKey(93)); err != nil {
		t.Fatal(err)
	}
	usage, err = s.AlertUsage(ctx, manager)
	if err != nil || usage.AlertReservedMinor != 3 || usage.AlertSettledMinor != 2 {
		t.Fatalf("after settle: %+v, %v", usage, err)
	}
	page, err := s.AlertConsumptions(ctx, manager, 0, "settled", nil, nil, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].PriceVersion != "price-v1" || page.Items[0].CoinsMinor != 2 {
		t.Fatalf("settled page: %+v, %v", page, err)
	}
}

func TestMergeAlertConsumptionsCombinesContinuousFrozenPrice(t *testing.T) {
	start := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	items := []AlertConsumption{
		{ID: 2, StartedAt: start.Add(10 * time.Second), EndedAt: start.Add(20 * time.Second), DurationSeconds: 10, CoinsMinor: 3, State: "settled", UnitSeconds: 3600, UnitPriceMinor: 100, PriceVersion: "v1"},
		{ID: 1, StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 2, State: "settled", UnitSeconds: 3600, UnitPriceMinor: 100, PriceVersion: "v1"},
	}
	merged := mergeAlertConsumptions(items)
	if len(merged) != 1 || merged[0].ID != 2 || merged[0].DurationSeconds != 20 || merged[0].CoinsMinor != 5 || !merged[0].StartedAt.Equal(start) || !merged[0].EndedAt.Equal(start.Add(20*time.Second)) {
		t.Fatalf("merged consumption: %+v", merged)
	}
}

func TestMergeAlertConsumptionsGroupsInterleavedSystems(t *testing.T) {
	start := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	items := []AlertConsumption{
		{ID: 4, SystemID: "B", StartedAt: start.Add(10 * time.Second), EndedAt: start.Add(20 * time.Second), DurationSeconds: 10, CoinsMinor: 2, State: "settled", UnitSeconds: 3600, UnitPriceMinor: 100, PriceVersion: "v1"},
		{ID: 3, SystemID: "A", StartedAt: start.Add(10 * time.Second), EndedAt: start.Add(20 * time.Second), DurationSeconds: 10, CoinsMinor: 2, State: "settled", UnitSeconds: 3600, UnitPriceMinor: 100, PriceVersion: "v1"},
		{ID: 2, SystemID: "B", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 1, State: "settled", UnitSeconds: 3600, UnitPriceMinor: 100, PriceVersion: "v1"},
		{ID: 1, SystemID: "A", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 1, State: "settled", UnitSeconds: 3600, UnitPriceMinor: 100, PriceVersion: "v1"},
	}
	merged := mergeAlertConsumptions(items)
	if len(merged) != 2 || merged[0].DurationSeconds != 20 || merged[1].DurationSeconds != 20 {
		t.Fatalf("merged interleaved systems: %+v", merged)
	}
}

func TestAlertUsageIncludesBatchedMonitorRewards(t *testing.T) {
	s, _, _ := rewardFixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES
		($1,'source',$2,$3,2,'监控时长奖励'),
		($1,'source',$4,$5,7,'监控时长奖励')`, manager,
		"sentry-monitor:legacy", rewardKey(110),
		"sentry-monitor-batch:batch-1", rewardKey(111)); err != nil {
		t.Fatal(err)
	}
	usage, err := s.AlertUsage(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	if usage.MonitorRewardMinor != 9 {
		t.Fatalf("monitor reward total: got %d, want 9", usage.MonitorRewardMinor)
	}
}

func TestAlertTimeConsumptionDisabledByDefault(t *testing.T) {
	s, _, _ := rewardFixture(t)
	err := s.ReserveAlertTime(context.Background(), AlertTimeGrantRequest{ID: rewardKey(80), AccountID: manager, RequestKey: rewardKey(81), UnitSeconds: 1, UnitPriceMinor: 1, ReservedSeconds: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if !errors.Is(err, ErrAlertConsumptionDisabled) {
		t.Fatalf("expected disabled error, got %v", err)
	}
}
