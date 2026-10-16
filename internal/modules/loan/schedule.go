package loan

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

// Calendar dates have a fixed business timezone, independent of UI language.
var loanCalendarZone = time.FixedZone("Asia/Shanghai", 8*60*60)

func loanCalendarDays(from, to time.Time) int {
	from, to = from.In(loanCalendarZone), to.In(loanCalendarZone)
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loanCalendarZone)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loanCalendarZone)
	return int(b.Sub(a) / (24 * time.Hour))
}

func applicationSchedule(in ApplicationInput, now time.Time) (int, *time.Time, error) {
	if in.InstallmentCount < 1 || in.InstallmentCount > 120 {
		return 0, nil, ErrInvalid
	}
	if in.RepaymentDate != "" {
		if in.InstallmentCount != 1 || in.IntervalDays != 0 {
			return 0, nil, ErrInvalid
		}
		date, err := time.ParseInLocation(time.DateOnly, in.RepaymentDate, loanCalendarZone)
		days := loanCalendarDays(now, date)
		if err != nil || days < 1 || days > 365 {
			return 0, nil, ErrInvalid
		}
		due := date.Add(24*time.Hour - time.Second).UTC()
		return days, &due, nil
	}
	if in.InstallmentCount == 1 && in.IntervalDays >= 1 && in.IntervalDays <= 365 || in.InstallmentCount > 1 && validLoanIntervalDays(in.IntervalDays) {
		return in.IntervalDays, nil, nil
	}
	return 0, nil, ErrInvalid
}

// A dated loan freezes its annual rate and deadline at application. Its quoted
// interest is an upper bound, finalized only from the verified contract date.
func fundedSchedule(c store.Case, completed string, now time.Time) (time.Time, time.Time, int, int64, error) {
	if c.RequestedDueAt == nil {
		return now, now.AddDate(0, 0, c.IntervalDays), c.IntervalDays, c.InterestMinor, nil
	}
	funded, err := time.Parse(time.RFC3339, completed)
	// ESI completion timestamps have second precision; submission has fractions.
	if err != nil || funded.Before(c.CreatedAt.Truncate(time.Second)) || funded.After(now) || !funded.Before(*c.RequestedDueAt) || c.InstallmentCount != 1 {
		return time.Time{}, time.Time{}, 0, 0, ErrContract
	}
	days := max(1, loanCalendarDays(funded, *c.RequestedDueAt))
	interest, err := simpleAnnualInterest(c.PrincipalMinor, c.InterestRateBPS, 1, days)
	if err != nil || interest > c.InterestMinor {
		return time.Time{}, time.Time{}, 0, 0, ErrRule
	}
	return funded.UTC(), *c.RequestedDueAt, days, interest, nil
}

func activateFundedCase(ctx context.Context, tx pgx.Tx, c store.Case, actor string, contract Contract, now time.Time) (store.Case, error) {
	funded, due, days, interest, err := fundedSchedule(c, contract.Completed, now)
	if err != nil {
		return c, err
	}
	previous := c
	c, err = store.ActivateCase(ctx, tx, c.ID, c.Version, actor, due, funded, days, interest)
	if err != nil {
		return c, err
	}
	if err = store.CreateInstallments(ctx, tx, c.ID, repaymentInstallments(c.PrincipalMinor, c.InterestMinor, c.InstallmentCount, c.IntervalDays, due)); err != nil {
		return c, err
	}
	if c.RequestedDueAt != nil {
		err = store.Audit(ctx, tx, c.ID, actor, "repayment_terms_finalized", previous, c)
	}
	return c, err
}
