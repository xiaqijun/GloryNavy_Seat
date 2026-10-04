package eve

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

const (
	CorporationStarbasesScope  = "esi-corporations.read_starbases.v1"
	CorporationStructuresScope = "esi-corporations.read_structures.v1"
)

// Structure is a read-only projection of an EVE corporation building. The
// payload deliberately keeps game IDs as IDs; names are resolved by SDE or
// the game client and are never guessed by this API.
type Structure struct {
	CorporationID   int64              `json:"corporation_id,string"`
	CorporationName string             `json:"corporation_name"`
	Kind            string             `json:"kind"`
	ID              int64              `json:"id,string"`
	Name            string             `json:"name"`
	TypeID          int64              `json:"type_id,string"`
	TypeName        string             `json:"type_name,omitempty"`
	SolarSystemID   int64              `json:"solar_system_id,string"`
	SolarSystemName string             `json:"solar_system_name,omitempty"`
	State           string             `json:"state"`
	FuelExpires     *time.Time         `json:"fuel_expires,omitempty"`
	ProfileID       int64              `json:"profile_id,string,omitempty"`
	UnanchorsAt     *time.Time         `json:"unanchors_at,omitempty"`
	Services        []StructureService `json:"services,omitempty"`
	Fuel            []StructureFuel    `json:"fuel,omitempty"`
	ObservedAt      time.Time          `json:"observed_at"`
	SourceCharacter int64              `json:"source_character_id,string"`
}

type StructureService struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type StructureFuel struct {
	TypeID   int64 `json:"type_id,string"`
	Quantity int64 `json:"quantity"`
}

type structureListItem struct {
	StructureID   int64              `json:"structure_id"`
	Name          string             `json:"name"`
	TypeID        int64              `json:"type_id"`
	SolarSystemID int64              `json:"solar_system_id"`
	State         string             `json:"state"`
	FuelExpires   *time.Time         `json:"fuel_expires"`
	ProfileID     int64              `json:"profile_id"`
	UnanchorsAt   *time.Time         `json:"unanchors_at"`
	Services      []StructureService `json:"services"`
}

type starbaseListItem struct {
	StarbaseID   int64      `json:"starbase_id"`
	SystemID     int64      `json:"system_id"`
	TypeID       int64      `json:"type_id"`
	State        string     `json:"state"`
	MoonID       int64      `json:"moon_id"`
	OnlinedAt    *time.Time `json:"onlined_at"`
	ReinforcedAt *time.Time `json:"reinforced_until"`
	UnanchorAt   *time.Time `json:"unanchor_at"`
}

type starbaseDetail struct {
	State       string          `json:"state"`
	OnlineSince *time.Time      `json:"online_since"`
	UnanchorAt  *time.Time      `json:"unanchor_at"`
	Fuel        []StructureFuel `json:"fuel"`
}

// ReadCorporationStructures reads the current ESI view for one corporation.
// ESI's shared cache applies the endpoint TTL (currently one hour), so this
// method does not perform a request inside a business write transaction.
func (s *AuthorizationService) ReadCorporationStructures(ctx context.Context, characterID, corporationID int64) ([]Structure, error) {
	if s == nil || s.pool == nil || s.esi == nil || characterID <= 0 || corporationID <= 0 {
		return nil, errors.New("structures unavailable")
	}
	a, err := s.Get(ctx, characterID)
	if err != nil || a.CorporationID != corporationID || (a.State != "ready" && a.State != "retry") || !a.ValidUntil.After(time.Now()) {
		return nil, errors.New("structure authorization unavailable")
	}
	if !slices.Contains(a.Scopes, CorporationStructuresScope) && !slices.Contains(a.Scopes, CorporationStarbasesScope) {
		return nil, errors.New("structure scope missing")
	}
	var generation int64
	if err = s.pool.QueryRow(ctx, "SELECT grant_generation FROM eve_credentials WHERE character_id=$1", characterID).Scan(&generation); err != nil || generation <= 0 {
		return nil, errors.New("structure authorization unavailable")
	}
	observed := time.Now().UTC()
	items := []Structure{}
	if slices.Contains(a.Scopes, CorporationStructuresScope) {
		var rows []structureListItem
		response, e := s.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/corporations/%d/structures/", corporationID), CharacterID: characterID, Generation: generation, Scopes: []string{CorporationStructuresScope}}, &rows)
		if e != nil {
			return nil, e
		}
		if !response.ValidatedAt.IsZero() {
			observed = response.ValidatedAt
		}
		for _, row := range rows {
			if row.StructureID <= 0 {
				continue
			}
			items = append(items, Structure{CorporationID: corporationID, CorporationName: a.CorporationName, Kind: "upwell", ID: row.StructureID, Name: row.Name, TypeID: row.TypeID, SolarSystemID: row.SolarSystemID, State: row.State, FuelExpires: row.FuelExpires, ProfileID: row.ProfileID, UnanchorsAt: row.UnanchorsAt, Services: row.Services, ObservedAt: observed, SourceCharacter: characterID})
		}
	}
	if slices.Contains(a.Scopes, CorporationStarbasesScope) {
		var rows []starbaseListItem
		response, e := s.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/corporations/%d/starbases/", corporationID), CharacterID: characterID, Generation: generation, Scopes: []string{CorporationStarbasesScope}}, &rows)
		if e != nil {
			return nil, e
		}
		if !response.ValidatedAt.IsZero() {
			observed = response.ValidatedAt
		}
		for _, row := range rows {
			if row.StarbaseID <= 0 {
				continue
			}
			fuel := []StructureFuel{}
			state, unanchor := row.State, row.UnanchorAt
			var detail starbaseDetail
			if _, e = s.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/corporations/%d/starbases/%d/", corporationID, row.StarbaseID), CharacterID: characterID, Generation: generation, Scopes: []string{CorporationStarbasesScope}}, &detail); e == nil {
				fuel, state, unanchor = detail.Fuel, detail.State, detail.UnanchorAt
			}
			items = append(items, Structure{CorporationID: corporationID, CorporationName: a.CorporationName, Kind: "pos", ID: row.StarbaseID, TypeID: row.TypeID, SolarSystemID: row.SystemID, State: state, UnanchorsAt: unanchor, Fuel: fuel, ObservedAt: observed, SourceCharacter: characterID})
		}
	}
	if len(items) == 0 && !slices.Contains(a.Scopes, CorporationStructuresScope) && !slices.Contains(a.Scopes, CorporationStarbasesScope) {
		return nil, errors.New("structure scope missing")
	}
	return items, nil
}
