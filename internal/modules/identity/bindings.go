package identity

import (
	"context"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
	"slices"
)

type Binding struct {
	ID           int64
	UserID, Name string
	OwnerHash    []byte
}

// Bindings optionally fences ownership for a caller's short publication transaction.
// Sorted character locks follow Complete/ChangeCharacter's lock order, before EVE locks.
func (s *Service) Bindings(ctx context.Context, tx pgx.Tx, ids []int64) ([]Binding, error) {
	q := store.New(s.pool)
	if tx != nil {
		sorted := slices.Clone(ids)
		slices.Sort(sorted)
		sorted = slices.Compact(sorted)
		for _, id := range sorted {
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", id); err != nil {
				return nil, err
			}
		}
		q = store.New(tx)
	}
	rows, err := q.BoundCharacters(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]Binding, 0, len(rows))
	for _, r := range rows {
		result = append(result, Binding{r.CharacterID, r.UserID.String(), r.Name, r.OwnerHash})
	}
	return result, nil
}
