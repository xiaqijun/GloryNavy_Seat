package store

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Settings struct {
	RatioBPS int   `json:"ratio_bps"`
	Version  int64 `json:"version"`
}

func Read(ctx context.Context, p *pgxpool.Pool) (Settings, error) {
	var v Settings
	err := p.QueryRow(ctx, `SELECT ratio_bps,version FROM market_settings WHERE id`).Scan(&v.RatioBPS, &v.Version)
	return v, err
}
func Save(ctx context.Context, tx pgx.Tx, actor string, v Settings) (Settings, error) {
	var out Settings
	err := tx.QueryRow(ctx, `UPDATE market_settings SET ratio_bps=$1,version=version+1,updated_at=now() WHERE id AND version=$2 RETURNING ratio_bps,version`, v.RatioBPS, v.Version).Scan(&out.RatioBPS, &out.Version)
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO market_settings_audit(actor_id,ratio_bps,version) VALUES($1,$2,$3)`, actor, out.RatioBPS, out.Version)
	return out, err
}
