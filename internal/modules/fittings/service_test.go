package fittings

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/testutil"
	"testing"
)

const owner = "11111111-1111-4111-8111-111111111111"
const other = "22222222-2222-4222-8222-222222222222"
const admin = "33333333-3333-4333-8333-333333333333"

func TestDraftOwnershipVersionAndIdempotency(t *testing.T) {
	ctx := context.Background()
	s := &Service{Pool: testutil.Database(t)}
	administrator := true
	s.Administrator = func(_ context.Context, u string) (bool, error) { return u == admin && administrator, nil }
	s.Own = func(_ context.Context, u string) ([]Character, error) {
		if u == owner {
			return []Character{{ID: 123, Name: "Pilot"}}, nil
		}
		return nil, nil
	}
	s.CanReadCharacter = func(_ context.Context, u string, id int64) (bool, error) { return u == owner && id == 123, nil }
	c := Edit{RequestKey: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Fit: Fit{Name: "Rifter", ShipTypeID: 587, SkillMode: "all5", Items: []Item{}}}
	first, err := s.Save(ctx, owner, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Save(ctx, owner, 0, c)
	if err != nil || replay.ID != first.ID || replay.Version != 1 {
		t.Fatal("idempotency", replay, err)
	}
	c.Fit.Name = "different"
	if _, err = s.Save(ctx, owner, 0, c); !errors.Is(err, ErrConflict) {
		t.Fatal("key reuse", err)
	}
	if _, err = s.Read(ctx, other, first.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign read", err)
	}
	read, err := s.Read(ctx, admin, first.ID)
	if err != nil || read.CanEdit {
		t.Fatal("admin read", read, err)
	}
	c.Version = 1
	if _, err = s.Save(ctx, admin, first.ID, c); !errors.Is(err, ErrConflict) {
		t.Fatal("admin foreign write", err)
	}
	if err = s.Delete(ctx, admin, first.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("admin foreign delete", err)
	}
	updated, err := s.Save(ctx, owner, first.ID, c)
	if err != nil || updated.Version != 2 {
		t.Fatal(updated, err)
	}
	if _, err = s.Save(ctx, owner, first.ID, c); !errors.Is(err, ErrConflict) {
		t.Fatal("stale write", err)
	}
	administrator = false
	if _, err = s.Read(ctx, admin, first.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("stale admin access", err)
	}
	c.Fit.SkillMode = "character"
	c.Fit.CharacterID = 999
	if _, err = s.Save(ctx, owner, first.ID, c); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign skills", err)
	}
	if err = s.Delete(ctx, owner, first.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ctx, owner, first.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("deleted fit", err)
	}
}
func TestFitValidation(t *testing.T) {
	valid := Fit{Name: "Rifter", ShipTypeID: 587, SkillMode: "all5", Items: []Item{{TypeID: 2873, Slot: "high", Index: 0, Quantity: 1, State: "active"}}}
	if err := validate(valid); err != nil {
		t.Fatal(err)
	}
	duplicate := valid
	duplicate.Items = append(append([]Item{}, valid.Items...), valid.Items[0])
	if validate(duplicate) == nil {
		t.Fatal("duplicate slot accepted")
	}
	for _, item := range []Item{{TypeID: 1, Slot: "high", Index: 8, Quantity: 1, State: "active"}, {TypeID: 1, Slot: "high", Quantity: 2, State: "active"}, {TypeID: 1, Slot: "cargo", Quantity: 0, State: "online"}, {TypeID: 1, Slot: "unknown", Quantity: 1, State: "online"}} {
		f := valid
		f.Items = []Item{item}
		if validate(f) == nil {
			t.Fatal("invalid item accepted", item)
		}
	}
}
