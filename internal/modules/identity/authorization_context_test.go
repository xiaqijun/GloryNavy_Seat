package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/testutil"
)

func TestAuthorizationBusinessContext(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	token, err := s.SignIn(context.Background(), 123, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	var authContext context.Context
	h := Handler{Service: s, Check: func(ctx context.Context, _ *Session, _ string, _ *http.Request) (bool, error) {
		authContext = ctx
		return true, nil
	}}
	for _, mode := range []string{"slow business", "parent deadline", "parent cancellation"} {
		t.Run(mode, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if Principal(r.Context()) == nil {
					t.Fatal("missing authenticated principal")
				}
				if deadline, ok := authContext.Deadline(); !ok || time.Until(deadline) > 3*time.Second {
					t.Fatal("authorization must remain bounded")
				}
				switch mode {
				case "slow business":
					<-authContext.Done()
					if _, err := pool.Exec(r.Context(), "SELECT 1"); err != nil {
						t.Fatalf("authentication deadline cancelled business: %v", err)
					}
				case "parent deadline":
					want, _ := parent.Deadline()
					got, ok := r.Context().Deadline()
					if !ok || !got.Equal(want) {
						t.Fatal("business must preserve the original request deadline")
					}
				case "parent cancellation":
					cancel()
					if !errors.Is(r.Context().Err(), context.Canceled) {
						t.Fatal("request cancellation must propagate")
					}
				}
				w.WriteHeader(http.StatusNoContent)
			})
			r := httptest.NewRequest("GET", "/", nil).WithContext(parent)
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
			w := httptest.NewRecorder()
			h.Authorize("welfare.self", next).ServeHTTP(w, r)
			if !called || w.Code != http.StatusNoContent {
				t.Fatalf("business not reached: %d", w.Code)
			}
		})
	}
}
