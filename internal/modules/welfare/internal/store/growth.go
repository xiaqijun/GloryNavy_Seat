package store

import (
	"context"
	"strconv"
)

// Historical policy/audit snapshots identify used legacy projects even after a
// fitting is removed or a project is reconfigured. Cases retain their original hull.
func GrowthState(ctx context.Context, db DB, account, kind string, ship, except int64) (string, error) {
	var state string
	err := db.QueryRow(ctx, `WITH policies AS (
 SELECT kind,config->>'ship_type_id' ship FROM welfare_policies
 UNION SELECT result->>'kind',result->'config'->>'ship_type_id' FROM welfare_audit WHERE action='configure'
 UNION SELECT 'growth_gila','17715' UNION SELECT 'growth_ishtar','12005'
 UNION SELECT 'growth_loki','29990' UNION SELECT 'growth_absolution','22448'
 ), history_used AS (
 SELECT h.key FROM welfare_members m CROSS JOIN LATERAL jsonb_each_text(m.history) h
 WHERE m.account_id=$1::uuid AND h.value='used' AND (h.key=$2 OR h.key='growth_ship_'||$3 OR EXISTS(SELECT 1 FROM policies p WHERE p.kind=h.key AND p.ship=$3))
 ), matching AS (
 SELECT c.state FROM welfare_cases c WHERE c.account_id=$1::uuid AND c.id<>$4 AND c.kind LIKE 'growth_%'
 AND c.state NOT IN ('cancelled','rejected') AND (c.detail->>'ship_type_id'=$3 OR (coalesce(c.detail->>'ship_type_id','0')='0' AND c.kind=$2))
 ) SELECT CASE
 WHEN EXISTS(SELECT 1 FROM matching WHERE state<>'completed') OR EXISTS(SELECT 1 FROM welfare_claims a JOIN welfare_cases c ON c.id=a.case_id WHERE c.account_id=$1::uuid AND c.id<>$4 AND c.state<>'completed' AND (a.claim_key='once:'||$1::text||':'||$2 OR a.claim_key='once:'||$1::text||':growth_ship_'||$3)) THEN 'pending'
 WHEN EXISTS(SELECT 1 FROM matching WHERE state='completed') OR EXISTS(SELECT 1 FROM history_used) THEN 'claimed'
 ELSE '' END`, account, kind, strconv.FormatInt(ship, 10), except).Scan(&state)
	return state, err
}
func GrowthOccupied(ctx context.Context, db DB, account, kind string, ship, except int64) (bool, error) {
	v, e := GrowthState(ctx, db, account, kind, ship, except)
	return v != "", e
}
