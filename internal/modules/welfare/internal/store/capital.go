package store

import (
	"context"
	"strconv"
)

// Caller holds the welfare publication lock. Pending requests also reserve eligibility.
func CapitalOccupied(ctx context.Context, db DB, account, kind string, contract, except int64) (bool, error) {
	var occupied bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM welfare_cases WHERE id<>$4 AND state NOT IN ('cancelled','rejected') AND ((account_id=$1::uuid AND kind=$2) OR (kind IN ('capital','supercarrier','titan') AND detail->>'contract_id'=$3)))`, account, kind, strconv.FormatInt(contract, 10), except).Scan(&occupied)
	return occupied, err
}
