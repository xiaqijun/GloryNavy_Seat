package app

import (
	"context"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/welfare"
)

func wireWelfareAttendance(s *welfare.Service, a *attendance.Service, accounts *identity.Service) {
	s.AttendanceLosses = func(ctx context.Context, corp, char int64, ids []int64) (map[int64]int64, error) {
		bindings, err := accounts.Bindings(ctx, nil, []int64{char})
		if err != nil {
			return nil, err
		}
		if len(bindings) != 1 {
			return nil, pgx.ErrNoRows
		}
		return a.ConfirmedReimbursementLosses(ctx, bindings[0].UserID, corp, char, ids)
	}
	s.GuardAttendanceLoss = a.LockReimbursementLoss
}
