package community

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestProfileValidationVersionsAndConfirmationInvalidation(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	identityService := identity.New(pool)
	token, err := identityService.SignIn(ctx, 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := identityService.Session(ctx, token)
	user := session.UserID
	s := New(pool)
	p, err := s.Get(ctx, user)
	if err != nil || p.Complete || p.Version != "0" || p.QQ.Confirmation != "unfilled" {
		t.Fatalf("empty: %+v %v", p, err)
	}
	for _, tc := range []struct {
		qq, kook string
		err      error
	}{{"1234", "Pilot", ErrQQ}, {"012345", "Pilot", ErrQQ}, {"１２３４５", "Pilot", ErrQQ}, {"123456", "", ErrKOOK}, {"123456", "a\nb", ErrKOOK}} {
		if _, err = s.Update(ctx, user, tc.qq, tc.kook, "0"); !errors.Is(err, tc.err) {
			t.Fatalf("validation: %v", err)
		}
	}
	p, err = s.Update(ctx, user, " 123456 ", " 舰长 🚀 ", "0")
	if err != nil || !p.Complete || p.QQ.Confirmation != "pending" || p.KOOK.Value != "舰长 🚀" || p.Version != "1" {
		t.Fatalf("save %+v %v", p, err)
	}
	// Fixtures simulate future trusted confirmations; there is no member confirmation API.
	for _, platform := range []string{"qq", "kook"} {
		_, err = pool.Exec(ctx, "INSERT INTO community_confirmations(user_id,platform,field_version,source,actor,event_id) VALUES($1,$2,1,'test','test-operator',$2)", user, platform)
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err = s.Update(ctx, user, "123456", "舰长 🚀", "1")
	if err != nil || p.Version != "1" || p.QQ.Confirmation != "confirmed" || p.KOOK.Confirmation != "confirmed" {
		t.Fatal("no-op reset confirmation")
	}
	p, err = s.Update(ctx, user, "234567", "舰长 🚀", "1")
	if err != nil || p.Version != "2" || p.QQ.Version != "2" || p.QQ.Confirmation != "pending" || p.KOOK.Version != "1" || p.KOOK.Confirmation != "confirmed" {
		t.Fatalf("targeted invalidation %+v %v", p, err)
	}
	if _, err = s.Update(ctx, user, "123456", "wrong", "1"); !errors.Is(err, ErrVersion) {
		t.Fatal("stale update accepted")
	}
	p, err = s.Update(ctx, user, "234567", "新昵称", "2")
	if err != nil || p.QQ.Version != "2" || p.KOOK.Version != "2" || p.KOOK.Confirmation != "pending" {
		t.Fatal("KOOK change not versioned")
	}
	var invalidated, events int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM community_confirmations WHERE invalidated_at IS NOT NULL").Scan(&invalidated)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM community_profile_events").Scan(&events)
	if invalidated != 2 || events != 3 {
		t.Fatalf("history invalidated=%d changes=%d", invalidated, events)
	}
	// A late confirmation for an older field version cannot confirm the current details.
	_, err = pool.Exec(ctx, "INSERT INTO community_confirmations(user_id,platform,field_version,source,actor,event_id) VALUES($1,'qq',1,'test','test-operator','late')", user)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = s.Get(ctx, user)
	if p.QQ.Confirmation != "pending" {
		t.Fatal("stale event confirmed new details")
	}
	_, err = identityService.Complete(ctx, 202, "Alt", "other", token, identity.LoginIntent{Kind: "link", UserID: user}, nil)
	if err != nil {
		t.Fatal(err)
	}
	altToken, _ := identityService.SignIn(ctx, 202, "Alt", "other", "")
	alt, _ := identityService.Session(ctx, altToken)
	otherView, _ := s.Get(ctx, alt.UserID)
	if otherView != p {
		t.Fatal("alternate did not share profile")
	}
}

func TestConcurrentProfileEditsDoNotOverwrite(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := users.Session(ctx, token)
	s := New(pool)
	outcomes := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"A", "B"} {
		wg.Go(func() { _, err := s.Update(ctx, session.UserID, "123456", name, "0"); outcomes <- err })
	}
	wg.Wait()
	close(outcomes)
	success, conflict := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, ErrVersion) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestOfficialQQBotSignature(t *testing.T) {
	s := New(nil)
	s.SetQQBotOfficial("app-id", "official-secret", "")
	body := []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE","id":"evt-1"}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature, err := s.qqOfficial.ValidationSignature(timestamp, string(body))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.qqOfficial.Verify(timestamp, signature, body, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err = s.qqOfficial.Verify(timestamp, strings.Repeat("0", 128), body, time.Now().UTC()); !errors.Is(err, ErrQQOfficialSignature) {
		t.Fatal("invalid official signature accepted", err)
	}
}
