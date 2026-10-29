package reminder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
)

func decodeEmail(w http.ResponseWriter, r *http.Request, value any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(&struct{}{}) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_request", "请求格式无效")
		return false
	}
	return true
}

func (h Handler) emailRetry(w http.ResponseWriter, r *http.Request) {
	user := h.User(r)
	id := chi.URLParam(r, "id")
	if !validID(id) {
		httpapi.Failure(w, r, 400, "invalid_request", "记录编号无效")
		return
	}
	if err := h.Service.admin(r.Context(), user); err != nil {
		respond(w, r, nil, err)
		return
	}
	tx, err := h.Service.Pool.Begin(r.Context())
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	defer tx.Rollback(context.Background())
	if h.Service.LockAccounts == nil {
		respond(w, r, nil, ErrForbidden)
		return
	}
	if err = h.Service.LockAccounts(r.Context(), tx, []string{user}); err != nil {
		respond(w, r, nil, err)
		return
	}
	if err = h.Service.admin(r.Context(), user); err == nil {
		err = store.RetryEmail(r.Context(), tx, id, user)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	respond(w, r, map[string]bool{"queued": err == nil}, err)
}
func (h Handler) emailSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.Service.EmailConfiguration(r.Context(), h.User(r))
	respond(w, r, v, err)
}
func (h Handler) saveEmailSettings(w http.ResponseWriter, r *http.Request) {
	var in EmailSettingsInput
	if !decodeEmail(w, r, &in) {
		return
	}
	v, err := h.Service.SaveEmailConfiguration(r.Context(), h.User(r), in)
	respond(w, r, v, err)
}
func (h Handler) emailDeliveries(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.admin(r.Context(), h.User(r)); err != nil {
		respond(w, r, nil, err)
		return
	}
	v, err := store.ListEmail(r.Context(), h.Service.Pool)
	respond(w, r, v, err)
}
func (h Handler) emailAccount(w http.ResponseWriter, r *http.Request) {
	v, err := h.Service.EmailAccount(r.Context(), h.User(r))
	respond(w, r, v, err)
}
func (h Handler) saveEmailAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision,string"`
		Enabled  bool  `json:"enabled"`
	}
	if !decodeEmail(w, r, &in) {
		return
	}
	v, err := h.Service.SaveEmailAccount(r.Context(), h.User(r), in.Revision, in.Enabled)
	respond(w, r, v, err)
}
func (h Handler) emailChallenge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Address string `json:"address"`
	}
	if !decodeEmail(w, r, &in) {
		return
	}
	err := h.Service.RequestEmailCode(r.Context(), h.User(r), in.Address)
	respond(w, r, map[string]bool{"sent": err == nil}, err)
}
func (h Handler) emailVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decodeEmail(w, r, &in) {
		return
	}
	v, err := h.Service.ConfirmEmailCode(r.Context(), h.User(r), in.Code)
	respond(w, r, v, err)
}
