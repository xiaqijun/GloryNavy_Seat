package identity

import (
	"encoding/json"
	"errors"
	"glorynavy.local/seat/internal/httpapi"
	"io"
	"net/http"
)

func cleanupResponse(w http.ResponseWriter, r *http.Request, v any, err error) {
	switch {
	case err == nil:
		httpapi.Respond(w, r, 200, v)
	case errors.Is(err, ErrCleanupForbidden):
		httpapi.Failure(w, r, 403, "forbidden", "仅站点管理员可配置角色清理")
	case errors.Is(err, ErrCleanupConfig):
		httpapi.Failure(w, r, 400, "invalid_cleanup", "请填写有效的保留军团 ID，启用时名单不能为空")
	case errors.Is(err, ErrCleanupConflict):
		httpapi.Failure(w, r, 409, "cleanup_conflict", "清理规则已更新，请重新读取")
	default:
		httpapi.Failure(w, r, 503, "cleanup_unavailable", "角色清理服务暂不可用")
	}
}
func cleanupBody(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		cleanupResponse(w, r, nil, ErrCleanupConfig)
		return false
	}
	return true
}
func (h Handler) corporationCleanup(w http.ResponseWriter, r *http.Request) {
	s := h.CorporationCleanup
	if s == nil {
		cleanupResponse(w, r, nil, errors.New("unavailable"))
		return
	}
	actor := Principal(r.Context()).UserID
	if err := s.authorized(r.Context(), actor); err != nil {
		cleanupResponse(w, r, nil, err)
		return
	}
	if r.Method == "GET" {
		v, err := s.Settings(r.Context(), actor)
		cleanupResponse(w, r, v, err)
		return
	}
	var in CleanupInput
	if !cleanupBody(w, r, &in) {
		return
	}
	v, err := s.Save(r.Context(), actor, in)
	cleanupResponse(w, r, v, err)
}
func (h Handler) corporationCleanupPreview(w http.ResponseWriter, r *http.Request) {
	s := h.CorporationCleanup
	if s == nil {
		cleanupResponse(w, r, nil, errors.New("unavailable"))
		return
	}
	actor := Principal(r.Context()).UserID
	if err := s.authorized(r.Context(), actor); err != nil {
		cleanupResponse(w, r, nil, err)
		return
	}
	var in struct {
		Corporations []string `json:"corporation_ids"`
	}
	if !cleanupBody(w, r, &in) {
		return
	}
	v, err := s.Preview(r.Context(), actor, in.Corporations)
	cleanupResponse(w, r, v, err)
}
