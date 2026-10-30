package approval

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"glorynavy.local/seat/internal/testutil"
)

func TestRefreshSourceRequiresCurrentSourceAccess(t *testing.T) {
	allowed := false
	called := 0
	s := &Service{Sources: []reviewqueue.Source{{ID: "welfare", Access: func(context.Context, string) (reviewqueue.Access, error) {
		return reviewqueue.Access{Allowed: allowed}, nil
	}}}, RefreshProjection: func(_ context.Context, source reviewqueue.Source) error {
		called++
		if source.ID != "welfare" {
			t.Fatal(source.ID)
		}
		return nil
	}}
	if err := s.RefreshSource(context.Background(), "member", "welfare"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unauthorized: %v", err)
	}
	allowed = true
	if err := s.RefreshSource(context.Background(), "member", "unknown"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown source: %v", err)
	}
	if err := s.RefreshSource(context.Background(), "member", "welfare"); err != nil || called != 1 {
		t.Fatalf("refresh: %d %v", called, err)
	}
	allowed = false
	if err := s.RefreshSource(context.Background(), "member", "welfare"); !errors.Is(err, pgx.ErrNoRows) || called != 1 {
		t.Fatalf("revoked access: %d %v", called, err)
	}
}

func TestManualRefreshFollowsConcurrentOldSnapshot(t *testing.T) {
	pool := testutil.Database(t)
	p := NewProjection(pool)
	ctx := context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	var version atomic.Int64
	version.Store(1)
	var calls atomic.Int64
	source := reviewqueue.Source{ID: "welfare", Snapshot: func(context.Context) ([]reviewqueue.Item, error) {
		v := version.Load()
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		state := "approved"
		if v == 2 {
			state = "rejected"
		}
		return []reviewqueue.Item{{Source: "welfare", ID: 152, Version: v, Account: "00000000-0000-4000-8000-000000000001", State: state, Time: time.Now()}}, nil
	}}
	oldResult, refreshResult := make(chan error, 1), make(chan error, 1)
	go func() { oldResult <- p.Reconcile(ctx, []reviewqueue.Source{source}) }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("old snapshot did not start")
	}
	version.Store(2)
	go func() { refreshResult <- p.Reconcile(ctx, []reviewqueue.Source{source}) }()
	close(release)
	if err := <-oldResult; err != nil {
		t.Fatal(err)
	}
	if err := <-refreshResult; err != nil {
		t.Fatal(err)
	}
	var gotVersion int64
	var state, bucket string
	if err := pool.QueryRow(ctx, `SELECT source_version,state,bucket FROM approval_items WHERE source='welfare' AND source_id=152`).Scan(&gotVersion, &state, &bucket); err != nil {
		t.Fatal(err)
	}
	if gotVersion != 2 || state != "rejected" || bucket != "history" {
		t.Fatalf("stale projection %d %s %s", gotVersion, state, bucket)
	}
}
