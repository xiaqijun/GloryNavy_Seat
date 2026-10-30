package attendance

import "net/http"

import "github.com/go-chi/chi/v5"

func (h Handler) eventSource(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	if r.Method == http.MethodGet {
		out, e := h.Service.EventSource(r.Context(), h.User(r), id)
		respond(w, r, out, e)
		return
	}
	var input struct {
		SourceID *int64 `json:"source_character_id,string"`
		Revision *int64 `json:"revision,string"`
	}
	if !readBody(w, r, &input) {
		return
	}
	if input.SourceID == nil || input.Revision == nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	change := SourceChange{SourceID: *input.SourceID, Revision: *input.Revision}
	out, err := h.Service.SetEventSource(r.Context(), h.User(r), id, change)
	respond(w, r, out, err)
}
