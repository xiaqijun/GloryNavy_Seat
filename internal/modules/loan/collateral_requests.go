package loan

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

type AssetItem struct {
	ItemID   int64  `json:"item_id,string"`
	TypeID   int64  `json:"type_id,string"`
	Quantity int64  `json:"quantity,string"`
	Name     string `json:"name"`
	Included bool   `json:"included"`
}
type AssetChoice struct {
	ItemID     int64       `json:"item_id,string"`
	TypeID     int64       `json:"type_id,string"`
	Name       string      `json:"name"`
	Quantity   int64       `json:"quantity,string"`
	Available  int64       `json:"available,string"`
	LocationID int64       `json:"location_id,string"`
	Items      []AssetItem `json:"items"`
}
type AssetCatalog struct {
	CharacterID int64         `json:"character_id,string"`
	ObservedAt  time.Time     `json:"observed_at"`
	Items       []AssetChoice `json:"items"`
}
type AssetSelection struct {
	ItemID   int64 `json:"item_id,string"`
	Quantity int64 `json:"quantity,string"`
}
type CollateralRequestInput struct {
	CharacterID int64            `json:"character_id,string"`
	Items       []AssetSelection `json:"items"`
}

func (s *Service) collateralRequestCase(ctx context.Context, actor string, id int64) (store.Case, error) {
	c, err := store.GetCase(ctx, s.db(), id)
	if err != nil {
		return c, err
	}
	if c.BorrowerAccountID != actor {
		return c, ErrForbidden
	}
	if c.State != "submitted" && c.State != "awaiting_acceptance" {
		return c, ErrConflict
	}
	return c, nil
}

func (s *Service) CollateralAssets(ctx context.Context, actor string, caseID, character int64) (AssetCatalog, error) {
	if caseID <= 0 || character <= 0 {
		return AssetCatalog{}, ErrInvalid
	}
	if _, err := s.collateralRequestCase(ctx, actor, caseID); err != nil {
		return AssetCatalog{}, err
	}
	if s.AssetChoices == nil {
		return AssetCatalog{}, ErrRule
	}
	catalog, err := s.AssetChoices(ctx, actor, character)
	if err != nil {
		return AssetCatalog{}, err
	}
	reserved, err := store.ReservedCollateralAssets(ctx, s.db(), character)
	if err != nil {
		return AssetCatalog{}, err
	}
	for i := range catalog.Items {
		catalog.Items[i].Available = max(0, catalog.Items[i].Quantity-reserved[catalog.Items[i].ItemID])
	}
	return catalog, nil
}

func (s *Service) CreateCollateralRequest(ctx context.Context, actor string, caseID int64, in CollateralRequestInput) (store.CollateralRequest, error) {
	if caseID <= 0 || in.CharacterID <= 0 || len(in.Items) == 0 || len(in.Items) > 100 {
		return store.CollateralRequest{}, ErrInvalid
	}
	c, err := s.collateralRequestCase(ctx, actor, caseID)
	if err != nil {
		return store.CollateralRequest{}, err
	}
	pool, err := store.GetPool(ctx, s.db(), c.PoolID)
	if err != nil {
		return store.CollateralRequest{}, err
	}
	if pool.CustodianCharacterID == nil || *pool.CustodianCharacterID == in.CharacterID {
		return store.CollateralRequest{}, ErrRule
	}
	if s.AssetChoices == nil {
		return store.CollateralRequest{}, ErrRule
	}
	// Fetch and name the authoritative asset snapshot before opening a write transaction.
	catalog, err := s.AssetChoices(ctx, actor, in.CharacterID)
	if err != nil {
		return store.CollateralRequest{}, err
	}
	if catalog.CharacterID != in.CharacterID || catalog.ObservedAt.IsZero() {
		return store.CollateralRequest{}, ErrRule
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
			return store.CollateralRequest{}, ErrInvalid
		}
		if location != 0 && location != candidate.LocationID {
			return store.CollateralRequest{}, ErrInvalid
		}
		location = candidate.LocationID
		if len(candidate.Items) > 1 && selection.Quantity != candidate.Quantity {
			return store.CollateralRequest{}, ErrInvalid
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
		return store.CollateralRequest{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.CollateralRequest{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.guardAccounts(ctx, tx, actor); err != nil {
		return store.CollateralRequest{}, err
	}
	locked, err := store.LockCase(ctx, tx, caseID)
	if err != nil {
		return store.CollateralRequest{}, err
	}
	if locked.BorrowerAccountID != actor || locked.State != "submitted" && locked.State != "awaiting_acceptance" {
		return store.CollateralRequest{}, ErrConflict
	}
	if err = store.LockCollateralAssets(ctx, tx, in.CharacterID); err != nil {
		return store.CollateralRequest{}, err
	}
	reserved, err := store.ReservedCollateralAssets(ctx, tx, in.CharacterID)
	if err != nil {
		return store.CollateralRequest{}, err
	}
	for _, v := range in.Items {
		if v.Quantity > candidates[v.ItemID].Quantity-reserved[v.ItemID] {
			return store.CollateralRequest{}, ErrConflict
		}
	}
	selections, _ := json.Marshal(in.Items)
	snapshot, _ := json.Marshal(items)
	out, err := store.InsertCollateralRequest(ctx, tx, store.CollateralRequest{PublicID: publicID("LNC"), CaseID: caseID, SourceCharacterID: in.CharacterID, RecipientCharacterID: *pool.CustodianCharacterID, Selections: selections, Items: snapshot, ObservedAt: catalog.ObservedAt})
	if err != nil {
		return out, err
	}
	if err = store.Audit(ctx, tx, caseID, actor, "collateral_requested", nil, out); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func (s *Service) CancelCollateralRequest(ctx context.Context, actor string, id, version int64) (store.CollateralRequest, error) {
	if id <= 0 || version <= 0 {
		return store.CollateralRequest{}, ErrInvalid
	}
	request, err := store.GetCollateralRequest(ctx, s.db(), id)
	if err != nil {
		return request, err
	}
	c, err := store.GetCase(ctx, s.db(), request.CaseID)
	if err != nil {
		return request, err
	}
	if c.BorrowerAccountID != actor {
		return request, ErrForbidden
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return request, err
	}
	defer tx.Rollback(ctx)
	if err = s.guardAccounts(ctx, tx, actor); err != nil {
		return request, err
	}
	c, err = store.LockCase(ctx, tx, c.ID)
	if err != nil {
		return request, err
	}
	if c.BorrowerAccountID != actor {
		return request, ErrForbidden
	}
	request, err = store.GetCollateralRequest(ctx, tx, id)
	if err != nil {
		return request, err
	}
	if request.State != "awaiting_delivery" || request.Version != version {
		return request, ErrConflict
	}
	if err = store.CancelCollateralRequest(ctx, tx, id, version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrConflict
		}
		return request, err
	}
	out, err := store.GetCollateralRequest(ctx, tx, id)
	if err != nil {
		return request, err
	}
	if err = store.Audit(ctx, tx, c.ID, actor, "collateral_request_cancelled", request, out); err != nil {
		return request, err
	}
	return out, tx.Commit(ctx)
}

func requestMatchesContract(request store.CollateralRequest, contract Contract) bool {
	completed, err := time.Parse(time.RFC3339, contract.Completed)
	if err != nil || completed.Before(request.CreatedAt.Truncate(time.Second)) || completed.After(time.Now().Add(time.Minute)) {
		return false
	}
	if contract.ForCorporation || contract.IssuerID != request.SourceCharacterID || contract.AcceptorID != request.RecipientCharacterID {
		return false
	}
	var raw []struct {
		Included    bool            `json:"included"`
		RawQuantity json.RawMessage `json:"raw_quantity"`
	}
	if json.Unmarshal(contract.Items, &raw) != nil {
		return false
	}
	for _, item := range raw {
		if !item.Included {
			return false
		}
		if q, ok := rawInt64(item.RawQuantity); ok && q == -2 {
			return false
		}
	}
	expected, err := collateralQuantities(request.Items)
	if err != nil {
		return false
	}
	actual, err := collateralQuantities(contract.Items)
	if err != nil || len(actual) != len(expected) {
		return false
	}
	for id, n := range expected {
		if actual[id] != n {
			return false
		}
	}
	return true
}
