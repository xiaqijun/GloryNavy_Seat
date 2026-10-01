package eve

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSkillsIndependentQueueCacheAndGeneration(t *testing.T) {
	s, ch := syncFixture(t)
	s.SetSkillsEnabled(true)
	ctx := context.Background()
	ch.authorization.Scopes = []string{CorporationRolesScope, SkillsReadScope, SkillQueueReadScope}
	if e := s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	if e := s.dispatch(ctx); e != nil {
		t.Fatal(e)
	}
	q := store.New(s.pool)
	rows, e := q.ListCharacterSync(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	var target store.EveSyncTarget
	for _, r := range rows {
		if r.Resource == "fittings" {
			t.Fatal("disabled fitting target created")
		}
		if r.Resource == "skillqueue" {
			target = r
		}
	}
	if target.ID == 0 || !target.ActiveJobID.Valid {
		t.Fatal("queue not scheduled")
	}
	calls := 0
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/skillqueue/") {
			t.Fatal(r.URL.Path)
		}
		res := jsonResponse([]SkillQueueEntry{{ID: 3300, Position: 0, Level: 4}})
		res.Header.Set("Expires", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat))
		return res, nil
	})
	if e = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "skillqueue"); e != nil {
		t.Fatal(e)
	}
	snap, e := s.auth.SkillSnapshot(ctx, ch.ID, "skillqueue")
	if e != nil || snap.Status != "ready" || len(snap.Payload) == 0 {
		t.Fatal(snap, e)
	}
	c, e := q.GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.collectFittingResource(ctx, target, c); e != nil || calls != 1 {
		t.Fatal("cache not reused", e, calls)
	}
	if e = s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	observed := *snap.ObservedAt
	snap, e = s.auth.SkillSnapshot(ctx, ch.ID, "skillqueue")
	if e != nil || len(snap.Payload) == 0 || snap.Status != "stale" || !snap.ObservedAt.Equal(observed) || snap.ValidUntil != nil {
		t.Fatal("same-owner grant must retain the observation without freshening it", snap, e)
	}
	// A delayed job from before reauthorization must not replace retained data.
	if e = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "skillqueue"); e != nil || calls != 1 {
		t.Fatal("old generation job was not fenced", e, calls)
	}
	if e = s.dispatch(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e = q.ListCharacterSync(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		if r.Resource == "skillqueue" {
			target = r
		}
	}
	// Respect the shared client's 200 ms pacing between real requests.
	time.Sleep(250 * time.Millisecond)
	if e = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "skillqueue"); e != nil {
		t.Fatal(e)
	}
	snap, e = s.auth.SkillSnapshot(ctx, ch.ID, "skillqueue")
	if e != nil || snap.Status != "ready" || calls != 2 {
		t.Fatal("new generation did not refresh the retained observation", snap, e, calls)
	}
	s.SetSkillsEnabled(false)
	s.SetFittingsEnabled(true)
	if !s.resourceEnabled("skills") || s.resourceEnabled("skillqueue") {
		t.Fatal("independent switches")
	}
}

func TestSkillSnapshotRetentionBoundaries(t *testing.T) {
	s, template := syncFixture(t)
	s.SetSkillsEnabled(true)
	ctx := context.Background()
	q := store.New(s.pool)
	for i, scenario := range []string{"same owner", "owner changed", "scope removed and restored", "revoked", "unlinked", "older generation"} {
		t.Run(scenario, func(t *testing.T) {
			ch := template
			ch.ID = int64(1000 + i)
			a := *template.authorization
			a.Scopes = []string{CorporationRolesScope, SkillsReadScope, SkillQueueReadScope}
			ch.authorization = &a
			if err := s.auth.Save(ctx, ch); err != nil {
				t.Fatal(err)
			}
			credential, err := q.GetCredential(ctx, ch.ID)
			if err != nil {
				t.Fatal(err)
			}
			observed := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
			for _, resource := range []string{"skills", "skillqueue", "fittings"} {
				if err := q.SaveFittingSnapshot(ctx, store.SaveFittingSnapshotParams{CharacterID: ch.ID, Resource: resource, Generation: credential.GrantGeneration, ObservedAt: timestamp(observed), Payload: []byte(`[]`)}); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "owner changed":
				ch.Owner = "another-owner"
			case "scope removed and restored":
				ch.authorization.Scopes = []string{CorporationRolesScope, SkillQueueReadScope}
				if err := s.auth.Save(ctx, ch); err != nil {
					t.Fatal(err)
				}
				blocked, err := s.auth.SkillSnapshot(ctx, ch.ID, "skills")
				if err != nil || len(blocked.Payload) != 0 {
					t.Fatal("removed scope readable", err)
				}
				ch.authorization.Scopes = []string{CorporationRolesScope, SkillsReadScope, SkillQueueReadScope}
			case "revoked":
				if err := s.auth.Revoke(ctx, ch.ID); err != nil {
					t.Fatal(err)
				}
			case "unlinked":
				if err := q.DeleteCredential(ctx, ch.ID); err != nil {
					t.Fatal(err)
				}
			case "older generation":
				if _, err := q.AdvanceGeneration(ctx, ch.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.auth.Save(ctx, ch); err != nil {
				t.Fatal(err)
			}
			for _, resource := range []string{"skills", "skillqueue"} {
				snap, err := s.auth.SkillSnapshot(ctx, ch.ID, resource)
				if err != nil {
					t.Fatal(err)
				}
				retained := scenario == "same owner" || scenario == "scope removed and restored" && resource == "skillqueue"
				if retained {
					if len(snap.Payload) == 0 || snap.Status != "stale" || !snap.ObservedAt.Equal(observed) {
						t.Fatal("continuous scope not retained", resource, snap)
					}
				} else if len(snap.Payload) != 0 {
					t.Fatal("invalid continuity exposed old snapshot", resource)
				}
			}
			// Fitting observations are outside this change, even for the same owner.
			var promoted int
			if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM eve_fitting_snapshots s JOIN eve_credentials c USING(character_id) WHERE s.character_id=$1 AND s.resource='fittings' AND s.generation=c.grant_generation`, ch.ID).Scan(&promoted); err != nil || promoted != 0 {
				t.Fatal("fittings unexpectedly retained", err)
			}
		})
	}
}
func TestSkillsQueueValidation(t *testing.T) {
	for _, raw := range []string{`null`, `[{"skill_id":3300,"queue_position":0,"finished_level":6}]`, `[{"skill_id":3300,"queue_position":0,"finished_level":4},{"skill_id":3301,"queue_position":0,"finished_level":1}]`, `[{"skill_id":3300,"queue_position":0,"finished_level":1,"start_date":"2026-09-15T02:00:00Z","finish_date":"2026-09-15T01:00:00Z"}]`} {
		if validSkillQueue([]byte(raw)) {
			t.Fatal(raw)
		}
	}
	if !validSkillQueue([]byte(`[]`)) || !validSkillQueue([]byte(`[{"skill_id":3300,"queue_position":0,"finished_level":1}]`)) {
		t.Fatal("empty and paused queues are valid")
	}
}
