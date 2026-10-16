package eve

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type CorporationName struct {
	ID   int64
	Name string
}

func (s *AuthorizationService) KnownCorporationNames(ctx context.Context) ([]CorporationName, error) {
	rows, err := store.KnownCorporationNames(ctx, s.pool)
	out := make([]CorporationName, 0, len(rows))
	for _, r := range rows {
		out = append(out, CorporationName{r.ID, r.Name})
	}
	return out, err
}
