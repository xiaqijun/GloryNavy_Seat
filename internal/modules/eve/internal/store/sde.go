package store

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SDENamesRelease describes the type and solar-system names profile, not a complete SDE import.
type SDENamesRelease struct {
	ID, Build, Count int64
	SystemCount      int64
	SHA256           string
	Reused           bool
}
type SDETypeName struct {
	ID               int64
	Chinese, English string
}

func ReadSDETypeNames(ctx context.Context, pool *pgxpool.Pool, ids []int64) ([]SDETypeName, error) {
	rows, err := pool.Query(ctx, `SELECT n.type_id,n.name_zh,n.name_en FROM eve_sde_type_names n JOIN eve_sde_active_names a ON a.release_id=n.release_id WHERE n.type_id=ANY($1::bigint[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SDETypeName{}
	for rows.Next() {
		var n SDETypeName
		if err := rows.Scan(&n.ID, &n.Chinese, &n.English); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ImportSDENames reads only official JSONL entries. No extraction or network I/O
// occurs in the publishing transaction. A failed import leaves no partial release.
func ImportSDENames(ctx context.Context, pool *pgxpool.Pool, filename string, expectedBuild int64) (SDENamesRelease, error) {
	return importSDENames(ctx, pool, filename, expectedBuild, 0)
}
func ImportSDENamesAutomatic(ctx context.Context, pool *pgxpool.Pool, filename string, build, fence int64) (SDENamesRelease, error) {
	if fence <= 0 {
		return SDENamesRelease{}, ErrSDEUpdateSuperseded
	}
	return importSDENames(ctx, pool, filename, build, fence)
}
func importSDENames(ctx context.Context, pool *pgxpool.Pool, filename string, expectedBuild, fence int64) (result SDENamesRelease, err error) {
	if expectedBuild <= 0 {
		return result, errors.New("a positive expected build is required")
	}
	f, err := os.Open(filename)
	if err != nil {
		return result, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return result, err
	}
	if info.Size() > 512<<20 {
		return result, errors.New("SDE ZIP exceeds 512 MiB")
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return result, err
	}
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	result.Build = expectedBuild
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		return result, err
	}
	entries := map[string]*zip.File{}
	for _, entry := range z.File {
		name := entry.Name
		if strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			return result, errors.New("unsafe ZIP entry")
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return result, errors.New("unsafe ZIP entry")
			}
		}
		if name != "_sde.jsonl" && name != "types.jsonl" && name != "mapSolarSystems.jsonl" {
			continue
		}
		if entries[name] != nil {
			return result, errors.New("duplicate SDE entry")
		}
		entries[name] = entry
	}
	if entries["_sde.jsonl"] == nil || entries["types.jsonl"] == nil || entries["mapSolarSystems.jsonl"] == nil {
		return result, errors.New("missing SDE metadata, types.jsonl or mapSolarSystems.jsonl")
	}
	if entries["_sde.jsonl"].UncompressedSize64 > 1<<20 || entries["types.jsonl"].UncompressedSize64 > 1<<30 || entries["mapSolarSystems.jsonl"].UncompressedSize64 > 64<<20 {
		return result, errors.New("SDE entry exceeds size limit")
	}
	meta, err := entries["_sde.jsonl"].Open()
	if err != nil {
		return result, err
	}
	data, err := io.ReadAll(io.LimitReader(meta, (1<<20)+1))
	meta.Close()
	if err != nil {
		return result, err
	}
	if len(data) > 1<<20 {
		return result, errors.New("SDE metadata too large")
	}
	matches := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var m struct {
			Key   string `json:"_key"`
			Build int64  `json:"buildNumber"`
		}
		if err = json.Unmarshal([]byte(line), &m); err != nil {
			return result, err
		}
		if m.Key == "sde" {
			matches++
			if m.Build != expectedBuild {
				return result, errors.New("SDE build does not match requested build")
			}
		}
	}
	if matches != 1 {
		return result, errors.New("SDE metadata must contain one sde record")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(740013)`); err != nil {
		return result, err
	}
	if fence > 0 {
		if err = guardSDEUpdate(ctx, tx, fence); err != nil {
			return result, err
		}
		var currentBuild int64
		if err = tx.QueryRow(ctx, `SELECT coalesce(max(r.build_number),0) FROM eve_sde_active_names a JOIN eve_sde_name_releases r ON r.id=a.release_id`).Scan(&currentBuild); err != nil {
			return result, err
		}
		if expectedBuild < currentBuild {
			return result, ErrSDEUpdateSuperseded
		}

	}

	err = tx.QueryRow(ctx, `SELECT id,type_count,system_count FROM eve_sde_name_releases WHERE build_number=$1 AND sha256=$2 AND mapper_version=2`, expectedBuild, result.SHA256).Scan(&result.ID, &result.Count, &result.SystemCount)
	if err == nil {
		result.Reused = true
		if fence > 0 {
			if err = activateSDENames(ctx, tx, result.ID); err != nil {
				return result, err
			}
		}
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	var previousBuild, previousCount int64
	err = tx.QueryRow(ctx, `SELECT r.build_number,r.type_count FROM eve_sde_name_releases r JOIN eve_sde_active_names a ON a.release_id=r.id`).Scan(&previousBuild, &previousCount)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if expectedBuild < previousBuild {
		return result, errors.New("older builds require explicit activation of an existing release")
	}
	if err = tx.QueryRow(ctx, `INSERT INTO eve_sde_name_releases(build_number,sha256,mapper_version) VALUES($1,$2,2) RETURNING id`, expectedBuild, result.SHA256).Scan(&result.ID); err != nil {
		return result, err
	}
	input, err := entries["types.jsonl"].Open()
	if err != nil {
		return result, err
	}
	defer input.Close()
	limited := &io.LimitedReader{R: input, N: (1 << 30) + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	batch := make([][]any, 0, 1000)
	flush := func() error {
		_, e := tx.CopyFrom(ctx, pgx.Identifier{"eve_sde_type_names"}, []string{"release_id", "type_id", "name_zh", "name_en"}, pgx.CopyFromRows(batch))
		batch = batch[:0]
		return e
	}
	for scanner.Scan() {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		var row struct {
			ID   *int64            `json:"_key"`
			Name map[string]string `json:"name"`
		}
		if err = json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return result, fmt.Errorf("type line %d: invalid JSON: %w", result.Count+1, err)
		}
		if row.ID == nil || *row.ID < 0 || strings.TrimSpace(row.Name["en"]) == "" {
			return result, fmt.Errorf("type line %d: missing ID or English name", result.Count+1)
		}
		if len(row.Name["en"]) > 4096 || len(row.Name["zh"]) > 4096 {
			return result, errors.New("type name exceeds size limit")
		}
		batch = append(batch, []any{result.ID, *row.ID, strings.TrimSpace(row.Name["zh"]), strings.TrimSpace(row.Name["en"])})
		result.Count++
		if result.Count > 1000000 {
			return result, errors.New("too many types")
		}
		if len(batch) == 1000 {
			if err = flush(); err != nil {
				return result, err
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return result, err
	}
	if limited.N <= 0 {
		return result, errors.New("types.jsonl exceeds 1 GiB")
	}
	if len(batch) > 0 {
		if err = flush(); err != nil {
			return result, err
		}
	}
	if result.Count == 0 || result.Count*5 < previousCount*4 {
		return result, errors.New("empty SDE or type count dropped by more than 20 percent")
	}
	result.SystemCount, err = importSolarSystemNames(ctx, tx, entries["mapSolarSystems.jsonl"], result.ID)
	if err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE eve_sde_name_releases SET type_count=$2,system_count=$3 WHERE id=$1`, result.ID, result.Count, result.SystemCount); err != nil {
		return result, err
	}
	if err = activateSDENames(ctx, tx, result.ID); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func activateSDENames(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `INSERT INTO eve_sde_active_names(singleton,release_id) SELECT true,id FROM eve_sde_name_releases WHERE id=$1 AND type_count>0 ON CONFLICT(singleton) DO UPDATE SET release_id=excluded.release_id`, id)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("ready SDE release not found")
	}
	return err
}
func ActivateSDENames(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(740013)`); err != nil {
		return err
	}
	if err = activateSDENames(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE eve_sde_update_state SET pinned=true,fence=fence+1,lease_until='epoch',last_status='pinned',last_error='' WHERE singleton`); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
