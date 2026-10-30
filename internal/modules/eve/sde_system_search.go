package eve

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
)

type solarSystemChoice struct {
	ID     int64  `json:"id,string"`
	Name   string `json:"name"`
	NameZh string `json:"name_zh"`
	NameEn string `json:"name_en"`
}

func (s *StaticDataService) SearchSolarSystems(ctx context.Context, term string) ([]solarSystemChoice, error) {
	rows, err := store.SearchSDESystems(ctx, s.pool, term)
	if err != nil {
		return nil, err
	}
	result := []solarSystemChoice{}
	for _, row := range rows {
		result = append(result, solarSystemChoice{ID: row.ID, Name: locale.Choose(ctx, row.NameZh, row.NameEn), NameZh: row.NameZh, NameEn: row.NameEn})
	}
	return result, nil
}

func (h *Handler) searchSolarSystems(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	if !utf8.ValidString(term) || len([]rune(term)) > 80 {
		httpapi.Failure(w, r, 400, "invalid_search", "搜索条件无效")
		return
	}
	result, err := (&StaticDataService{pool: h.pool}).SearchSolarSystems(r.Context(), term)
	if err != nil {
		httpapi.Failure(w, r, 503, "systems_unavailable", "星系数据暂不可用")
		return
	}
	httpapi.Respond(w, r, 200, result)
}
