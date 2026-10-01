package eve

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFittingsSnapshotCacheGenerationAndEmptyReplacement(t *testing.T) {
	s, ch := syncFixture(t)
	s.SetFittingsEnabled(true)
	ctx := context.Background()
	ch.authorization.Scopes = []string{CorporationRolesScope, FittingsReadScope, SkillsReadScope}
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	q := store.New(s.pool)
	rows, err := q.ListCharacterSync(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	var target store.EveSyncTarget
	for _, r := range rows {
		if r.Resource == "fittings" {
			target = r
		}
	}
	if target.ID == 0 || !target.ActiveJobID.Valid {
		t.Fatal("fitting job not queued")
	}
	calls := 0
	empty := false
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/fittings/") {
			t.Fatalf("unexpected route: %s", r.URL.Path)
		}
		data := []SavedFitting{{ID: 1, Name: "Rifter", ShipTypeID: 587, Items: []SavedFittingItem{}}}
		if empty {
			data = []SavedFitting{}
		}
		resp := jsonResponse(data)
		resp.Header.Set("Expires", time.Now().Add(5*time.Minute).UTC().Format(http.TimeFormat))
		return resp, nil
	})
	if err = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "fittings"); err != nil {
		t.Fatal(err)
	}
	raw, at, err := s.auth.FittingSnapshot(ctx, ch.ID, "fittings")
	if err != nil || at == nil || !strings.Contains(string(raw), "Rifter") {
		t.Fatal(string(raw), at, err)
	}
	if err = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "fittings"); err != nil || calls != 1 {
		t.Fatal("replay called ESI", calls, err)
	}
	c, err := q.GetCredential(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.collectFittingResource(ctx, target, c)
	if err != nil || calls != 1 || result.next.Before(time.Now().Add(4*time.Minute)) {
		t.Fatal("official cache not reused", calls, err)
	}
	// An explicit empty ESI response must remove deleted game fits, never preserve ghosts.
	if err = q.DeletePrivateCache(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	empty = true
	if _, err = s.pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	result, err = s.collectFittingResource(ctx, target, c)
	if err != nil || string(result.fitting.payload) != "[]" {
		t.Fatal("empty ESI fit list", err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE eve_sync_targets SET lease_until=NULL,state='queued',active_job_id=$2 WHERE id=$1", target.ID, target.ActiveJobID.Int64); err != nil {
		t.Fatal(err)
	}
	claim, err := q.ClaimSync(ctx, store.ClaimSyncParams{ID: target.ID, Generation: target.Generation, ActiveJobID: target.ActiveJobID})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(ctx, claim, c, target.ActiveJobID.Int64, result, nil); err != nil {
		t.Fatal(err)
	}
	raw, _, err = s.auth.FittingSnapshot(ctx, ch.ID, "fittings")
	if err != nil || string(raw) != "[]" {
		t.Fatal("empty snapshot replacement", string(raw), err)
	}
	// Revocation/new authorization makes old snapshots unreadable immediately.
	if err = s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.auth.FittingSnapshot(ctx, ch.ID, "fittings"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("old generation readable", err)
	}
	result.fitting.payload = json.RawMessage(`[{"fitting_id":2,"name":"stale"}]`)
	if err = s.finish(ctx, claim, c, target.ActiveJobID.Int64, result, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.auth.FittingSnapshot(ctx, ch.ID, "fittings"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("old generation published", err)
	}
}
