package lp

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/platform/locale"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct{ Service *Service }

func (h Handler) Module() module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "lp", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "access", APIVersion: 1}, {ID: "eve", APIVersion: 1}}}, Permissions: []string{"lp.self"}, Routes: []module.Route{
		{Method: "GET", Path: "/catalog", Permission: "lp.self", Handler: http.HandlerFunc(h.catalog)},
		{Method: "GET", Path: "/offers", Permission: "lp.self", Handler: http.HandlerFunc(h.offers)},
		{Method: "GET", Path: "/prices", Permission: "lp.self", Handler: http.HandlerFunc(h.prices)},
	}}
}
func result(w http.ResponseWriter, r *http.Request, value any, err error) {
	if errors.Is(err, ErrInput) {
		httpapi.Failure(w, r, 400, "lp_invalid_query", locale.Choose(r.Context(), "LP 查询条件无效", "Invalid LP query"))
		return
	}
	if errors.Is(err, ErrBusy) {
		w.Header().Set("Retry-After", "5")
		httpapi.Failure(w, r, 429, "lp_busy", locale.Choose(r.Context(), "市场查询繁忙，请稍后重试", "Market query busy; try again shortly"))
		return
	}
	if err != nil {
		httpapi.Failure(w, r, 503, "lp_unavailable", locale.Choose(r.Context(), "LP 数据暂不可用，请重试", "LP data unavailable; please retry"))
		return
	}
	httpapi.Respond(w, r, 200, value)
}
func (h Handler) catalog(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Reference(r.Context())
	result(w, r, v, e)
}
func (h Handler) offers(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(r.URL.Query().Get("corporation"), 10, 64)
	if e != nil || id <= 0 || id > 2147483647 {
		result(w, r, nil, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	v, e := h.Service.Offers(ctx, id)
	result(w, r, v, e)
}
func (h Handler) prices(w http.ResponseWriter, r *http.Request) {
	region, e := strconv.ParseInt(r.URL.Query().Get("region"), 10, 64)
	parts := strings.Split(r.URL.Query().Get("types"), ",")
	if e != nil || len(parts) > 100 {
		result(w, r, nil, ErrInput)
		return
	}
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, e := strconv.ParseInt(part, 10, 64)
		if e != nil {
			result(w, r, nil, ErrInput)
			return
		}
		ids = append(ids, id)
	}
	v, e := h.Service.Market.Prices(r.Context(), region, ids)
	result(w, r, v, e)
}
