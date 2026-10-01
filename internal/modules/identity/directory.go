package identity

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
	"strconv"
)

// Directory is only exposed by the host through an access.manage-protected route.
type DirectoryMember struct {
	UserID         string
	Main           Character
	CharacterCount int32
}

func (s *Service) SearchMembers(ctx context.Context, search, after string) ([]DirectoryMember, string, error) {
	var cursor pgtype.UUID
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	if err := cursor.Scan(after); err != nil {
		return nil, "", err
	}
	rows, err := store.New(s.pool).SearchMembers(ctx, store.SearchMembersParams{AfterID: cursor, Search: search})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > 25 {
		rows = rows[:25]
		next = rows[24].ID.String()
	}
	result := make([]DirectoryMember, 0, len(rows))
	for _, r := range rows {
		result = append(result, DirectoryMember{r.ID.String(), Character{strconv.FormatInt(r.CharacterID, 10), r.Name}, r.CharacterCount})
	}
	return result, next, nil
}
