package access

import (
	"context"
	"slices"
	"time"
)

// CanViewCorporation is the outer scope boundary, independent of business
// abilities. Global/manual grants cannot expand it. Character facts are supplied
// by the host after verifying active bindings and EVE ownership.
func (a Account) CanViewCorporation(id int64) bool {
	if id <= 0 {
		return false
	}
	if !a.restrictCorporations {
		return true
	}
	if !slices.Contains(a.corporations, id) {
		return false
	}
	if a.Administrator {
		return true
	}
	for _, f := range a.Characters {
		if f.Corporation.ID == id && Fresh(f, time.Now()) {
			return true
		}
	}
	return false
}

func (s *Service) CanViewCorporation(ctx context.Context, user string, id int64) (bool, error) {
	a, err := s.Account(ctx, user)
	if err != nil {
		return false, err
	}
	return a.CanViewCorporation(id), nil
}
