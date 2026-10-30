package approval

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// RefreshSource only refreshes the shared read model; source authorization is
// rechecked directly and the response never exposes the source snapshot.
func (s *Service) RefreshSource(ctx context.Context, user, sourceID string) error {
	for _, source := range s.Sources {
		if source.ID != sourceID {
			continue
		}
		if source.Access == nil {
			return pgx.ErrNoRows
		}
		access, err := source.Access(ctx, user)
		if err != nil {
			return err
		}
		if !access.Allowed {
			return pgx.ErrNoRows
		}
		if s.RefreshProjection == nil {
			return errors.New("approval projection refresh unavailable")
		}
		return s.RefreshProjection(ctx, source)
	}
	return pgx.ErrNoRows
}

func (h Handler) refresh(w http.ResponseWriter, r *http.Request) {
	err := h.Service.RefreshSource(r.Context(), h.User(r), chi.URLParam(r, "source"))
	respond(w, r, map[string]bool{"refreshed": err == nil}, err)
}
