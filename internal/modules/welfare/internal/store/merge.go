package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
)

type mergeConflict struct{}

func (mergeConflict) Error() string { return "welfare qualification reservations overlap" }
func (mergeConflict) MergeBlockReason() string {
	return "福利资格存在重叠，请先核对双方已领取记录并处理待交付预留，再合并账号"
}

func Merge(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	// The participant exists even with UI disabled, but pre-Goose-31 installations
	// must still be able to preview the already delivered account-merge flow.
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT to_regclass('welfare_cases') IS NOT NULL`).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return json.RawMessage(`{"cases":0}`), nil
	}
	if e := Lock(ctx, tx); e != nil {
		return nil, e
	}
	sm, e := Profile(ctx, tx, source)
	if e != nil {
		return nil, e
	}
	tm, e := Profile(ctx, tx, target)
	if e != nil {
		return nil, e
	}
	// Capital pending applications reserve a category even before approval creates claim keys.
	var capitalConflict bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM welfare_cases a JOIN welfare_cases b ON a.kind=b.kind WHERE a.account_id=$1 AND b.account_id=$2 AND a.kind IN ('supercarrier','titan') AND a.state NOT IN ('cancelled','rejected') AND b.state NOT IN ('cancelled','rejected') AND (a.state<>'completed' OR b.state<>'completed'))`, source, target).Scan(&capitalConflict); e != nil {
		return nil, e
	}
	if capitalConflict {
		return nil, mergeConflict{}
	}
	for _, pair := range []struct {
		account string
		history map[string]string
	}{{source, tm.History}, {target, sm.History}} {
		for _, kind := range []string{"supercarrier", "titan"} {
			if pair.history[kind] != "used" {
				continue
			}
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM welfare_cases WHERE account_id=$1 AND kind=$2 AND state NOT IN ('cancelled','rejected','completed'))`, pair.account, kind).Scan(&capitalConflict); e != nil {
				return nil, e
			}
			if capitalConflict {
				return nil, mergeConflict{}
			}
		}
	}
	var out json.RawMessage
	if e := tx.QueryRow(ctx, `SELECT jsonb_build_object('cases',(SELECT count(*) FROM welfare_cases WHERE account_id=$1),'fingerprint',md5(coalesce((SELECT string_agg(to_jsonb(c)::text,'' ORDER BY id) FROM welfare_cases c WHERE account_id IN($1::uuid,$2::uuid)),'')||coalesce((SELECT string_agg(to_jsonb(m)::text,'' ORDER BY account_id) FROM welfare_members m WHERE account_id IN($1::uuid,$2::uuid)),'')))`, source, target).Scan(&out); e != nil {
		return nil, e
	}
	// Check hull-level conflicts across project IDs in both directions, including legacy history.
	for _, pair := range [][2]string{{source, target}, {target, source}} {
		rows, err := tx.Query(ctx, "SELECT id,kind,detail->>'ship_type_id',state FROM welfare_cases WHERE account_id=$1::uuid AND kind LIKE 'growth_%' AND state NOT IN ('cancelled','rejected')", pair[0])
		if err != nil {
			return nil, err
		}
		type hold struct {
			id                int64
			kind, ship, state string
		}
		holds := []hold{}
		for rows.Next() {
			var h hold
			if err = rows.Scan(&h.id, &h.kind, &h.ship, &h.state); err != nil {
				rows.Close()
				return nil, err
			}
			holds = append(holds, h)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		for _, h := range holds {
			ship, err := strconv.ParseInt(h.ship, 10, 64)
			if err != nil {
				return nil, err
			}
			state, err := GrowthState(ctx, tx, pair[1], h.kind, ship, 0)
			if err != nil {
				return nil, err
			}
			if state != "" && (h.state != "completed" || state != "claimed") {
				return nil, mergeConflict{}
			}
		}
	}
	// A pending reservation cannot silently disappear when two verified accounts merge.
	rows, e := tx.Query(ctx, `SELECT a.claim_key,a.case_id,b.case_id,c.state,d.state FROM welfare_claims a JOIN welfare_cases c ON c.id=a.case_id LEFT JOIN welfare_claims b ON b.claim_key=replace(a.claim_key,$1::text,$2::text) LEFT JOIN welfare_cases d ON d.id=b.case_id WHERE a.claim_key LIKE '%'||$1||'%'`, source, target)
	if e != nil {
		return nil, e
	}
	type claim struct {
		key        string
		id         int64
		other      *int64
		state      string
		otherState *string
	}
	claims := []claim{}
	for rows.Next() {
		var c claim
		if e = rows.Scan(&c.key, &c.id, &c.other, &c.state, &c.otherState); e != nil {
			rows.Close()
			return nil, e
		}
		claims = append(claims, c)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, e
	}
	for _, c := range claims {
		if c.other != nil && (c.state != "completed" || c.otherState == nil || *c.otherState != "completed") {
			return nil, mergeConflict{}
		}
		if strings.HasPrefix(c.key, "once:"+source+":") && tm.History[strings.TrimPrefix(c.key, "once:"+source+":")] == "used" && c.state != "completed" {
			return nil, mergeConflict{}
		}
	}
	for kind, state := range sm.History {
		if state != "used" {
			continue
		}
		var pending bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM welfare_claims a JOIN welfare_cases c ON c.id=a.case_id WHERE a.claim_key=$1 AND c.state<>'completed')`, "once:"+target+":"+kind).Scan(&pending); e != nil {
			return nil, e
		}
		if pending {
			return nil, mergeConflict{}
		}
	}
	if !apply {
		return out, nil
	}
	tm.Verified = tm.Verified && sm.Verified
	for k, v := range sm.History {
		if v == "used" {
			tm.History[k] = "used"
		} else if tm.History[k] != "used" && tm.History[k] != v {
			tm.History[k] = "unknown"
		}
	}
	tm.Months = append(tm.Months, sm.Months...)
	slices.Sort(tm.Months)
	tm.Months = slices.Compact(tm.Months)
	if sm.Version > 0 || tm.Version > 0 {
		if e = SaveProfile(ctx, tx, tm); e != nil {
			return nil, e
		}
	}
	for _, c := range claims {
		if _, e = tx.Exec(ctx, `DELETE FROM welfare_claims WHERE claim_key=$1`, c.key); e != nil {
			return nil, e
		}
		if c.other == nil {
			if e = Claim(ctx, tx, strings.ReplaceAll(c.key, source, target), c.id); e != nil {
				return nil, e
			}
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE welfare_cases SET original_account_id=coalesce(original_account_id,account_id),account_id=$2::uuid,claim_keys=ARRAY(SELECT replace(k,$1::text,$2::text) FROM unnest(claim_keys) k) WHERE account_id=$1::uuid`, source, target); e != nil {
		return nil, e
	}
	// Preserve the retired profile as historical evidence. Runtime writes to it are fenced by identity.
	return out, nil
}
