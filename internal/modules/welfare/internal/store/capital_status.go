package store

import "context"

// CapitalApplication only returns the caller's own active entitlement, across
// corporations, matching the account-wide capital claim rule.
func CapitalApplication(ctx context.Context, db DB, account, kind string) (Case, error) {
	return scan(db.QueryRow(ctx, "SELECT "+cols+" FROM welfare_cases WHERE account_id=$1::uuid AND kind=$2 AND state NOT IN ('cancelled','rejected') ORDER BY id DESC LIMIT 1", account, kind))
}
