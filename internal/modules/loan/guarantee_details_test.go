package loan

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func TestGuaranteeDetailsNamesAndFallbacks(t *testing.T) {
	rows := []store.Guarantee{
		{ID: 1, GuarantorCharacterID: 222, AmountMinor: 200_000_000_000, State: "accepted"},
		{ID: 2, GuarantorCharacterID: 333, AmountMinor: 50_000_000_000, State: "invited"},
		{ID: 3, GuarantorCharacterID: 222, State: "released"},
		{ID: 4, GuarantorAccountID: "legacy-account"},
	}
	calls := 0
	s := &Service{CharacterNames: func(_ context.Context, ids []int64) (map[int64]string, error) {
		calls++
		if !reflect.DeepEqual(ids, []int64{222, 333}) {
			t.Fatalf("name lookup IDs: %v", ids)
		}
		return map[int64]string{222: "  Guarantor One  ", 0: "Not a character"}, nil
	}}
	got := s.guaranteeDetails(context.Background(), rows)
	if calls != 1 || len(got) != len(rows) {
		t.Fatalf("calls=%d, rows=%d", calls, len(got))
	}
	for i, name := range []string{"Guarantor One", "", "Guarantor One", ""} {
		if got[i].GuarantorCharacterName != name || !reflect.DeepEqual(got[i].Guarantee, rows[i]) {
			t.Fatalf("record %d changed or wrong name: %+v", i, got[i])
		}
	}
	for _, unavailable := range []func(context.Context, []int64) (map[int64]string, error){
		nil,
		func(context.Context, []int64) (map[int64]string, error) { return nil, errors.New("unavailable") },
	} {
		s.CharacterNames = unavailable
		for i, row := range s.guaranteeDetails(context.Background(), rows) {
			if row.GuarantorCharacterName != "" || !reflect.DeepEqual(row.Guarantee, rows[i]) {
				t.Fatalf("unavailable resolver changed guarantee: %+v", row)
			}
		}
	}
	empty, err := json.Marshal(s.guaranteeDetails(context.Background(), nil))
	if err != nil || string(empty) != "[]" {
		t.Fatalf("empty guarantee JSON: %s, %v", empty, err)
	}
}

func TestLoanDetailGuarantorNamesAfterAuthorization(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	accounts := make([]string, 4)
	for i := range accounts {
		if err := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(&accounts[i]); err != nil {
			t.Fatal(err)
		}
	}
	borrower, manager, guarantor, outsider := accounts[0], accounts[1], accounts[2], accounts[3]
	var poolID int64
	if err := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared) VALUES('corporation',98000001,'Name test','open','{}',$1::uuid,true) RETURNING id`, manager).Scan(&poolID); err != nil {
		t.Fatal(err)
	}
	c, err := store.CreateCase(ctx, db, publicID("LN"), poolID, borrower, 111, 200_000_000_000, 0, 0, 1, 30, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddGuarantee(ctx, db, c.ID, guarantor, 222, 200_000_000_000, ""); err != nil {
		t.Fatal(err)
	}
	before, err := store.ListGuarantees(ctx, db, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	lookupFails := false
	s := &Service{Pool: db, IsAdministrator: func(_ context.Context, actor string) (bool, error) { return actor == manager, nil }, CharacterNames: func(_ context.Context, ids []int64) (map[int64]string, error) {
		calls++
		if !reflect.DeepEqual(ids, []int64{222}) {
			t.Fatalf("unexpected IDs: %v", ids)
		}
		if lookupFails {
			return nil, errors.New("display lookup unavailable")
		}
		return map[int64]string{222: "Guarantor Pilot"}, nil
	}}
	for _, actor := range []string{outsider, guarantor} {
		if _, err := s.Detail(ctx, actor, c.ID); !errors.Is(err, ErrForbidden) {
			t.Fatalf("unauthorized detail: %v", err)
		}
	}
	if calls != 0 {
		t.Fatal("names resolved before authorization")
	}
	for _, actor := range []string{borrower, manager} {
		detail, err := s.Detail(ctx, actor, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(detail)
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Guarantees []map[string]any `json:"guarantees"`
		}
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.Guarantees) != 1 {
			t.Fatalf("guarantees: %s", encoded)
		}
		g := decoded.Guarantees[0]
		if g["guarantor_character_name"] != "Guarantor Pilot" || g["guarantor_character_id"] != "222" || g["amount_minor"] != float64(200_000_000_000) || g["guarantor_account_id"] != guarantor {
			t.Fatalf("detail guarantee JSON: %s", encoded)
		}
	}
	lookupFails = true
	detail, err := s.Detail(ctx, borrower, c.ID)
	if err != nil {
		t.Fatalf("name lookup failed loan read: %v", err)
	}
	encoded, err := json.Marshal(detail["guarantees"])
	if err != nil {
		t.Fatal(err)
	}
	var fallback []map[string]any
	if err := json.Unmarshal(encoded, &fallback); err != nil {
		t.Fatal(err)
	}
	if _, present := fallback[0]["guarantor_character_name"]; present || fallback[0]["guarantor_character_id"] != "222" {
		t.Fatalf("fallback: %s", encoded)
	}
	after, err := store.ListGuarantees(ctx, db, c.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("detail mutated guarantees: %v", err)
	}
}
