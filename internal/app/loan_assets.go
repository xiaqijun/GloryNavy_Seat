package app

import (
	"context"
	"sort"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
)

// Personal asset reads stay behind the owning EVE service. Loan receives only
// named, deliverable roots and their complete nested contents, never credentials.
func loanAssetChoices(ctx context.Context, account string, character int64, identities *identity.Service, reader *eve.AuthorizationService, names *eve.StaticDataService) (loan.AssetCatalog, error) {
	out := loan.AssetCatalog{CharacterID: character, Items: []loan.AssetChoice{}}
	owned, err := identities.ActiveCharacters(ctx, account)
	if err != nil {
		return out, err
	}
	var owner []byte
	for _, c := range owned {
		if c.ID == character {
			owner = c.OwnerHash
			break
		}
	}
	if len(owner) == 0 {
		return out, loan.ErrForbidden
	}
	if reader == nil {
		return out, loan.ErrRule
	}
	snapshot, err := reader.AssetSnapshot(ctx, character, owner)
	if err != nil {
		return out, err
	}
	out.ObservedAt = snapshot.ObservedAt
	byID := map[int64]eve.AssetHolding{}
	children := map[int64][]eve.AssetHolding{}
	ids := []int64{}
	for _, item := range snapshot.Items {
		byID[item.ItemID] = item
		children[item.LocationID] = append(children[item.LocationID], item)
		ids = append(ids, item.TypeID)
	}
	labels, err := names.TypeNames(ctx, ids)
	if err != nil {
		return out, err
	}
	for _, root := range snapshot.Items {
		if _, nested := byID[root.LocationID]; nested || root.LocationFlag != "Hangar" || root.Quantity <= 0 || root.BlueprintCopy {
			continue
		}
		bundle := []loan.AssetItem{}
		seen := map[int64]bool{}
		var collect func(eve.AssetHolding) bool
		collect = func(item eve.AssetHolding) bool {
			if seen[item.ItemID] || item.BlueprintCopy || item.Quantity <= 0 || len(bundle) >= 500 {
				return false
			}
			seen[item.ItemID] = true
			bundle = append(bundle, loan.AssetItem{ItemID: item.ItemID, TypeID: item.TypeID, Quantity: item.Quantity, Name: labels[item.TypeID].Name, Included: true})
			for _, child := range children[item.ItemID] {
				if !collect(child) {
					return false
				}
			}
			return true
		}
		if !collect(root) {
			continue
		}
		out.Items = append(out.Items, loan.AssetChoice{ItemID: root.ItemID, TypeID: root.TypeID, Name: labels[root.TypeID].Name, Quantity: root.Quantity, Available: root.Quantity, LocationID: root.LocationID, Items: bundle})
	}
	sort.Slice(out.Items, func(i, j int) bool {
		if out.Items[i].Name == out.Items[j].Name {
			return out.Items[i].ItemID < out.Items[j].ItemID
		}
		return out.Items[i].Name < out.Items[j].Name
	})
	return out, nil
}
