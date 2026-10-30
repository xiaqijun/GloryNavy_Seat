package eve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
)

func TestCorporationMailUsesExplicitCorpAndZeroCost(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = []string{CorporationRolesScope, MailSendScope}
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	gen, ok, err := s.auth.MailGrant(ctx, ch.ID, httpapi.Hash(ch.Owner))
	if err != nil || !ok {
		t.Fatal(gen, ok, err)
	}
	calls := 0
	broken := false
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var p struct {
			ApprovedCost int `json:"approved_cost"`
			Recipients   []struct {
				ID   int64  `json:"recipient_id"`
				Type string `json:"recipient_type"`
			} `json:"recipients"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if p.ApprovedCost != 0 || len(p.Recipients) != 1 || p.Recipients[0].ID != 456 || p.Recipients[0].Type != "corporation" {
			t.Fatal(p)
		}
		if broken {
			return nil, errors.New("response lost")
		}
		res := jsonResponse(1234)
		res.StatusCode = 201
		return res, nil
	})
	clear := func() { _, _ = s.pool.Exec(ctx, `UPDATE eve_esi_limits SET next_request_at='epoch'`) }
	clear()
	r := s.auth.SendCorporationMail(ctx, ch.ID, httpapi.Hash(ch.Owner), gen, 456, "Fleet", "body")
	if r.State != "sent" || calls != 1 {
		t.Fatal(r, calls)
	}
	broken = true
	clear()
	r = s.auth.SendCorporationMail(ctx, ch.ID, httpapi.Hash(ch.Owner), gen, 456, "Fleet", "body")
	if r.State != "unknown" || calls != 2 {
		t.Fatal(r, calls)
	}
	if r = s.auth.SendCorporationMail(ctx, ch.ID, []byte("wrong"), gen, 456, "Fleet", "body"); r.Reason != "identity_changed" || calls != 2 {
		t.Fatal(r, calls)
	}
}
