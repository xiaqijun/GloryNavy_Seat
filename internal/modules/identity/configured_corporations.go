package identity

import (
	"context"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
)

type CleanupCorporation struct {
	ID   int64  `json:"id,string"`
	Name string `json:"name"`
}

// ConfiguredCorporations is an internal host boundary, not a public endpoint.
// The configured scope remains in force when weekly cleanup is disabled.
func (s *CorporationCleanup) ConfiguredCorporations(ctx context.Context) ([]int64, error) {
	p, err := store.ReadCleanupPolicy(ctx, s.Pool, false)
	return p.Corporations, err
}
