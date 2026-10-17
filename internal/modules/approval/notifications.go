package approval

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/reviewqueue"
)

// PendingNotificationKeys exposes only opaque deduplication references. Every
// page reauthorizes against the owning sources, without the index or scope cache.
// A partial source failure is not an empty queue and must not close receipts.
func (s *Service) PendingNotificationKeys(ctx context.Context, user string) ([]string, error) {
	live := &Service{Sources: s.Sources}
	keys := []string{}
	token := ""
	for {
		page, err := live.listLegacy(ctx, user, reviewqueue.Filter{View: "pending", Sort: "id_asc"}, token)
		if errors.Is(err, pgx.ErrNoRows) {
			return keys, nil
		}
		if err != nil {
			return nil, err
		}
		if len(page.Unavailable) > 0 {
			return nil, errors.New("approval notification source unavailable")
		}
		for _, item := range page.Items {
			if item.Account == user {
				continue
			}
			if !slices.ContainsFunc(item.Actions, func(a string) bool { return a == "approve" || a == "approve_cancel" || a == "cancelled" }) {
				continue
			}
			// Edits which leave an item pending do not trigger another reminder.
			// A different review phase (e.g. cancellation) has a separate reference.
			keys = append(keys, fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", item.Source, item.ID, item.Status)))))
		}
		if page.Next == "" {
			break
		}
		if page.Next == token {
			return nil, errors.New("approval notification cursor did not advance")
		}
		token = page.Next
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys), nil
}
