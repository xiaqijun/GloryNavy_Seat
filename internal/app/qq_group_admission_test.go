package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
)

func TestQQGroupAdmissionOptionsRequireCurrentConfiguredCorporation(t *testing.T) {
	f := newCleanupFixture(t)
	member, _ := f.sign(t, 101)
	outsider, outsiderSession := f.sign(t, 102)
	f.facts(t, 101, 10)
	f.facts(t, 102, 20)
	f.due(t)
	if err := f.acl.SetAdministrator(t.Context(), outsiderSession.UserID, true); err != nil {
		t.Fatal(err)
	}
	a, err := New(f.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test",
		[]string{"system", "identity", "eve", "access", "community"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	status := func(token string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/community/qq/group/options", nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w.Code
	}
	for _, tc := range []struct {
		name  string
		token string
		code  int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"configured corporation member", member, http.StatusOK},
		{"external administrator", outsider, http.StatusForbidden},
	} {
		if got := status(tc.token); got != tc.code {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.code)
		}
	}
	f.exec(t, "UPDATE eve_role_snapshots SET valid_until=now()-interval '1 second' WHERE character_id=101")
	f.exec(t, "UPDATE eve_character_profiles SET valid_until=now()-interval '1 second' WHERE character_id=101")
	if got := status(member); got != http.StatusForbidden {
		t.Errorf("expired corporation evidence: got %d, want 403", got)
	}
}
