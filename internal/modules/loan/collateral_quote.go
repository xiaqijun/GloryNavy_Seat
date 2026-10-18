package loan

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

type preparedCollateral struct {
	c          store.Case
	pool       store.Pool
	officer    Character
	catalog    AssetCatalog
	candidates map[int64]AssetChoice
	items      []AssetItem
}

type CollateralQuote struct {
	Security       SecuritySummary `json:"security"`
	ValuationMinor int64           `json:"valuation_minor"`
	CoveredMinor   int64           `json:"covered_minor"`
	RequiredMinor  int64           `json:"required_minor"`
	ShortfallMinor int64           `json:"shortfall_minor"`
	CanSubmit      bool            `json:"can_submit"`
}

func (s *Service) prepareCollateralSelection(ctx context.Context, actor string, caseID int64, in CollateralRequestInput) (preparedCollateral, error) {
	if caseID <= 0 || in.CharacterID <= 0 || len(in.Items) == 0 || len(in.Items) > 100 {
		return preparedCollateral{}, ErrInvalid
	}
	c, err := s.collateralRequestCase(ctx, actor, caseID)
	if err != nil {
		return preparedCollateral{}, err
	}
	pool, err := store.GetPool(ctx, s.db(), c.PoolID)
	if err != nil {
		return preparedCollateral{}, err
	}
	officer, err := s.chooseOfficer(ctx, pool, in.RecipientCharacterID)
	if err != nil {
		return preparedCollateral{}, err
	}
	if officer.ID == in.CharacterID {
		return preparedCollateral{}, ErrRule
	}
	if s.AssetChoices == nil {
		return preparedCollateral{}, ErrRule
	}
	// Fetch and name the authoritative asset snapshot before opening a write transaction.
	catalog, err := s.AssetChoices(ctx, actor, in.CharacterID)
	if err != nil {
		return preparedCollateral{}, err
	}
	if catalog.CharacterID != in.CharacterID || catalog.ObservedAt.IsZero() {
		return preparedCollateral{}, ErrRule
	}
	reserved, err := store.ReservedCollateralAssets(ctx, s.db(), in.CharacterID)
	if err != nil {
		return preparedCollateral{}, err
	}
	candidates := map[int64]AssetChoice{}
	for _, v := range catalog.Items {
		candidates[v.ItemID] = v
	}
	seen := map[int64]bool{}
	items := []AssetItem{}
	var location int64
	for _, selection := range in.Items {
		candidate, ok := candidates[selection.ItemID]
		if !ok || seen[selection.ItemID] || selection.Quantity <= 0 || selection.Quantity > candidate.Quantity || len(candidate.Items) == 0 {
			return preparedCollateral{}, ErrInvalid
		}
		if selection.Quantity > candidate.Quantity-reserved[candidate.ItemID] {
			return preparedCollateral{}, ErrConflict
		}
		if location != 0 && location != candidate.LocationID {
			return preparedCollateral{}, ErrInvalid
		}
		location = candidate.LocationID
		if len(candidate.Items) > 1 && selection.Quantity != candidate.Quantity {
			return preparedCollateral{}, ErrInvalid
		}
		seen[selection.ItemID] = true
		for _, item := range candidate.Items {
			if item.ItemID == selection.ItemID {
				item.Quantity = selection.Quantity
			}
			items = append(items, item)
		}
	}
	if len(items) > 500 {
		return preparedCollateral{}, ErrInvalid
	}
	return preparedCollateral{c: c, pool: pool, officer: officer, catalog: catalog, candidates: candidates, items: items}, nil
}

func (s *Service) selectionValue(ctx context.Context, items []AssetItem) (int64, error) {
	if s.CollateralValue == nil {
		return 0, ErrCollateralQuote
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return 0, err
	}
	value, err := s.CollateralValue(ctx, raw)
	if err != nil || value <= 0 || value > 9007199254740991 {
		return 0, ErrCollateralQuote
	}
	return value, nil
}

func (s *Service) QuoteCollateralRequest(ctx context.Context, actor string, caseID int64, in CollateralRequestInput) (CollateralQuote, error) {
	prepared, err := s.prepareCollateralSelection(ctx, actor, caseID, in)
	if err != nil {
		return CollateralQuote{}, err
	}
	security, err := s.caseSecurity(ctx, s.db(), prepared.c, 0)
	if err != nil {
		return CollateralQuote{}, err
	}
	value, err := s.selectionValue(ctx, prepared.items)
	if err != nil {
		return CollateralQuote{}, err
	}
	covered := coveredByHaircut(value, collateralAdvanceBPS)
	return CollateralQuote{Security: security, ValuationMinor: value, CoveredMinor: covered, RequiredMinor: security.UnallocatedMinor, ShortfallMinor: max(0, security.UnallocatedMinor-covered), CanSubmit: security.CreditReady && !security.LegacyPending && security.UnallocatedMinor > 0 && covered >= security.UnallocatedMinor}, nil
}
