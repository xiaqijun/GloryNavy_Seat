package store

import "context"

// AssignedPermissionAccounts provides candidates, not an authorization decision.
func (q *Queries) AssignedPermissionAccounts(ctx context.Context, permission string) ([]string, error) {
	rows, err := q.db.Query(ctx, `SELECT DISTINCT a.user_id::text FROM access_assignments a JOIN access_roles r ON r.id=a.role_id WHERE r.grants @> jsonb_build_array(jsonb_build_object('permission',$1::text)) ORDER BY 1`, permission)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (q *Queries) LockAssignedRoles(ctx context.Context, user string) error {
	rows, err := q.db.Query(ctx, `SELECT r.id FROM access_roles r JOIN access_assignments a ON a.role_id=r.id WHERE a.user_id=$1::uuid ORDER BY r.id FOR SHARE OF r,a`, user)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}
