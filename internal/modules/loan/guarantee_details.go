package loan

import (
	"context"
	"strings"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

// guaranteeDetail adds display data without changing the stored guarantee or
// the evidence written by guarantee decisions. Names never determine ownership.
type guaranteeDetail struct {
	store.Guarantee
	GuarantorCharacterName string `json:"guarantor_character_name,omitempty"`
}

// Call only after the case read authorization succeeds. Resolve stored role IDs
// in one batch, independently of the current invitation candidate directory.
func (s *Service) guaranteeDetails(ctx context.Context, rows []store.Guarantee) []guaranteeDetail {
	out := make([]guaranteeDetail, len(rows))
	ids := make([]int64, 0, len(rows))
	seen := make(map[int64]bool, len(rows))
	for i, row := range rows {
		out[i].Guarantee = row
		if row.GuarantorCharacterID > 0 && !seen[row.GuarantorCharacterID] {
			seen[row.GuarantorCharacterID] = true
			ids = append(ids, row.GuarantorCharacterID)
		}
	}
	if s.CharacterNames == nil || len(ids) == 0 {
		return out
	}
	names, err := s.CharacterNames(ctx, ids)
	if err != nil {
		return out
	}
	for i := range out {
		if out[i].GuarantorCharacterID > 0 {
			out[i].GuarantorCharacterName = strings.TrimSpace(names[out[i].GuarantorCharacterID])
		}
	}
	return out
}
