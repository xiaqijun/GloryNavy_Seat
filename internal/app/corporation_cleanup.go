package app

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
)

func corporationCleanupService(pool *pgxpool.Pool, acl *access.Service, reader *eve.AuthorizationService) *identity.CorporationCleanup {
	if reader == nil {
		reader = eve.ReadAuthorization(pool)
	}
	return &identity.CorporationCleanup{Pool: pool, Administrator: acl.IsAdministrator, Remove: eve.RemoveCharacterTx,
		Evidence: func(ctx context.Context, tx pgx.Tx, ids []int64) ([]identity.CorporationEvidence, error) {
			rows, err := reader.CleanupMembership(ctx, tx, ids)
			if err != nil {
				return nil, err
			}
			out := make([]identity.CorporationEvidence, 0, len(rows))
			for _, r := range rows {
				out = append(out, identity.CorporationEvidence{ID: r.CharacterID, Owner: r.OwnerHash, CorporationID: r.CorporationID, Name: r.CorporationName, Reliable: r.Reliable})
			}
			return out, nil
		}}
}
