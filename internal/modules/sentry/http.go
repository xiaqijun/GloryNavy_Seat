package sentry

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
)

type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) Module() module.Definition {
	return module.Definition{
		Manifest:    module.Manifest{ID: "sentry", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "access", APIVersion: 1}}},
		Permissions: []string{"sentry.self"},
		Routes: []module.Route{
			{Method: http.MethodGet, Path: "/keys", Permission: "sentry.self", Handler: http.HandlerFunc(h.list)},
			{Method: http.MethodPost, Path: "/keys", Permission: "sentry.self", Handler: http.HandlerFunc(h.create)},
			{Method: http.MethodPost, Path: "/keys/{id}/rotate", Permission: "sentry.self", Handler: http.HandlerFunc(h.rotate)},
			{Method: http.MethodDelete, Path: "/keys/{id}", Permission: "sentry.self", Handler: http.HandlerFunc(h.revoke)},
			{Method: http.MethodPost, Path: "/alert-grants", Permission: "sentry.self", Handler: http.HandlerFunc(h.createAlertGrant)},
			{Method: http.MethodDelete, Path: "/alert-grants/{id}", Permission: "sentry.self", Handler: http.HandlerFunc(h.revokeAlertGrant)},
			{Method: http.MethodGet, Path: "/alert-usage", Permission: "sentry.self", Handler: http.HandlerFunc(h.alertUsage)},
			{Method: http.MethodGet, Path: "/alert-consumptions", Permission: "sentry.self", Handler: http.HandlerFunc(h.alertConsumptions)},
		},
	}
}

func (h Handler) alertUsage(w http.ResponseWriter, r *http.Request) {
	usage, err := h.Service.ReadAlertUsage(r.Context(), h.User(r))
	h.respond(w, r, usage, err)
}

func (h Handler) alertConsumptions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	before := int64(0)
	if raw := strings.TrimSpace(query.Get("before")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			h.respond(w, r, nil, ErrAlertUsageInvalid)
			return
		}
		before = parsed
	}
	limit := 30
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			h.respond(w, r, nil, ErrAlertUsageInvalid)
			return
		}
		limit = parsed
	}
	state := strings.TrimSpace(query.Get("state"))
	if state != "" && state != "reserved" && state != "settled" && state != "released" && state != "refunded" {
		h.respond(w, r, nil, ErrAlertUsageInvalid)
		return
	}
	var from, to *time.Time
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			h.respond(w, r, nil, ErrAlertUsageInvalid)
			return
		}
		from = &parsed
	}
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil || (from != nil && !parsed.After(*from)) {
			h.respond(w, r, nil, ErrAlertUsageInvalid)
			return
		}
		to = &parsed
	}
	page, err := h.Service.ReadAlertConsumptions(r.Context(), h.User(r), before, state, from, to, limit)
	h.respond(w, r, page, err)
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.List(r.Context(), h.User(r))
	h.respond(w, r, map[string]any{"items": items}, err)
}

func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
		RequestKey  string   `json:"request_key"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		h.respond(w, r, nil, ErrInvalid)
		return
	}
	key, err := h.Service.Create(r.Context(), h.User(r), body.Name, body.Permissions, body.RequestKey)
	if err != nil {
		h.respond(w, r, nil, err)
		return
	}
	status := http.StatusCreated
	// A replay can confirm the existing operation but must never return the
	// one-time secret again. Make the replay visible to clients without
	// treating it as a newly created key.
	if key.Secret == "" {
		status = http.StatusOK
	}
	h.respondStatus(w, r, status, key, nil)
}

func (h Handler) revoke(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.Revoke(r.Context(), h.User(r), chi.URLParam(r, "id")); err != nil {
		h.respond(w, r, nil, err)
		return
	}
	h.respond(w, r, map[string]bool{"revoked": true}, nil)
}

func (h Handler) rotate(w http.ResponseWriter, r *http.Request) {
	key, err := h.Service.Rotate(r.Context(), h.User(r), chi.URLParam(r, "id"))
	if err != nil {
		h.respond(w, r, nil, err)
		return
	}
	h.respondStatus(w, r, http.StatusOK, key, nil)
}

func (h Handler) createAlertGrant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		KeyID           string `json:"key_id"`
		RequestKey      string `json:"request_key"`
		ReservedSeconds int64  `json:"reserved_seconds"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		h.respond(w, r, nil, ErrInvalid)
		return
	}
	grant, err := h.Service.CreateAlertGrant(r.Context(), h.User(r), body.KeyID, body.RequestKey, body.ReservedSeconds)
	h.respondStatus(w, r, http.StatusCreated, grant, err)
}

func (h Handler) revokeAlertGrant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RequestKey string `json:"request_key"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		h.respond(w, r, nil, ErrInvalid)
		return
	}
	err := h.Service.RevokeAlertGrant(r.Context(), h.User(r), chi.URLParam(r, "id"), body.RequestKey)
	h.respond(w, r, map[string]bool{"revoked": err == nil}, err)
}

func (h Handler) respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	h.respondStatus(w, r, http.StatusOK, data, err)
}

func (h Handler) respondStatus(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	switch {
	case err == nil:
		httpapi.Respond(w, r, status, data)
	case errors.Is(err, pgx.ErrNoRows):
		httpapi.Failure(w, r, http.StatusNotFound, "sentry_key_not_found", "密钥不存在或无权访问")
	case errors.Is(err, ErrInvalid):
		httpapi.Failure(w, r, http.StatusBadRequest, "invalid_sentry_key", "请检查密钥名称、权限或时间额度参数")
	case errors.Is(err, ErrAlertUsageInvalid):
		httpapi.Failure(w, r, http.StatusBadRequest, "invalid_alert_usage", "请检查预警消费查询条件")
	case errors.Is(err, ErrAlertDisabled):
		httpapi.Failure(w, r, http.StatusServiceUnavailable, "alert_consumption_disabled", "预警按时间收费尚未启用")
	case errors.Is(err, ErrConflict):
		httpapi.Failure(w, r, http.StatusConflict, "sentry_key_conflict", "密钥操作正在同步或状态已变化")
	case errors.Is(err, ErrRemoteUnavailable):
		httpapi.Failure(w, r, http.StatusServiceUnavailable, "sentry_unavailable", "预警平台密钥服务暂未配置或不可用")
	case errors.Is(err, ErrAlertUsageUnavailable):
		httpapi.Failure(w, r, http.StatusServiceUnavailable, "alert_usage_unavailable", "预警消费账单暂不可用")
	case err != nil:
		httpapi.Failure(w, r, http.StatusServiceUnavailable, "sentry_unavailable", "预警平台密钥服务暂不可用")
	}
}
