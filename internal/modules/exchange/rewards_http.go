package exchange

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

func cursorQuery(r *http.Request, name string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	return number(raw)
}
func (h Handler) shop(w http.ResponseWriter, r *http.Request) {
	after, err := cursorQuery(r, "after")
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope != "" && scope != "all" && scope != "listed" {
		respond(w, r, nil, ErrInvalid)
		return
	}
	subject, err := h.readSubject(r)
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	out, err := h.Service.shopFor(r.Context(), h.User(r), subject, after, scope == "listed")
	respond(w, r, out, err)
}
func (h Handler) editShop(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var c ShopEdit
		if !readBody(w, r, &c) {
			return
		}
		err := h.Service.EditShop(r.Context(), h.User(r), kind, c)
		respond(w, r, map[string]bool{"saved": err == nil}, err)
	}
}
func (h Handler) claimReward(w http.ResponseWriter, r *http.Request) {
	var c ClaimReward
	if !readBody(w, r, &c) {
		return
	}
	id, err := h.Service.ClaimReward(r.Context(), h.User(r), c)
	respond(w, r, map[string]string{"id": strconv.FormatInt(id, 10)}, err)
}
func (h Handler) orders(w http.ResponseWriter, r *http.Request) {
	before, err := cursorQuery(r, "before")
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope != "" && scope != "mine" && scope != "all" {
		respond(w, r, nil, ErrInvalid)
		return
	}
	out, err := h.Service.RewardOrders(r.Context(), h.User(r), scope == "all", before)
	respond(w, r, out, err)
}
func (h Handler) decideOrder(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var c OrderDecision
	if !readBody(w, r, &c) {
		return
	}
	err = h.Service.DecideOrder(r.Context(), h.User(r), id, c)
	respond(w, r, map[string]bool{"saved": err == nil}, err)
}
func (h Handler) searchTypes(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.shopAdmin(r.Context(), h.User(r)); err != nil {
		respond(w, r, nil, err)
		return
	}
	if h.Service.SearchTypes == nil {
		respond(w, r, nil, ErrUnavailable)
		return
	}
	rows, err := h.Service.SearchTypes(r.Context(), r.URL.Query().Get("q"))
	out := []map[string]string{}
	for _, row := range rows {
		out = append(out, map[string]string{"id": strconv.FormatInt(row.ID, 10), "name": row.Name})
	}
	respond(w, r, out, err)
}
