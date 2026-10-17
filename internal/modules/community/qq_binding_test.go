package community

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func qqBindingFixture(t *testing.T) (*Service, *pgxpool.Pool, string, string) {
	t.Helper()
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 101, "Binding Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	s := New(pool)
	if _, err = s.Update(ctx, session.UserID, "123456789", "Binding Pilot", "0"); err != nil {
		t.Fatal(err)
	}
	code, _, err := s.CreateQQOfficialChallenge(ctx, session.UserID)
	if err != nil {
		t.Fatal(err)
	}
	return s, pool, session.UserID, code
}

func TestQQBindingCodeSingleUseAndReplay(t *testing.T) {
	s, pool, user, code := qqBindingFixture(t)
	ctx := context.Background()
	body := []byte(`{"id":"first"}`)
	if err := s.ConfirmQQOfficial(ctx, "first", "member-one", code, body); err != nil {
		t.Fatal(err)
	}
	var used bool
	if err := pool.QueryRow(ctx, "SELECT used_at IS NOT NULL FROM community_qq_link_challenges WHERE code_hash=$1", officialChallengeHash(code)).Scan(&used); err != nil || !used {
		t.Fatalf("code consumed=%v: %v", used, err)
	}
	if err := s.ConfirmQQOfficial(ctx, "first", "member-one", code, body); err != nil {
		t.Fatalf("identical event retry: %v", err)
	}
	if err := s.ConfirmQQOfficial(ctx, "second", "member-two", code, []byte(`{"id":"second"}`)); !errors.Is(err, ErrQQChallenge) {
		t.Fatalf("used code rebound identity: %v", err)
	}
	var openid string
	if err := pool.QueryRow(ctx, "SELECT openid FROM community_qq_bindings WHERE user_id=$1", user).Scan(&openid); err != nil || openid != "member-one" {
		t.Fatalf("binding changed: %q %v", openid, err)
	}
	var events, confirmations int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM community_bot_events), (SELECT count(*) FROM community_confirmations WHERE invalidated_at IS NULL)").Scan(&events, &confirmations); err != nil || events != 1 || confirmations != 1 {
		t.Fatalf("audit duplication: %d %d %v", events, confirmations, err)
	}
}

func TestQQBindingCodeConcurrentRecipients(t *testing.T) {
	s, _, _, code := qqBindingFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"one", "two"} {
		wg.Go(func() {
			<-start
			results <- s.ConfirmQQOfficial(context.Background(), id, "member-"+id, code, []byte(id))
		})
	}
	close(start)
	wg.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrQQChallenge) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("success=%d rejected=%d", success, rejected)
	}
}

func TestQQBindingCodeConsumptionRollsBackWithBinding(t *testing.T) {
	s, pool, _, code := qqBindingFixture(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE FUNCTION fail_qq_code_consumption() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test consumption failure'; END $$;
CREATE TRIGGER fail_qq_code_consumption BEFORE UPDATE ON community_qq_link_challenges FOR EACH ROW EXECUTE FUNCTION fail_qq_code_consumption();`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmQQOfficial(ctx, "failure", "member", code, []byte("payload")); err == nil {
		t.Fatal("expected failed transaction")
	}
	var bindings, events, confirmations, consumed int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM community_qq_bindings), (SELECT count(*) FROM community_bot_events), (SELECT count(*) FROM community_confirmations), (SELECT count(*) FROM community_qq_link_challenges WHERE used_at IS NOT NULL)`).Scan(&bindings, &events, &confirmations, &consumed)
	if err != nil || bindings+events+confirmations+consumed != 0 {
		t.Fatalf("partial transaction: %d %d %d %d %v", bindings, events, confirmations, consumed, err)
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER fail_qq_code_consumption ON community_qq_link_challenges"); err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmQQOfficial(ctx, "retry", "member", code, []byte("retry")); err != nil {
		t.Fatal(err)
	}
}

func TestQQBindingCodeRejectsExpiredReplacedAndChangedProfile(t *testing.T) {
	s, pool, user, expired := qqBindingFixture(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "UPDATE community_qq_link_challenges SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmQQOfficial(ctx, "expired", "member", expired, []byte("expired")); !errors.Is(err, ErrQQChallenge) {
		t.Fatalf("expired code: %v", err)
	}
	old, _, err := s.CreateQQOfficialChallenge(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	current, _, err := s.CreateQQOfficialChallenge(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmQQOfficial(ctx, "replaced", "member", old, []byte("replaced")); !errors.Is(err, ErrQQChallenge) {
		t.Fatalf("replaced code: %v", err)
	}
	if _, err = s.Update(ctx, user, "234567890", "Binding Pilot", "1"); err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmQQOfficial(ctx, "changed", "member", current, []byte("changed")); !errors.Is(err, ErrQQChallenge) {
		t.Fatalf("changed profile code: %v", err)
	}
}
