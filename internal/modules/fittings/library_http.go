package fittings

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

func (h Handler) libraryContext(w http.ResponseWriter, r *http.Request) {
	user := h.User(r)
	corps, e := h.Service.LibraryCorporations(r.Context(), user)
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	chars, e := h.Service.LibraryCharacters(r.Context(), user)
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	admin, e := h.Service.Administrator(r.Context(), user)
	reply(w, r, map[string]any{"corporations": corps, "characters": chars, "can_manage": admin}, e)
}
func (h Handler) libraryList(w http.ResponseWriter, r *http.Request) {
	id, e := number(r.URL.Query().Get("corporation_id"))
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	v, e := h.Service.LibraryList(r.Context(), h.User(r), id)
	reply(w, r, v, e)
}
func (h Handler) libraryRead(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	v, e := h.Service.LibraryRead(r.Context(), h.User(r), id)
	reply(w, r, v, e)
}
func (h Handler) librarySave(w http.ResponseWriter, r *http.Request) {
	var id int64
	var e error
	if r.Method != "POST" {
		id, e = number(chi.URLParam(r, "id"))
		if e != nil {
			reply(w, r, nil, e)
			return
		}
	}
	var c LibraryEdit
	if !body(w, r, &c) {
		return
	}
	v, e := h.Service.LibraryImport(r.Context(), h.User(r), id, c, r.Method == "DELETE")
	reply(w, r, v, e)
}
func (h Handler) libraryRequirements(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	v, e := h.Service.LibraryRead(r.Context(), h.User(r), id)
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	req, e := prerequisites(v.Fit)
	reply(w, r, map[string]any{"name": v.Name, "corporation_id": strconv.FormatInt(v.CorporationID, 10), "build": strconv.FormatInt(referenceBuild, 10), "requirements": req}, e)
}
func (h Handler) libraryGameSave(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	var c struct {
		Version     int64  `json:"version,string"`
		CharacterID int64  `json:"character_id,string"`
		RequestKey  string `json:"request_key"`
	}
	if !body(w, r, &c) {
		return
	}
	v, e := h.Service.SaveToGame(r.Context(), h.User(r), id, c.Version, c.CharacterID, c.RequestKey)
	reply(w, r, v, e)
}
