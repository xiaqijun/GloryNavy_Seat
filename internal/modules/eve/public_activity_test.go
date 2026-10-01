package eve

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

func TestPublicCombatMonths(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	data, err := decodePublicCombat([]byte(`{"id":98530802,"type":"corporationID","months":{"202609":{"year":2026,"month":9,"shipsDestroyed":580,"iskDestroyed":123469219545},"202608":{"year":2026,"month":8,"shipsLost":2}}}`), now)
	if err != nil || len(data.Months) != 6 || data.Months[0].Month != "2026-04" || data.Months[0].Kills != nil || *data.Months[4].Kills != 0 || *data.Months[5].Kills != 580 {
		t.Fatal(data, err)
	}
	for _, raw := range []string{`{"error":"upstream error"}`, `{"id":1,"type":"corporationID","months":{}}`, `{"id":98530802,"type":"corporationID","months":{"202609":{"year":2026,"month":9,"shipsDestroyed":-1}}}`} {
		if _, err := decodePublicCombat([]byte(raw), now); err == nil {
			t.Fatal("accepted invalid stats", raw)
		}
	}
}

func TestPublicOnlineCountsCharactersAndRejectsInvalidSamples(t *testing.T) {
	now := time.Now().UTC()
	var bindings []PublicOnlineBinding
	var rows []store.PublicOnlineLatestRow
	for i := int64(1); i <= 9; i++ {
		bindings = append(bindings, PublicOnlineBinding{ID: i, OwnerHash: []byte("owner")})
		rows = append(rows, store.PublicOnlineLatestRow{CharacterID: i, OwnerHash: []byte("owner"), State: "ready", Scopes: []string{OnlineReadScope}, GrantGeneration: 2, Generation: 2, Online: pgtype.Bool{Bool: true, Valid: true}, ObservedAt: timestamp(now.Add(-time.Minute))})
	}
	rows[2].Online.Bool = false
	rows[3].ObservedAt = timestamp(now.Add(-301 * time.Second))
	rows[4].Generation = 1
	rows[5].Scopes = nil
	rows[6].State = "reauthorize"
	rows[7].OwnerHash = []byte("changed")
	rows[8].ObservedAt = timestamp(now.Add(time.Minute))
	rows = append(rows, rows[0]) // Defensive deduplication of the same character, never by account.
	got := summarizePublicOnline(bindings, rows, now)
	if got.Characters == nil || *got.Characters != 2 || got.Covered != 3 || got.Bound != 8 {
		t.Fatal(got)
	}
	if !got.ExpiresAt.Equal(now.Add(240 * time.Second)) {
		t.Fatal("freshness extended", got.ExpiresAt)
	}
	rows[3].ObservedAt = timestamp(now.Add(-210 * time.Second))
	got = summarizePublicOnline(bindings, rows, now)
	if got.Characters == nil || *got.Characters != 3 || got.Covered != 4 || !got.ExpiresAt.Equal(now.Add(90*time.Second)) {
		t.Fatal("valid delayed sample discarded", got)
	}
	got = summarizePublicOnline(bindings, rows, now.Add(7*time.Minute))
	if got.Characters != nil || got.Covered != 0 {
		t.Fatal("stale samples turned into zero/online", got)
	}
	if got = summarizePublicOnline(nil, rows, now); got.Bound != 0 || got.Characters != nil {
		t.Fatal("unbound characters published", got)
	}
}

type publicRoundTripper func(*http.Request) (*http.Response, error)

func (f publicRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPublicActivityIsolationAndIndependentFailure(t *testing.T) {
	s := NewPublicActivity()
	calls := 0
	fail := false
	s.client = &http.Client{Transport: publicRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != publicCombatURL || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") == "" {
			t.Error("unsafe upstream request")
		}
		if fail {
			return nil, errors.New("private upstream detail")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":98530802,"type":"corporationID","months":{},"info":{"ceoID":123}}`))}, nil
	})}
	onlineCalls := 0
	s.Online = func(context.Context) (*PublicOnline, error) {
		onlineCalls++
		if fail {
			return nil, errors.New("private database detail")
		}
		n := 2
		return &PublicOnline{Characters: &n, Covered: 3, Bound: 4, UpdatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}, nil
	}
	get := func() string {
		r := httptest.NewRecorder()
		s.ServeHTTP(r, httptest.NewRequest("GET", "/public/activity?corporation_id=1&character_id=99", nil))
		if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(r.Code)
		}
		return r.Body.String()
	}
	for range 2 {
		body := get()
		if strings.Contains(body, "ceoID") || strings.Contains(body, "character_id") || !strings.Contains(body, `"characters":2`) {
			t.Fatal(body)
		}
	}
	if calls != 1 || onlineCalls != 1 {
		t.Fatal("cache not shared", calls, onlineCalls)
	}
	s.online.data.ExpiresAt = time.Now().Add(-time.Second)
	if body := get(); !strings.Contains(body, `"characters":2`) || onlineCalls != 2 || calls != 1 {
		t.Fatal("expired sample not refreshed independently", body, onlineCalls, calls)
	}
	s.combat.next = time.Time{}
	s.online.next = time.Time{}
	fail = true
	body := get()
	if !strings.Contains(body, `"stale":true`) || !strings.Contains(body, `"online":null`) || strings.Contains(body, "private") {
		t.Fatal(body)
	}
	get()
	if calls != 2 || onlineCalls != 3 {
		t.Fatal("failure backoff missing")
	}
}
