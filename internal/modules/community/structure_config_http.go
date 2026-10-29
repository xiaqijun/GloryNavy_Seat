package community

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"glorynavy.local/seat/internal/httpapi"
)

func structureConfigFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrStructureWatchForbidden):
		httpapi.Failure(w, r, 403, "forbidden", "仅管理员可访问")
	case errors.Is(err, ErrVersion):
		httpapi.Failure(w, r, 409, "qq_structure_conflict", "配置已更新，请刷新后重试")
	case errors.Is(err, ErrStructureWatchInvalid):
		httpapi.Failure(w, r, 400, "qq_structure_invalid", ErrStructureWatchInvalid.Error())
	case errors.Is(err, ErrStructureWatchBot):
		httpapi.Failure(w, r, 409, "qq_structure_bot", ErrStructureWatchBot.Error())
	case errors.Is(err, ErrStructureWatchSource):
		httpapi.Failure(w, r, 409, "qq_structure_source", ErrStructureWatchSource.Error())
	default:
		httpapi.Failure(w, r, 503, "qq_structure_unavailable", "建筑提醒暂不可用")
	}
}

func (h Handler) getStructureWatch(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Structure.Settings(r.Context(), h.User(r))
	if err != nil {
		structureConfigFailure(w, r, err)
		return
	}
	httpapi.Respond(w, r, 200, settings)
}

func (h Handler) saveStructureWatch(w http.ResponseWriter, r *http.Request) {
	var in StructureWatchInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		structureConfigFailure(w, r, ErrStructureWatchInvalid)
		return
	}
	settings, err := h.Structure.SaveSettings(r.Context(), h.User(r), in)
	if err != nil {
		structureConfigFailure(w, r, err)
		return
	}
	httpapi.Respond(w, r, 200, settings)
}

func (h Handler) structureDeliveries(w http.ResponseWriter, r *http.Request) {
	items, err := h.Structure.Deliveries(r.Context(), h.User(r))
	if err != nil {
		structureConfigFailure(w, r, err)
		return
	}
	httpapi.Respond(w, r, 200, items)
}
