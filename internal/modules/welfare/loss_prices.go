package welfare

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/market"
	"net/http"
	"time"
)

type lossPrice struct {
	Mid        *string    `json:"mid"`
	ObservedAt *time.Time `json:"observed_at"`
}

// Quote only a loss already visible to this actor. Prices are a fresh market
// reference, never part of the immutable ESI report or an approved award.
func (s *Service) quoteLoss(ctx context.Context, actor string, corp, character, id int64) ([]lossPrice, error) {
	if s.Losses == nil || corp <= 0 || character <= 0 || id <= 0 {
		return nil, ErrInvalid
	}
	losses, err := s.Losses(ctx, actor, corp, character, 0, id)
	if err != nil {
		return nil, err
	}
	if len(losses) != 1 || losses[0].ID != id || losses[0].CharacterID != character || losses[0].CorporationID != corp {
		return nil, pgx.ErrNoRows
	}
	loss := losses[0]
	if len(loss.Items) > 2000 {
		return nil, ErrInvalid
	}
	prices := make([]lossPrice, len(loss.Items))
	items := make([]market.Item, 0, len(loss.Items))
	indices := make([]int, 0, len(loss.Items))
	for i, item := range loss.Items {
		if item.TypeID <= 0 || item.Quantity < 0 || item.Quantity > 1000000000 {
			return nil, ErrInvalid
		}
		if item.Quantity == 0 {
			continue
		}
		items = append(items, market.Item{TypeID: item.TypeID, Quantity: item.Quantity, Name: item.Name})
		indices = append(indices, i)
	}
	if len(items) == 0 {
		return prices, nil
	}
	if s.EstimateLoss == nil {
		return nil, ErrValuation
	}
	quote, _, err := s.EstimateLoss(ctx, items)
	if err != nil {
		return nil, err
	}
	if len(quote.Lines) != len(indices) {
		return nil, ErrValuation
	}
	for i, line := range quote.Lines {
		if line.TypeID != items[i].TypeID || line.Quantity != items[i].Quantity {
			return nil, ErrValuation
		}
		prices[indices[i]] = lossPrice{Mid: line.Mid, ObservedAt: line.ObservedAt}
	}
	return prices, nil
}

func (h Handler) lossPrices(w http.ResponseWriter, r *http.Request) {
	corp := number(r.URL.Query().Get("corporation_id"))
	character := number(r.URL.Query().Get("character_id"))
	id := number(r.URL.Query().Get("killmail_id"))
	if corp == 0 || character == 0 || id == 0 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	prices, err := h.Service.quoteLoss(r.Context(), h.User(r), corp, character, id)
	if err != nil {
		if errors.Is(err, ErrValuation) {
			httpapi.Failure(w, r, 503, "loss_price_unavailable", "市场报价暂不可用，请重试")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrInvalid) {
			respond(w, r, nil, err)
			return
		}
		httpapi.Failure(w, r, 503, "loss_price_unavailable", "市场报价暂不可用，请重试")
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": prices})
}
