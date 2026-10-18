package loan

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func setTestOfficers(s *Service, rows ...Character) {
	s.LoanOfficers = func(context.Context, int64) ([]Character, error) { return rows, nil }
	s.LockOfficerCharacters = func(context.Context, pgx.Tx, []int64) error { return nil }
	s.GuardLoanOfficer = func(_ context.Context, _ pgx.Tx, _ int64, id int64) error {
		for _, c := range rows {
			if c.ID == id {
				return nil
			}
		}
		return ErrOfficer
	}
}

func TestOfficerSelectionAndCashContracts(t *testing.T) {
	ctx := context.Background()
	corp := int64(10)
	legacy := int64(999)
	p := store.Pool{CorporationID: &corp, CustodianCharacterID: &legacy}
	s := &Service{}
	setTestOfficers(s, Character{ID: 22, Name: "Alt"}, Character{ID: 21, Name: "Main", IsMain: true})
	if c, e := s.chooseOfficer(ctx, p, 0); e != nil || c.ID != 21 {
		t.Fatal(c, e)
	}
	if c, e := s.chooseOfficer(ctx, p, 22); e != nil || c.ID != 22 {
		t.Fatal(c, e)
	}
	if _, e := s.chooseOfficer(ctx, p, 999); !errors.Is(e, ErrOfficer) {
		t.Fatal("legacy target admitted", e)
	}
	c := Contract{Type: "item_exchange", Status: "finished", Completed: time.Now().UTC().Format(time.RFC3339), ItemsReady: true, Items: json.RawMessage(`[]`), IssuerID: 11, AcceptorID: 22, Price: "100", Reward: "0"}
	if got, e := s.cashOfficer(ctx, p, c, cashParty{"character", 11}, 10000, false); e != nil || got.ID != 22 {
		t.Fatal(got, e)
	}
	for _, mutate := range []func(*Contract){func(c *Contract) { c.AcceptorID = 999 }, func(c *Contract) { c.Price = "99" }, func(c *Contract) { c.Status = "outstanding" }, func(c *Contract) { c.AcceptorID = 11 }} {
		bad := c
		mutate(&bad)
		if _, e := s.cashOfficer(ctx, p, bad, cashParty{"character", 11}, 10000, false); e == nil {
			t.Fatalf("invalid accepted: %+v", bad)
		}
	}
	if _, e := s.cashOfficer(ctx, p, c, cashParty{"character", 11}, 10000, true); e == nil {
		t.Fatal("wrong direction accepted")
	}
	setTestOfficers(s)
	if _, e := s.cashOfficer(ctx, p, c, cashParty{"character", 11}, 10000, false); !errors.Is(e, ErrOfficer) {
		t.Fatal(e)
	}
}

func TestCollateralOfficerAltAndRevocation(t *testing.T) {
	f := newAutoSecurityFixture(t)
	setTestOfficers(f.s, Character{ID: 333, AccountID: f.guarantor, IsMain: true}, Character{ID: 334, AccountID: f.guarantor})
	in := f.input
	in.RecipientCharacterID = 334
	// A permission revoked between preview and commit must not reserve assets.
	f.s.GuardLoanOfficer = func(context.Context, pgx.Tx, int64, int64) error { return ErrOfficer }
	if _, err := f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, in); !errors.Is(err, ErrOfficer) {
		t.Fatal(err)
	}
	rows, err := store.ListCollateralRequests(f.ctx, f.db, f.c.ID)
	if err != nil || len(rows) != 0 {
		t.Fatal("failed write persisted", rows, err)
	}
	setTestOfficers(f.s, Character{ID: 333, AccountID: f.guarantor, IsMain: true}, Character{ID: 334, AccountID: f.guarantor})
	row, err := f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, in)
	if err != nil || row.RecipientCharacterID != 334 {
		t.Fatal(row, err)
	}
	// Changing the default never retargets a saved request.
	setTestOfficers(f.s, Character{ID: 333, AccountID: f.guarantor}, Character{ID: 334, AccountID: f.guarantor, IsMain: true})
	saved, err := store.GetCollateralRequest(f.ctx, f.db, row.ID)
	if err != nil || saved.RecipientCharacterID != 334 {
		t.Fatal(saved, err)
	}
}

func TestContributionOfficerRevocationRollbackAndAlt(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	var lender, officer string
	for _, p := range []*string{&lender, &officer} {
		if err := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	var pool int64
	if err := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared,custodian_character_id) VALUES('corporation',10,'Pool','open','{}',$1,true,999) RETURNING id`, officer).Scan(&pool); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE officer_claims(id bigint PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	contract := Contract{ID: 9001, OwnerKind: "character", OwnerID: 111, Type: "item_exchange", Status: "finished", ItemsReady: true, Items: json.RawMessage(`[]`), IssuerID: 111, AcceptorID: 334, Price: "100", Reward: "0", Completed: time.Now().UTC().Format(time.RFC3339)}
	s := &Service{Pool: db, Characters: func(context.Context, string) ([]Character, error) { return []Character{{ID: 111}}, nil }, LockAccounts: func(context.Context, pgx.Tx, []string) error { return nil }, ReadContractTx: func(context.Context, pgx.Tx, string, string, int64, int64) (Contract, error) { return contract, nil }, Contracts: ContractAdapter{ClaimFunc: func(ctx context.Context, tx pgx.Tx, id int64, _ string, _ int64) error {
		_, err := tx.Exec(ctx, `INSERT INTO officer_claims VALUES($1)`, id)
		return err
	}}}
	setTestOfficers(s, Character{ID: 333, AccountID: officer, IsMain: true}, Character{ID: 334, AccountID: officer})
	c, err := s.AddContribution(ctx, lender, ContributionInput{LenderKind: "personal", SourceCharacterID: 111, AmountMinor: 10000})
	if err != nil {
		t.Fatal(err)
	}
	input := DepositInput{Version: c.Version, ContractKind: "character", ContractOwnerID: 111, ContractID: 9001}
	s.GuardLoanOfficer = func(context.Context, pgx.Tx, int64, int64) error { return ErrOfficer }
	if _, err = s.VerifyContribution(ctx, lender, c.ID, input); !errors.Is(err, ErrOfficer) {
		t.Fatal(err)
	}
	var claims, ledger int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM officer_claims),(SELECT count(*) FROM loan_pool_ledger)`).Scan(&claims, &ledger); err != nil || claims != 0 || ledger != 0 {
		t.Fatal(claims, ledger, err)
	}
	setTestOfficers(s, Character{ID: 333, AccountID: officer, IsMain: true}, Character{ID: 334, AccountID: officer})
	contract.AcceptorID = 999
	if _, err = s.VerifyContribution(ctx, lender, c.ID, input); !errors.Is(err, ErrContract) {
		t.Fatal("legacy target accepted", err)
	}
	contract.AcceptorID = 334
	if out, err := s.VerifyContribution(ctx, lender, c.ID, input); err != nil || out.State != "funded" {
		t.Fatal(out, err)
	}
	if _, err = s.VerifyContribution(ctx, lender, c.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate", err)
	}
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM officer_claims),(SELECT count(*) FROM loan_pool_ledger)`).Scan(&claims, &ledger); err != nil || claims != 1 || ledger != 1 {
		t.Fatal(claims, ledger, err)
	}
}
