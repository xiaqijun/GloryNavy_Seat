package eve

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"testing"

	"glorynavy.local/seat/internal/testutil"
)

func TestNarrowLoginPreservesExistingAdditionalScopes(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s, err := NewAuthorization(pool, nil, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	broad := Character{ID: 123, Name: "Pilot", Owner: "owner", authorization: &authorization{AccessToken: "old-access", RefreshToken: "old-refresh", Scopes: []string{CorporationRolesScope, MailSendScope, FleetWriteScope}}}
	if err = s.Save(ctx, broad); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE eve_credentials SET state='ready' WHERE character_id=123`); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err = pool.QueryRow(ctx, `SELECT grant_generation FROM eve_credentials WHERE character_id=123`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	narrow := Character{ID: 123, Name: "Pilot", Owner: "owner", authorization: &authorization{AccessToken: "new-access", RefreshToken: "new-refresh", Scopes: []string{CorporationRolesScope}}}
	if err = s.Save(PreserveExistingScopes(ctx), narrow); err != nil {
		t.Fatal(err)
	}
	var after int64
	var scopes []string
	if err = pool.QueryRow(ctx, `SELECT grant_generation,scopes FROM eve_credentials WHERE character_id=123`).Scan(&after, &scopes); err != nil {
		t.Fatal(err)
	}
	if !hasScopes(scopes, []string{MailSendScope, FleetWriteScope}) || after != before {
		t.Fatalf("existing grant was replaced: before=%d after=%d scopes=%v", before, after, scopes)
	}
}

func TestScopeReplacementRejectsIncomparableGrants(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s, err := NewAuthorization(pool, nil, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	old := Character{ID: 124, Name: "Pilot", Owner: "owner", authorization: &authorization{RefreshToken: "old", Scopes: []string{CorporationRolesScope, MailSendScope}}}
	if err = s.Save(ctx, old); err != nil {
		t.Fatal(err)
	}
	newGrant := Character{ID: 124, Name: "Pilot", Owner: "owner", authorization: &authorization{RefreshToken: "new", Scopes: []string{CorporationRolesScope, FleetWriteScope}}}
	if err = s.Save(PreserveExistingScopes(ctx), newGrant); !errors.Is(err, ErrReauthorize) {
		t.Fatalf("incomparable grant: %v", err)
	}
	var scopes []string
	if err = pool.QueryRow(ctx, `SELECT scopes FROM eve_credentials WHERE character_id=124`).Scan(&scopes); err != nil {
		t.Fatal(err)
	}
	if !hasScopes(scopes, []string{MailSendScope}) || hasScopes(scopes, []string{FleetWriteScope}) {
		t.Fatal(scopes)
	}
}

func TestFullGrantSetsRollbackGuard(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s, err := NewAuthorization(pool, nil, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ch := Character{ID: 125, Name: "Pilot", Owner: "owner", authorization: &authorization{RefreshToken: "full", Scopes: AllESIScopes()}}
	if err = s.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	var seen bool
	if err = pool.QueryRow(ctx, `SELECT full_grants_seen FROM eve_full_scope_grant_state WHERE singleton`).Scan(&seen); err != nil || !seen {
		t.Fatalf("guard %v: %v", seen, err)
	}
}
