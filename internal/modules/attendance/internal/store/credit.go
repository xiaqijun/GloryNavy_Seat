package store

import (
	"context"
	"time"
)

// AccountAttendanceRate is the account's closed-event roll-call rate over a
// bounded window. Events are counted once even when an account has multiple
// characters in the same fleet.
type AccountAttendanceRate struct {
	EligibleEvents int64
	AttendedEvents int64
}

func (q *Queries) AccountAttendanceRate(ctx context.Context, accountID string, since, until time.Time) (AccountAttendanceRate, error) {
	var out AccountAttendanceRate
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT a.event_id)::bigint,
		       COUNT(DISTINCT a.event_id) FILTER (WHERE a.present)::bigint
		FROM attendance_entries a
		JOIN attendance_events e ON e.id=a.event_id
		WHERE a.account_id=$1::uuid
		  AND e.state='closed'
		  AND e.starts_at >= $2
		  AND e.starts_at < $3`, accountID, since, until).Scan(&out.EligibleEvents, &out.AttendedEvents)
	return out, err
}

// AlliancePAPFulfillmentRate evaluates each complete retained alliance
// snapshot in the window. A complete snapshot with no row for the account is
// a zero result; incomplete or missing months are excluded from the
// denominator so a data outage never becomes a credit penalty.
type AlliancePAPFulfillmentRate struct {
	EligibleMonths  int64
	FulfilledMonths int64
}

func (q *Queries) AlliancePAPFulfillmentRate(ctx context.Context, accountID string, since, until time.Time, target int32) (AlliancePAPFulfillmentRate, error) {
	var out AlliancePAPFulfillmentRate
	err := q.db.QueryRow(ctx, `
		WITH months AS (
			SELECT h.month
			FROM attendance_alliance_pap_sync_history h
			WHERE h.state='ready' AND h.complete
			  AND h.month >= $2::date AND h.month <= $3::date
		), account_points AS (
			SELECT p.month, COALESCE(SUM(p.pap), 0) AS points
			FROM attendance_alliance_pap_snapshot p
			WHERE p.account_id=$1::uuid
			  AND p.month >= $2::date AND p.month <= $3::date
			GROUP BY p.month
		)
		SELECT COUNT(*)::bigint,
		       COUNT(*) FILTER (WHERE COALESCE(a.points, 0) >= $4::numeric)::bigint
		FROM months m
		LEFT JOIN account_points a ON a.month=m.month`, accountID, since, until, target).Scan(&out.EligibleMonths, &out.FulfilledMonths)
	return out, err
}
