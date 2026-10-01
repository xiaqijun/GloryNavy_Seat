package identity

import (
	"context"

	"glorynavy.local/seat/internal/modules/identity/internal/store"
)

// MainCharacterIDs is a host-only display and delivery projection. Callers
// must first authorize the supplied accounts; this does not grant access.
func (s *Service) MainCharacterIDs(ctx context.Context, accounts []string) (map[string]int64, error) {
	if len(accounts) == 0 {
		return map[string]int64{}, nil
	}
	return store.New(s.pool).MainCharacterIDs(ctx, accounts)
}
