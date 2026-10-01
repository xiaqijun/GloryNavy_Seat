package fittings

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve"
)

func IsShipType(id int64) bool  { return references[id].Category == 6 }
func ShipGroup(id int64) string { return references[id].Group }

// SearchShips reuses the local SDE name service and fitting category reference.
func (s *Service) SearchShips(ctx context.Context, q string) ([]eve.StaticTypeName, error) {
	rows, e := s.Search(ctx, q)
	if e != nil {
		return nil, e
	}
	out := []eve.StaticTypeName{}
	for _, r := range rows {
		if IsShipType(r.ID) {
			out = append(out, r)
		}
	}
	return out, nil
}
