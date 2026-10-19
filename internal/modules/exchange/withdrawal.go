package exchange

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

// A missing/unreadable snapshot is not proof that delivery has not begun.
func withdrawable(d store.Delivery) bool {
	var evidence []json.RawMessage
	return !d.Started && d.Status == "waiting_contract" && json.Unmarshal(d.Evidence, &evidence) == nil && len(evidence) == 0
}

// StartSettlementTx is the host-injected preparation boundary for a merged
// contract. The batch and every order's start marker commit together.
func (s *Service) StartSettlementTx(ctx context.Context, tx pgx.Tx, actor, account string, id int64) error {
	return s.startDeliveryTx(ctx, tx, actor, account, id, 0)
}

func (s *Service) startDeliveryTx(ctx context.Context, tx pgx.Tx, actor, account string, id, version int64) error {
	if err := s.shopAdmin(ctx, actor); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.Redemption(ctx, id)
	if err != nil {
		return err
	}
	if account != "" && row.AccountID.String() != account {
		return ErrConflict
	}
	if row.AccountID.String() == actor {
		return pgx.ErrNoRows
	}
	if s.Bindings == nil || s.LockAccounts == nil {
		return ErrUnavailable
	}
	bindings, err := s.Bindings(ctx, tx, []int64{row.RecipientID})
	if err != nil {
		return err
	}
	if len(bindings) != 1 || bindings[0].UserID != row.AccountID.String() {
		return pgx.ErrNoRows
	}
	if err = s.LockAccounts(ctx, tx, []string{row.AccountID.String(), actor}); err != nil {
		return err
	}
	if err = q.LockAccount(ctx, row.AccountID.String()); err != nil {
		return err
	}
	owner := row.AccountID
	row, err = q.LockRedemption(ctx, id)
	if err != nil {
		return err
	}
	if row.AccountID != owner || row.State != "pending" || (version > 0 && row.Version != version) {
		return ErrConflict
	}
	if err = s.shopAdmin(ctx, actor); err != nil {
		return err
	}
	return store.StartDelivery(ctx, tx, id, actor)
}

func (s *Service) StartDelivery(ctx context.Context, actor string, id, version int64) (Handoff, error) {
	if version < 1 {
		return Handoff{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Handoff{}, err
	}
	defer tx.Rollback(context.Background())
	if err = s.startDeliveryTx(ctx, tx, actor, "", id, version); err != nil {
		return Handoff{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Handoff{}, err
	}
	return s.Handoff(ctx, actor, id)
}

func (h Handler) startHandoff(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var body struct {
		Version int64 `json:"version,string"`
	}
	if !readBody(w, r, &body) {
		return
	}
	out, err := h.Service.StartDelivery(r.Context(), h.User(r), id, body.Version)
	respond(w, r, out, err)
}
