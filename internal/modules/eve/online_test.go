package eve

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"net/http"
	"testing"
	"time"
)

func TestOnlineSamplingCacheScopeAndGeneration(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, OnlineReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	s.SetOnlineEnabled(true)
	if err := s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	target := contractTarget(t, s, ch.ID, "online")
	credential, err := store.New(s.pool).GetCredential(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	contractTransport(t, s, func(r *http.Request) *http.Response {
		calls++
		if r.URL.Path != "/characters/123/online/" {
			t.Error(r.URL.Path)
		}
		resp := jsonResponse(map[string]any{"online": true})
		resp.Header.Set("Cache-Control", "max-age=60")
		return resp
	})
	first, err := s.collectOnline(ctx, target, credential)
	if err != nil || first.online == nil || !first.online.value || time.Until(first.next) < 50*time.Second {
		t.Fatal(first, err)
	}
	cached, err := s.collectOnline(ctx, target, credential)
	if err != nil || cached.online != nil || calls != 1 {
		t.Fatal("cache hit invented new sample", cached, err, calls)
	}
	if err = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "online"); err != nil {
		t.Fatal(err)
	}
	var samples int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_online_samples").Scan(&samples); err != nil || samples != 0 {
		t.Fatal("cached observations resampled", samples, err)
	}
	ch.authorization.Scopes = []string{CorporationRolesScope}
	if err = s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err = s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	target = contractTarget(t, s, ch.ID, "online")
	if target.State != "blocked" || target.Reason != "missing_scope" {
		t.Fatal(target)
	}
	// A grant changed while collection was in flight cannot publish old samples.
	if err = s.finish(ctx, target, credential, target.ActiveJobID.Int64, first, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_online_samples").Scan(&samples); err != nil || samples != 0 {
		t.Fatal("generation fence", samples, err)
	}
}
func TestOnlineIntervalsRespectFailuresGapsAndGrantBoundaries(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	q := store.New(s.pool)
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, p := range []struct {
		sec   int
		state pgtype.Bool
		gen   int64
	}{{0, pgtype.Bool{Bool: true, Valid: true}, 2}, {60, pgtype.Bool{Bool: true, Valid: true}, 2}, {90, pgtype.Bool{}, 2}, {120, pgtype.Bool{Bool: true, Valid: true}, 2}, {180, pgtype.Bool{Bool: true, Valid: true}, 2}, {600, pgtype.Bool{Bool: true, Valid: true}, 2}, {660, pgtype.Bool{Bool: false, Valid: true}, 2}, {720, pgtype.Bool{Bool: true, Valid: true}, 2}, {780, pgtype.Bool{Bool: true, Valid: true}, 3}, {840, pgtype.Bool{Bool: true, Valid: true}, 3}, {1050, pgtype.Bool{Bool: true, Valid: true}, 3}, {1410, pgtype.Bool{Bool: true, Valid: true}, 3}} {
		if err := q.SaveOnlineSample(ctx, store.SaveOnlineSampleParams{CorporationID: 10, CharacterID: ch.ID, Generation: p.gen, ObservedAt: timestamp(start.Add(time.Duration(p.sec) * time.Second)), Online: p.state}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := s.auth.OnlineData(ctx, []int64{ch.ID}, start, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	seconds := 0.
	for _, p := range data.Spans {
		seconds += p.End.Sub(p.Start).Seconds()
	}
	if seconds != 390 || len(data.Spans) != 3 {
		t.Fatal("counted failure/gap/offline/grant boundary", seconds, data.Spans)
	}
	if len(data.Characters) != 1 || data.Characters[0].State != "missing_scope" {
		t.Fatal("scope freshness", data.Characters)
	}
	other, err := s.auth.OnlineData(ctx, []int64{ch.ID}, start, time.Now(), 20)
	if err != nil || len(other.Spans) != 0 || len(other.Counts) != 0 {
		t.Fatal("previous corporation data leaked", other, err)
	}
}

func TestFleetGatewayUsesAuthorizedRosterAndFencesGrant(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, FleetReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	contractTransport(t, s, func(r *http.Request) *http.Response {
		switch r.URL.Path {
		case "/characters/123/fleet/":
			return esiResponse(map[string]any{"fleet_id": 999})
		case "/fleets/999/members/":
			return esiResponse([]map[string]any{{"character_id": 123}, {"character_id": 124}})
		case "/characters/affiliation/":
			return esiResponse([]map[string]any{{"character_id": 123, "corporation_id": 10}, {"character_id": 124, "corporation_id": 20}})
		case "/universe/names/":
			return esiResponse([]map[string]any{{"id": 123, "name": "Pilot", "category": "character"}, {"id": 124, "name": "Guest", "category": "character"}})
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			return &http.Response{StatusCode: 500, Header: http.Header{}, Body: http.NoBody}
		}
	})
	fleet, err := s.auth.Fleet(ctx, ch.ID, httpapi.Hash(ch.Owner))
	if err != nil || len(fleet.Members) != 2 || fleet.Members[1].CorporationID != 20 || fleet.ObservedAt.IsZero() {
		t.Fatal(fleet, err)
	}
	if _, err = s.auth.Fleet(ctx, ch.ID, []byte("wrong-owner")); !errors.Is(err, ErrReauthorize) {
		t.Fatal("owner mismatch accepted", err)
	}
	if err = s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = s.auth.GuardFleet(ctx, tx, fleet); !errors.Is(err, ErrReauthorize) {
		t.Fatal("stale fleet grant accepted", err)
	}
}
