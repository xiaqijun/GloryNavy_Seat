package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

func SeedWalletTargets(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, `INSERT INTO eve_sync_targets(character_id,resource,generation,display_name,state,reason)
 SELECT c.character_id,r.resource,c.grant_generation,coalesce(p.name,''),
 CASE WHEN c.state='reauthorize' OR NOT r.scope=ANY(c.scopes) THEN 'blocked' ELSE 'idle' END,
 CASE WHEN c.state='reauthorize' THEN 'reauthorize' WHEN NOT r.scope=ANY(c.scopes) THEN 'missing_scope' ELSE '' END
 FROM eve_credentials c LEFT JOIN eve_character_profiles p USING(character_id)
 CROSS JOIN (VALUES ('wallet_balance','esi-wallet.read_character_wallet.v1'),('wallet_journal','esi-wallet.read_character_wallet.v1'),('wallet_transactions','esi-wallet.read_character_wallet.v1'),
 ('corporation_wallet_balance','esi-wallet.read_corporation_wallets.v1'),('corporation_wallet_journal','esi-wallet.read_corporation_wallets.v1'),('corporation_wallet_transactions','esi-wallet.read_corporation_wallets.v1'),('corporation_wallet_divisions','esi-corporations.read_divisions.v1')) r(resource,scope)
 ON CONFLICT(character_id,resource) DO UPDATE SET generation=excluded.generation,state=excluded.state,reason=excluded.reason,active_job_id=NULL,lease_until=NULL,fence=eve_sync_targets.fence+1,next_due_at=now(),failures=0
 WHERE eve_sync_targets.generation<>excluded.generation OR eve_sync_targets.reason='module_disabled'`)
	return e
}

// Election is per corporation/resource. Game roles gate the ESI source, not site access.
func WalletSource(ctx context.Context, db DBTX, corp int64, resource, scope string, names bool) (int64, error) {
	var id int64
	e := db.QueryRow(ctx, `SELECT c.character_id FROM eve_credentials c JOIN eve_role_snapshots s USING(character_id)
 JOIN eve_sync_targets t ON t.character_id=c.character_id AND t.resource=$2
 WHERE s.corporation_id=$1 AND s.valid_until>now() AND s.owner_hash=c.owner_hash
 AND c.state IN ('ready','retry') AND (c.roles_not_before IS NULL OR c.roles_not_before<=now())
 AND $3=ANY(c.scopes) AND t.state<>'blocked'
 AND (s.ceo_id=c.character_id OR 'Director'=ANY(s.roles) OR (NOT $4 AND s.roles && ARRAY['Accountant','Junior_Accountant']))
 ORDER BY c.character_id LIMIT 1`, corp, resource, scope, names).Scan(&id)
	return id, e
}
func WalletCursor(ctx context.Context, db DBTX, target, generation, owner int64) (json.RawMessage, error) {
	var b json.RawMessage
	e := db.QueryRow(ctx, `SELECT payload FROM eve_wallet_cursors WHERE target_id=$1 AND generation=$2 AND owner_id=$3`, target, generation, owner).Scan(&b)
	return b, e
}
func SaveWalletCursor(ctx context.Context, db DBTX, target, generation, owner int64, b []byte) error {
	_, e := db.Exec(ctx, `INSERT INTO eve_wallet_cursors VALUES($1,$2,$3,$4) ON CONFLICT(target_id) DO UPDATE SET generation=excluded.generation,owner_id=excluded.owner_id,payload=excluded.payload`, target, generation, owner, b)
	return e
}
func DeleteWalletCursor(ctx context.Context, db DBTX, target int64) error {
	_, e := db.Exec(ctx, `DELETE FROM eve_wallet_cursors WHERE target_id=$1`, target)
	return e
}
func SaveWalletObservation(ctx context.Context, db DBTX, kind string, owner int64, division int, part string, id, source int64, hash []byte, at, occurred any, raw []byte) error {
	_, e := db.Exec(ctx, `INSERT INTO eve_wallet_observations(owner_kind,owner_id,division,kind,record_id,source_character_id,owner_hash,observed_at,occurred_at,payload,entry_key)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,CASE WHEN $4='journal' THEN md5($10::jsonb::text) ELSE '' END) ON CONFLICT(owner_kind,owner_id,division,kind,record_id,entry_key) DO UPDATE SET source_character_id=excluded.source_character_id,owner_hash=excluded.owner_hash,observed_at=excluded.observed_at,occurred_at=excluded.occurred_at,payload=excluded.payload`, kind, owner, division, part, id, source, hash, at, occurred, raw)
	return e
}

type WalletFilter struct {
	Kind, Part, Search, Direction, Ref string
	BeforeEntry                        string
	Owner, Before, Party, ID           int64
	Division                           int
	From, Until                        any
}
type WalletRow struct {
	ID       int64
	Division int
	Payload  json.RawMessage
	Observed string
	EntryKey string
}

type WalletSummaryRow struct {
	OwnerID  int64
	Balance  *string
	Observed *time.Time
	Income   string
	Expense  string
}

type WalletIncomeTrendRow struct {
	Period        string
	Income        string
	Expense       string
	ActiveMembers int
}

type WalletFinanceTrendRow struct {
	Period  string
	Income  string
	Expense string
	Tax     string
	Net     string
}

// ReadCharacterWalletSummaries returns only current credential-owned local
// observations. A record ID can have several historical journal variants;
// only its latest observation contributes to the current monthly total.
func ReadCharacterWalletSummaries(ctx context.Context, db DBTX, ownerIDs []int64, from time.Time) ([]WalletSummaryRow, error) {
	if len(ownerIDs) == 0 {
		return []WalletSummaryRow{}, nil
	}
	balances, e := db.Query(ctx, `SELECT DISTINCT ON (w.owner_id) w.owner_id,w.payload->>'balance',w.observed_at
 FROM eve_wallet_observations w JOIN eve_credentials c ON c.character_id=w.source_character_id
 WHERE w.owner_kind='character' AND w.owner_id=ANY($1::bigint[]) AND w.kind='balance'
 AND w.owner_hash=c.owner_hash AND c.state<>'reauthorize'
 AND 'esi-wallet.read_character_wallet.v1'=ANY(c.scopes)
 ORDER BY w.owner_id,w.observed_at DESC,w.source_character_id DESC`, ownerIDs)
	if e != nil {
		return nil, e
	}
	byID := make(map[int64]*WalletSummaryRow, len(ownerIDs))
	for _, id := range ownerIDs {
		byID[id] = &WalletSummaryRow{OwnerID: id, Income: "0", Expense: "0"}
	}
	for balances.Next() {
		var id int64
		var balance *string
		var observed time.Time
		if e = balances.Scan(&id, &balance, &observed); e != nil {
			balances.Close()
			return nil, e
		}
		if row := byID[id]; row != nil {
			row.Balance, row.Observed = balance, &observed
		}
	}
	e = balances.Err()
	balances.Close()
	if e != nil {
		return nil, e
	}
	journal, e := db.Query(ctx, `WITH latest AS (
 SELECT DISTINCT ON (w.owner_id,w.record_id) w.owner_id,w.record_id,(w.payload->>'amount')::numeric AS amount
 FROM eve_wallet_observations w JOIN eve_credentials c ON c.character_id=w.source_character_id
 WHERE w.owner_kind='character' AND w.owner_id=ANY($1::bigint[]) AND w.kind='journal'
 AND w.occurred_at >= $2 AND w.owner_hash=c.owner_hash AND c.state<>'reauthorize'
 AND 'esi-wallet.read_character_wallet.v1'=ANY(c.scopes)
 ORDER BY w.owner_id,w.record_id,w.observed_at DESC,w.entry_key DESC
 ) SELECT owner_id,COALESCE(sum(greatest(amount,0)),0)::text,
 COALESCE(sum(greatest(-amount,0)),0)::text FROM latest GROUP BY owner_id`, ownerIDs, from)
	if e != nil {
		return nil, e
	}
	for journal.Next() {
		var id int64
		var income, expense string
		if e = journal.Scan(&id, &income, &expense); e != nil {
			journal.Close()
			return nil, e
		}
		if row := byID[id]; row != nil {
			row.Income, row.Expense = income, expense
		}
	}
	e = journal.Err()
	journal.Close()
	if e != nil {
		return nil, e
	}
	out := make([]WalletSummaryRow, 0, len(ownerIDs))
	for _, id := range ownerIDs {
		out = append(out, *byID[id])
	}
	return out, nil
}

// ReadCorporationWalletSummary returns an aggregate over the authorized
// divisions of one corporation. The caller supplies the divisions after the
// object-level wallet permission check; the query still verifies that every
// observation belongs to a current credential with the matching owner hash
// and corporation wallet scope.
func ReadCorporationWalletSummary(ctx context.Context, db DBTX, ownerID int64, divisions []int, from time.Time) (WalletSummaryRow, error) {
	row := WalletSummaryRow{OwnerID: ownerID, Income: "0", Expense: "0"}
	if len(divisions) == 0 {
		return row, nil
	}
	var balance string
	var observed *time.Time
	e := db.QueryRow(ctx, `WITH latest AS (
 SELECT DISTINCT ON (w.division) w.division,
   NULLIF(w.payload->>'balance','')::numeric AS balance,w.observed_at
 FROM eve_wallet_observations w
 JOIN eve_credentials c ON c.character_id=w.source_character_id
 WHERE w.owner_kind='corporation' AND w.owner_id=$1 AND w.division=ANY($2::int[])
   AND w.kind='balance' AND w.owner_hash=c.owner_hash
   AND c.state<>'reauthorize' AND 'esi-wallet.read_corporation_wallets.v1'=ANY(c.scopes)
 ORDER BY w.division,w.observed_at DESC,w.source_character_id DESC
 ) SELECT COALESCE(sum(balance),0)::text,max(observed_at) FROM latest`, ownerID, divisions).Scan(&balance, &observed)
	if e != nil {
		return row, e
	}
	if observed != nil {
		row.Balance = &balance
	}
	row.Observed = observed
	var income, expense string
	e = db.QueryRow(ctx, `WITH latest AS (
 SELECT DISTINCT ON (w.division,w.record_id,w.entry_key)
   NULLIF(w.payload->>'amount','')::numeric AS amount
 FROM eve_wallet_observations w
 JOIN eve_credentials c ON c.character_id=w.source_character_id
 WHERE w.owner_kind='corporation' AND w.owner_id=$1 AND w.division=ANY($2::int[])
   AND w.kind='journal' AND w.occurred_at >= $3 AND w.owner_hash=c.owner_hash
   AND c.state<>'reauthorize' AND 'esi-wallet.read_corporation_wallets.v1'=ANY(c.scopes)
 ORDER BY w.division,w.record_id,w.entry_key,w.observed_at DESC,w.source_character_id DESC
 ) SELECT COALESCE(sum(greatest(amount,0)),0)::text,
          COALESCE(sum(greatest(-amount,0)),0)::text FROM latest`, ownerID, divisions, from).Scan(&income, &expense)
	if e != nil {
		return row, e
	}
	row.Income, row.Expense = income, expense
	return row, nil
}

// ReadCorporationWalletFinanceTrend returns one row per UTC calendar month
// for the authorized corporation wallet divisions. It uses the same latest
// observation and credential-owner checks as the summary endpoint, while
// retaining tax and net values for the operations report.
func ReadCorporationWalletFinanceTrend(ctx context.Context, db DBTX, ownerID int64, divisions []int, from, until time.Time) ([]WalletFinanceTrendRow, error) {
	if len(divisions) == 0 {
		return []WalletFinanceTrendRow{}, nil
	}
	rows, e := db.Query(ctx, `WITH latest AS (
 SELECT DISTINCT ON (w.division,w.record_id,w.entry_key)
   w.occurred_at,
   COALESCE(NULLIF(w.payload->>'amount','')::numeric,0) AS amount,
   GREATEST(COALESCE(NULLIF(w.payload->>'tax','')::numeric,0),0) AS tax
 FROM eve_wallet_observations w
 JOIN eve_credentials c ON c.character_id=w.source_character_id
 WHERE w.owner_kind='corporation' AND w.owner_id=$1 AND w.division=ANY($2::int[])
   AND w.kind='journal' AND w.occurred_at >= $3 AND w.occurred_at < $4
   AND w.owner_hash=c.owner_hash
   AND c.state<>'reauthorize' AND 'esi-wallet.read_corporation_wallets.v1'=ANY(c.scopes)
 ORDER BY w.division,w.record_id,w.entry_key,w.observed_at DESC,w.source_character_id DESC
 )
 SELECT to_char(date_trunc('month',occurred_at AT TIME ZONE 'UTC'),'YYYY-MM') AS period,
        COALESCE(sum(greatest(amount,0)),0)::text,
        COALESCE(sum(greatest(-amount,0)),0)::text,
        COALESCE(sum(tax),0)::text,
        COALESCE(sum(amount),0)::text
 FROM latest GROUP BY 1 ORDER BY 1`, ownerID, divisions, from, until)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []WalletFinanceTrendRow{}
	for rows.Next() {
		var row WalletFinanceTrendRow
		if e = rows.Scan(&row.Period, &row.Income, &row.Expense, &row.Tax, &row.Net); e != nil {
			return nil, e
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ReadCorporationPersonalWalletSummary aggregates readable personal wallet
// snapshots for currently valid characters in one corporation. Credential
// ownership, state and scope are checked in the query so historical data from
// an old owner or revoked credential cannot leak into the dashboard.
func ReadCorporationPersonalWalletSummary(ctx context.Context, db DBTX, corporationID int64, from time.Time, characterIDs []int64) (WalletSummaryRow, error) {
	row := WalletSummaryRow{OwnerID: corporationID, Income: "0", Expense: "0"}
	if len(characterIDs) == 0 {
		return row, nil
	}
	var balance *string
	var observed *time.Time
	e := db.QueryRow(ctx, `WITH eligible AS (
 SELECT c.character_id,c.owner_hash
 FROM eve_credentials c
 JOIN eve_character_profiles p ON p.character_id=c.character_id
 WHERE c.character_id=ANY($2::bigint[]) AND p.corporation_id=$1 AND p.valid_until>now()
   AND c.state<>'reauthorize' AND 'esi-wallet.read_character_wallet.v1'=ANY(c.scopes)
 ), latest_balance AS (
 SELECT DISTINCT ON (w.owner_id) w.owner_id,
   NULLIF(w.payload->>'balance','')::numeric AS balance,w.observed_at
 FROM eve_wallet_observations w JOIN eligible e ON e.character_id=w.source_character_id
 WHERE w.owner_kind='character' AND w.kind='balance' AND w.owner_hash=e.owner_hash
 ORDER BY w.owner_id,w.observed_at DESC,w.source_character_id DESC
 )
 SELECT CASE WHEN count(balance)>0 THEN sum(balance)::text ELSE NULL END,max(observed_at)
 FROM latest_balance`, corporationID, characterIDs).Scan(&balance, &observed)
	if e != nil {
		return row, e
	}
	row.Balance, row.Observed = balance, observed
	e = db.QueryRow(ctx, `WITH eligible AS (
 SELECT c.character_id,c.owner_hash
 FROM eve_credentials c
 JOIN eve_character_profiles p ON p.character_id=c.character_id
 WHERE c.character_id=ANY($3::bigint[]) AND p.corporation_id=$1 AND p.valid_until>now()
   AND c.state<>'reauthorize' AND 'esi-wallet.read_character_wallet.v1'=ANY(c.scopes)
 ), latest AS (
 SELECT DISTINCT ON (w.owner_id,w.record_id) w.owner_id,
   NULLIF(w.payload->>'amount','')::numeric AS amount
 FROM eve_wallet_observations w JOIN eligible e ON e.character_id=w.source_character_id
 WHERE w.owner_kind='character' AND w.kind='journal' AND w.occurred_at >= $2
   AND w.owner_hash=e.owner_hash
 ORDER BY w.owner_id,w.record_id,w.observed_at DESC,w.entry_key DESC
 )
 SELECT COALESCE(sum(greatest(amount,0)),0)::text,
        COALESCE(sum(greatest(-amount,0)),0)::text
 FROM latest`, corporationID, from, characterIDs).Scan(&row.Income, &row.Expense)
	return row, e
}

// ReadCorporationPersonalWalletIncomeTrend aggregates the current local
// observations into UTC calendar months. The caller supplies the currently
// readable character IDs; the query repeats credential/profile checks so a
// stale binding or revoked credential cannot enter a report.
func ReadCorporationPersonalWalletIncomeTrend(ctx context.Context, db DBTX, corporationID int64, from, until time.Time, characterIDs []int64) ([]WalletIncomeTrendRow, error) {
	if len(characterIDs) == 0 {
		return []WalletIncomeTrendRow{}, nil
	}
	rows, e := db.Query(ctx, `WITH eligible AS (
 SELECT c.character_id,c.owner_hash
 FROM eve_credentials c
 JOIN eve_character_profiles p ON p.character_id=c.character_id
 WHERE c.character_id=ANY($3::bigint[]) AND p.corporation_id=$1 AND p.valid_until>now()
   AND c.state<>'reauthorize' AND 'esi-wallet.read_character_wallet.v1'=ANY(c.scopes)
 ), latest AS (
 SELECT DISTINCT ON (w.owner_id,w.record_id,w.entry_key)
   w.owner_id,w.occurred_at,NULLIF(w.payload->>'amount','')::numeric AS amount
 FROM eve_wallet_observations w JOIN eligible e ON e.character_id=w.source_character_id
 WHERE w.owner_kind='character' AND w.kind='journal'
   AND w.occurred_at >= $2 AND w.occurred_at < $4
   AND w.owner_hash=e.owner_hash
 ORDER BY w.owner_id,w.record_id,w.entry_key,w.observed_at DESC
 )
 SELECT to_char(date_trunc('month',occurred_at AT TIME ZONE 'UTC'),'YYYY-MM') AS period,
        COALESCE(sum(greatest(amount,0)),0)::text,
        COALESCE(sum(greatest(-amount,0)),0)::text,
        count(DISTINCT owner_id) FILTER (WHERE amount > 0)::int
 FROM latest GROUP BY 1 ORDER BY 1`, corporationID, from, characterIDs, until)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []WalletIncomeTrendRow{}
	for rows.Next() {
		var row WalletIncomeTrendRow
		if e = rows.Scan(&row.Period, &row.Income, &row.Expense, &row.ActiveMembers); e != nil {
			return nil, e
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func ReadWallet(ctx context.Context, db DBTX, f WalletFilter) ([]WalletRow, error) {
	rows, e := db.Query(ctx, `SELECT w.record_id,w.division,w.payload,to_char(w.observed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),w.entry_key
 FROM eve_wallet_observations w JOIN eve_credentials c ON c.character_id=w.source_character_id
 WHERE w.owner_kind=$1 AND w.owner_id=$2 AND w.kind=$3 AND ($4::int=-1 OR w.division=$4)
 AND w.owner_hash=c.owner_hash AND c.state<>'reauthorize'
 AND (CASE WHEN w.kind='divisions' THEN 'esi-corporations.read_divisions.v1' WHEN w.owner_kind='character' THEN 'esi-wallet.read_character_wallet.v1' ELSE 'esi-wallet.read_corporation_wallets.v1' END)=ANY(c.scopes)
 AND ($5::bigint=0 OR w.record_id<$5 OR (w.record_id=$5 AND $13::text<>'' AND w.entry_key>$13)) AND ($6::bigint=0 OR w.record_id=$6)
 AND ($7::timestamptz IS NULL OR w.occurred_at>=$7) AND ($8::timestamptz IS NULL OR w.occurred_at<$8)
 AND ($9::text='' OR strpos(lower(coalesce(w.payload->>'description','')||' '||coalesce(w.payload->>'reason','')),lower($9))>0 OR w.record_id::text=$9)
 AND ($10::text='' OR w.payload->>'ref_type'=$10)
 AND ($11::bigint=0 OR w.payload->>'first_party_id'=$11::text OR w.payload->>'second_party_id'=$11::text OR w.payload->>'client_id'=$11::text)
 AND ($12::text='' OR ($12='in' AND (w.payload->>'amount')::numeric>0) OR ($12='out' AND (w.payload->>'amount')::numeric<0) OR ($12='buy' AND (w.payload->>'is_buy')::boolean) OR ($12='sell' AND NOT (w.payload->>'is_buy')::boolean))
 ORDER BY w.record_id DESC,w.entry_key,w.division LIMIT 51`, f.Kind, f.Owner, f.Part, f.Division, f.Before, f.ID, f.From, f.Until, f.Search, f.Ref, f.Party, f.Direction, f.BeforeEntry)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []WalletRow{}
	for rows.Next() {
		var r WalletRow
		if e = rows.Scan(&r.ID, &r.Division, &r.Payload, &r.Observed, &r.EntryKey); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
