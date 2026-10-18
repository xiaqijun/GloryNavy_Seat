package access

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/access/internal/store"
)

// AssignedPermission requires a manual site role. Administrator and game roles
// alone never nominate someone to receive another member's loan contracts.
func (s *Service) AssignedPermission(ctx context.Context, tx pgx.Tx, user, permission string, target Corporation) (bool, error) {
	if !known(permission) {
		return false, nil
	}
	q := store.New(s.pool)
	if tx != nil {
		q = store.New(tx)
		if err := q.LockAssignedRoles(ctx, user); err != nil {
			return false, err
		}
	}
	a, err := s.Account(ctx, user)
	if err != nil || !a.CanViewCorporation(target.ID) {
		return false, err
	}
	id, err := uuid(user)
	if err != nil {
		return false, err
	}
	rows, err := q.UserRoles(ctx, id)
	if err != nil {
		return false, err
	}
	roles, err := decodeRoles(rows)
	if err != nil {
		return false, err
	}
	grants := []Grant{}
	for _, r := range roles {
		grants = append(grants, r.Grants...)
	}
	return Evaluate(false, nil, grants, permission, target, time.Now()), nil
}

func (s *Service) AssignedPermissionMembers(ctx context.Context, permission string, target Corporation) ([]string, error) {
	candidates, err := store.New(s.pool).AssignedPermissionAccounts(ctx, permission)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, user := range candidates {
		ok, err := s.AssignedPermission(ctx, nil, user, permission, target)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, user)
		}
	}
	return out, nil
}
