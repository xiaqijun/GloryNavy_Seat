package attendance

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

func (h Handler) fleetNotice(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	n, err := h.Service.FleetNotice(r.Context(), h.User(r), id)
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	ev, err := store.New(h.Service.Pool).GetEvent(r.Context(), id)
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	characters := []map[string]string{}
	if h.Service.ReportBindings != nil {
		members, e := h.Service.ReportBindings(r.Context(), h.User(r), "", ev.CorporationID)
		if e != nil {
			respond(w, r, nil, e)
			return
		}
		for _, member := range members {
			if member.ID > 0 && member.Name != "" {
				characters = append(characters, map[string]string{"id": strconv.FormatInt(member.ID, 10), "name": member.Name})
			}
		}
	}
	respond(w, r, map[string]any{"notice": n, "characters": characters}, nil)
}

func (h Handler) fleetMOTDStatus(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	ev, err := store.New(h.Service.Pool).GetEvent(r.Context(), id)
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	if err = h.Service.require(r.Context(), h.User(r), ev.CorporationID); err != nil {
		respond(w, r, nil, err)
		return
	}
	u, err := store.LatestFleetMOTD(r.Context(), h.Service.Pool, id)
	if errors.Is(err, pgx.ErrNoRows) {
		respond(w, r, map[string]any{"state": "none"}, nil)
		return
	}
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	respond(w, r, map[string]any{"state": u.State, "reason": u.Reason, "updated_at": u.UpdatedAt}, nil)
}

func (h Handler) syncFleetMOTD(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var input FleetMOTDInput
	if !readBody(w, r, &input) {
		return
	}
	err = h.Service.QueueFleetNoticeMOTD(r.Context(), h.User(r), id, input)
	respond(w, r, map[string]any{"queued": true}, err)
}

func (h Handler) latestFleetDispatch(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	data, err := h.Service.LatestFleetDispatch(r.Context(), h.User(r), id)
	respond(w, r, map[string]any{"dispatch": data}, err)
}

func (h Handler) fleetDispatch(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var input FleetDispatchInput
	if !readBody(w, r, &input) {
		return
	}
	data, err := h.Service.FleetDispatch(r.Context(), h.User(r), id, input)
	if errors.Is(err, ErrFleetChannelUnavailable) {
		httpapi.Failure(w, r, 422, "fleet_channel_unavailable", "推送渠道未启用或未配置，请检查提醒设置")
		return
	}
	respond(w, r, data, err)
}

func (h Handler) saveFleetNotice(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var input FleetNoticeInput
	// A fitting DNA snapshot and multilingual notice can exceed the shared
	// 4 KiB limit; keep a bounded limit specific to this endpoint.
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF {
		respond(w, r, nil, ErrInvalid)
		return
	}
	n, err := h.Service.SaveFleetNotice(r.Context(), h.User(r), id, input)
	respond(w, r, n, err)
}
