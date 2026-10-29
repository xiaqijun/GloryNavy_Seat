package store

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TryQQGatewayLock(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	var acquired bool
	err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('community.qq.gateway', 0))`).Scan(&acquired)
	return acquired, err
}

func UnlockQQGateway(ctx context.Context, conn *pgxpool.Conn) error {
	_, err := conn.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended('community.qq.gateway', 0))`)
	return err
}
