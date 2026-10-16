package loan

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

type autoSecurityFixture struct {
	*singleCreditFixture
	borrower, guarantor string
	c                   store.Case
	input               CollateralRequestInput
	price               string
}

func newAutoSecurityFixture(t *testing.T) *autoSecurityFixture {
	f := &autoSecurityFixture{singleCreditFixture: newSingleCreditFixture(t, 10_000_000_000_000), price: "1250000000"}
	f.borrower = f.account()
	f.guarantor = f.account()
	f.credit(f.borrower, 1_000_000_000_000, "active", 0)
	f.credit(f.guarantor, 500_000_000_000, "active", 0)
	f.c = f.application(f.borrower, 1_200_000_000_000)
	if _, err := f.db.Exec(f.ctx, `UPDATE loan_pools SET custodian_character_id=333,config='{"collateral_haircut_bps":7000}' WHERE id=$1`, f.poolID); err != nil {
		t.Fatal(err)
	}
	f.s.AccountForCharacter = func(context.Context, int64) (string, error) { return f.guarantor, nil }
	f.s.AssetChoices = func(_ context.Context, actor string, character int64) (AssetCatalog, error) {
		if actor != f.borrower || character != 111 {
			return AssetCatalog{}, ErrForbidden
		}
		return AssetCatalog{CharacterID: 111, ObservedAt: time.Now(), Items: []AssetChoice{{ItemID: 501, TypeID: 34, Quantity: 10, LocationID: 6001, Items: []AssetItem{{ItemID: 501, TypeID: 34, Quantity: 10, Included: true}}}}}, nil
	}
	f.s.CollateralValue = func(ctx context.Context, raw json.RawMessage) (int64, error) {
		return ValueCollateralItems(ctx, raw, func(context.Context, []int64) (map[int64]string, error) { return map[int64]string{34: f.price}, nil })
	}
	f.input = CollateralRequestInput{CharacterID: 111, CoverageMinor: 200_000_000_000, Items: []AssetSelection{{ItemID: 501, Quantity: 2}}}
	return f
}

func TestAutomaticGuaranteeUsesOnlyUnallocatedExcess(t *testing.T) {
	f := newAutoSecurityFixture(t)
	within := f.application(f.borrower, 900_000_000_000)
	if _, err := f.s.AddGuarantee(f.ctx, f.borrower, within.ID, GuaranteeInput{GuarantorCharacterID: 222}); !errors.Is(err, ErrNoSecurityNeeded) {
		t.Fatal("within credit", err)
	}
	if _, err := f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222, AmountMinor: 300_000_000_000}); !errors.Is(err, ErrConflict) {
		t.Fatal("forged amount", err)
	}
	g, err := f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222})
	if err != nil || g.AmountMinor != 200_000_000_000 {
		t.Fatal("automatic excess", g, err)
	}
	summary, err := f.s.caseSecurity(f.ctx, f.db, f.c, 0)
	if err != nil || summary.EffectiveMinor != 0 || summary.PendingMinor != g.AmountMinor || summary.UnallocatedMinor != 0 {
		t.Fatal(summary, err)
	}
	if _, err = f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222}); !errors.Is(err, ErrNoSecurityNeeded) {
		t.Fatal("duplicate invitation", err)
	}
	if _, err = f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, f.input); !errors.Is(err, ErrNoSecurityNeeded) {
		t.Fatal("duplicate coverage", err)
	}
	if err = f.approve(f.c); !errors.Is(err, ErrCoverage) {
		t.Fatal("pending invitation approved loan", err)
	}
	if _, err = f.s.DecideGuarantee(f.ctx, f.guarantor, g.ID, DecisionInput{State: "rejected", Version: g.Version}); err != nil {
		t.Fatal(err)
	}
	// Existing security combines with credit; only the remaining 1.5B is invited.
	f.collateral(f.c, 50_000_000_000, "approved")
	g, err = f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222})
	if err != nil || g.AmountMinor != 150_000_000_000 {
		t.Fatal("combined coverage", g, err)
	}
	if _, err = f.s.DecideGuarantee(f.ctx, f.guarantor, g.ID, DecisionInput{State: "accepted", Version: g.Version}); err != nil {
		t.Fatal(err)
	}
	if err = f.approve(f.c); err != nil {
		t.Fatal("covered loan", err)
	}
}

func TestCollateralEightTenthsThresholdAndFrozenTarget(t *testing.T) {
	f := newAutoSecurityFixture(t)
	short := f.input
	short.Items = []AssetSelection{{ItemID: 501, Quantity: 1}}
	quote, err := f.s.QuoteCollateralRequest(f.ctx, f.borrower, f.c.ID, short)
	if err != nil || quote.CanSubmit || quote.CoveredMinor != 100_000_000_000 || quote.ShortfallMinor != 100_000_000_000 {
		t.Fatal("insufficient preview", quote, err)
	}
	if _, err = f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, short); !errors.Is(err, ErrCollateralValue) {
		t.Fatal("bypassed preview", err)
	}
	f.price = "1249999999.99"
	if _, err = f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, f.input); !errors.Is(err, ErrCollateralValue) {
		t.Fatal("rounding allowed short collateral", err)
	}
	f.price = ""
	if _, err = f.s.QuoteCollateralRequest(f.ctx, f.borrower, f.c.ID, f.input); !errors.Is(err, ErrCollateralQuote) {
		t.Fatal("missing price", err)
	}
	f.price = "1250000000"
	quote, err = f.s.QuoteCollateralRequest(f.ctx, f.borrower, f.c.ID, f.input)
	if err != nil || !quote.CanSubmit || quote.ValuationMinor != 250_000_000_000 || quote.RequiredMinor != 200_000_000_000 {
		t.Fatal("exact threshold", quote, err)
	}
	request, err := f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, f.input)
	if err != nil || request.CoverageMinor != 200_000_000_000 || request.ValuationMinor != 250_000_000_000 {
		t.Fatal("frozen target", request, err)
	}
	if amount, err := store.Coverage(f.ctx, f.db, f.c.ID); err != nil || amount != 0 {
		t.Fatal("pending delivery effective", amount, err)
	}
	if _, err = f.s.CancelCollateralRequest(f.ctx, f.borrower, request.ID, request.Version); err != nil {
		t.Fatal(err)
	}
	summary, err := f.s.caseSecurity(f.ctx, f.db, f.c, 0)
	if err != nil || summary.UnallocatedMinor != 200_000_000_000 {
		t.Fatal("cancel did not release allocation", summary, err)
	}
	// An updated credit snapshot changes the required amount; the previously
	// displayed target cannot silently bind more collateral.
	f.credit(f.borrower, 900_000_000_000, "active", 0)
	if _, err = f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, f.input); !errors.Is(err, ErrConflict) {
		t.Fatal("stale amount accepted", err)
	}
}

func TestAutomaticSecurityConcurrentChoices(t *testing.T) {
	f := newAutoSecurityFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222, AmountMinor: 200_000_000_000})
		results <- err
	}()
	go func() {
		<-start
		_, err := f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, f.input)
		results <- err
	}()
	close(start)
	a, b := <-results, <-results
	if a != nil {
		a, b = b, a
	}
	if a != nil || !errors.Is(b, ErrNoSecurityNeeded) {
		t.Fatal("double allocation", a, b)
	}
	summary, err := f.s.caseSecurity(f.ctx, f.db, f.c, 0)
	if err != nil || summary.PendingMinor != 200_000_000_000 || summary.UnallocatedMinor != 0 {
		t.Fatal(summary, err)
	}
}

func TestAutomaticSecurityAuthorizationAndMinorUnits(t *testing.T) {
	f := newAutoSecurityFixture(t)
	if _, err := f.s.QuoteCollateralRequest(f.ctx, f.manager, f.c.ID, f.input); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign assets", err)
	}
	f.credit(f.borrower, f.c.PrincipalMinor-1, "active", 0)
	g, err := f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222})
	if err != nil || g.AmountMinor != 1 {
		t.Fatal("one minor unit", g, err)
	}
	// No obsolete invitation may reserve credit after its excess disappeared.
	f.credit(f.borrower, f.c.PrincipalMinor, "active", 0)
	if _, err = f.s.DecideGuarantee(f.ctx, f.guarantor, g.ID, DecisionInput{State: "accepted", Version: g.Version}); !errors.Is(err, ErrConflict) {
		t.Fatal("obsolete guarantee", err)
	}
}

func TestCollateralCombinedItemsAndLegacyReservation(t *testing.T) {
	f := newAutoSecurityFixture(t)
	original := f.s.AssetChoices
	f.s.AssetChoices = func(ctx context.Context, actor string, character int64) (AssetCatalog, error) {
		catalog, err := original(ctx, actor, character)
		catalog.Items = append(catalog.Items, AssetChoice{ItemID: 502, TypeID: 34, Quantity: 1, LocationID: 6001, Items: []AssetItem{{ItemID: 502, TypeID: 34, Quantity: 1, Included: true}}})
		return catalog, err
	}
	input := f.input
	input.Items = []AssetSelection{{ItemID: 501, Quantity: 1}, {ItemID: 502, Quantity: 1}}
	quote, err := f.s.QuoteCollateralRequest(f.ctx, f.borrower, f.c.ID, input)
	if err != nil || !quote.CanSubmit || quote.ValuationMinor != 250_000_000_000 {
		t.Fatal("combined item values", quote, err)
	}
	legacy, err := store.InsertCollateralRequest(f.ctx, f.db, store.CollateralRequest{PublicID: publicID("LNC"), CaseID: f.c.ID, SourceCharacterID: 111, RecipientCharacterID: 333, Selections: json.RawMessage(`[{"item_id":"501","quantity":"1"}]`), Items: json.RawMessage(`[{"item_id":"501","type_id":34,"quantity":1,"included":true}]`), ObservedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	quote, err = f.s.QuoteCollateralRequest(f.ctx, f.borrower, f.c.ID, input)
	if err != nil || quote.CanSubmit || !quote.Security.LegacyPending || quote.Security.EffectiveMinor != 0 {
		t.Fatal("unknown legacy target", quote, err)
	}
	if _, err = f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222}); !errors.Is(err, ErrConflict) {
		t.Fatal("overlapping legacy request", err)
	}
	if _, err = f.s.CancelCollateralRequest(f.ctx, f.borrower, legacy.ID, legacy.Version); err != nil {
		t.Fatal(err)
	}
	f.credit(f.borrower, 1_000_000_000_000, "suspended", 0)
	quote, err = f.s.QuoteCollateralRequest(f.ctx, f.borrower, f.c.ID, input)
	if err != nil || quote.CanSubmit || quote.Security.CreditReady {
		t.Fatal("inactive credit", quote, err)
	}
}
