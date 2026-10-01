package eve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"slices"
	"time"
)

const FleetReadScope = "esi-fleets.read_fleet.v1"
const OnlineReadScope = "esi-location.read_online.v1"

// ErrFleetDisbanded means the source character is no longer in a fleet. It is
// distinct from a transient ESI failure so attendance can close the event
// without hiding an outage as a disband.
var ErrFleetDisbanded = errors.New("fleet disbanded")

type FleetMember struct {
	ID            int64     `json:"id,string"`
	Name          string    `json:"name"`
	CorporationID int64     `json:"corporation_id,string"`
	ShipTypeID    int64     `json:"ship_type_id,string"`
	SolarSystemID int64     `json:"solar_system_id,string"`
	JoinedAt      time.Time `json:"joined_at"`
}
type FleetSnapshot struct {
	CharacterID, Generation, FleetID int64
	OwnerHash                        []byte
	ObservedAt                       time.Time
	Members                          []FleetMember
}

// Fleet captures only the current roster. The ESI response, not site roles,
// decides whether the source character may read that fleet.
func (s *AuthorizationService) Fleet(ctx context.Context, id int64, owner []byte) (FleetSnapshot, error) {
	var result FleetSnapshot
	if s.esi == nil {
		return result, errESI
	}
	c, err := store.New(s.pool).GetCredential(ctx, id)
	if err != nil {
		return result, err
	}
	if subtle.ConstantTimeCompare(owner, c.OwnerHash) != 1 {
		return result, ErrReauthorize
	}
	result = FleetSnapshot{CharacterID: id, Generation: c.GrantGeneration, OwnerHash: slices.Clone(owner)}
	var fleet struct {
		ID int64 `json:"fleet_id"`
	}
	req := ESIRequest{Method: "GET", Path: fmt.Sprintf("/characters/%d/fleet/", id), CharacterID: id, Generation: c.GrantGeneration, Scopes: []string{FleetReadScope}}
	if _, err = s.esi.Request(ctx, req, &fleet); err != nil {
		var fault syncFault
		if errors.As(err, &fault) && fault.Status == 404 {
			return result, ErrFleetDisbanded
		}
		return result, err
	}
	if fleet.ID <= 0 {
		return result, ErrFleetDisbanded
	}
	result.FleetID = fleet.ID
	var members []struct {
		ID            int64     `json:"character_id"`
		ShipTypeID    int64     `json:"ship_type_id"`
		SolarSystemID int64     `json:"solar_system_id"`
		JoinedAt      time.Time `json:"join_time"`
	}
	req.Path = fmt.Sprintf("/fleets/%d/members/", fleet.ID)
	response, err := s.esi.Request(ctx, req, &members)
	if err != nil {
		var fault syncFault
		if errors.As(err, &fault) && fault.Status == 404 {
			return result, ErrFleetDisbanded
		}
		return result, err
	}
	if len(members) == 0 || len(members) > 256 {
		return result, errESI
	}
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m.ID <= 0 || slices.Contains(ids, m.ID) {
			return result, errESI
		}
		ids = append(ids, m.ID)
	}
	result.ObservedAt = response.ValidatedAt
	if result.ObservedAt.IsZero() || time.Since(result.ObservedAt) > 2*time.Minute {
		return result, errESI
	}
	result.Members, err = s.ActivityProfiles(ctx, ids)
	for i := range result.Members {
		for _, m := range members {
			if m.ID == result.Members[i].ID {
				result.Members[i].ShipTypeID = m.ShipTypeID
				result.Members[i].SolarSystemID = m.SolarSystemID
				result.Members[i].JoinedAt = m.JoinedAt
			}
		}
	}
	return result, err
}

func (s *AuthorizationService) ActivityProfiles(ctx context.Context, ids []int64) ([]FleetMember, error) {
	if s.esi == nil || len(ids) == 0 || len(ids) > 256 {
		return nil, errESI
	}
	body, _ := json.Marshal(ids)
	var affiliations []struct {
		ID          int64 `json:"character_id"`
		Corporation int64 `json:"corporation_id"`
	}
	if _, err := s.esi.Request(ctx, ESIRequest{Method: "POST", Path: "/characters/affiliation/", Body: body}, &affiliations); err != nil {
		return nil, err
	}
	var names []struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
	}
	if _, err := s.esi.Request(ctx, ESIRequest{Method: "POST", Path: "/universe/names/", Body: body}, &names); err != nil {
		return nil, err
	}
	nameMap := map[int64]string{}
	for _, n := range names {
		if n.Category == "character" {
			nameMap[n.ID] = n.Name
		}
	}
	corps := map[int64]int64{}
	for _, a := range affiliations {
		corps[a.ID] = a.Corporation
	}
	result := make([]FleetMember, 0, len(ids))
	for _, id := range ids {
		if corps[id] <= 0 || nameMap[id] == "" {
			return nil, errESI
		}
		result = append(result, FleetMember{ID: id, Name: nameMap[id], CorporationID: corps[id]})
	}
	return result, nil
}

// GuardFleet must follow identity ownership locks and precede the event lock.
func (s *AuthorizationService) GuardFleet(ctx context.Context, tx pgx.Tx, f FleetSnapshot) error {
	c, err := store.New(tx).GetCredentialForUpdate(ctx, f.CharacterID)
	if err != nil {
		return err
	}
	if c.GrantGeneration != f.Generation || c.State == "reauthorize" || !slices.Contains(c.Scopes, FleetReadScope) || subtle.ConstantTimeCompare(c.OwnerHash, f.OwnerHash) != 1 {
		return ErrReauthorize
	}
	return nil
}
func (s *AuthorizationService) ActivityCorporations(ctx context.Context) ([]int64, error) {
	return store.New(s.pool).ActivityCorporations(ctx)
}
func (s *AuthorizationService) ActivityCharacters(ctx context.Context, corp int64) ([]int64, error) {
	return store.New(s.pool).ActivityCharacters(ctx, corp)
}
