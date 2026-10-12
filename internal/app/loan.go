package app

import (
	"context"
	"math/big"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
)

// loanIncomeEvidence aggregates only the account's current character-wallet
// snapshots. It does not read corporation wallets, account age or auth state as
// score inputs; authorization only determines whether a current snapshot is
// available.
func loanIncomeEvidence(ctx context.Context, account string, from time.Time, identities *identity.Service, reader *eve.AuthorizationService) (loan.IncomeEvidence, error) {
	characters, err := identities.ActiveCharacters(ctx, account)
	if err != nil {
		return loan.IncomeEvidence{}, err
	}
	ids := make([]int64, 0, len(characters))
	for _, character := range characters {
		ids = append(ids, character.ID)
	}
	rows, err := reader.WalletSummaries(ctx, ids, from)
	if err != nil {
		return loan.IncomeEvidence{}, err
	}
	net := new(big.Rat)
	samples := 0
	var observed time.Time
	for _, row := range rows {
		income, okIncome := new(big.Rat).SetString(row.Income)
		expense, okExpense := new(big.Rat).SetString(row.Expense)
		if !okIncome || !okExpense {
			continue
		}
		net.Add(net, income)
		net.Sub(net, expense)
		if row.Observed != nil {
			samples++
			if row.Observed.After(observed) {
				observed = *row.Observed
			}
		}
	}
	if samples == 0 {
		return loan.IncomeEvidence{Available: false}, nil
	}
	// Convert the observed window to a conservative monthly minor-unit value.
	minor := new(big.Rat).Mul(net, big.NewRat(100, 1))
	minorNum := new(big.Int).Quo(minor.Num(), minor.Denom())
	if minorNum.Sign() < 0 {
		minorNum.SetInt64(0)
	}
	days := int64(time.Since(from).Hours() / 24)
	if days < 1 {
		days = 1
	}
	monthly := new(big.Int).Mul(minorNum, big.NewInt(30))
	monthly.Quo(monthly, big.NewInt(days))
	if !monthly.IsInt64() {
		monthly.SetInt64(^int64(0))
	}
	return loan.IncomeEvidence{MonthlyNetIncomeMinor: monthly.Int64(), Samples: samples, Available: true, ObservedAt: observed, SnapshotID: observed.UTC().Format(time.RFC3339)}, nil
}
