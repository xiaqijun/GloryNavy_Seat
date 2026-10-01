package identity

import (
	"context"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
)

// MainCharacterNames is a host-only display projection. Callers must first
// authorize the supplied accounts; this does not authorize access to their data.
func (s *Service) MainCharacterNames(ctx context.Context, accounts []string) (map[string]string, error) {
	if len(accounts) == 0 {
		return map[string]string{}, nil
	}
	return store.New(s.pool).MainCharacterNames(ctx, accounts)
}
