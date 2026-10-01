package identity

import (
	"context"
	"glorynavy.local/seat/internal/testutil"
	"testing"
)

func TestMainCharacterNames(t *testing.T) {
	ctx := context.Background()
	s := New(testutil.Database(t))
	token, err := s.SignIn(ctx, 101, "Main", "owner-main", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Complete(ctx, 202, "Alt", "owner-alt", token, LoginIntent{Kind: "link", UserID: session.UserID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SignIn(ctx, 303, "Other account", "owner-other", "")
	if err != nil {
		t.Fatal(err)
	}
	names, err := s.MainCharacterNames(ctx, []string{session.UserID})
	if err != nil || len(names) != 1 || names[session.UserID] != "Main" {
		t.Fatalf("main projection: %v %v", names, err)
	}
	if err = s.ChangeCharacter(ctx, token, 202, false, nil); err != nil {
		t.Fatal(err)
	}
	names, err = s.MainCharacterNames(ctx, []string{session.UserID})
	if err != nil || names[session.UserID] != "Alt" {
		t.Fatalf("changed main: %v %v", names, err)
	}
	names, err = s.MainCharacterNames(ctx, nil)
	if err != nil || len(names) != 0 {
		t.Fatalf("empty scope: %v %v", names, err)
	}
}
