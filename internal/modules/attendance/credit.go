package attendance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

// PAPCreditSnapshot is a small, account-attributed projection for consumers
// such as loan. It does not expose attendance tables or write PAP data.
type PAPCreditSnapshot struct {
	Points     int64
	Complete   bool
	ObservedAt time.Time
	SnapshotID string
}

// CreditPAP returns confirmed PAP from the recent retained period. Missing
// records are neutral to a consumer's score; this method never treats an
// incomplete sync as zero evidence.
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
	points := row.Points
	if points < 0 {
		points = 0
	}
	return PAPCreditSnapshot{Points: points, Complete: true, ObservedAt: now, SnapshotID: now.Format("20060102")}, nil
}
