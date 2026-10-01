package exchange

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"io"
	"net/http"
	"strconv"
)

type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) Module() module.Definition {
	routes := []module.Route{}
	for _, r := range []struct {
		method, path string
		handler      http.HandlerFunc
	}{
		{"GET", "/catalog", h.catalog}, {"POST", "/catalog", h.saveCatalog},
		{"GET", "/rewards/{id}/valuation", h.rewardValuation}, {"GET", "/rewards", h.shop}, {"POST", "/rewards/rate", h.editShop("rate")}, {"POST", "/rewards/items", h.editShop("reward")}, {"GET", "/rewards/types", h.searchTypes}, {"POST", "/rewards/claim", h.claimReward}, {"GET", "/rewards/orders/{id}/handoff", h.handoff}, {"GET", "/rewards/orders", h.orders}, {"POST", "/rewards/orders/{id}", h.decideOrder},
		{"GET", "/context", h.context}, {"GET", "/wallet", h.wallet}, {"POST", "/sources", h.editSource},
	} {
		routes = append(routes, module.Route{Method: r.method, Path: r.path, Permission: "exchange.self", Handler: r.handler})
	}
	return module.Definition{Manifest: module.Manifest{ID: "exchange", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"exchange.self"}, Routes: routes}
}
func respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	code, msg, status := "exchange_unavailable", "兑换服务暂不可用", 503
	switch {
	case err == nil:
		httpapi.Respond(w, r, 200, data)
		return
	case errors.Is(err, ErrInvalid):
		code, msg, status = "invalid_exchange", "请检查兑换数量、比例及必填内容", 400
	case errors.Is(err, ErrDeliveryPending):
		code, msg, status = "delivery_pending", "请先在游戏中处理交付合同，等待同步后再确认取消", 409
	case errors.Is(err, ErrConflict):
		code, msg, status = "exchange_conflict", "报价或记录已更新，请关闭表单、刷新后重试", 409
	case errors.Is(err, ErrInsufficientCoinsMinor):
		code, msg, status = "insufficient_coins", "可用果壳币不足，请刷新后重试", 409
	case errors.Is(err, ErrRewardUnavailable):
		code, msg, status = "reward_unavailable", "奖励未上架、库存不足或币值未配置", 409
	case errors.Is(err, pgx.ErrNoRows):
		code, msg, status = "exchange_unavailable", "记录不存在或无权访问", 404
	}
	httpapi.Failure(w, r, status, code, msg)
}
func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 262144))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		respond(w, r, nil, ErrInvalid)
		return false
	}
	return true
}
func number(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		return 0, ErrInvalid
	}
	return id, nil
}
