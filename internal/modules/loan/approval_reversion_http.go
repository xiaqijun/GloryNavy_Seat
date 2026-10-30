package loan

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h Handler) revertReview(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var input RevertReviewInput
	if !read(w, r, &input) {
		return
	}
	out, err := h.Service.RevertReview(r.Context(), h.User(r), id, input)
	respond(w, r, out, err)
}
