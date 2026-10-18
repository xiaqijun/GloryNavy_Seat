package access

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
)

type ScopeOption struct {
	ID   int64  `json:"id,string"`
	Name string `json:"name"`
}

type ScopeOptions struct {
	Corporations []ScopeOption `json:"corporations"`
	Alliances    []ScopeOption `json:"alliances"`
}

var errScopeOptionsDenied = errors.New("scope options forbidden")

// ScopeOptions decorates configured, currently visible corporations. Labels do
// not grant access; all business reads and writes retain their object checks.
func (s *Service) ScopeOptions(ctx context.Context, actor string) (ScopeOptions, error) {
	out := ScopeOptions{Corporations: []ScopeOption{}, Alliances: []ScopeOption{}}
	a, err := s.Account(ctx, actor)
	if err != nil {
		return out, err
	}
	if !a.CanManage {
		return out, errScopeOptionsDenied
	}
	if s.lookup == nil {
		return out, nil
	}
	seen, alliances := map[int64]bool{}, map[int64]bool{}
	for _, id := range a.corporations {
		if seen[id] || !a.CanViewCorporation(id) {
			continue
		}
		seen[id] = true
		corp, err := s.lookup(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, err
		}
		if corp.ID != id {
			continue
		}
		out.Corporations = append(out.Corporations, ScopeOption{ID: id, Name: corp.Name})
		if corp.AllianceID > 0 {
			alliances[corp.AllianceID] = true
		}
	}
	ids := make([]int64, 0, len(alliances))
	for id := range alliances {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var names map[string]string
	if s.ScopeNames != nil && len(ids) > 0 {
		names, err = s.ScopeNames(ctx, ids)
		if err != nil {
			return out, err
		}
	}
	for _, id := range ids {
		out.Alliances = append(out.Alliances, ScopeOption{ID: id, Name: names[strconv.FormatInt(id, 10)]})
	}
	for _, options := range [][]ScopeOption{out.Corporations, out.Alliances} {
		sort.Slice(options, func(i, j int) bool {
			if options[i].Name == options[j].Name {
				return options[i].ID < options[j].ID
			}
			return options[i].Name < options[j].Name
		})
	}
	return out, nil
}

func (h Handler) scopeOptions(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.ScopeOptions(r.Context(), h.User(r))
	if errors.Is(err, errScopeOptionsDenied) {
		httpapi.Failure(w, r, 403, "forbidden", "没有权限管理权限")
		return
	}
	if err != nil {
		httpapi.Failure(w, r, 503, "access_unavailable", "权限服务暂不可用")
		return
	}
	httpapi.Respond(w, r, 200, out)
}
