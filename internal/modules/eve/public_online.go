package eve

import (
	"bytes"
	"context"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"slices"
	"time"
)

type PublicOnlineBinding struct {
	ID        int64
	OwnerHash []byte
}

// The host supplies current identity bindings; SQL stays in the EVE module.
func (s *AuthorizationService) PublicOnline(ctx context.Context, bindings []PublicOnlineBinding) (*PublicOnline, error) {
	ids := make([]int64, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.ID)
	}
	rows, err := store.New(s.pool).PublicOnlineLatest(ctx, store.PublicOnlineLatestParams{Ids: ids, CorporationID: publicCorporationID})
	if err != nil {
		return nil, err
	}
	return summarizePublicOnline(bindings, rows, time.Now().UTC()), nil
}
func summarizePublicOnline(bindings []PublicOnlineBinding, rows []store.PublicOnlineLatestRow, now time.Time) *PublicOnline {
	owners := map[int64]PublicOnlineBinding{}
	for _, b := range bindings {
		owners[b.ID] = b
	}
	bound, covered, online := map[int64]bool{}, map[int64]bool{}, map[int64]bool{}
	result := &PublicOnline{UpdatedAt: now, ExpiresAt: now.Add(onlineSampleMaxGap)}
	for _, r := range rows {
		b, ok := owners[r.CharacterID]
		if !ok || len(b.OwnerHash) == 0 || !bytes.Equal(b.OwnerHash, r.OwnerHash) {
			continue
		}
		bound[b.ID] = true
		if r.State == "reauthorize" || !slices.Contains(r.Scopes, OnlineReadScope) || r.Generation != r.GrantGeneration || !r.Online.Valid || !r.ObservedAt.Valid || r.ObservedAt.Time.After(now) || now.Sub(r.ObservedAt.Time) > onlineSampleMaxGap {
			continue
		}
		covered[b.ID] = true
		if r.Online.Bool {
			online[b.ID] = true
		}
		if until := r.ObservedAt.Time.Add(onlineSampleMaxGap); until.Before(result.ExpiresAt) {
			result.ExpiresAt = until
		}
	}
	result.Bound = len(bound)
	result.Covered = len(covered)
	if len(covered) > 0 {
		count := len(online)
		result.Characters = &count
	}
	return result
}
