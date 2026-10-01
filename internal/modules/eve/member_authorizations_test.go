package eve

import (
	"bytes"
	"context"
	"testing"
	"time"

	"glorynavy.local/seat/internal/testutil"
)

func TestMemberAuthorizationsMatchIndividualOwnership(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	owner := bytes.Repeat([]byte{1}, 32)
	for _, row := range []struct {
		id    int64
		state string
	}{{101, "ready"}, {102, "retry"}, {103, "pending"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO eve_credentials(character_id,owner_hash,sealed,state) VALUES($1,$2,$3,$4)`, row.id, owner, []byte{1}, row.state); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		id      int64
		corp    int64
		expires time.Time
	}{{101, 99, time.Now().Add(time.Hour)}, {102, 100, time.Now().Add(-time.Hour)}} {
		if _, err := pool.Exec(ctx, `INSERT INTO eve_role_snapshots(character_id,owner_hash,corporation_id,corporation_name,ceo_id,roles,roles_at_hq,roles_at_base,roles_at_other,synced_at,valid_until)
			VALUES($1,$2,$3,'Corp',999,'{}','{}','{}','{}',now(),$4)`, row.id, owner, row.corp, row.expires); err != nil {
			t.Fatal(err)
		}
	}
	reader := ReadAuthorization(pool)
	batch, err := reader.MemberAuthorizations(ctx, []int64{101, 102, 103, 104})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 3 {
		t.Fatalf("got %d credentials, want 3", len(batch))
	}
	for _, id := range []int64{101, 102, 103} {
		individual, err := reader.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		got := batch[id]
		if got.CharacterID != individual.CharacterID || !bytes.Equal(got.OwnerHash, individual.OwnerHash) || got.CorporationID != individual.CorporationID || !got.ValidUntil.Equal(individual.ValidUntil) {
			t.Fatalf("batch authorization differs for %d", id)
		}
	}
	if _, found := batch[104]; found {
		t.Fatal("missing credential was treated as an active authorization")
	}
}
