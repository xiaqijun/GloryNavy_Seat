package welfare

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"testing"
	"time"
)

func repairedCase(t *testing.T) (*Service, Case, string) {
	t.Helper()
	s, _ := fixture(t)
	ctx := context.Background()
	old := "PVP-20261001-11111111-1111-4111-8111-111111111111"
	v, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "approved", Detail: raw(Detail{CharacterID: 123, Reviewer: adminID}), Award: 12345, Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, `UPDATE welfare_cases SET settlement_reference='PVP-20261001-000100',created_at='2026-10-01T00:00:00Z' WHERE id=$1`, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO welfare_case_reference_repairs(case_id,previous_reference,replacement_reference,operator_label,reason) VALUES($1,$2,'PVP-20261001-000100','test','explicit repair')`, v.ID, old)
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.Read(ctx, s.Pool, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 456: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	return s, v, old
}
func TestRepairedCaseOldAndNewTitlesCompleteWithoutRewritingEvidence(t *testing.T) {
	for _, oldTitle := range []bool{true, false} {
		t.Run(map[bool]string{true: "old", false: "new"}[oldTitle], func(t *testing.T) {
			s, v, old := repairedCase(t)
			ctx := context.Background()
			title := v.Reference
			if oldTitle {
				title = old
			}
			c := eve.DeliveryContract{ID: 567, OwnerKind: "character", OwnerID: 123, Title: title, Type: "item_exchange", Status: "finished", IssuerID: 456, IssuerCorporationID: 10, AssigneeID: 123, AcceptorID: 123, Issued: time.Now().Add(-time.Hour), Completed: time.Now().Add(-time.Minute).Format(time.RFC3339), ItemsReady: true, Price: "0", Reward: "123", ContentToken: "unchanged"}
			calls := map[string]int{}
			s.PaymentContracts = func(_ context.Context, _ pgx.Tx, owner string, char int64, ref string, _ time.Time) ([]eve.DeliveryContract, error) {
				if owner != userID || char != 123 {
					t.Fatal("scope changed")
				}
				calls[ref]++
				if ref == title {
					return []eve.DeliveryContract{c}, nil
				}
				return nil, nil
			}
			if err := s.CheckDelivery(ctx, v.ID); err != nil {
				t.Fatal(err)
			}
			got, err := store.Read(ctx, s.Pool, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			var d Detail
			json.Unmarshal(got.Detail, &d)
			if got.State != "completed" || got.Reference != v.Reference || d.Delivery == nil || d.Delivery.Contract.Title != title || d.Delivery.Contract.ContentToken != "unchanged" || calls[old] != 2 || calls[v.Reference] != 2 {
				t.Fatal(got, d, calls)
			}
			if err := s.CheckDelivery(ctx, v.ID); err != nil {
				t.Fatal(err)
			}
			var n int
			s.Pool.QueryRow(ctx, "SELECT count(*) FROM eve_delivery_claims WHERE contract_id=567").Scan(&n)
			if n != 1 {
				t.Fatal(n)
			}
		})
	}
}
func TestRepairedCaseMultipleTitlesAndCancellationGuard(t *testing.T) {
	s, v, old := repairedCase(t)
	ctx := context.Background()
	s.PaymentContracts = func(_ context.Context, _ pgx.Tx, _ string, _ int64, ref string, _ time.Time) ([]eve.DeliveryContract, error) {
		id := int64(567)
		if ref == old {
			id = 568
		}
		return []eve.DeliveryContract{{ID: id, Title: ref, IssuerID: 456, Status: "outstanding"}}, nil
	}
	if err := s.CheckDelivery(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(ctx, s.Pool, v.ID)
	var d Detail
	json.Unmarshal(got.Detail, &d)
	if d.PaymentStatus != "multiple_contracts" || got.State != "approved" {
		t.Fatal(got, d)
	}
	s.PaymentContracts = func(_ context.Context, _ pgx.Tx, _ string, _ int64, ref string, _ time.Time) ([]eve.DeliveryContract, error) {
		if ref == old {
			return []eve.DeliveryContract{{ID: 568, Title: old, Status: "outstanding"}}, nil
		}
		return nil, nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.guardLossCancellation(ctx, tx, adminID, v, Detail{CharacterID: 123}); err != ErrCancellationPayment {
		t.Fatal("old contract did not block cancellation", err)
	}
}
func TestRepairedCaseAliasesAreObjectBoundAndExact(t *testing.T) {
	s, v, old := repairedCase(t)
	ctx := context.Background()
	for _, args := range []struct {
		id  int64
		ref string
	}{{v.ID + 1, v.Reference}, {v.ID, "PVP-20261001-000101"}} {
		refs, err := store.CaseReferenceAliases(ctx, s.Pool, args.id, args.ref)
		if err != nil || len(refs) != 0 {
			t.Fatal(refs, err)
		}
	}
	refs := []string{v.Reference, old}
	if got := caseForContractReference(v, refs, old+"x"); got.Reference != v.Reference {
		t.Fatal("prefix alias accepted")
	}
	if got := caseForContractReference(v, refs, old); got.Reference != old || v.Reference == old {
		t.Fatal("original case changed")
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO welfare_case_reference_repairs(case_id,previous_reference,replacement_reference,operator_label,reason) VALUES($1,$2,$3,'test','retry')`, v.ID, old, v.Reference); err == nil {
		t.Fatal("repair overwritten")
	}
}
