package store

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"strings"
)

func ReadSDESystemNames(ctx context.Context, pool *pgxpool.Pool, ids []int64) ([]SDETypeName, error) {
	rows, err := pool.Query(ctx, `SELECT n.solar_system_id,n.name_zh,n.name_en FROM eve_sde_solar_system_names n JOIN eve_sde_active_names a ON a.release_id=n.release_id WHERE n.solar_system_id=ANY($1::bigint[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SDETypeName{}
	for rows.Next() {
		var n SDETypeName
		if err = rows.Scan(&n.ID, &n.Chinese, &n.English); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func importSolarSystemNames(ctx context.Context, tx pgx.Tx, entry *zip.File, release int64) (int64, error) {
	input, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer input.Close()
	limited := &io.LimitedReader{R: input, N: (64 << 20) + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var count int64
	batch := make([][]any, 0, 1000)
	flush := func() error {
		_, e := tx.CopyFrom(ctx, pgx.Identifier{"eve_sde_solar_system_names"}, []string{"release_id", "solar_system_id", "name_zh", "name_en"}, pgx.CopyFromRows(batch))
		batch = batch[:0]
		return e
	}
	for scanner.Scan() {
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		var row struct {
			ID   *int64            `json:"_key"`
			Name map[string]string `json:"name"`
		}
		if err = json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return 0, err
		}
		if row.ID == nil || *row.ID <= 0 || strings.TrimSpace(row.Name["en"]) == "" || len(row.Name["en"]) > 4096 || len(row.Name["zh"]) > 4096 {
			return 0, errors.New("invalid solar-system ID or name")
		}
		count++
		if count > 100000 {
			return 0, errors.New("too many solar systems")
		}
		batch = append(batch, []any{release, *row.ID, strings.TrimSpace(row.Name["zh"]), strings.TrimSpace(row.Name["en"])})
		if len(batch) == 1000 {
			if err = flush(); err != nil {
				return 0, err
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return 0, err
	}
	if limited.N <= 0 {
		return 0, errors.New("solar systems exceed size limit")
	}
	if len(batch) > 0 {
		if err = flush(); err != nil {
			return 0, err
		}
	}
	var previous int64
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(r.system_count),0) FROM eve_sde_name_releases r JOIN eve_sde_active_names a ON a.release_id=r.id`).Scan(&previous); err != nil {
		return 0, err
	}
	if count == 0 || count*5 < previous*4 {
		return 0, errors.New("empty solar systems or count dropped by more than 20 percent")
	}
	return count, nil
}
