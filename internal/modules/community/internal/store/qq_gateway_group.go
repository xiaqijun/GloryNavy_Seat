package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// QQGatewayGroupEnabled holds the row until transaction completion so a
// concurrent administrator change cannot race event publication.
func (q *Queries) QQGatewayGroupEnabled(ctx context.Context, group string) (bool, error) {
	var enabled bool
	err := q.db.QueryRow(ctx, `SELECT enabled FROM community_qq_group_settings WHERE group_openid = $1 FOR SHARE`, group).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}
