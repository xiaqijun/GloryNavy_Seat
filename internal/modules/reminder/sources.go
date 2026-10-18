package reminder

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"time"
)

func readSource(ctx context.Context, source Source, user, kind string, since time.Time) ([]string, error) {
	if source == nil {
		return nil, ErrEvents
	}
	floor := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if since.Before(floor) {
		since = floor
	}
	keys, err := source(ctx, user, since)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			return nil, ErrEvents
		}
		if kind != "approval_pending" {
			key = fmt.Sprintf("%s:%x", kind, sha256.Sum256([]byte(key)))
		}
		out = append(out, key)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
