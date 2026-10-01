package app

import (
	"context"
	"crypto/subtle"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/welfare"
)

func wireWelfareLosses(s *welfare.Service, accounts *identity.Service, reader *eve.AuthorizationService, static *eve.StaticDataService) {
	s.Names = static
	s.SearchItems = static.SearchTypes
	decorate := func(ctx context.Context, rows []welfare.Loss, detail bool) ([]welfare.Loss, error) {
		types, systems, entities := []int64{}, []int64{}, []int64{}
		for i := range rows {
			if !detail {
				rows[i].Items = []eve.BattleItem{}
				rows[i].Attackers = nil
			}
			types = append(types, rows[i].ShipTypeID)
			systems = append(systems, rows[i].SolarSystemID)
			for _, item := range rows[i].Items {
				types = append(types, item.TypeID)
			}
			for _, attacker := range rows[i].Attackers {
				if attacker.ShipTypeID > 0 {
					types = append(types, attacker.ShipTypeID)
				}
				if attacker.WeaponTypeID > 0 {
					types = append(types, attacker.WeaponTypeID)
				}
				if attacker.CharacterID > 0 {
					entities = append(entities, attacker.CharacterID)
				}
				if attacker.CorporationID > 0 {
					entities = append(entities, attacker.CorporationID)
				}
				if attacker.AllianceID > 0 {
					entities = append(entities, attacker.AllianceID)
				}
			}
		}
		names, e := static.TypeNames(ctx, types)
		if e != nil {
			return nil, e
		}
		locations, e := static.SolarSystemNames(ctx, systems)
		if e != nil {
			return nil, e
		}
		people := reader.CachedLossNames(ctx, entities)
		name := func(id int64, m map[int64]eve.StaticTypeName) string {
			if v, ok := m[id]; ok && v.Name != "" {
				return v.Name
			}
			return fmt.Sprint(id)
		}
		for i := range rows {
			rows[i].ShipName = name(rows[i].ShipTypeID, names)
			rows[i].SolarSystemName = name(rows[i].SolarSystemID, locations)
			for j := range rows[i].Items {
				rows[i].Items[j].Name = name(rows[i].Items[j].TypeID, names)
			}
			for j := range rows[i].Attackers {
				a := &rows[i].Attackers[j]
				if a.Name == "" {
					a.Name = people[a.CharacterID]
				}
				if a.CorporationName == "" {
					a.CorporationName = people[a.CorporationID]
				}
				if a.AllianceName == "" {
					a.AllianceName = people[a.AllianceID]
				}
				if a.ShipTypeID > 0 {
					a.ShipName = name(a.ShipTypeID, names)
				}
				if a.WeaponTypeID > 0 {
					a.WeaponName = name(a.WeaponTypeID, names)
				}
			}
		}
		return rows, nil
	}
	s.Losses = func(ctx context.Context, user string, corp, char, before, id int64) ([]welfare.Loss, error) {
		chars, e := s.Characters(ctx, user)
		if e != nil {
			return nil, e
		}
		allowed := false
		for _, c := range chars {
			if c.ID == char && c.CorporationID == corp {
				allowed = true
			}
		}
		if !allowed {
			admin, e := s.Administrator(ctx, user)
			if e != nil {
				return nil, e
			}
			if !admin {
				return nil, pgx.ErrNoRows
			}
			chars, e = s.Members(ctx, user, corp)
			if e != nil {
				return nil, e
			}
			for _, c := range chars {
				if c.ID == char && c.CorporationID == corp {
					allowed = true
				}
			}
		}
		if !allowed {
			return nil, pgx.ErrNoRows
		}
		binding, e := accounts.Bindings(ctx, nil, []int64{char})
		if e != nil {
			return nil, e
		}
		if len(binding) != 1 {
			return nil, pgx.ErrNoRows
		}
		a, e := reader.Get(ctx, char)
		if e != nil {
			return nil, e
		}
		if subtle.ConstantTimeCompare(a.OwnerHash, binding[0].OwnerHash) != 1 {
			return nil, pgx.ErrNoRows
		}
		rows, e := reader.CharacterLosses(ctx, char, before, id)
		if e != nil {
			return nil, e
		}
		return decorate(ctx, rows, id > 0)
	}
	s.LatestLoss = func(ctx context.Context, account string, char, id int64) (*welfare.Loss, error) {
		bindings, e := accounts.Bindings(ctx, nil, []int64{char})
		if e != nil {
			return nil, e
		}
		if len(bindings) != 1 || bindings[0].UserID != account {
			return nil, pgx.ErrNoRows
		}
		credential, e := reader.Get(ctx, char)
		if e != nil {
			return nil, e
		}
		if subtle.ConstantTimeCompare(credential.OwnerHash, bindings[0].OwnerHash) != 1 {
			return nil, pgx.ErrNoRows
		}
		rows, e := reader.CharacterLosses(ctx, char, 0, id)
		if e != nil {
			return nil, e
		}
		if len(rows) != 1 {
			return nil, pgx.ErrNoRows
		}
		rows, e = decorate(ctx, rows, true)
		if e != nil {
			return nil, e
		}
		return &rows[0], nil
	}
	s.GuardLoss = func(ctx context.Context, tx pgx.Tx, user string, char, id int64) error {
		rows, e := accounts.Bindings(ctx, tx, []int64{char})
		if e != nil {
			return e
		}
		if len(rows) != 1 || rows[0].UserID != user {
			return pgx.ErrNoRows
		}
		return reader.GuardCharacterLoss(ctx, tx, char, id, rows[0].OwnerHash)
	}
}
