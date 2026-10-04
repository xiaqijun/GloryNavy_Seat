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

func TestMergeMonitorRewardsBridgesClientAndLegacySystemBoundaries(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	items := []MonitorRewardRecord{
		{ContributionID: "new", ClientID: "client-2", SystemID: "S-KSWL", SystemName: "S-KSWL", StartedAt: start.Add(10 * time.Second), EndedAt: start.Add(20 * time.Second), DurationSeconds: 10, CoinsMinor: 2},
		{ContributionID: "old", ClientID: "client-1", SystemID: "legacy:s-kswl", SystemName: "S-KSWL", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 1},
	}
	merged := mergeMonitorRewards(items)
	if len(merged) != 1 || merged[0].DurationSeconds != 20 || merged[0].CoinsMinor != 3 {
		t.Fatalf("merged client/system boundary: %+v", merged)
	}
}

func TestMergeMonitorRewardsGroupsInterleavedSystems(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	items := []MonitorRewardRecord{
		{ContributionID: "b2", SystemID: "B", StartedAt: start.Add(10 * time.Second), EndedAt: start.Add(20 * time.Second), DurationSeconds: 10, CoinsMinor: 2},
		{ContributionID: "a2", SystemID: "A", StartedAt: start.Add(10 * time.Second), EndedAt: start.Add(20 * time.Second), DurationSeconds: 10, CoinsMinor: 2},
		{ContributionID: "b1", SystemID: "legacy:b", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 1},
		{ContributionID: "a1", SystemID: "A", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 1},
	}
	merged := mergeMonitorRewards(items)
	if len(merged) != 2 || merged[0].DurationSeconds != 20 || merged[1].DurationSeconds != 20 {
		t.Fatalf("merged interleaved systems: %+v", merged)
	}
}

func TestMergeMonitorRewardsBridgesShortSamplingGap(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	items := []MonitorRewardRecord{
		{ContributionID: "new", ClientID: "ry-client", SystemID: "R-YWID", StartedAt: start.Add(13 * time.Second), EndedAt: start.Add(15 * time.Second), DurationSeconds: 2, CoinsMinor: 1},
		{ContributionID: "old", ClientID: "ry-client", SystemID: "legacy:r-ywid", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 2},
	}
	merged := mergeMonitorRewards(items)
	if len(merged) != 1 || merged[0].DurationSeconds != 12 || merged[0].CoinsMinor != 3 {
		t.Fatalf("merged sampling gap: %+v", merged)
	}
}

func TestMergeMonitorRewardsKeepsLongGapSeparate(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	items := []MonitorRewardRecord{
		{ContributionID: "new", ClientID: "ry-client", SystemID: "R-YWID", StartedAt: start.Add(6*time.Minute + 1*time.Second), EndedAt: start.Add(6*time.Minute + 3*time.Second), DurationSeconds: 2, CoinsMinor: 1},
		{ContributionID: "old", ClientID: "ry-client", SystemID: "R-YWID", StartedAt: start, EndedAt: start.Add(10 * time.Second), DurationSeconds: 10, CoinsMinor: 2},
	}
	merged := mergeMonitorRewards(items)
	if len(merged) != 2 {
		t.Fatalf("long sampling gap must stay separate: %+v", merged)
	}
}
