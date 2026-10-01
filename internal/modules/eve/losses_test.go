package eve

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestKillmailReferenceValidation(t *testing.T) {
	for _, r := range []killRef{{1, strings.Repeat("a", 40)}} {
		if !validKillRef(r) {
			t.Fatal(r)
		}
	}
	for _, r := range []killRef{{0, strings.Repeat("a", 40)}, {1, strings.Repeat("a", 39) + "/"}, {1, "https://example.com"}} {
		if validKillRef(r) {
			t.Fatal("unsafe ref")
		}
	}
}
func TestLossSyncProgressReplayFenceAndRetention(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	s.SetLossesEnabled(true)
	ch.authorization.Scopes = append(ch.authorization.Scopes, LossReadScope)
	if e := s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	if e := s.dispatch(ctx); e != nil {
		t.Fatal(e)
	}
	q := store.New(s.pool)
	targets, e := q.ListCharacterSync(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	var target store.EveSyncTarget
	for _, v := range targets {
		if v.Resource == "killmails" {
			target = v
		}
	}
	if target.ID == 0 || !target.ActiveJobID.Valid {
		t.Fatal("not queued")
	}
	cred, e := q.GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	listCalls, detailCalls := 0, 0
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		var data any
		if strings.Contains(r.URL.Path, "/recent/") {
			listCalls++
			data = []killRef{{101, strings.Repeat("a", 40)}, {102, strings.Repeat("b", 40)}}
		} else if strings.Contains(r.URL.Path, "/universe/names/") {
			data = []map[string]any{{"id": 9001, "name": "Attacker One", "category": "character"}, {"id": 1001, "name": "Test Corporation", "category": "corporation"}, {"id": 2001, "name": "Test Alliance", "category": "alliance"}}
		} else {
			detailCalls++
			id, victim := int64(101), int64(999)
			if strings.Contains(r.URL.Path, "/102/") {
				id = 102
				victim = ch.ID
			} else if strings.Contains(r.URL.Path, "/103/") {
				id = 103
				victim = ch.ID
			}
			data = map[string]any{"killmail_id": id, "killmail_time": "2026-09-01T01:00:00Z", "solar_system_id": 30000142, "victim": map[string]any{"character_id": victim, "corporation_id": 10, "ship_type_id": 17715, "damage_taken": 5000, "items": []map[string]any{{"item_type_id": 34, "flag": 5, "quantity_destroyed": 2, "quantity_dropped": 3}}}, "attackers": []map[string]any{{"character_id": 9001, "corporation_id": 1001, "alliance_id": 2001, "ship_type_id": 587, "weapon_type_id": 34, "damage_done": 5000, "final_blow": true}}}
		}
		r2 := jsonResponse(data)
		r2.Header.Set("X-Pages", "1")
		r2.Header.Set("Expires", time.Now().Add(5*time.Minute).UTC().Format(http.TimeFormat))
		return r2, nil
	})
	collect := func() syncResult {
		t.Helper()
		for i := 0; i < 10; i++ {
			v, e := s.collectLosses(ctx, target, cred)
			if e == nil {
				return v
			}
			var retry retryError
			if !errors.As(e, &retry) {
				t.Fatal(e)
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatal("budget never recovered")
		return syncResult{}
	}
	publish := func(v syncResult) {
		t.Helper()
		claim, e := q.ClaimSync(ctx, store.ClaimSyncParams{ID: target.ID, Generation: target.Generation, ActiveJobID: target.ActiveJobID})
		if e != nil {
			t.Fatal(e)
		}
		e = s.finish(ctx, claim, cred, target.ActiveJobID.Int64, v, nil)
		var snooze *river.JobSnoozeError
		if e != nil && !errors.As(e, &snooze) {
			t.Fatal(e)
		}
	}
	first := collect()
	if first.losses.Done || first.losses.Loss.CharacterID == ch.ID {
		t.Fatal("did not separate attack")
	}
	publish(first)
	rows, e := s.auth.CharacterLosses(ctx, ch.ID, 0, 0)
	if e != nil || len(rows) != 0 {
		t.Fatal("attack exposed as loss", e)
	}
	second := collect()
	if !second.losses.Done {
		t.Fatal("cursor failed to resume")
	}
	publish(second)
	rows, e = s.auth.CharacterLosses(ctx, ch.ID, 0, 0)
	if e != nil || len(rows) != 1 || rows[0].ID != 102 || rows[0].Items[0].Quantity != 5 || rows[0].DamageTaken != 5000 || len(rows[0].Attackers) != 1 || rows[0].Attackers[0].Name != "Attacker One" || rows[0].Attackers[0].CorporationName != "Test Corporation" || rows[0].Attackers[0].AllianceName != "Test Alliance" {
		t.Fatal(rows, e)
	}
	again := collect()
	if !again.losses.Done || again.losses.Ref != nil || listCalls != 1 || detailCalls != 2 {
		t.Fatal("cache/dedup failed", listCalls, detailCalls)
	}
	if _, e = s.pool.Exec(ctx, `UPDATE eve_character_killmails SET payload=payload-'attackers' WHERE character_id=$1 AND killmail_id=102`, ch.ID); e != nil {
		t.Fatal(e)
	}
	backfill := collect()
	if backfill.losses.Ref == nil || backfill.losses.Ref.ID != 102 || detailCalls != 3 {
		t.Fatal("old projected loss was not enriched", detailCalls)
	}
	publish(backfill)
	rows, e = s.auth.CharacterLosses(ctx, ch.ID, 0, 102)
	if e != nil || len(rows) != 1 || len(rows[0].Attackers) != 1 {
		t.Fatal("backfill did not restore participants", e)
	}
	// This loss is absent from the cached recent references but retains its own
	// verified hash. A completed pass must still fetch its detail.
	if _, e = s.pool.Exec(ctx, `INSERT INTO eve_character_killmails(character_id,killmail_id,owner_hash,killmail_hash,victim_id,occurred_at,observed_at,payload)
 SELECT character_id,103,owner_hash,$2,victim_id,occurred_at,observed_at,(payload-'attackers') || '{"id":"103"}'::jsonb
 FROM eve_character_killmails WHERE character_id=$1 AND killmail_id=102`, ch.ID, strings.Repeat("c", 40)); e != nil {
		t.Fatal(e)
	}
	archived := collect()
	if archived.losses.Ref == nil || archived.losses.Ref.ID != 103 || detailCalls != 4 {
		t.Fatal("archived loss was not fetched by saved hash", detailCalls)
	}
	publish(archived)
	rows, e = s.auth.CharacterLosses(ctx, ch.ID, 0, 103)
	if e != nil || len(rows) != 1 || len(rows[0].Attackers) != 1 {
		t.Fatal("archived participants not restored", e)
	}
	if _, e = s.pool.Exec(ctx, `DELETE FROM eve_character_killmails WHERE character_id=$1 AND killmail_id=103`, ch.ID); e != nil {
		t.Fatal(e)
	}
	// A rotated grant cannot publish old work, but same-owner recorded losses remain readable.
	if e = s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	if e = s.finish(ctx, target, cred, target.ActiveJobID.Int64, second, nil); e != nil {
		t.Fatal(e)
	}
	rows, e = s.auth.CharacterLosses(ctx, ch.ID, 0, 0)
	if e != nil || len(rows) != 1 {
		t.Fatal("same owner history disappeared", e)
	}
	ch.authorization.Scopes = []string{CorporationRolesScope}
	if e = s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	rows, e = s.auth.CharacterLosses(ctx, ch.ID, 0, 0)
	if e != nil || len(rows) != 0 {
		t.Fatal("removed scope still readable", e)
	}
	if _, e = store.LossCursor(ctx, s.pool, target.ID, cred.GrantGeneration); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("completed cursor retained", e)
	}
	ch.authorization.Scopes = append(ch.authorization.Scopes, LossReadScope)
	ch.Owner = "different owner"
	if e = s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	rows, e = s.auth.CharacterLosses(ctx, ch.ID, 0, 0)
	if e != nil || len(rows) != 0 {
		t.Fatal("new game owner inherited losses", e)
	}
	// Payload is a projected record, never the private discovery hash.
	b, _ := json.Marshal(second.losses.Loss)
	if strings.Contains(string(b), "killmail_hash") {
		t.Fatal("private hash leaked")
	}
}
