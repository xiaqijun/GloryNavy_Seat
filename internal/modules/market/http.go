// Package market owns pasted-list appraisals and their global ratio policy.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/market/internal/store"
	"io"
	"net/http"
)

type Service struct {
	Pool          *pgxpool.Pool
	Administrator func(context.Context, string) (bool, error)
	Resolve       func(context.Context, []string) (map[string]eve.StaticTypeName, error)
	Prices        func(context.Context, int64) (eve.MarketPrices, error)
	Contract      func(context.Context, string, string, int64, int64) (eve.DeliveryContract, error)
}
type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) Module() module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "market", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"market.self"}, Routes: []module.Route{{Method: "GET", Path: "/settings", Permission: "market.self", Handler: http.HandlerFunc(h.settings)}, {Method: "POST", Path: "/settings", Permission: "market.self", Handler: http.HandlerFunc(h.configure)}, {Method: "POST", Path: "/estimate-contract", Permission: "market.self", Handler: http.HandlerFunc(h.estimateContract)}, {Method: "POST", Path: "/estimate", Permission: "market.self", Handler: http.HandlerFunc(h.estimate)}}}
}
func failure(w http.ResponseWriter, r *http.Request) {
	httpapi.Failure(w, r, 503, "market_unavailable", "估价暂不可用，请稍后重试")
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_request", "输入格式无效")
		return false
	}
	return true
}
func (h Handler) settings(w http.ResponseWriter, r *http.Request) {
	v, err := store.Read(r.Context(), h.Service.Pool)
	if err != nil {
		failure(w, r)
		return
	}
	admin, err := h.Service.Administrator(r.Context(), h.User(r))
	if err != nil {
		failure(w, r)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"settings": v, "administrator": admin})
}
func (h Handler) configure(w http.ResponseWriter, r *http.Request) {
	var v store.Settings
	if !decode(w, r, &v) {
		return
	}
	if v.RatioBPS < 0 || v.RatioBPS > 100000 || v.Version < 1 {
		httpapi.Failure(w, r, 400, "invalid_ratio", "比例须为 0%–1000%")
		return
	}
	admin, err := h.Service.Administrator(r.Context(), h.User(r))
	if err != nil {
		failure(w, r)
		return
	}
	if !admin {
		httpapi.Failure(w, r, 403, "forbidden", "仅管理员可设置比例")
		return
	}
	tx, err := h.Service.Pool.Begin(r.Context())
	if err != nil {
		failure(w, r)
		return
	}
	defer tx.Rollback(context.Background())
	admin, err = h.Service.Administrator(r.Context(), h.User(r))
	if err != nil || !admin {
		httpapi.Failure(w, r, 403, "forbidden", "仅管理员可设置比例")
		return
	}
	out, err := store.Save(r.Context(), tx, h.User(r), v)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 409, "settings_changed", "配置已变化，请刷新后重试")
		return
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		failure(w, r)
		return
	}
	httpapi.Respond(w, r, 200, out)
}
func (h Handler) estimate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &input) {
		return
	}
	if _, err := parse(input.Text); err != nil {
		httpapi.Failure(w, r, 400, "invalid_items", err.Error())
		return
	}
	v, err := store.Read(r.Context(), h.Service.Pool)
	if err != nil {
		failure(w, r)
		return
	}
	out, err := h.Service.Estimate(r.Context(), input.Text, v.RatioBPS)
	if err != nil {
		failure(w, r)
		return
	}
	httpapi.Respond(w, r, 200, out)
}
