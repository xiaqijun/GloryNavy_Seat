package eve

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/httpapi"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBattleFittingAssetsPagesScopeCacheAndShipMismatch(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, AssetsReadScope, ShipReadScope, LossReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	assetCalls := 0
	contractTransport(t, s, func(r *http.Request) *http.Response {
		if r.Header.Get("Authorization") == "" {
			t.Error("private data fetched without member token")
		}
		if strings.HasSuffix(r.URL.Path, "/ship/") {
			return esiResponse(map[string]any{"ship_item_id": 999, "ship_type_id": 587, "ship_name": "ship"})
		}
		if strings.HasSuffix(r.URL.Path, "/assets/") {
			assetCalls++
			var body any
			if r.URL.Query().Get("page") == "1" {
				body = []map[string]any{{"item_id": 1, "location_id": 999, "type_id": 34, "quantity": 2, "location_flag": "HiSlot0"}, {"item_id": 2, "location_id": 888, "type_id": 35, "quantity": 3, "location_flag": "Cargo"}}
			} else {
				body = []map[string]any{{"item_id": 3, "location_id": 999, "type_id": 36, "quantity": 4, "location_flag": "DroneBay"}}
			}
			v := esiResponse(body)
			v.Header.Set("X-Pages", "2")
			return v
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
		return nil
	})
	p, err := s.auth.BattleCredential(ctx, ch.ID, httpapi.Hash(ch.Owner), "fitting")
	if err != nil {
		t.Fatal(err)
	}
	fit, err := s.auth.BattleFitting(ctx, p, 587, time.Now())
	if err != nil || len(fit.Items) != 2 || fit.ShipItemID != 999 || fit.AssetsObservedAt.IsZero() {
		t.Fatal(fit, err)
	}
	if fit.Items[0].TypeID != 34 || fit.Items[1].TypeID != 36 {
		t.Fatal("unrelated assets leaked", fit.Items)
	}
	if _, err = s.auth.BattleFitting(ctx, p, 587, time.Now()); err != nil || assetCalls != 2 {
		t.Fatal("cache not reused", assetCalls, err)
	}
	if _, err = s.auth.BattleFitting(ctx, p, 588, time.Now()); err == nil {
		t.Fatal("changed ship accepted")
	}
	if _, err = s.auth.BattleFitting(ctx, p, 587, time.Now().Add(-4*time.Minute)); err == nil {
		t.Fatal("historical fit fabricated")
	}
	if _, err = s.auth.BattleCredential(ctx, ch.ID, []byte("wrong"), "fitting"); !errors.Is(err, ErrReauthorize) {
		t.Fatal(err)
	}
	if err = s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = s.auth.GuardBattle(ctx, tx, p); !errors.Is(err, ErrReauthorize) {
		t.Fatal("stale grant published", err)
	}
}
func TestBattleLossesFilterVictimWindowAndKeepDestroyedDropped(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, LossReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	hash := strings.Repeat("a", 40)
	contractTransport(t, s, func(r *http.Request) *http.Response {
		if strings.Contains(r.URL.Path, "/recent/") {
			v := esiResponse([]map[string]any{{"killmail_id": 1, "killmail_hash": hash}, {"killmail_id": 2, "killmail_hash": hash}, {"killmail_id": 3, "killmail_hash": hash}})
			v.Header.Set("X-Pages", "2")
			return v
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("public killmail inherited private token")
		}
		id, victim, at := 1, int64(123), now
		if strings.Contains(r.URL.Path, "/2/") {
			id = 2
			victim = 456
		}
		if strings.Contains(r.URL.Path, "/3/") {
			id = 3
			at = now.Add(-2 * time.Hour)
		}
		return esiResponse(map[string]any{"killmail_id": id, "killmail_time": at, "solar_system_id": 30000142, "victim": map[string]any{"character_id": victim, "ship_type_id": 587, "items": []map[string]any{{"item_type_id": 34, "flag": 27, "quantity_destroyed": 2, "quantity_dropped": 1, "items": []map[string]any{{"item_type_id": 35, "flag": 5, "quantity_dropped": 4}}}}}})
	})
	p, err := s.auth.BattleCredential(ctx, ch.ID, httpapi.Hash(ch.Owner), "losses")
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.auth.BattleLosses(ctx, p, 1, now.Add(-time.Hour), now.Add(time.Minute))
	if err != nil || len(page.Losses) != 1 || !page.More {
		t.Fatal(page, err)
	}
	if len(page.Losses[0].Items) != 2 || page.Losses[0].Items[0].Quantity != 3 || page.Losses[0].Items[1].Slot != "27/5" {
		t.Fatal(page.Losses)
	}
	reason, until, terminal := BattleFailure(retryError{Until: now.Add(time.Hour)})
	if reason != "rate_limited" || until.IsZero() || terminal {
		t.Fatal(reason, until, terminal)
	}
}
