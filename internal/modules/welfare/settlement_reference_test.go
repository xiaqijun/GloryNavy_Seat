package welfare

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/migrations"
)

func TestSettlementReferenceUpgradeAndStability(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 40); err != nil {
		t.Fatal(err)
	}
	var oldID int64
	if err = s.Pool.QueryRow(ctx, `INSERT INTO welfare_cases(account_id,corporation_id,kind,state,detail,award_minor,claim_keys) VALUES($1,10,'solo','approved','{}',100,'{}') RETURNING id`, userID).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	old, err := store.Read(ctx, s.Pool, oldID)
	if err != nil || !strings.HasPrefix(old.Reference, "GNV-WF-") {
		t.Fatalf("legacy: %+v %v", old, err)
	}
	c := eve.DeliveryContract{ID: 999, ContentToken: "fixed", Type: "item_exchange", Title: old.Reference, IssuerCorporationID: 10, IssuerID: 456, AssigneeID: 123, Issued: old.CreatedAt.Add(time.Second), ItemsReady: true, Price: "0", Reward: "1", Status: "outstanding"}
	if got := lossPaymentState(old, Detail{CharacterID: 123}, c); got != "awaiting_acceptance" {
		t.Fatal("legacy matching", got)
	}
	seen := map[string]bool{}
	for _, kind := range []string{"srp", "solo", "solo", "supercarrier"} {
		v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: kind, State: "submitted", Detail: raw(Detail{}), Keys: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		prefix := map[string]string{"srp": "SRP", "solo": "PVP", "supercarrier": "WF"}[kind]
		pattern := `^` + prefix + `-` + v.CreatedAt.UTC().Format("20060102") + `-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`
		if !regexp.MustCompile(pattern).MatchString(v.Reference) || seen[v.Reference] {
			t.Fatal("invalid/duplicate reference", v.Reference)
		}
		seen[v.Reference] = true
		original := v.Reference
		v.Reference = "client-must-not-change-reference"
		v.Award = 200
		updated, err := store.Save(ctx, s.Pool, v)
		if err != nil || updated.Reference != original {
			t.Fatal("update regenerated reference", updated.Reference, err)
		}
		if _, err = s.Pool.Exec(ctx, `UPDATE welfare_cases SET settlement_reference=$1 WHERE id=$2`, original, oldID); err == nil {
			t.Fatal("missing unique constraint")
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = store.Merge(ctx, tx, userID, otherID, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var references []string
	if err = s.Pool.QueryRow(ctx, `SELECT array_agg(settlement_reference) FROM welfare_cases WHERE account_id=$1`, otherID).Scan(&references); err != nil {
		t.Fatal(err)
	}
	if len(references) != len(seen)+1 {
		t.Fatal("merge dropped cases")
	}
	for _, ref := range references {
		if ref != old.Reference && !seen[ref] {
			t.Fatal("merge changed reference", ref)
		}
	}
	var atBoundary string
	if err = s.Pool.QueryRow(ctx, `SELECT gn_settlement_reference('EX','2026-09-21T00:30:00+08:00'::timestamptz)`).Scan(&atBoundary); err != nil || !strings.HasPrefix(atBoundary, "EX-20260920-") {
		t.Fatal("UTC date boundary", atBoundary, err)
	}
	if _, err = provider.DownTo(ctx, 40); err == nil {
		t.Fatal("unsafe rollback accepted after new references")
	}
}
