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
	// Points is retained as a compatibility projection for existing consumers.
	// New credit scoring uses the explicit attendance and alliance fulfillment
	// fields below instead of combining raw PAP units.
	Points                    float64
	Complete                  bool
	ObservedAt                time.Time
	SnapshotID                string
	CorporationAttendanceBPS  int32
	CorporationEligibleEvents int64
	CorporationAttendedEvents int64
	AllianceFulfillmentBPS    int32
	AllianceEligibleMonths    int64
	AllianceFulfilledMonths   int64
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

	// Credit uses a bounded, interpretable split: the local signal is the
	// account's closed-event attendance rate over the existing one-year local
	// PAP evidence window, while the alliance signal is the number of complete
	// PAP snapshots meeting the configured monthly requirement over six months.
	activitySince := now.AddDate(-1, 0, 0)
	allianceSince := now.AddDate(0, -6, 0)
	attendanceRate, attendanceErr := queries.AccountAttendanceRate(ctx, account, activitySince, now)
	allianceRequirement, requirementErr := queries.PAPRequirement(ctx)
	var allianceRate store.AlliancePAPFulfillmentRate
	var allianceRateErr error
	if requirementErr == nil {
		allianceRate, allianceRateErr = queries.AlliancePAPFulfillmentRate(ctx, account, allianceSince, now, allianceRequirement.MonthlyPoints)
	}
	var corporationBPS, allianceBPS int32
	if attendanceErr == nil && attendanceRate.EligibleEvents > 0 {
		corporationBPS = int32((attendanceRate.AttendedEvents*10000 + attendanceRate.EligibleEvents/2) / attendanceRate.EligibleEvents)
	}
	if allianceRateErr == nil && allianceRate.EligibleMonths > 0 {
		allianceBPS = int32((allianceRate.FulfilledMonths*10000 + allianceRate.EligibleMonths/2) / allianceRate.EligibleMonths)
	}

	return PAPCreditSnapshot{
		Points: points, Complete: true, ObservedAt: now, SnapshotID: snapshotID,
		CorporationAttendanceBPS:  corporationBPS,
		CorporationEligibleEvents: attendanceRate.EligibleEvents,
		CorporationAttendedEvents: attendanceRate.AttendedEvents,
		AllianceFulfillmentBPS:    allianceBPS,
		AllianceEligibleMonths:    allianceRate.EligibleMonths,
		AllianceFulfilledMonths:   allianceRate.FulfilledMonths,
	}, nil
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
