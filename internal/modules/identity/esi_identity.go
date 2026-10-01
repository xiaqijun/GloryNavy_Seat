package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
)

// ValidESIIdentity checks the current binding without exposing identity storage.
func (s *Service) ValidESIIdentity(ctx context.Context, id int64, owner []byte) (bool, error) {
	ch, err := store.New(s.pool).GetCharacter(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ch.Status == "active" && subtle.ConstantTimeCompare(ch.OwnerHash, owner) == 1, nil
}
