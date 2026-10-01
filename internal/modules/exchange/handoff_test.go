package exchange

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"regexp"
	"testing"
)

func TestHandoffRecipientReferenceAndCurrentAdministrator(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	claim := quote(sh, 701)
	claim.RecipientID = 4
	id, e := s.ClaimReward(ctx, member, claim)
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.Handoff(ctx, manager, id)
	if e != nil || !regexp.MustCompile(`^EX-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`).MatchString(out.Reference) || out.RecipientID != 4 || out.State != "pending" {
		t.Fatal(out, e)
	}
	if again, e := s.ClaimReward(ctx, member, claim); e != nil || again != id {
		t.Fatal("replay", again, e)
	}
	// A replay or subsequent library edit cannot regenerate the saved reference.
	if _, e = s.Pool.Exec(ctx, `UPDATE exchange_rewards SET content='{"fittings":[],"items":[{"type_id":"35","quantity":999}]}'`); e != nil {
		t.Fatal(e)
	}
	again, e := s.Handoff(ctx, manager, id)
	if e != nil || again.Reference != out.Reference {
		t.Fatal("unstable reference", again, e)
	}
	if _, e = s.Handoff(ctx, member, id); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("member access", e)
	}
	s.Administrator = func(context.Context, string) (bool, error) { return false, nil }
	if _, e = s.Handoff(ctx, manager, id); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("revoked administrator", e)
	}
	var state string
	if e = s.Pool.QueryRow(ctx, `SELECT state FROM exchange_redemptions WHERE id=$1`, id).Scan(&state); e != nil || state != "pending" {
		t.Fatal("read changed order", state, e)
	}
}

func TestDeliverySettlementReferenceUniquenessAndLegacy(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	first := quote(sh, 710)
	first.RecipientID = 4
	id, e := s.ClaimReward(ctx, member, first)
	if e != nil {
		t.Fatal(e)
	}
	original, e := s.Handoff(ctx, manager, id)
	if e != nil {
		t.Fatal(e)
	}
	sh, e = s.Shop(ctx, member, 0)
	if e != nil {
		t.Fatal(e)
	}
	second := quote(sh, 711)
	second.RecipientID = 1
	id2, e := s.ClaimReward(ctx, manager, second)
	if e != nil {
		t.Fatal(e)
	}
	other, e := s.Handoff(ctx, manager, id2)
	if e != nil || original.Reference == other.Reference {
		t.Fatal("duplicate reference", other, e)
	}
	if _, e = s.Pool.Exec(ctx, `UPDATE exchange_redemptions SET settlement_reference=$1 WHERE id=$2`, original.Reference, id2); e == nil {
		t.Fatal("unique constraint missing")
	}
	// Legacy values are persisted exactly as the upgrade backfill writes them.
	legacy := fmt.Sprintf("GNV-EX-%d", id)
	if _, e = s.Pool.Exec(ctx, `UPDATE exchange_redemptions SET settlement_reference=$1 WHERE id=$2`, legacy, id); e != nil {
		t.Fatal(e)
	}
	old, e := s.Handoff(ctx, manager, id)
	if e != nil || old.Reference != legacy {
		t.Fatal(old, e)
	}
	installDelivery(t, s, id, 1, "finished")
	if e = s.CheckDelivery(ctx, id); e != nil {
		t.Fatal(e)
	}
	deliveryState(t, s, id, "fulfilled", "fulfilled")
}
