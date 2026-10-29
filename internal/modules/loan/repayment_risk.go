package loan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

type RepaymentRisk struct {
	Status                 string `json:"status"`
	Days                   int    `json:"days"`
	EstimatedCapacityMinor int64  `json:"estimated_capacity_minor"`
	TotalDueMinor          int64  `json:"total_due_minor"`
	Token                  string `json:"token,omitempty"`
}

// Risk is advisory: collateral and a manager's decision may still justify the
// loan. The token binds acknowledgment to case version, evaluation, policy,
// and the effective calendar duration; it never conveys authorization.
func repaymentRisk(c store.Case, e store.CreditEvaluation, p store.CreditPolicy, obligations []store.RepaymentObligation, unknownObligations bool, now time.Time) RepaymentRisk {
	days := c.IntervalDays * c.InstallmentCount
	if c.RequestedDueAt != nil {
		days = max(0, loanCalendarDays(now, *c.RequestedDueAt))
	}
	credit := creditFromEvaluation(e)
	r := RepaymentRisk{Status: "unknown", Days: days, TotalDueMinor: c.TotalDueMinor}
	if credit.IncomeAvailable && credit.MonthlyIncomeMinor > 0 && days > 0 && !unknownObligations {
		r.Status = "adequate"
		var cumulative int64
		for i, installment := range repaymentInstallments(c.PrincipalMinor, c.InterestMinor, c.InstallmentCount, c.IntervalDays, now) {
			cumulative += installment.PrincipalMinor + installment.InterestMinor
			periodDays := c.IntervalDays * (i + 1)
			if c.RequestedDueAt != nil {
				periodDays = days
			}
			due := cumulative
			for _, other := range obligations {
				if loanCalendarDays(now, other.DueAt) <= periodDays {
					if other.OutstandingMinor > 0 && due > int64(^uint64(0)>>1)-other.OutstandingMinor {
						due = int64(^uint64(0) >> 1)
					} else {
						due += other.OutstandingMinor
					}
				}
			}
			capacity := safeScaled(credit.MonthlyIncomeMinor, int64(p.DebtRatioBPS)*int64(periodDays), 10000*30)
			r.EstimatedCapacityMinor = capacity
			if capacity < due {
				r.Status, r.Days, r.TotalDueMinor = "insufficient", periodDays, due
				break
			}
		}
	}
	if r.Status != "adequate" {
		s := fmt.Sprintf("loan-risk-v1:%d:%d:%d:%d:%d:%s:%d:%d:%t", c.ID, c.Version, e.ID, p.Version, days, r.Status, r.EstimatedCapacityMinor, r.TotalDueMinor, unknownObligations)
		for _, o := range obligations {
			s += fmt.Sprintf(":%d:%d:%d:%s", o.CaseID, o.InstallmentID, o.OutstandingMinor, o.DueAt.UTC().Format(time.RFC3339Nano))
		}
		h := sha256.Sum256([]byte(s))
		r.Token = hex.EncodeToString(h[:])
	}
	return r
}
