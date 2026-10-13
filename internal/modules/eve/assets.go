package eve

import (
	"context"
	"crypto/subtle"
	"fmt"
	"slices"
	"time"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

const (
	assetSnapshotMaxPages = 1000
	assetSnapshotMaxItems = 100000
)

// AssetHolding is a validated personal asset row returned by ESI. Corporation
// assets are deliberately outside this type and are never requested here.
type AssetHolding struct {
	ItemID       int64
	TypeID       int64
	Quantity     int64
	LocationID   int64
	LocationType string
	LocationFlag string
	BlueprintCopy bool
	Singleton    bool
}

// AssetSnapshot is one complete, owner-bound ESI observation for a character.
// The timestamps are conservative across all pages so callers can use the
// oldest page as the evidence cutoff.
type AssetSnapshot struct {
	CharacterID      int64
	Items            []AssetHolding
	ObservedAt       time.Time
	ContentUpdatedAt time.Time
	ExpiresAt        time.Time
}

// AssetSnapshot reads only the currently authorized character asset endpoint.
// The caller must first establish that id belongs to the intended site account
// and pass the binding owner hash; this method repeats the credential/fence
// check before any ESI request.
func (s *AuthorizationService) AssetSnapshot(ctx context.Context, id int64, owner []byte) (AssetSnapshot, error) {
	out := AssetSnapshot{CharacterID: id, Items: []AssetHolding{}}
	if id <= 0 || len(owner) == 0 || s.esi == nil {
		return out, errESI
	}
	c, err := store.New(s.pool).GetCredential(ctx, id)
	if err != nil {
		return out, err
	}
	if c.State == "reauthorize" || subtle.ConstantTimeCompare(c.OwnerHash, owner) != 1 {
		return out, ErrReauthorize
	}
	if !slices.Contains(c.Scopes, AssetsReadScope) {
		return out, syncFault{Reason: "missing_scope"}
	}
	if c.GrantGeneration <= 0 {
		return out, ErrReauthorize
	}
	pages := 1
	for page := 1; page <= pages; page++ {
		var rows []struct {
			ItemID        int64  `json:"item_id"`
			LocationID    int64  `json:"location_id"`
			LocationType  string `json:"location_type"`
			LocationFlag  string `json:"location_flag"`
			TypeID        int64  `json:"type_id"`
			Quantity      int64  `json:"quantity"`
			BlueprintCopy bool   `json:"is_blueprint_copy"`
			Singleton     bool   `json:"is_singleton"`
		}
		meta, err := s.esi.Request(ctx, ESIRequest{
			Method:      "GET",
			Path:        fmt.Sprintf("/characters/%d/assets/?page=%d", id, page),
			CharacterID: id,
			Generation:  c.GrantGeneration,
			Scopes:      []string{AssetsReadScope},
			ExpectPages: true,
		}, &rows)
		if err != nil {
			return AssetSnapshot{}, err
		}
		if meta.Pages < 1 || meta.Pages > assetSnapshotMaxPages {
			return AssetSnapshot{}, syncFault{Reason: "assets_incomplete"}
		}
		if page == 1 {
			pages = meta.Pages
		} else if pages != meta.Pages {
			return AssetSnapshot{}, syncFault{Reason: "assets_changed"}
		}
		if meta.ValidatedAt.IsZero() {
			return AssetSnapshot{}, errESI
		}
		if out.ObservedAt.IsZero() || meta.ValidatedAt.Before(out.ObservedAt) {
			out.ObservedAt = meta.ValidatedAt
		}
		if !meta.ContentUpdatedAt.IsZero() && (out.ContentUpdatedAt.IsZero() || meta.ContentUpdatedAt.Before(out.ContentUpdatedAt)) {
			out.ContentUpdatedAt = meta.ContentUpdatedAt
		}
		if !meta.ExpiresAt.IsZero() && (out.ExpiresAt.IsZero() || meta.ExpiresAt.Before(out.ExpiresAt)) {
			out.ExpiresAt = meta.ExpiresAt
		}
		if len(rows)+len(out.Items) > assetSnapshotMaxItems {
			return AssetSnapshot{}, syncFault{Reason: "assets_incomplete"}
		}
		for _, row := range rows {
			if row.ItemID <= 0 || row.TypeID <= 0 || row.LocationID <= 0 || row.Quantity < 0 || row.LocationType == "" || row.LocationFlag == "" {
				return AssetSnapshot{}, errESI
			}
			out.Items = append(out.Items, AssetHolding{
				ItemID: row.ItemID, TypeID: row.TypeID, Quantity: row.Quantity,
				LocationID: row.LocationID, LocationType: row.LocationType,
				LocationFlag: row.LocationFlag, BlueprintCopy: row.BlueprintCopy,
				Singleton: row.Singleton,
			})
		}
	}
	return out, nil
}
