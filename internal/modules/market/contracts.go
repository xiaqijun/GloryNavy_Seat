package market

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve"
)

// Select one exchange side; never add offered and requested goods together.
func contractItems(c eve.DeliveryContract, side string, now time.Time) ([]Item, error) {
	if c.Status != "outstanding" || (c.Type != "item_exchange" && c.Type != "auction") ||
		!c.Expired.After(now) || c.AcceptorID != 0 || c.Accepted != "" {
		return nil, errors.New("合同已接取、过期或不支持物品估价，请重新选择")
	}
	if !c.ItemsReady {
		return nil, errors.New("合同物品尚未同步完成，请稍后重试")
	}
	items := []Item{}
	for _, item := range c.Items {
		if item.Included != (side == "included") {
			continue
		}
		if item.RawQuantity != nil && *item.RawQuantity == -2 {
			return nil, errors.New("合同包含蓝图拷贝，无法按市场原版价格估价")
		}
		if item.TypeID <= 0 || item.Quantity <= 0 || item.Quantity > 1000000000 {
			return nil, errors.New("合同物品数量超出估价范围")
		}
		items = append(items, Item{TypeID: item.TypeID, Quantity: item.Quantity, Name: item.Name})
	}
	if len(items) == 0 {
		return nil, errors.New("合同所选一方没有物品")
	}
	if len(items) > 2001 {
		return nil, errors.New("合同物品过多，无法完整估价")
	}
	return items, nil
}

func (h Handler) estimateContract(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Kind     string `json:"owner_kind"`
		Owner    int64  `json:"owner_id,string"`
		Contract int64  `json:"contract_id,string"`
		Side     string `json:"side"`
	}
	if !decode(w, r, &input) {
		return
	}
	if (input.Kind != "character" && input.Kind != "corporation") || input.Owner <= 0 || input.Contract <= 0 || (input.Side != "included" && input.Side != "requested") {
		httpapi.Failure(w, r, 400, "invalid_request", "输入格式无效")
		return
	}
	if h.Service.Contract == nil {
		failure(w, r)
		return
	}
	c, err := h.Service.Contract(r.Context(), h.User(r), input.Kind, input.Owner, input.Contract)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "contract_unavailable", "合同不存在或没有查看权限")
		return
	}
	if err != nil {
		failure(w, r)
		return
	}
	items, err := contractItems(c, input.Side, time.Now())
	if err != nil {
		httpapi.Failure(w, r, 409, "contract_not_appraisable", err.Error())
		return
	}
	result, _, err := h.Service.EstimateItems(r.Context(), items)
	if err != nil {
		failure(w, r)
		return
	}
	// Quotes may take time. Recheck access and terms before returning private items.
	current, err := h.Service.Contract(r.Context(), h.User(r), input.Kind, input.Owner, input.Contract)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "contract_unavailable", "合同不存在或没有查看权限")
		return
	}
	if err != nil {
		failure(w, r)
		return
	}
	if _, err = contractItems(current, input.Side, time.Now()); err != nil || current.ContentToken != c.ContentToken {
		httpapi.Failure(w, r, 409, "contract_changed", "合同内容或状态已变化，请重新估价")
		return
	}
	httpapi.Respond(w, r, 200, result)
}
