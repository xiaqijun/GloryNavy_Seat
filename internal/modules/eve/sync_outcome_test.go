package eve

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"testing"
	"time"
)

func TestSyncWaitOutcomesRemainDeferredWithoutFailures(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	cases := []struct {
		reason   string
		err      error
		outcome  string
		failures int32
	}{
		{"rate_limited", retryError{Until: time.Now().Add(time.Minute)}, "deferred", 0},
		{"shared_source", syncFault{Reason: "shared_source", Temporary: true}, "deferred", 0},
		{"authorization_pending", syncFault{Reason: "authorization_pending", Temporary: true}, "deferred", 0},
		{"corporation_changed", syncFault{Reason: "corporation_changed", Temporary: true}, "deferred", 0},
		{"upstream_unavailable", syncFault{Reason: "upstream_unavailable", Status: 503, Temporary: true}, "failed", 1},
	}
	for _, tc := range cases {
		if err := s.auth.Save(ctx, ch); err != nil {
			t.Fatal(err)
		}
		target := authorizationTarget(t, s)
		q := store.New(s.pool)
		credential, err := q.GetCredential(ctx, ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		claim, err := q.ClaimSync(ctx, store.ClaimSyncParams{ID: target.ID, Generation: target.Generation, ActiveJobID: target.ActiveJobID})
		if err != nil {
			t.Fatal(err)
		}
		if err = q.BeginSyncRun(ctx, store.BeginSyncRunParams{TargetID: target.ID, JobID: target.ActiveJobID.Int64, Fence: claim.Fence}); err != nil {
			t.Fatal(err)
		}
		_ = s.finish(ctx, claim, credential, target.ActiveJobID.Int64, syncResult{next: time.Now().Add(time.Minute)}, tc.err)
		rows, err := q.ListSyncRuns(ctx, target.ID)
		if err != nil || len(rows) == 0 {
			t.Fatal(err)
		}
		current, err := q.GetSyncTarget(ctx, target.ID)
		if err != nil {
			t.Fatal(err)
		}
		if rows[0].Outcome != tc.outcome || rows[0].Reason != tc.reason || current.State != "deferred" || current.Failures != tc.failures || current.LastSuccessAt.Valid {
			t.Fatalf("%s: outcome=%s state=%s failures=%d", tc.reason, rows[0].Outcome, current.State, current.Failures)
		}
	}
}
