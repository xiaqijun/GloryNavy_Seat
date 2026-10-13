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

// loanAssetEvidence aggregates complete personal asset snapshots for every
// active bound character. It intentionally excludes wallets, corporation
// assets, and any holding whose market average is unavailable; an incomplete
// observation remains neutral rather than inflating a member's score.
func loanAssetEvidence(ctx context.Context, account string, identities *identity.Service, reader *eve.AuthorizationService) (loan.CreditEvidence, error) {
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
	for _, character := range characters {
		snapshot, err := reader.AssetSnapshot(ctx, character.ID, character.OwnerHash)
		if err != nil {
			return loan.CreditEvidence{}, err
		}
		observed := snapshot.ContentUpdatedAt
		if observed.IsZero() {
			observed = snapshot.ObservedAt
		}
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
	if len(counts) == 0 {
		if cutoff.IsZero() {
			cutoff = time.Now().UTC()
		}
		sort.Strings(parts)
		hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
		return loan.CreditEvidence{AssetAvailable: true, AssetSnapshotID: "assets-v1-" + hex.EncodeToString(hash[:8]), EvidenceCutoff: cutoff}, nil
	}
	prices, err := reader.ESI().MarketAveragePrices(ctx)
	if err != nil {
		return loan.CreditEvidence{}, err
	}
	total := new(big.Rat)
	ids := make([]int64, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		quote, ok := prices[id]
		if !ok || quote.Mid == nil {
			return loan.CreditEvidence{}, fmt.Errorf("asset price unavailable for type %d", id)
		}
		price, ok := new(big.Rat).SetString(*quote.Mid)
		if !ok || price.Sign() <= 0 {
			return loan.CreditEvidence{}, fmt.Errorf("invalid asset price for type %d", id)
		}
		if !quote.ObservedAt.IsZero() && (cutoff.IsZero() || quote.ObservedAt.Before(cutoff)) {
			cutoff = quote.ObservedAt
		}
		total.Add(total, new(big.Rat).Mul(price, new(big.Rat).SetInt(counts[id])))
	}
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
	return loan.CreditEvidence{AssetAvailable: true, AssetValueMinor: minor.Int64(), AssetSnapshotID: "assets-v1-" + hex.EncodeToString(hash[:8]), EvidenceCutoff: cutoff}, nil
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
