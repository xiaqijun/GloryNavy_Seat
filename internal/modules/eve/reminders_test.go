package eve

import (
	"bytes"
	"context"
	"glorynavy.local/seat/internal/testutil"
	"testing"
)

func TestEveReminderOwnershipGenerationFreshnessAndRecipient(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner := bytes.Repeat([]byte{1}, 32)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO eve_credentials(character_id,owner_hash,sealed,state) VALUES(101,$1,'x','reauthorize')`, owner)
	auth := &AuthorizationService{pool: db}
	keys, e := auth.AuthorizationReminderKeys(ctx, 101, owner)
	if e != nil || len(keys) != 1 {
		t.Fatal(keys, e)
	}
	keys, e = auth.AuthorizationReminderKeys(ctx, 101, bytes.Repeat([]byte{2}, 32))
	if e != nil || len(keys) != 0 {
		t.Fatal("owner leak", keys, e)
	}
	exec(`UPDATE eve_credentials SET state='ready'`)
	exec(`INSERT INTO eve_sync_targets(character_id,resource,generation,state,last_success_at,failures,updated_at) VALUES(101,'character_contracts',1,'idle',now(),0,now())`)
	exec(`INSERT INTO eve_contracts(owner_kind,owner_id,contract_id,source_character_id,source_generation,contract_type,status,payload) VALUES('character',101,123,101,1,'item_exchange','outstanding',jsonb_build_object('assignee_id',101,'date_expired',now()+interval '1 day'))`)
	h := &ContractHTTP{pool: db, LookupOwner: func(context.Context, string, string, int64) (ContractOwner, error) {
		return ContractOwner{Kind: "character", ID: 101}, nil
	}}
	keys, e = h.ContractReminderKeys(ctx, "self", 101, owner)
	if e != nil || len(keys) != 1 {
		t.Fatal("valid contract", keys, e)
	}
	for _, sql := range []string{`UPDATE eve_contracts SET source_generation=2`, `UPDATE eve_contracts SET source_generation=1,payload=jsonb_set(payload,'{assignee_id}','202')`, `UPDATE eve_contracts SET payload=jsonb_set(payload,'{assignee_id}','101'),status='finished'`, `UPDATE eve_contracts SET status='outstanding'; UPDATE eve_sync_targets SET last_success_at=now()-interval '3 hours'`} {
		exec(sql)
		keys, e = h.ContractReminderKeys(ctx, "self", 101, owner)
		if e != nil || len(keys) != 0 {
			t.Fatal("invalid contract", keys, e)
		}
	}
	exec(`UPDATE eve_sync_targets SET state='failed',failures=2`)
	keys, e = auth.SyncReminderKeys(ctx)
	if e != nil || len(keys) != 0 {
		t.Fatal("normal retry alerted", keys, e)
	}
	exec(`UPDATE eve_sync_targets SET failures=3`)
	keys, e = auth.SyncReminderKeys(ctx)
	if e != nil || len(keys) != 1 {
		t.Fatal("persistent failure missing", keys, e)
	}
	exec(`UPDATE eve_credentials SET grant_generation=2`)
	keys, e = auth.SyncReminderKeys(ctx)
	if e != nil || len(keys) != 0 {
		t.Fatal("old generation", keys, e)
	}
}
