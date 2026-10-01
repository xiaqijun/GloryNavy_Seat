package fittings

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/testutil"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

type libraryNames struct{}

func (libraryNames) TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return map[int64]eve.StaticTypeName{}, nil
}

func TestLibraryEFTAndRequirements(t *testing.T) {
	f, e := parseEFT("[Rifter, 集结护卫]\n[Empty low slot]\nDamage Control II /offline\n200mm AutoCannon II, Barrage S\nBarrage S x1000\nDamage Control II x2")
	if e != nil {
		t.Fatal(e)
	}
	p, e := gamePayload(f, "集结")
	if e != nil {
		t.Fatal(e)
	}
	if p.Items[0].Flag != "LoSlot1" || p.Items[0].Quantity != 1 || p.Items[2].Flag != "Cargo" || p.Items[2].Quantity != 1000 {
		t.Fatal(p)
	}
	round, e := parseEFT(exportEFT(f))
	if e != nil || !reflect.DeepEqual(f, round) {
		t.Fatal("round trip", f, round, e)
	}
	req, e := prerequisites(f)
	if e != nil {
		t.Fatal(e)
	}
	levels := map[int64]int{}
	for _, r := range req {
		if levels[r.ID] > 0 {
			t.Fatal("duplicate")
		}
		levels[r.ID] = r.Level
	}
	// Small autocannon specialization recursively requires Gunnery and Small Projectile Turret V.
	if levels[3300] < 1 || levels[3302] != 5 || levels[3329] < 1 {
		t.Fatal("missing transitive prerequisites", levels)
	}
	bare, _ := parseEFT("[Rifter, Test]\nDamage Control II x2")
	bareReq, e := prerequisites(bare)
	if e != nil {
		t.Fatal(e)
	}
	shipReq, e := prerequisites(Fit{ShipTypeID: 587})
	if e != nil || !reflect.DeepEqual(bareReq, shipReq) {
		t.Fatal("spare cargo included")
	}
	for _, text := range []string{"[Unknown, Test]\nDamage Control II", "[Rifter, Test]\nUnknown Module", "[Rifter, Test]\nDamage Control II x0"} {
		if _, e := parseEFT(text); e == nil {
			t.Fatal("invalid EFT accepted", text)
		}
	}
	noAmmo, _ := parseEFT("[Rifter, Test]\n200mm AutoCannon II, Barrage S")
	if _, e := gamePayload(noAmmo, ""); e == nil {
		t.Fatal("ammo quantity invented")
	}
}

func TestLibraryPermissionsVersionAndGameSave(t *testing.T) {
	ctx := context.Background()
	administrator := true
	s := &Service{Pool: testutil.Database(t), Names: libraryNames{}}
	s.Administrator = func(_ context.Context, u string) (bool, error) { return u == admin && administrator, nil }
	s.LibraryCorporations = func(_ context.Context, u string) ([]LibraryCorporation, error) {
		if u == other {
			return nil, nil
		}
		return []LibraryCorporation{{ID: 900}}, nil
	}
	s.Own = func(_ context.Context, u string) ([]Character, error) {
		if u == owner {
			return []Character{{ID: 123}, {ID: 124}, {ID: 125}}, nil
		}
		return nil, nil
	}
	c := LibraryEdit{CorporationID: 900, RequestKey: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", EFT: "[Rifter, Test]\nDamage Control II"}
	if _, e := s.LibraryImport(ctx, owner, 0, c, false); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("member import", e)
	}
	v, e := s.LibraryImport(ctx, admin, 0, c, false)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := s.LibraryImport(ctx, admin, 0, c, false)
	if e != nil || replay.ID != v.ID {
		t.Fatal("import replay", e)
	}
	if _, e = s.LibraryRead(ctx, other, v.ID); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("foreign corp", e)
	}
	if _, e = s.SaveToGame(ctx, admin, v.ID, 1, 123, c.RequestKey); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("admin wrote member", e)
	}
	var calls atomic.Int32
	s.GameSave = func(_ context.Context, _ string, id int64, _ eve.GameFitting) eve.FittingWriteResult {
		calls.Add(1)
		if id == 124 {
			return eve.FittingWriteResult{State: "unknown", Reason: "confirmation_required"}
		}
		return eve.FittingWriteResult{ID: 999, State: "saved"}
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.SaveToGame(ctx, owner, v.ID, 1, 123, c.RequestKey)
			if e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate external write", calls.Load())
	}
	saved, e := s.SaveToGame(ctx, owner, v.ID, 1, 123, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	if e != nil || saved.State != "saved" || saved.FittingID == nil {
		t.Fatal(saved, e)
	}
	if _, e = s.SaveToGame(ctx, owner, v.ID, 1, 124, c.RequestKey); !errors.Is(e, ErrConflict) {
		t.Fatal("key reused for other character", e)
	}
	key := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	for i := 0; i < 2; i++ {
		result, e := s.SaveToGame(ctx, owner, v.ID, 1, 124, key)
		if e != nil || result.State != "unknown" {
			t.Fatal(result, e)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("ambiguous write retried", calls.Load())
	}
	c.Version = 1
	c.EFT = "[Rifter, Changed]\nDamage Control II"
	v, e = s.LibraryImport(ctx, admin, v.ID, c, false)
	if e != nil || v.Version != 2 {
		t.Fatal(v, e)
	}
	if _, e = s.LibraryImport(ctx, admin, v.ID, c, false); !errors.Is(e, ErrConflict) {
		t.Fatal("stale update", e)
	}
	administrator = false
	c.Version = 2
	if _, e = s.LibraryImport(ctx, admin, v.ID, c, true); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("revoked admin", e)
	}
	administrator = true
	if _, e = s.LibraryImport(ctx, admin, v.ID, c, true); e != nil {
		t.Fatal(e)
	}
	var audits int
	if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM fittings_library_audit").Scan(&audits); e != nil || audits != 3 {
		t.Fatal("audit", audits, e)
	}
}
