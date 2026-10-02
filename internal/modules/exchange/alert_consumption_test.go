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

func TestAlertTimeReservationSettlementReleaseAndRefund(t *testing.T) {
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
	if err := s.ReleaseAlertGrant(ctx, grantID, rewardKey(74)); err != nil {
		t.Fatal(err)
	}
	sh, err = s.Shop(ctx, manager, 0)
	if err != nil || sh.Reserved != 0 || sh.Spent != 2 {
		t.Fatalf("after release: %+v, %v", sh, err)
	}
	if err := s.RefundAlertTime(ctx, grantID, "interval-1", rewardKey(75)); err != nil {
		t.Fatal(err)
	}
	sh, err = s.Shop(ctx, manager, 0)
	if err != nil || sh.Spent != 0 || sh.Reserved != 0 {
		t.Fatalf("after refund: %+v, %v", sh, err)
	}
	if err := s.ReserveAlertInterval(ctx, AlertTimeIntervalRequest{GrantID: grantID, IntervalID: "interval-2", RequestKey: rewardKey(78), StartedAt: start, EndedAt: start.Add(2 * time.Second)}); err != nil {
		t.Fatalf("replacement interval after refund: %v", err)
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
	if err := s.ReleaseAlertGrant(ctx, grantID, rewardKey(94)); err != nil {
		t.Fatal(err)
	}
	usage, err = s.AlertUsage(ctx, manager)
	if err != nil || usage.AlertReservedMinor != 0 || usage.AlertReleasedMinor != 3 || usage.AlertSettledMinor != 2 {
		t.Fatalf("after release: %+v, %v", usage, err)
	}
	if err := s.RefundAlertTime(ctx, grantID, "usage-interval", rewardKey(95)); err != nil {
		t.Fatal(err)
	}
	usage, err = s.AlertUsage(ctx, manager)
	if err != nil || usage.AlertRefundedMinor != 2 || usage.AlertSettledMinor != 0 {
		t.Fatalf("after refund: %+v, %v", usage, err)
	}
}

func TestAlertTimeConsumptionDisabledByDefault(t *testing.T) {
	s, _, _ := rewardFixture(t)
	err := s.ReserveAlertTime(context.Background(), AlertTimeGrantRequest{ID: rewardKey(80), AccountID: manager, RequestKey: rewardKey(81), UnitSeconds: 1, UnitPriceMinor: 1, ReservedSeconds: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if !errors.Is(err, ErrAlertConsumptionDisabled) {
		t.Fatalf("expected disabled error, got %v", err)
	}
}
