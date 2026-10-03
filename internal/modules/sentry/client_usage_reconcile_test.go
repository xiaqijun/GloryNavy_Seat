package sentry

import "testing"

func TestMergeContinuousClientUsage(t *testing.T) {
	usages := []ClientUsage{
		{UsageID: "u-1", AccountID: "a", KeyID: "k", ClientID: "c", StartedAt: "2026-10-03T00:00:00Z", EndedAt: "2026-10-03T00:00:10Z", DurationSeconds: 10},
		{UsageID: "u-2", AccountID: "a", KeyID: "k", ClientID: "c", StartedAt: "2026-10-03T00:00:10Z", EndedAt: "2026-10-03T00:00:25Z", DurationSeconds: 15},
		{UsageID: "u-3", AccountID: "a", KeyID: "k", ClientID: "c", StartedAt: "2026-10-03T00:00:30Z", EndedAt: "2026-10-03T00:00:40Z", DurationSeconds: 10},
		{UsageID: "u-4", AccountID: "a", KeyID: "other", ClientID: "c", StartedAt: "2026-10-03T00:00:40Z", EndedAt: "2026-10-03T00:00:50Z", DurationSeconds: 10},
	}
	merged := mergeContinuousClientUsage(usages, 3600)
	if len(merged) != 3 {
		t.Fatalf("merged rows: got %d, want 3: %+v", len(merged), merged)
	}
	if merged[0].DurationSeconds != 25 || merged[0].StartedAt != usages[0].StartedAt || merged[0].EndedAt != usages[1].EndedAt {
		t.Fatalf("first merged interval: %+v", merged[0])
	}
	if merged[0].UsageID == usages[0].UsageID || merged[0].UsageID == usages[1].UsageID {
		t.Fatal("merged interval must use a deterministic batch id")
	}
}

func TestMergeContinuousClientUsageSplitsAtGrantLimit(t *testing.T) {
	usages := []ClientUsage{
		{UsageID: "u-1", AccountID: "a", KeyID: "k", ClientID: "c", StartedAt: "2026-10-03T00:00:00Z", EndedAt: "2026-10-03T00:00:10Z", DurationSeconds: 10},
		{UsageID: "u-2", AccountID: "a", KeyID: "k", ClientID: "c", StartedAt: "2026-10-03T00:00:10Z", EndedAt: "2026-10-03T00:00:20Z", DurationSeconds: 10},
	}
	merged := mergeContinuousClientUsage(usages, 15)
	if len(merged) != 2 {
		t.Fatalf("grant limit must split batches: %+v", merged)
	}
}
