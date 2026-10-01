package store

import "context"

type PAPRequirement struct {
	MonthlyPoints int32
	Version       int64
}

func (q *Queries) PAPRequirement(ctx context.Context) (PAPRequirement, error) {
	var r PAPRequirement
	err := q.db.QueryRow(ctx, `SELECT monthly_points,version FROM attendance_pap_requirement WHERE singleton`).Scan(&r.MonthlyPoints, &r.Version)
	return r, err
}
func (q *Queries) LockPAPRequirement(ctx context.Context) (PAPRequirement, error) {
	var r PAPRequirement
	err := q.db.QueryRow(ctx, `SELECT monthly_points,version FROM attendance_pap_requirement WHERE singleton FOR UPDATE`).Scan(&r.MonthlyPoints, &r.Version)
	return r, err
}
func (q *Queries) UpdatePAPRequirement(ctx context.Context, actor string, old PAPRequirement, points int32) error {
	_, err := q.db.Exec(ctx, `UPDATE attendance_pap_requirement SET monthly_points=$1,version=version+1 WHERE singleton`, points)
	if err != nil {
		return err
	}
	_, err = q.db.Exec(ctx, `INSERT INTO attendance_pap_requirement_audit(version,actor_id,previous_points,monthly_points) VALUES($1,$2::uuid,$3,$4)`, old.Version+1, actor, old.MonthlyPoints, points)
	return err
}
