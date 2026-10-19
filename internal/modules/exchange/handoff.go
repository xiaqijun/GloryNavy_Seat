package exchange

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/contractamount"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

type Handoff struct {
	Started       bool   `json:"started"`
	CanStart      bool   `json:"can_start"`
	Amount        string `json:"amount"`
	Reference     string `json:"reference"`
	RecipientID   int64  `json:"recipient_id,string"`
	RecipientName string `json:"recipient_name"`
	State         string `json:"state"`
}

// Return the saved recipient and settlement reference without resolving items.
// This endpoint never creates a game contract or changes delivery state.
func (s *Service) Handoff(ctx context.Context, actor string, id int64) (Handoff, error) {
	out := Handoff{}
	if e := s.shopAdmin(ctx, actor); e != nil {
		return out, e
	}
	r, e := store.New(s.Pool).Redemption(ctx, id)
	if e != nil {
		return out, e
	}
	delivery, e := store.ReadDelivery(ctx, s.Pool, id)
	if e != nil {
		return out, e
	}
	out.Started = delivery.Started
	out.CanStart = r.State == "pending" && r.AccountID.String() != actor
	out.Reference = r.SettlementReference
	out.RecipientID = r.RecipientID
	out.RecipientName = r.RecipientName
	out.State = r.State
	var content PhysicalReward
	if e = json.Unmarshal(r.RewardContent, &content); e != nil {
		return Handoff{}, e
	}
	out.Amount = strconv.FormatInt(contractamount.WholeISK(content.ISKMinor), 10)
	if e = s.shopAdmin(ctx, actor); e != nil {
		return Handoff{}, e
	}
	return out, nil
}

func (h Handler) handoff(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	out, e := h.Service.Handoff(r.Context(), h.User(r), id)
	respond(w, r, out, e)
}
