package courier

import (
	"encoding/json"
	"errors"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"io"
	"net/http"
)

type Handler struct{ Service *Service }

func (h Handler) Module() module.Definition {
	return module.Definition{
		Manifest:    module.Manifest{ID: "courier", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "access", APIVersion: 1}}},
		Permissions: []string{"courier.self"},
		Routes: []module.Route{
			{Method: "GET", Path: "/routes", Permission: "courier.self", Handler: http.HandlerFunc(h.routes)},
			{Method: "POST", Path: "/appraisal", Permission: "courier.self", Handler: http.HandlerFunc(h.appraisal)},
		},
	}
}
func unavailable(w http.ResponseWriter, r *http.Request) {
	httpapi.Failure(w, r, 503, "courier_unavailable", "快递服务暂不可用，请重试")
}
func (h Handler) routes(w http.ResponseWriter, r *http.Request) {
	v, err := h.Service.Routes(r.Context())
	if err != nil {
		unavailable(w, r)
		return
	}
	httpapi.Respond(w, r, 200, v)
}
func (h Handler) appraisal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var body struct {
		Text string `json:"appraisal_text"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || d.Decode(&struct{}{}) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_items", "物品清单无效或过长")
		return
	}
	v, err := h.Service.Appraise(r.Context(), body.Text)
	if errors.Is(err, ErrInput) {
		httpapi.Failure(w, r, 400, "invalid_items", "物品清单无效或过长")
		return
	}
	if errors.Is(err, ErrBusy) {
		w.Header().Set("Retry-After", "5")
		httpapi.Failure(w, r, 429, "courier_busy", "快递估价繁忙，请稍后重试")
		return
	}
	if err != nil {
		unavailable(w, r)
		return
	}
	httpapi.Respond(w, r, 200, v)
}
