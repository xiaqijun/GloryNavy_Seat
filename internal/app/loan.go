package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
)

type loanAssetValuation struct {
	total         *big.Rat
	pricedTypes   int
	unpricedTypes int
	cutoff        time.Time
}

// valueLoanAssets converts only positively quoted holdings to ISK minor units.
// ESI's market average table legitimately omits items with no recent trades;
// those holdings remain in the unpriced count and do not erase other values.
func valueLoanAssets(counts map[int64]*big.Int, prices map[int64]eve.MarketPrices, cutoff time.Time) loanAssetValuation {
	result := loanAssetValuation{total: new(big.Rat), cutoff: cutoff}
	ids := make([]int64, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		quote, ok := prices[id]
		if !ok || quote.Mid == nil {
			result.unpricedTypes++
			continue
		}
		price, ok := new(big.Rat).SetString(*quote.Mid)
		if !ok || price.Sign() <= 0 {
			result.unpricedTypes++
			continue
		}
		result.pricedTypes++
		validated := quote.ValidatedAt
		if validated.IsZero() {
			validated = quote.ObservedAt
		}
		if !validated.IsZero() && (result.cutoff.IsZero() || validated.Before(result.cutoff)) {
			result.cutoff = validated
		}
		result.total.Add(result.total, new(big.Rat).Mul(price, new(big.Rat).SetInt(counts[id])))
	}
	return result
}

// loanAssetEvidence aggregates personal asset snapshots for every active bound
// character. It intentionally excludes wallets and corporation assets. A
// character snapshot that cannot be read is recorded as a partial observation;
// holdings with no ESI market average are skipped rather than invalidating all
// other priced holdings. Both cases are retained in the evidence metadata.
func loanAssetEvidence(ctx context.Context, account string, identities *identity.Service, reader *eve.AuthorizationService, pledged func(context.Context, string) (map[int64]int64, error)) (loan.CreditEvidence, error) {
	if reader == nil || reader.ESI() == nil {
		return loan.CreditEvidence{}, eve.ErrReauthorize
	}
	characters, err := identities.ActiveCharacters(ctx, account)
	if err != nil {
		return loan.CreditEvidence{}, err
	}
	if len(characters) == 0 {
		return loan.CreditEvidence{}, eve.ErrReauthorize
	}
	counts := map[int64]*big.Int{}
	var cutoff time.Time
	parts := make([]string, 0, len(characters)+1)
	assetErrors := make([]string, 0)
	snapshotsRead := 0
	for _, character := range characters {
		snapshot, err := reader.AssetSnapshot(ctx, character.ID, character.OwnerHash)
		if err != nil {
			assetErrors = append(assetErrors, fmt.Sprintf("character %d: %v", character.ID, err))
			continue
		}
		snapshotsRead++
		observed := snapshot.ObservedAt
		if !observed.IsZero() && (cutoff.IsZero() || observed.Before(cutoff)) {
			cutoff = observed
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%s", character.ID, snapshot.ObservedAt.UTC().Format(time.RFC3339Nano), snapshot.ContentUpdatedAt.UTC().Format(time.RFC3339Nano)))
		for _, item := range snapshot.Items {
			if item.Quantity <= 0 {
				continue
			}
			quantity, ok := counts[item.TypeID]
			if !ok {
				quantity = new(big.Int)
				counts[item.TypeID] = quantity
			}
			quantity.Add(quantity, big.NewInt(item.Quantity))
		}
	}
	if pledged != nil {
		reserved, err := pledged(ctx, account)
		if err != nil {
			return loan.CreditEvidence{}, err
		}
		for typeID, quantity := range reserved {
			if current := counts[typeID]; current != nil {
				current.Sub(current, big.NewInt(quantity))
				if current.Sign() <= 0 {
					delete(counts, typeID)
				}
			}
		}
	}
	if len(counts) == 0 {
		if cutoff.IsZero() {
			cutoff = time.Now().UTC()
		}
		sort.Strings(parts)
		hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
		evidence := loan.CreditEvidence{AssetAvailable: snapshotsRead > 0, AssetSnapshotID: "assets-v1-" + hex.EncodeToString(hash[:8]), EvidenceCutoff: cutoff}
		if len(assetErrors) > 0 {
			evidence.AssetEvidenceError = strings.Join(assetErrors, "; ")
		}
		if snapshotsRead == 0 {
			return evidence, fmt.Errorf("personal asset snapshots unavailable: %s", evidence.AssetEvidenceError)
		}
		return evidence, nil
	}
	prices, err := reader.ESI().MarketAveragePrices(ctx)
	if err != nil {
		return loan.CreditEvidence{AssetEvidenceError: err.Error()}, err
	}
	total := new(big.Rat)
	valuation := valueLoanAssets(counts, prices, cutoff)
	pricedTypes := valuation.pricedTypes
	unpricedTypes := valuation.unpricedTypes
	total = valuation.total
	cutoff = valuation.cutoff
	// Convert ISK to the application's integer minor-unit representation.
	minorRat := new(big.Rat).Mul(total, big.NewRat(100, 1))
	minor := new(big.Int).Quo(minorRat.Num(), minorRat.Denom())
	if !minor.IsInt64() {
		minor.SetInt64(math.MaxInt64)
	}
	if cutoff.IsZero() {
		cutoff = time.Now().UTC()
	}
	sort.Strings(parts)
	hash := sha256.Sum256([]byte(strings.Join(parts, "|") + fmt.Sprintf("|market:%s", cutoff.UTC().Format(time.RFC3339Nano))))
	evidence := loan.CreditEvidence{
		AssetAvailable:     pricedTypes > 0,
		AssetValueMinor:    minor.Int64(),
		AssetPricedTypes:   pricedTypes,
		AssetUnpricedTypes: unpricedTypes,
		AssetSnapshotID:    "assets-v1-" + hex.EncodeToString(hash[:8]),
		EvidenceCutoff:     cutoff,
	}
	if len(assetErrors) > 0 {
		evidence.AssetEvidenceError = strings.Join(assetErrors, "; ")
	}
	if pricedTypes == 0 {
		if evidence.AssetEvidenceError == "" {
			evidence.AssetEvidenceError = "no personal asset holdings have a usable market average"
		}
		return evidence, fmt.Errorf("%s", evidence.AssetEvidenceError)
	}
	return evidence, nil
}

// loanIncomeEvidence aggregates only the account's current character-wallet
// snapshots. It does not read corporation wallets, account age or auth state as
// score inputs; authorization only determines whether a current snapshot is
// available.
func loanIncomeEvidence(ctx context.Context, account string, from time.Time, identities *identity.Service, reader *eve.AuthorizationService) (loan.IncomeEvidence, error) {
	characters, err := identities.ActiveCharacters(ctx, account)
	if err != nil {
		return loan.IncomeEvidence{}, err
	}
	ids := make([]eve.CreditWalletOwner, 0, len(characters))
	for _, character := range characters {
		ids = append(ids, eve.CreditWalletOwner{ID: character.ID, OwnerHash: character.OwnerHash})
	}
	rows, err := reader.CreditWalletSummaries(ctx, ids, from)
	if err != nil {
		return loan.IncomeEvidence{}, err
	}
	incomeTotal := new(big.Rat)
	expenseTotal := new(big.Rat)
	samples := 0
	var observed time.Time
	for _, row := range rows {
		income, okIncome := new(big.Rat).SetString(row.Income)
		expense, okExpense := new(big.Rat).SetString(row.Expense)
		if !okIncome || !okExpense {
			continue
		}
		if income.Sign() > 0 {
			incomeTotal.Add(incomeTotal, income)
		}
		if expense.Sign() > 0 {
			expenseTotal.Add(expenseTotal, expense)
		}
		if !row.ObservedAt.IsZero() {
			samples++
			if observed.IsZero() || row.ObservedAt.Before(observed) {
				observed = row.ObservedAt
			}
		}
	}
	if samples == 0 {
		return loan.IncomeEvidence{Available: false}, nil
	}
	days := int64(time.Since(from).Hours() / 24)
	if days < 1 {
		days = 1
	}
	monthly := func(total *big.Rat) int64 {
		// Keep income and consumption as separate positive observations. Never
		// subtract the two into a net-income value.
		minor := new(big.Rat).Mul(total, big.NewRat(100, 1))
		minorNum := new(big.Int).Quo(minor.Num(), minor.Denom())
		monthlyMinor := new(big.Int).Mul(minorNum, big.NewInt(30))
		monthlyMinor.Quo(monthlyMinor, big.NewInt(days))
		if !monthlyMinor.IsInt64() {
			monthlyMinor.SetInt64(^int64(0))
		}
		return monthlyMinor.Int64()
	}
	return loan.IncomeEvidence{MonthlyIncomeMinor: monthly(incomeTotal), MonthlyExpenseMinor: monthly(expenseTotal), Samples: samples, Available: true, ObservedAt: observed, SnapshotID: observed.UTC().Format(time.RFC3339)}, nil
}
