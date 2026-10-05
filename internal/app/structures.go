package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/structures"
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
			typeIDs, systemIDs := []int64{}, []int64{}
			for _, row := range rows {
				typeIDs = append(typeIDs, row.TypeID)
				systemIDs = append(systemIDs, row.SolarSystemID)
			}
			if names != nil && len(rows) > 0 {
				typeNames, _ := names.TypeNames(ctx, typeIDs)
				systemNames, _ := names.SolarSystemNames(ctx, systemIDs)
				for i := range rows {
					if n, ok := typeNames[rows[i].TypeID]; ok {
						rows[i].TypeName = n.Name
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
