package attendance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

// PAPCreditSnapshot is a small, account-attributed projection for consumers
// such as loan. It does not expose attendance tables or write PAP data.
type PAPCreditSnapshot struct {
	// Points is the combined PAP activity signal. Alliance PAP can carry two
	// decimal places, so this projection keeps the value as a float for the
	// consumer instead of silently rounding the upstream snapshot.
	Points     float64
	Complete   bool
	ObservedAt time.Time
	SnapshotID string
}

// CreditPAP returns the account's confirmed local roll-call PAP plus the
// latest complete retained alliance PAP snapshot. The two sources are intentionally
// kept separate in attendance storage; this projection combines them only for
// consumers such as loan credit scoring. Missing or incomplete alliance data
// is neutral and never treated as a hard zero.
func (s *Service) CreditPAP(ctx context.Context, account string) (PAPCreditSnapshot, error) {
	id, err := uuid(account)
	if err != nil {
		return PAPCreditSnapshot{}, err
	}
	now := time.Now().UTC()
	row, err := store.New(s.Pool).PAPTotals(ctx, store.PAPTotalsParams{
		Since:     pgtype.Timestamptz{Time: now.AddDate(-1, 0, 0), Valid: true},
		Until:     pgtype.Timestamptz{Time: now, Valid: true},
		AccountID: id,
	})
	if err != nil {
		return PAPCreditSnapshot{}, err
	}
	localPoints := row.Points
	if localPoints < 0 {
		localPoints = 0
	}

	points := float64(localPoints)
	snapshotID := now.Format("20060102")
	queries := store.New(s.Pool)
	alliance, allianceErr := queries.AlliancePAPAccount(ctx, account)
	if allianceErr != nil || !confirmedAllianceSnapshot(alliance, now) {
		// A failed current-month sync must not erase the last complete retained
		// month. The retention window matches the local PAP evidence window.
		alliance, allianceErr = queries.AlliancePAPLatestAccount(ctx, account, now.AddDate(-1, 0, 0))
	}
	if allianceErr == nil {
		if alliancePoints, ok := confirmedAllianceCreditPoints(alliance, now); ok {
			points += alliancePoints
			snapshotID = fmt.Sprintf("%s:alliance-%s-%d", snapshotID, alliance.Month.Format("200601"), alliance.Version)
		}
	}

	return PAPCreditSnapshot{Points: points, Complete: true, ObservedAt: now, SnapshotID: snapshotID}, nil
}

func confirmedAllianceCreditPoints(report store.AlliancePAPAccountReport, now time.Time) (float64, bool) {
	if !confirmedAllianceSnapshot(report, now) {
		return 0, false
	}
	units, err := alliancePAPUnits(report.Points)
	if err != nil {
		return 0, false
	}
	return float64(units) / 100, true
}

func confirmedAllianceSnapshot(report store.AlliancePAPAccountReport, now time.Time) bool {
	if report.State != "ready" || !report.Complete || report.Month.IsZero() {
		return false
	}
	month := report.Month.UTC()
	cutoff := now.UTC().AddDate(-1, 0, 0)
	return !month.After(now.UTC()) && !month.Before(cutoff)
}
