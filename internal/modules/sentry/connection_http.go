package sentry

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"glorynavy.local/seat/internal/httpapi"
)

func (h Handler) readConnection(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Service.ReadConnection(r.Context(), h.User(r))
	h.respondConnection(w, r, settings, err)
}

func (h Handler) editConnection(w http.ResponseWriter, r *http.Request) {
	var body ConnectionEdit
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		h.respondConnection(w, r, ConnectionSettings{}, errConnectionInvalid)
		return
	}
	settings, err := h.Service.EditConnection(r.Context(), h.User(r), body)
	h.respondConnection(w, r, settings, err)
}

func (h Handler) respondConnection(w http.ResponseWriter, r *http.Request, settings ConnectionSettings, err error) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case err == nil:
		httpapi.Respond(w, r, http.StatusOK, settings)
	case errors.Is(err, errConnectionPermission):
		httpapi.Failure(w, r, http.StatusForbidden, "sentry_connection_forbidden", "只有站点管理员可以配置 Sentry 接入")
	case errors.Is(err, errConnectionConflict):
		httpapi.Failure(w, r, http.StatusConflict, "sentry_connection_conflict", "接入配置已更新，请刷新后重试")
	case errors.Is(err, errConnectionInvalid):
		httpapi.Failure(w, r, http.StatusBadRequest, "invalid_sentry_connection", "请填写有效的 Sentry HTTPS 地址和服务令牌")
	case errors.Is(err, errConnectionCheck):
		httpapi.Failure(w, r, http.StatusBadGateway, "sentry_connection_check_failed", "Sentry 鉴权未通过，请检查地址、令牌和接入开关")
	default:
		httpapi.Failure(w, r, http.StatusServiceUnavailable, "sentry_connection_unavailable", "Sentry 接入配置暂不可用")
	}
}
