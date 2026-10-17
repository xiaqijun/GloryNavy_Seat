package eve

import (
	"context"
	"encoding/json"
	"errors"
	"glorynavy.local/seat/internal/httpapi"
	"net/http"
	"testing"
)

func TestMailWriteScopeZeroCostNoCacheAndUncertain(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = []string{CorporationRolesScope, MailSendScope}
	if e := s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	generation, ready, err := s.auth.MailGrant(ctx, ch.ID, httpapi.Hash(ch.Owner))
	if err != nil || !ready {
		t.Fatal(generation, ready, err)
	}
	calls := 0
	broken := false
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload struct {
			ApprovedCost int `json:"approved_cost"`
			Recipients   []struct {
				ID   int64  `json:"recipient_id"`
				Type string `json:"recipient_type"`
			} `json:"recipients"`
		}
		if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
			t.Fatal(e)
		}
		if r.Method != "POST" || r.Header.Get("If-None-Match") != "" || payload.ApprovedCost != 0 || len(payload.Recipients) != 1 || payload.Recipients[0].ID != 202 || payload.Recipients[0].Type != "character" {
			t.Fatal("unsafe mail request", payload)
		}
		if broken {
			return nil, errors.New("response lost")
		}
		res := jsonResponse(999)
		res.StatusCode = 201
		return res, nil
	})
	send := func() MailWriteResult {
		s.pool.Exec(ctx, `UPDATE eve_esi_limits SET next_request_at=now()-interval '1 second'`)
		return s.auth.SendMail(ctx, ch.ID, httpapi.Hash(ch.Owner), generation, 202, "Subject", "Body")
	}
	for i := 0; i < 2; i++ {
		r := send()
		if r.State != "sent" || r.ID != 999 {
			t.Fatal(r)
		}
	}
	if calls != 2 {
		t.Fatal("POST cached")
	}
	broken = true
	if r := send(); r.State != "unknown" || calls != 3 {
		t.Fatal("unknown retried", r, calls)
	}
	if r := s.auth.SendMail(ctx, ch.ID, httpapi.Hash(ch.Owner), generation+1, 202, "Subject", "Body"); r.Reason != "identity_changed" || calls != 3 {
		t.Fatal(r, calls)
	}
	ch.authorization.Scopes = []string{CorporationRolesScope}
	if e := s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	generation, ready, err = s.auth.MailGrant(ctx, ch.ID, httpapi.Hash(ch.Owner))
	if err != nil || ready {
		t.Fatal(ready, err)
	}
	if r := send(); r.Reason != "missing_scope" || calls != 3 {
		t.Fatal(r, calls)
	}
}
