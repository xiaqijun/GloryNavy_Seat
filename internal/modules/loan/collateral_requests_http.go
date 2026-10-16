package loan

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h Handler) collateralAssets(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	character, err := number(r.URL.Query().Get("character_id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	out, err := h.Service.CollateralAssets(r.Context(), h.User(r), id, character)
	respond(w, r, out, err)
}
func (h Handler) collateralRequest(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var in CollateralRequestInput
	if !read(w, r, &in) {
		return
	}
	out, err := h.Service.CreateCollateralRequest(r.Context(), h.User(r), id, in)
	respond(w, r, out, err)
}
func (h Handler) collateralRequestCancel(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !read(w, r, &in) {
		return
	}
	out, err := h.Service.CancelCollateralRequest(r.Context(), h.User(r), id, in.Version)
	respond(w, r, out, err)
}

func (h Handler) collateralQuote(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var in CollateralRequestInput
	if !read(w, r, &in) {
		return
	}
	out, err := h.Service.QuoteCollateralRequest(r.Context(), h.User(r), id, in)
	respond(w, r, out, err)
}
