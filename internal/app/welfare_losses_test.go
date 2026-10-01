package app

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/welfare"
	"glorynavy.local/seat/internal/testutil"
	"testing"
	"time"
)

func TestWelfareLossHostObjectAccess(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	token, e := accounts.SignIn(ctx, 123, "Pilot", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	session, e := accounts.Session(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	// Use real bindings and EVE authorization ownership while keeping corporation policy controlled.
	_, e = pool.Exec(ctx, `INSERT INTO eve_credentials(character_id,owner_hash,sealed,scopes,state) VALUES(123,$1,'test',ARRAY['esi-killmails.read_killmails.v1'],'ready')`, httpapi.Hash("owner"))
	if e != nil {
		t.Fatal(e)
	}
	admin := false
	s := &welfare.Service{Administrator: func(context.Context, string) (bool, error) { return admin, nil }, Characters: func(_ context.Context, user string) ([]welfare.Character, error) {
		if user == session.UserID {
			return []welfare.Character{{ID: 123, CorporationID: 10}}, nil
		}
		return nil, nil
	}, Members: func(context.Context, string, int64) ([]welfare.Character, error) {
		return []welfare.Character{{ID: 123, CorporationID: 10}}, nil
	}}
	wireWelfareLosses(s, accounts, eve.ReadAuthorization(pool), eve.NewStaticData(pool, "", time.Hour))
	if _, e = s.Losses(ctx, session.UserID, 10, 123, 0, 0); e != nil {
		t.Fatal("self read", e)
	}
	if _, e = s.Losses(ctx, "other", 10, 123, 0, 0); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("foreign read", e)
	}
	admin = true
	if _, e = s.Losses(ctx, "other", 10, 123, 0, 0); e != nil {
		t.Fatal("admin read", e)
	}
	admin = false
	if _, e = s.Losses(ctx, "other", 10, 123, 0, 0); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("revoked administrator", e)
	}
	if _, e = s.Losses(ctx, session.UserID, 20, 123, 0, 0); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("wrong corporation", e)
	}
}
