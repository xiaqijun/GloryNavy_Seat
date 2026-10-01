package attendance

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

type PAPCoinQuote struct {
	Token      string `json:"token"`
	Mode       string `json:"mode"`
	Points     int64  `json:"points"`
	Converted  int64  `json:"converted"`
	Pending    int64  `json:"pending"`
	CoinsMinor int64  `json:"coins_minor"`
	Characters int    `json:"characters"`
	UnitScale  int64  `json:"unit_scale"`
}
type PAPConversion struct {
	Version    int64  `json:"version,string"`
	RequestKey string `json:"request_key"`
	Token      string `json:"token"`
	Reason     string `json:"reason"`
	Month      string `json:"month,omitempty"`
}

func (s *Service) ConvertPAP(ctx context.Context, user string, id int64, c *PAPConversion) (PAPCoinQuote, error) {
	var out PAPCoinQuote
	if s.Administrator == nil || s.CoinConversion == nil {
		return out, pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, pgx.ErrNoRows
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	e, err := q.LockEvent(ctx, id)
	if err != nil {
		return out, err
	}
	if err = s.require(ctx, user, e.CorporationID); err != nil {
		return out, err
	}
	if e.State != "closed" || !e.PapIssued {
		return out, ErrConflict
	}
	key, reason, token := "", "", ""
	if c != nil {
		if c.Version != e.Version {
			return out, ErrConflict
		}
		if _, err = uuid(c.RequestKey); err != nil {
			return out, err
		}
		key, reason, token = c.RequestKey, c.Reason, c.Token
	}
	rows, err := q.PAPAwards(ctx, id)
	if err != nil {
		return out, err
	}
	awards := []PAPCoinAward{}
	for _, r := range rows {
		awards = append(awards, PAPCoinAward{Reference: strconv.FormatInt(id, 10) + "/" + strconv.FormatInt(r.CharacterID, 10), AccountID: r.AccountID.String(), Previous: int64(r.Points), Units: int64(r.Points)})
	}
	out, err = s.CoinConversion(ctx, tx, user, key, reason, token, awards)
	if err != nil {
		return out, err
	}
	if c != nil {
		err = tx.Commit(ctx)
	}
	return out, err
}

func (h Handler) conversion(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var c *PAPConversion
	if r.Method == "POST" {
		c = new(PAPConversion)
		if !readBody(w, r, c) {
			return
		}
	}
	out, err := h.Service.ConvertPAP(r.Context(), h.User(r), id, c)
	if c != nil {
		respond(w, r, map[string]bool{"saved": err == nil}, err)
	} else {
		respond(w, r, out, err)
	}
}
