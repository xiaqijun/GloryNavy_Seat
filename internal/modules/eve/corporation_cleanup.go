package eve

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type CleanupEvidence struct {
	CharacterID     int64
	OwnerHash       []byte
	CorporationID   int64
	CorporationName string
	Reliable        bool
}

// CleanupMembership uses only current, successful cached facts. Holding the
// credentials in the publication lock order fences refresh and sync writers.
func (s *AuthorizationService) CleanupMembership(ctx context.Context, tx pgx.Tx, ids []int64) ([]CleanupEvidence, error) {
	db := store.DBTX(s.pool)
	if tx != nil {
		db = tx
	}
	rows, err := store.CleanupMembership(ctx, db, ids, tx != nil, time.Now())
	if err != nil {
		return nil, err
	}
	out := make([]CleanupEvidence, 0, len(rows))
	for _, r := range rows {
		out = append(out, CleanupEvidence{r.CharacterID, r.OwnerHash, r.CorporationID, r.CorporationName, r.Reliable})
	}
	return out, nil
}
