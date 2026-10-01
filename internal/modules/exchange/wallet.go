package exchange

import (
	"context"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"math"
	"net/http"
	"strconv"
	"time"
)

// Award is a source-owned entitlement snapshot, not an unrestricted wallet credit.
// References are stable per source entitlement; Previous is the pre-change PAP balance.
type Award struct {
	Reference, AccountID string
	Previous, Units      int64
}

func (s *Service) ReconcileTx(ctx context.Context, tx pgx.Tx, source, key, reason string, awards []Award) error {
	plans, _, err := s.prepareCoins(ctx, tx, source, awards, false)
	if err != nil {
		return err
	}
	return saveCoins(ctx, tx, source, key, reason, plans)
}

type SourceRate struct {
	Mode         string `json:"mode"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	MinorPerUnit int64  `json:"minor_per_unit"`
	Version      int64  `json:"version,string"`
}
type SourceEdit struct {
	Mode         string `json:"mode"`
	ID           string `json:"id"`
	MinorPerUnit int64  `json:"minor_per_unit"`
	Version      int64  `json:"version,string"`
	RequestKey   string `json:"request_key"`
}

func (s *Service) EditSource(ctx context.Context, user string, c SourceEdit) error {
	if err := s.shopAdmin(ctx, user); err != nil {
		return err
	}
	if _, ok := s.Sources[c.ID]; !ok || c.MinorPerUnit < 1 || c.MinorPerUnit > 100000000 || c.Version < 1 {
		return ErrInvalid
	}
	actor, err := uuid(user)
	if err != nil {
		return err
	}
	key, err := uuid(c.RequestKey)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	row, err := q.LockSourceRate(ctx, c.ID)
	if err != nil {
		return err
	}
	fp := shopFingerprint("source", c)
	if c.Mode == "" {
		c.Mode = row.ConversionMode
	}
	if c.Mode != "manual" && c.Mode != "automatic" {
		return ErrInvalid
	}
	if done, e := shopReplay(ctx, q, actor, key, "source", fp); done || e != nil {
		return e
	}
	if row.Version != c.Version {
		return ErrConflict
	}
	if err = q.SetSourceRate(ctx, store.SetSourceRateParams{SourceID: c.ID, MinorPerUnit: c.MinorPerUnit, ConversionMode: c.Mode}); err != nil {
		return err
	}
	payload := []byte(`{"mode":"` + c.Mode + `","previous_mode":"` + row.ConversionMode + `","minor_per_unit":` + strconv.FormatInt(c.MinorPerUnit, 10) + `,"previous":` + strconv.FormatInt(row.MinorPerUnit, 10) + `}`)
	if err = q.ShopAudit(ctx, store.ShopAuditParams{ActorID: actor, RequestKey: key, Kind: "source", Fingerprint: fp, Payload: payload}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	chars, err := h.Service.Own(r.Context(), h.User(r))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	rows, err := store.New(h.Service.Pool).SourceRates(r.Context())
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	rates := []SourceRate{}
	for _, row := range rows {
		if name, ok := h.Service.Sources[row.SourceID]; ok {
			rates = append(rates, SourceRate{Mode: row.ConversionMode, ID: row.SourceID, Name: name, MinorPerUnit: row.MinorPerUnit, Version: row.Version})
		}
	}
	respond(w, r, map[string]any{"characters": chars, "sources": rates}, nil)
}
func (h Handler) editSource(w http.ResponseWriter, r *http.Request) {
	var c SourceEdit
	if !readBody(w, r, &c) {
		return
	}
	err := h.Service.EditSource(r.Context(), h.User(r), c)
	respond(w, r, map[string]bool{"saved": err == nil}, err)
}

type CoinEntry struct {
	ID        int64     `json:"id,string"`
	Kind      string    `json:"kind"`
	Reference string    `json:"reference"`
	Delta     int64     `json:"delta_minor"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (h Handler) wallet(w http.ResponseWriter, r *http.Request) {
	before, err := cursorQuery(r, "before")
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	if before == 0 {
		before = math.MaxInt64
	}
	subject, err := h.readSubject(r)
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	actor, err := uuid(subject)
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	rows, err := store.New(h.Service.Pool).CoinHistory(r.Context(), store.CoinHistoryParams{AccountID: actor, BeforeID: before})
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	next := ""
	if len(rows) > 30 {
		rows = rows[:30]
		next = strconv.FormatInt(rows[29].ID, 10)
	}
	items := []CoinEntry{}
	for _, row := range rows {
		items = append(items, CoinEntry{row.ID, row.Kind, row.Reference, row.Delta, row.Reason, row.CreatedAt.Time})
	}
	respond(w, r, map[string]any{"items": items, "next_cursor": next}, nil)
}

func (h Handler) readSubject(r *http.Request) (string, error) {
	actor := h.User(r)
	subject := r.URL.Query().Get("member")
	if subject == "" || subject == actor {
		return actor, nil
	}
	if _, err := uuid(subject); err != nil {
		return "", err
	}
	if err := h.Service.shopAdmin(r.Context(), actor); err != nil {
		return "", err
	}
	if h.Service.MemberExists == nil {
		return "", pgx.ErrNoRows
	}
	ok, err := h.Service.MemberExists(r.Context(), subject)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", pgx.ErrNoRows
	}
	return subject, nil
}
