package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSDEUpdateSuperseded = errors.New("SDE update superseded or pinned")

type SDEUpdateStatus struct {
	Pinned          bool       `json:"pinned"`
	Fence           int64      `json:"-"`
	LeaseUntil      time.Time  `json:"lease_until"`
	NextCheckAt     time.Time  `json:"next_check_at"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
	LastSuccessAt   *time.Time `json:"last_success_at"`
	LastStatus      string     `json:"last_status"`
	LastError       string     `json:"last_error"`
	FailureCount    int        `json:"failure_count"`
	ObservedBuild   int64      `json:"observed_build"`
	ActiveReleaseID int64      `json:"active_release_id,string"`
	ActiveBuild     int64      `json:"active_build"`
	ActiveSHA256    string     `json:"active_sha256"`
	MapperVersion   int        `json:"mapper_version"`
	SystemCount     int64      `json:"system_count"`
	TypeCount       int64      `json:"type_count"`
}

func SDEStatus(ctx context.Context, pool *pgxpool.Pool) (s SDEUpdateStatus, err error) {
	err = pool.QueryRow(ctx, `SELECT s.pinned,s.fence,s.lease_until,s.next_check_at,s.last_checked_at,s.last_success_at,s.last_status,s.last_error,s.failure_count,s.observed_build,coalesce(r.id,0),coalesce(r.build_number,0),coalesce(r.type_count,0),coalesce(r.sha256,''),coalesce(r.mapper_version,0),coalesce(r.system_count,0) FROM eve_sde_update_state s LEFT JOIN eve_sde_active_names a ON true LEFT JOIN eve_sde_name_releases r ON r.id=a.release_id WHERE s.singleton`).Scan(&s.Pinned, &s.Fence, &s.LeaseUntil, &s.NextCheckAt, &s.LastCheckedAt, &s.LastSuccessAt, &s.LastStatus, &s.LastError, &s.FailureCount, &s.ObservedBuild, &s.ActiveReleaseID, &s.ActiveBuild, &s.TypeCount, &s.ActiveSHA256, &s.MapperVersion, &s.SystemCount)
	return
}

func SDEUpdateDue(ctx context.Context, db DBTX) (bool, error) {
	var due bool
	err := db.QueryRow(ctx, `SELECT NOT pinned AND next_check_at<=now() AND lease_until<=now() FROM eve_sde_update_state WHERE singleton`).Scan(&due)
	return due, err
}

func ClaimSDEUpdate(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var fence int64
	err := pool.QueryRow(ctx, `UPDATE eve_sde_update_state SET fence=fence+1,lease_until=now()+interval '20 minutes',last_status='running' WHERE singleton AND NOT pinned AND next_check_at<=now() AND lease_until<=now() RETURNING fence`).Scan(&fence)
	return fence, err
}

// Called inside the publish transaction, after the import advisory lock.
func guardSDEUpdate(ctx context.Context, tx pgx.Tx, fence int64) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT NOT pinned AND fence=$1 AND lease_until>now() FROM eve_sde_update_state WHERE singleton FOR UPDATE`, fence).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrSDEUpdateSuperseded
	}
	return nil
}

func FinishSDEUpdate(ctx context.Context, pool *pgxpool.Pool, fence, build int64, outcome, reason string, interval time.Duration) error {
	// A late worker cannot replace a pin, a newer lease, or its status.
	_, err := pool.Exec(ctx, `UPDATE eve_sde_update_state SET lease_until='epoch',last_checked_at=now(),observed_build=CASE WHEN $2>0 THEN $2 ELSE observed_build END,last_status=$3,last_error=$4,last_success_at=CASE WHEN $4='' THEN now() ELSE last_success_at END,next_check_at=now()+CASE WHEN $4='' THEN $5::bigint*interval '1 second' ELSE least(21600,300*power(2,least(failure_count,7))) * interval '1 second' END,failure_count=CASE WHEN $4='' THEN 0 ELSE least(failure_count+1,1000) END WHERE singleton AND fence=$1 AND NOT pinned`, fence, build, outcome, reason, int64(interval/time.Second))
	return err
}

func ResumeSDEUpdates(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(740013)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE eve_sde_update_state SET pinned=false,fence=fence+1,lease_until='epoch',next_check_at=now(),last_status='pending',last_error='',failure_count=0 WHERE singleton`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
