package eve

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"slices"
	"time"
)

const AssetsReadScope = "esi-assets.read_assets.v1"
const ShipReadScope = "esi-location.read_ship_type.v1"
const LossReadScope = "esi-killmails.read_killmails.v1"

// BattleProof never contains access or refresh tokens. Guard again at publication.
type BattleProof struct {
	CharacterID, Generation int64
	OwnerHash               []byte
	Scopes                  []string
}
type BattleItem struct {
	TypeID    int64  `json:"type_id,string"`
	Name      string `json:"name"`
	Slot      string `json:"slot"`
	Quantity  int64  `json:"quantity"`
	Destroyed int64  `json:"destroyed"`
	Dropped   int64  `json:"dropped"`
}
type BattleFitting struct {
	ShipItemID       int64        `json:"ship_item_id,string"`
	ShipTypeID       int64        `json:"ship_type_id,string"`
	ShipObservedAt   time.Time    `json:"ship_observed_at"`
	AssetsObservedAt time.Time    `json:"assets_observed_at"`
	AssetsContentAt  time.Time    `json:"assets_content_at"`
	Items            []BattleItem `json:"items"`
}
type BattleLoss struct {
	ID, CharacterID, ShipTypeID, SolarSystemID int64
	At                                         time.Time
	Items                                      []BattleItem
}
type BattleLossPage struct {
	Losses []BattleLoss
	More   bool
	Next   time.Time
}

func (s *AuthorizationService) BattleCredential(ctx context.Context, id int64, owner []byte, kind string) (BattleProof, error) {
	p := BattleProof{CharacterID: id, OwnerHash: slices.Clone(owner)}
	if s.esi == nil {
		return p, errESI
	}
	c, err := store.New(s.pool).GetCredential(ctx, id)
	if err != nil {
		return p, err
	}
	p.Generation = c.GrantGeneration
	p.Scopes = []string{LossReadScope}
	if kind == "fitting" {
		p.Scopes = []string{AssetsReadScope, ShipReadScope}
	}
	if subtle.ConstantTimeCompare(c.OwnerHash, owner) != 1 || c.State == "reauthorize" {
		return p, ErrReauthorize
	}
	for _, scope := range p.Scopes {
		if !slices.Contains(c.Scopes, scope) {
			return p, syncFault{Reason: "missing_scope"}
		}
	}
	return p, nil
}
func (s *AuthorizationService) GuardBattle(ctx context.Context, tx pgx.Tx, p BattleProof) error {
	c, err := store.New(tx).GetCredentialForUpdate(ctx, p.CharacterID)
	if err != nil {
		return err
	}
	if c.GrantGeneration != p.Generation || c.State == "reauthorize" || subtle.ConstantTimeCompare(c.OwnerHash, p.OwnerHash) != 1 {
		return ErrReauthorize
	}
	for _, scope := range p.Scopes {
		if !slices.Contains(c.Scopes, scope) {
			return ErrReauthorize
		}
	}
	return nil
}
func battleRequest(p BattleProof, path string) ESIRequest {
	return ESIRequest{Method: "GET", Path: path, CharacterID: p.CharacterID, Generation: p.Generation, Scopes: p.Scopes}
}

// Assets are a cached observation, not an atomic fitting at fleet capture time.
func (s *AuthorizationService) BattleFitting(ctx context.Context, p BattleProof, shipType int64, observed time.Time) (BattleFitting, error) {
	out := BattleFitting{Items: []BattleItem{}}
	if time.Since(observed) > 3*time.Minute {
		return out, syncFault{Reason: "capture_expired"}
	}
	var ship struct {
		ID   int64 `json:"ship_item_id"`
		Type int64 `json:"ship_type_id"`
	}
	r, err := s.esi.Request(ctx, battleRequest(p, fmt.Sprintf("/characters/%d/ship/", p.CharacterID)), &ship)
	if err != nil {
		return out, err
	}
	if ship.ID <= 0 || ship.Type != shipType {
		return out, syncFault{Reason: "ship_changed"}
	}
	out.ShipItemID = ship.ID
	out.ShipTypeID = ship.Type
	out.ShipObservedAt = r.ValidatedAt
	pages := 1
	for page := 1; page <= pages; page++ {
		var assets []struct {
			ID       int64  `json:"item_id"`
			Location int64  `json:"location_id"`
			Type     int64  `json:"type_id"`
			Slot     string `json:"location_flag"`
			Quantity int64  `json:"quantity"`
		}
		req := battleRequest(p, fmt.Sprintf("/characters/%d/assets/?page=%d", p.CharacterID, page))
		req.ExpectPages = true
		a, e := s.esi.Request(ctx, req, &assets)
		if e != nil {
			return out, e
		}
		if a.Pages < 1 || a.Pages > 100 {
			return out, syncFault{Reason: "assets_incomplete"}
		}
		if page == 1 {
			pages = a.Pages
		} else if pages != a.Pages {
			return out, syncFault{Reason: "assets_changed"}
		}
		if out.AssetsObservedAt.IsZero() || a.ValidatedAt.Before(out.AssetsObservedAt) {
			out.AssetsObservedAt = a.ValidatedAt
		}
		if out.AssetsContentAt.IsZero() || a.ContentUpdatedAt.Before(out.AssetsContentAt) {
			out.AssetsContentAt = a.ContentUpdatedAt
		}
		for _, a := range assets {
			if a.Location == ship.ID {
				if a.Type <= 0 || a.Quantity < 0 {
					return out, errESI
				}
				out.Items = append(out.Items, BattleItem{TypeID: a.Type, Slot: a.Slot, Quantity: a.Quantity})
			}
		}
	}
	var after struct {
		ID   int64 `json:"ship_item_id"`
		Type int64 `json:"ship_type_id"`
	}
	if _, err = s.esi.Request(ctx, battleRequest(p, fmt.Sprintf("/characters/%d/ship/", p.CharacterID)), &after); err != nil {
		return out, err
	}
	if after.ID != ship.ID || after.Type != ship.Type {
		return out, syncFault{Reason: "ship_changed"}
	}
	if len(out.Items) == 0 {
		return out, syncFault{Reason: "assets_unavailable"}
	}
	return out, nil
}

func (s *AuthorizationService) BattleLosses(ctx context.Context, p BattleProof, page int, since, until time.Time) (BattleLossPage, error) {
	out := BattleLossPage{Losses: []BattleLoss{}, Next: time.Now().Add(5 * time.Minute)}
	if page < 1 || page > 1000 {
		return out, errESI
	}
	var refs []struct {
		ID   int64  `json:"killmail_id"`
		Hash string `json:"killmail_hash"`
	}
	req := battleRequest(p, fmt.Sprintf("/characters/%d/killmails/recent/?page=%d", p.CharacterID, page))
	req.ExpectPages = true
	r, err := s.esi.Request(ctx, req, &refs)
	if err != nil {
		return out, err
	}
	if r.Pages < 1 || r.Pages > 1000 {
		return out, errESI
	}
	out.More = page < r.Pages
	if r.ExpiresAt.After(out.Next) {
		out.Next = r.ExpiresAt
	}
	for _, ref := range refs {
		// ESI-generated hashes only; never accept a URL or unrestricted path from users.
		if ref.ID <= 0 || len(ref.Hash) != 40 {
			return out, errESI
		}
		for _, c := range ref.Hash {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return out, errESI
			}
		}
		var loss struct {
			ID     int64     `json:"killmail_id"`
			At     time.Time `json:"killmail_time"`
			System int64     `json:"solar_system_id"`
			Victim struct {
				ID    int64      `json:"character_id"`
				Ship  int64      `json:"ship_type_id"`
				Items []killItem `json:"items"`
			} `json:"victim"`
		}
		if _, err = s.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/killmails/%d/%s/", ref.ID, ref.Hash)}, &loss); err != nil {
			return out, err
		}
		if loss.ID != ref.ID || loss.At.IsZero() {
			return out, errESI
		}
		if loss.Victim.ID != p.CharacterID || loss.At.Before(since) || loss.At.After(until) {
			continue
		}
		if loss.Victim.Ship <= 0 || loss.System <= 0 {
			return out, errESI
		}
		items := []BattleItem{}
		flattenKillItems(loss.Victim.Items, "", &items)
		out.Losses = append(out.Losses, BattleLoss{loss.ID, loss.Victim.ID, loss.Victim.Ship, loss.System, loss.At, items})
	}
	return out, nil
}

type killItem struct {
	Type      int64      `json:"item_type_id"`
	Flag      int64      `json:"flag"`
	Destroyed int64      `json:"quantity_destroyed"`
	Dropped   int64      `json:"quantity_dropped"`
	Items     []killItem `json:"items"`
}

func flattenKillItems(items []killItem, parent string, out *[]BattleItem) {
	for _, i := range items {
		slot := fmt.Sprint(i.Flag)
		if parent != "" {
			slot = parent + "/" + slot
		}
		*out = append(*out, BattleItem{TypeID: i.Type, Slot: slot, Quantity: i.Destroyed + i.Dropped, Destroyed: i.Destroyed, Dropped: i.Dropped})
		flattenKillItems(i.Items, slot, out)
	}
}
func BattleFailure(err error) (string, time.Time, bool) {
	if errors.Is(err, ErrReauthorize) || errors.Is(err, pgx.ErrNoRows) {
		return "reauthorize", time.Time{}, true
	}
	var f syncFault
	if errors.As(err, &f) {
		switch f.Reason {
		case "missing_scope", "ship_changed", "capture_expired", "assets_unavailable":
			return f.Reason, time.Time{}, true
		}
		if f.Status == 403 {
			return "forbidden", time.Time{}, true
		}
	}
	var r retryError
	if errors.As(err, &r) {
		return "rate_limited", r.Until, false
	}
	return "esi_unavailable", time.Time{}, false
}
