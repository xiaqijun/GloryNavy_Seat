package identity

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/testutil"
)

func TestMultiCharacterLifecycle(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	ctx := context.Background()
	token, err := s.SignIn(ctx, 101, "Main", "owner-a", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := s.Session(ctx, token)
	intent := LoginIntent{Kind: "link", UserID: session.UserID}
	linked, err := s.Complete(ctx, 202, "Alt", "owner-b", token, intent, nil)
	if err != nil || linked != token {
		t.Fatalf("link: %v", err)
	}
	if _, err = s.Complete(ctx, 202, "Alt", "owner-b", token, intent, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Characters(ctx, session.UserID)
	if len(rows) != 2 || !rows[0].IsMain || rows[0].ID != "101" {
		t.Fatalf("characters: %+v", rows)
	}
	current, _ := s.Session(ctx, token)
	if current.Character.ID != "101" {
		t.Fatal("link changed authenticating character")
	}
	altToken, err := s.SignIn(ctx, 202, "Alt", "owner-b", "")
	if err != nil {
		t.Fatal(err)
	}
	alt, _ := s.Session(ctx, altToken)
	if alt.UserID != session.UserID {
		t.Fatal("alternate created separate user")
	}
	if err = s.ChangeCharacter(ctx, token, 101, true, func(context.Context, pgx.Tx, int64, string) error { return nil }); !errors.Is(err, ErrMain) {
		t.Fatalf("main unlink: %v", err)
	}
	if err = s.ChangeCharacter(ctx, token, 202, false, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Characters(ctx, session.UserID)
	if rows[0].ID != "202" || !rows[0].IsMain {
		t.Fatal("main not changed")
	}
	current, _ = s.Session(ctx, token)
	if current.Character.ID != "101" {
		t.Fatal("main change rewrote session")
	}
	cleaned := false
	if err = s.ChangeCharacter(ctx, token, 101, true, func(_ context.Context, _ pgx.Tx, id int64, user string) error {
		cleaned = id == 101 && user == session.UserID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !cleaned {
		t.Fatal("cleanup not called")
	}
	if current, _ = s.Session(ctx, token); current != nil {
		t.Fatal("unlinked session survived")
	}
	if current, _ = s.Session(ctx, altToken); current == nil {
		t.Fatal("other session revoked")
	}
	if err = s.ChangeCharacter(ctx, altToken, 202, true, func(context.Context, pgx.Tx, int64, string) error { return nil }); !errors.Is(err, ErrMain) {
		t.Fatal("last/main removed")
	}
	var events int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM identity_character_events").Scan(&events)
	if events != 4 {
		t.Fatalf("audit events %d", events)
	}
}

func TestLinkConflictsSessionAndRollback(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	ctx := context.Background()
	token, _ := s.SignIn(ctx, 101, "Main", "owner-a", "")
	session, _ := s.Session(ctx, token)
	otherToken, _ := s.SignIn(ctx, 202, "Other", "owner-b", "")
	intent := LoginIntent{Kind: "link", UserID: session.UserID}
	saved := false
	save := func(context.Context, pgx.Tx) error { saved = true; return nil }
	if _, err := s.Complete(ctx, 202, "Other", "DIFFERENT", token, intent, save); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross user: %v", err)
	}
	if saved {
		t.Fatal("conflict saved credential")
	}
	if other, _ := s.Session(ctx, otherToken); other == nil {
		t.Fatal("conflict revoked other user's session")
	}
	if _, err := s.Complete(ctx, 303, "Alt", "owner-c", token, LoginIntent{Kind: "reauthorize", UserID: session.UserID, ExpectedID: 101}, save); !errors.Is(err, ErrCharacter) {
		t.Fatalf("wrong character: %v", err)
	}
	if _, err := s.Complete(ctx, 303, "Alt", "owner-c", token, intent, func(context.Context, pgx.Tx) error { return errors.New("credential write failed") }); err == nil {
		t.Fatal("failed credential committed")
	}
	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM identity_characters WHERE character_id=303").Scan(&count)
	if count != 0 {
		t.Fatal("partial binding survived")
	}
	if _, err := s.Complete(ctx, 404, "New login", "owner-d", "", LoginIntent{Kind: "login"}, func(context.Context, pgx.Tx) error { return errors.New("failed") }); err == nil {
		t.Fatal("failed login committed")
	}
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM identity_users").Scan(&count)
	if count != 2 {
		t.Fatal("failed login left orphan user")
	}
	_ = s.Logout(ctx, token)
	if _, err := s.Complete(ctx, 303, "Alt", "owner-c", token, intent, save); !errors.Is(err, ErrSession) {
		t.Fatalf("expired session: %v", err)
	}
	if saved {
		t.Fatal("invalid flow saved credential")
	}
}

func TestConcurrentBindingsHaveSingleOwner(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	ctx := context.Background()
	a, _ := s.SignIn(ctx, 101, "A", "a", "")
	b, _ := s.SignIn(ctx, 202, "B", "b", "")
	outcomes := make(chan error, 2)
	var wg sync.WaitGroup
	for _, token := range []string{a, b} {
		wg.Go(func() {
			session, _ := s.Session(ctx, token)
			_, err := s.Complete(ctx, 303, "Alt", "c", token, LoginIntent{Kind: "link", UserID: session.UserID}, nil)
			outcomes <- err
		})
	}
	wg.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestUnlinkRollbackAndLastActiveProtection(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	ctx := context.Background()
	token, _ := s.SignIn(ctx, 101, "Main", "a", "")
	session, _ := s.Session(ctx, token)
	_, err := s.Complete(ctx, 202, "Alt", "b", token, LoginIntent{Kind: "link", UserID: session.UserID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	altToken, _ := s.SignIn(ctx, 202, "Alt", "b", "")
	if err = s.ChangeCharacter(ctx, token, 202, true, func(context.Context, pgx.Tx, int64, string) error { return errors.New("cleanup failed") }); err == nil {
		t.Fatal("cleanup failure ignored")
	}
	if alt, _ := s.Session(ctx, altToken); alt == nil {
		t.Fatal("failed unlink revoked session")
	}
	_, err = s.SignIn(ctx, 101, "Main", "new-owner", "")
	if !errors.Is(err, ErrOwnership) {
		t.Fatal(err)
	}
	if err = s.ChangeCharacter(ctx, altToken, 202, true, func(context.Context, pgx.Tx, int64, string) error { return nil }); !errors.Is(err, ErrLast) {
		t.Fatalf("last active: %v", err)
	}
	if err = s.ChangeCharacter(ctx, altToken, 202, false, nil); err != nil {
		t.Fatal("valid alternate cannot replace blocked main", err)
	}
}
