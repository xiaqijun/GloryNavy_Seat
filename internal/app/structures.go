package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/structures"
	"glorynavy.local/seat/internal/platform/locale"
)

type structureAccounts interface {
	Bindings(context.Context, pgx.Tx, []int64) ([]identity.Binding, error)
}
type structureAccess interface {
	Can(context.Context, string, string, access.Corporation) (bool, error)
	IsAdministrator(context.Context, string) (bool, error)
}
type structureData interface {
	ReadStructureSnapshots(context.Context) ([]eve.StructureSnapshot, error)
}
type structureNames interface {
	TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	SolarSystemNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
}

// posFuelExpiry estimates when a POS fuel bay will run out from the latest
// snapshot. ESI exposes POS quantities but no expiry timestamp. The control
// tower's size determines the standard fuel-block rate; faction reductions
// are derived from the English SDE name. Sovereignty discounts are omitted
// because the snapshot does not carry sovereignty state.
func posFuelExpiry(observed time.Time, towerName string, fuel []eve.StructureFuel, englishFuelNames map[int64]eve.StaticTypeName) *time.Time {
	if observed.IsZero() || len(fuel) == 0 {
		return nil
	}
	name := strings.ToLower(towerName)
	rate := 0.0
	switch {
	case strings.Contains(name, "small control tower") || strings.Contains(name, "control tower small") || strings.Contains(name, "小型控制塔"):
		rate = 10
	case strings.Contains(name, "medium control tower") || strings.Contains(name, "control tower medium") || strings.Contains(name, "中型控制塔"):
		rate = 20
	case strings.Contains(name, "large control tower") || strings.Contains(name, "control tower large") || strings.Contains(name, "大型控制塔"):
		rate = 40
	case strings.Contains(name, "control tower") || strings.Contains(name, "控制塔"):
		// Standard and faction large towers omit the size suffix in SDE names.
		rate = 40
	default:
		return nil
	}
	if strings.Contains(name, "true sansha") || strings.Contains(name, "dark blood") || strings.Contains(name, "dread guristas") || strings.Contains(name, "shadow") || strings.Contains(name, "domination") {
		rate *= 0.8
	} else if strings.Contains(name, "sansha") || strings.Contains(name, "blood") || strings.Contains(name, "guristas") || strings.Contains(name, "serpentis") || strings.Contains(name, "angel") {
		rate *= 0.9
	}
	minimumHours := 0.0
	for _, item := range fuel {
		fuelName := strings.ToLower(englishFuelNames[item.TypeID].Name)
		consumption := 0.0
		switch {
		case strings.Contains(fuelName, "fuel block"):
			consumption = rate
		case strings.Contains(fuelName, "starbase charter"):
			consumption = 1
		default:
			continue
		}
		if item.Quantity <= 0 {
			return nil
		}
		hours := float64(item.Quantity) / consumption
		if minimumHours == 0 || hours < minimumHours {
			minimumHours = hours
		}
	}
	if minimumHours <= 0 {
		return nil
	}
	expires := observed.Add(time.Duration(minimumHours * float64(time.Hour)))
	return &expires
}

func structuresHandler(accounts structureAccounts, acl structureAccess, data structureData, names structureNames) structures.Handler {
	h := structures.Handler{User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	h.Read = func(ctx context.Context, user string, corporationID int64) ([]eve.Structure, error) {
		if data == nil {
			return nil, errors.New("structures unavailable")
		}
		admin, err := acl.IsAdministrator(ctx, user)
		if err != nil {
			return nil, err
		}
		snapshots, err := data.ReadStructureSnapshots(ctx)
		if err != nil {
			return nil, err
		}
		items := []eve.Structure{}
		seen := map[int64]bool{}
		var lastErr error
		validBinding := func(source eve.StructureSource) (bool, error) {
			bindings, e := accounts.Bindings(ctx, nil, []int64{source.CharacterID})
			if e != nil {
				return false, e
			}
			for _, b := range bindings {
				if b.ID == source.CharacterID && len(b.OwnerHash) > 0 && subtle.ConstantTimeCompare(b.OwnerHash, source.OwnerHash) == 1 {
					return true, nil
				}
			}
			return false, nil
		}
		for _, snapshot := range snapshots {
			source := snapshot.Source
			if seen[source.CorporationID] || (corporationID > 0 && corporationID != source.CorporationID) {
				continue
			}
			target := access.Corporation{ID: source.CorporationID, Name: source.CorporationName, AllianceID: source.AllianceID, CEOID: source.CEOID}
			allowed, e := acl.Can(ctx, user, "corporation.structure", target)
			if e != nil {
				return nil, e
			}
			if !allowed {
				continue
			}
			bound := admin
			if !admin {
				bound, e = validBinding(source)
			}
			if e != nil {
				return nil, e
			}
			if !bound {
				continue
			}
			rows := snapshot.Rows
			// Snapshot rows have already been checked against the current grant
			// generation, owner hash, role validity and freshness by the store query.
			// Recheck the viewer binding before returning the local data.
			bound = admin
			if !admin {
				bound, e = validBinding(source)
			}
			if e != nil {
				return nil, e
			}
			allowed, e = acl.Can(ctx, user, "corporation.structure", target)
			if e != nil {
				return nil, e
			}
			if !allowed {
				return nil, pgx.ErrNoRows
			}
			if !bound {
				lastErr = errors.New("structure source binding missing")
				continue
			}
			typeIDs, systemIDs, fuelTypeIDs := []int64{}, []int64{}, []int64{}
			for _, row := range rows {
				typeIDs = append(typeIDs, row.TypeID)
				systemIDs = append(systemIDs, row.SolarSystemID)
				for _, fuel := range row.Fuel {
					fuelTypeIDs = append(fuelTypeIDs, fuel.TypeID)
				}
			}
			if names != nil && len(rows) > 0 {
				typeNames, _ := names.TypeNames(ctx, typeIDs)
				englishTypeNames, _ := names.TypeNames(locale.With(ctx, "en"), typeIDs)
				fuelNames, _ := names.TypeNames(ctx, fuelTypeIDs)
				englishFuelNames, _ := names.TypeNames(locale.With(ctx, "en"), fuelTypeIDs)
				systemNames, _ := names.SolarSystemNames(ctx, systemIDs)
				for i := range rows {
					if n, ok := typeNames[rows[i].TypeID]; ok {
						rows[i].TypeName = n.Name
					}
					for j := range rows[i].Fuel {
						if n, ok := fuelNames[rows[i].Fuel[j].TypeID]; ok {
							rows[i].Fuel[j].Name = n.Name
						}
					}
					// An offline control tower keeps its fuel inventory but does not
					// consume fuel, so there is no meaningful depletion deadline.
					if rows[i].Kind == "pos" {
						if strings.EqualFold(strings.TrimSpace(rows[i].State), "online") {
							rows[i].FuelExpires = posFuelExpiry(rows[i].ObservedAt, englishTypeNames[rows[i].TypeID].Name, rows[i].Fuel, englishFuelNames)
						} else {
							rows[i].FuelExpires = nil
						}
					}
					if n, ok := systemNames[rows[i].SolarSystemID]; ok {
						rows[i].SolarSystemName = n.Name
					}
				}
			}
			seen[source.CorporationID] = true
			items = append(items, rows...)
		}
		if len(items) == 0 && lastErr != nil {
			return nil, lastErr
		}
		if corporationID > 0 && !seen[corporationID] {
			return nil, pgx.ErrNoRows
		}
		return items, nil
	}
	return h
}
