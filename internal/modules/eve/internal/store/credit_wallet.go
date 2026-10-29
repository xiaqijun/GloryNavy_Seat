package store

import (
	"context"
	"strconv"
	"time"
)

// CreditWalletRow is a current-generation personal journal observation.
// The caller supplies characters bound to the same account.
type CreditWalletRow struct {
	OwnerID         int64
	Income, Expense string
	ObservedAt      time.Time
}

type CreditWalletOwner struct {
	ID        int64
	OwnerHash []byte
}

func ReadCreditWalletSummaries(ctx context.Context, db DBTX, owners []CreditWalletOwner, from time.Time) ([]CreditWalletRow, error) {
	if len(owners) == 0 {
		return []CreditWalletRow{}, nil
	}
	ids := make([]string, len(owners))
	ownerIDs := make([]int64, len(owners))
	hashes := make([][]byte, len(owners))
	for i, owner := range owners {
		ids[i] = strconv.FormatInt(owner.ID, 10)
		ownerIDs[i], hashes[i] = owner.ID, owner.OwnerHash
	}
	rows, err := db.Query(ctx, `WITH owners AS (
 SELECT c.character_id,t.last_success_at
 FROM unnest($1::bigint[],$4::bytea[]) AS binding(character_id,owner_hash)
 JOIN eve_credentials c ON c.character_id=binding.character_id AND c.owner_hash=binding.owner_hash
 JOIN eve_sync_targets t ON t.character_id=c.character_id AND t.resource='wallet_journal' AND t.generation=c.grant_generation
 WHERE c.state<>'reauthorize' AND 'esi-wallet.read_character_wallet.v1'=ANY(c.scopes) AND t.state<>'blocked' AND t.last_success_at IS NOT NULL
), latest AS (
 SELECT DISTINCT ON(w.owner_id,w.record_id) w.owner_id,w.record_id,(w.payload->>'amount')::numeric amount,w.payload->>'ref_type' ref_type,w.payload->>'first_party_id' first_party,w.payload->>'second_party_id' second_party
 FROM eve_wallet_observations w JOIN eve_credentials c ON c.character_id=w.source_character_id JOIN owners o ON o.character_id=w.owner_id
 WHERE w.owner_kind='character' AND w.kind='journal' AND w.occurred_at >= $2 AND w.owner_hash=c.owner_hash AND c.character_id=w.owner_id
 ORDER BY w.owner_id,w.record_id,w.observed_at DESC,w.entry_key DESC
)
SELECT o.character_id,o.last_success_at,
 COALESCE(sum(greatest(l.amount,0)) FILTER (WHERE (l.ref_type='player_donation' AND l.first_party=ANY($3::text[]) AND l.second_party=ANY($3::text[]) AND l.first_party<>l.second_party) IS NOT TRUE),0)::text,
 COALESCE(sum(greatest(-l.amount,0)) FILTER (WHERE (l.ref_type='player_donation' AND l.first_party=ANY($3::text[]) AND l.second_party=ANY($3::text[]) AND l.first_party<>l.second_party) IS NOT TRUE),0)::text
FROM owners o LEFT JOIN latest l ON l.owner_id=o.character_id GROUP BY o.character_id,o.last_success_at ORDER BY o.character_id`, ownerIDs, from, ids, hashes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CreditWalletRow{}
	for rows.Next() {
		var r CreditWalletRow
		if err := rows.Scan(&r.OwnerID, &r.ObservedAt, &r.Income, &r.Expense); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
