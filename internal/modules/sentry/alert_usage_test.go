package sentry

import (
	"testing"
	"time"
)

func TestMergeMonitorRewardsKeepsContinuousTotal(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	items := []MonitorRewardRecord{
		{ContributionID: "u-3", ClientID: "client", SystemName: "S-KSWL", StartedAt: start.Add(20 * time.Second), EndedAt: start.Add(30 * time.Second), DurationSeconds: 10, CoinsMinor: 0},
		{ContributionID: "u-2", ClientID: "client", SystemName: "S-KSWL", StartedAt: start.Add(10 * time.Second), EndedAt: start.Add(20 * time.Second), DurationSeconds: 10, CoinsMinor: 2},
		{ContributionID: "u-1", ClientID: "client", SystemName: "S-KSWL", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 3},
	}
	merged := mergeMonitorRewards(items)
	if len(merged) != 1 || merged[0].DurationSeconds != 30 || merged[0].CoinsMinor != 5 || !merged[0].StartedAt.Equal(start) || !merged[0].EndedAt.Equal(start.Add(30*time.Second)) {
		t.Fatalf("merged rewards: %+v", merged)
	}
}
