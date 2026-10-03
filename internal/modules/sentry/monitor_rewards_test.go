package sentry

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

type monitorRewardFundingFake struct {
	calls []struct {
		account string
		amount  int64
	}
}

func (f *monitorRewardFundingFake) CreditMonitorRewardTx(_ context.Context, _ pgx.Tx, account, _ string, _ string, amount int64) error {
	f.calls = append(f.calls, struct {
		account string
		amount  int64
	}{account: account, amount: amount})
	return nil
}

func TestMonitorRewardsBatchWalletCreditResolvesRemoteKey(t *testing.T) {
	f := newPricingFixture(t)
	ctx := context.Background()
	if _, err := f.service.EditTimePricing(ctx, f.admin, TimePricingEdit{
		AlertHourlyPriceMinor:    100,
		MonitorHourlyRewardMinor: 1800,
		Version:                  0,
	}); err != nil {
		t.Fatal(err)
	}
	localKey := "11111111-1111-4111-8111-111111111111"
	remoteKey := "22222222-2222-4222-8222-222222222222"
	operation := "33333333-3333-4333-8333-333333333333"
	if _, err := f.pool.Exec(ctx, `INSERT INTO sentry_keys(id,account_id,operation_id,remote_key_id,name,key_prefix,key_hash,permissions,status) VALUES($1,$2,$3,$4,'monitor','eve_',decode(repeat('00',32),'hex'),ARRAY['monitor'],'active')`, localKey, f.member, operation, remoteKey); err != nil {
		t.Fatal(err)
	}
	funding := &monitorRewardFundingFake{}
	f.service.MonitorRewardFunding = funding
	contributions := []MonitorContribution{
		{ContributionID: "contribution-1", AccountID: f.member, KeyID: remoteKey, ClientID: "client-1", SystemName: "Jita", PrimaryGeneration: 1, StartedAt: "2026-10-03T00:00:00Z", EndedAt: "2026-10-03T00:00:02Z", DurationSeconds: 2, Eligibility: "eligible"},
		{ContributionID: "contribution-2", AccountID: f.member, KeyID: remoteKey, ClientID: "client-1", SystemName: "Jita", PrimaryGeneration: 1, StartedAt: "2026-10-03T00:00:02Z", EndedAt: "2026-10-03T00:00:04Z", DurationSeconds: 2, Eligibility: "eligible"},
	}
	if err := f.service.settleMonitorContributions(ctx, contributions, "", "cursor-2"); err != nil {
		t.Fatal(err)
	}
	if len(funding.calls) != 1 || funding.calls[0].account != f.member || funding.calls[0].amount != 2 {
		t.Fatalf("funding calls = %#v, want one aggregated 2-minor credit", funding.calls)
	}
	var rewarded int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM sentry_monitor_rewards WHERE account_id=$1 AND state='rewarded'`, f.member).Scan(&rewarded); err != nil {
		t.Fatal(err)
	}
	if rewarded != 2 {
		t.Fatalf("rewarded rows = %d, want 2 evidence rows", rewarded)
	}
	_ = localKey
}
