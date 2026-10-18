package loan

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func TestApplicationRepaymentChoices(t *testing.T) {
	now := time.Date(2026, 10, 16, 16, 5, 0, 0, time.UTC) // Oct 17 in Beijing.
	for _, tt := range []struct {
		name  string
		input ApplicationInput
		days  int
		valid bool
	}{
		{"month", ApplicationInput{InstallmentCount: 1, IntervalDays: 30}, 30, true},
		{"year", ApplicationInput{InstallmentCount: 1, IntervalDays: 365}, 365, true},
		{"custom", ApplicationInput{InstallmentCount: 1, IntervalDays: 45}, 45, true},
		{"installments", ApplicationInput{InstallmentCount: 3, IntervalDays: 14}, 14, true},
		{"unsupported installment interval", ApplicationInput{InstallmentCount: 3, IntervalDays: 365}, 0, false},
		{"zero term", ApplicationInput{InstallmentCount: 1}, 0, false},
		{"long term", ApplicationInput{InstallmentCount: 1, IntervalDays: 366}, 0, false},
		{"dated", ApplicationInput{InstallmentCount: 1, RepaymentDate: "2026-11-17"}, 31, true},
		{"year boundary", ApplicationInput{InstallmentCount: 1, RepaymentDate: "2027-10-17"}, 365, true},
		{"same Beijing day", ApplicationInput{InstallmentCount: 1, RepaymentDate: "2026-10-17"}, 0, false},
		{"too far", ApplicationInput{InstallmentCount: 1, RepaymentDate: "2027-10-18"}, 0, false},
		{"invalid date", ApplicationInput{InstallmentCount: 1, RepaymentDate: "2027-02-29"}, 0, false},
		{"conflicting term", ApplicationInput{InstallmentCount: 1, IntervalDays: 30, RepaymentDate: "2026-11-17"}, 0, false},
		{"dated installments", ApplicationInput{InstallmentCount: 2, RepaymentDate: "2026-11-17"}, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			days, due, err := applicationSchedule(tt.input, now)
			if (err == nil) != tt.valid || days != tt.days {
				t.Fatalf("days=%d due=%v err=%v", days, due, err)
			}
			if tt.valid && tt.input.RepaymentDate != "" && (due == nil || due.In(loanCalendarZone).Format("2006-01-02 15:04:05") != tt.input.RepaymentDate+" 23:59:59") {
				t.Fatalf("deadline=%v", due)
			}
		})
	}
}

func TestSinglePaymentContractSettlesExactInterest(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	var borrower, manager string
	for _, target := range []*string{&borrower, &manager} {
		if err := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	var poolID int64
	if err := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared,custodian_character_id) VALUES('corporation',98000001,'Test','open','{"min_principal_minor":100,"max_installments":12}',$1::uuid,true,222) RETURNING id`, manager).Scan(&poolID); err != nil {
		t.Fatal(err)
	}
	if err := store.PoolLedger(ctx, db, poolID, 0, 0, 1_000_000_000, "admin_deposit", manager); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE test_loan_claims(contract_id bigint PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	contract := Contract{ID: 9001, OwnerKind: "character", OwnerID: 222, Type: "item_exchange", Status: "finished", Price: "1000000", Reward: "0", IssuerID: 222, AcceptorID: 111, Items: []byte(`[]`), ItemsReady: true}
	s := &Service{Pool: db, Characters: func(context.Context, string) ([]Character, error) { return []Character{{ID: 111}}, nil }, IsAdministrator: func(context.Context, string) (bool, error) { return true, nil }, AccountForCharacter: func(context.Context, int64) (string, error) { return manager, nil }, LockAccounts: func(context.Context, pgx.Tx, []string) error { return nil }, ReadContractTx: func(context.Context, pgx.Tx, string, string, int64, int64) (Contract, error) { return contract, nil }, Contracts: ContractAdapter{ClaimFunc: func(ctx context.Context, tx pgx.Tx, id int64, _ string, _ int64) error {
		_, err := tx.Exec(ctx, `INSERT INTO test_loan_claims VALUES($1)`, id)
		return err
	}}}
	setTestOfficers(s, Character{ID: 222, AccountID: manager, IsMain: true})
	date := time.Now().In(loanCalendarZone).AddDate(0, 0, 30).Format(time.DateOnly)
	c, err := s.CreateApplication(ctx, borrower, ApplicationInput{PoolID: poolID, BorrowerCharacterID: 111, PrincipalMinor: 100_000_000, InstallmentCount: 1, RepaymentDate: date})
	if err != nil {
		t.Fatal(err)
	}
	c, err = store.TransitionCase(ctx, db, c.ID, c.Version, "approved", manager, "")
	if err != nil {
		t.Fatal(err)
	}
	input := PaymentInput{Kind: "disbursement", Version: c.Version, ContractKind: "character", ContractOwnerID: 222, ContractID: contract.ID}
	contract.Completed = c.CreatedAt.Add(-time.Hour).Format(time.RFC3339)
	if _, err = s.createSharedPayment(ctx, manager, c.ID, input); err != ErrContract {
		t.Fatalf("historical funding accepted: %v", err)
	}
	var payments, claims int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM loan_payment_intents),(SELECT count(*) FROM test_loan_claims)`).Scan(&payments, &claims); err != nil || payments != 0 || claims != 0 {
		t.Fatalf("rollback payments=%d claims=%d err=%v", payments, claims, err)
	}
	contract.Completed = time.Now().UTC().Format(time.RFC3339)
	if _, err = s.createSharedPayment(ctx, manager, c.ID, input); err != nil {
		t.Fatal("fund:", err)
	}
	c, err = store.GetCase(ctx, db, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.TotalDueMinor%100 == 0 {
		t.Fatal("fixture must exercise fractional ISK interest")
	}
	contract.ID = 9002
	contract.IssuerID = 111
	contract.AcceptorID = 222
	contract.Price = fmt.Sprintf("%d.%02d", c.TotalDueMinor/100, c.TotalDueMinor%100)
	repay := PaymentInput{Kind: "repayment", Version: c.Version, ContractKind: "character", ContractOwnerID: 222, ContractID: contract.ID, ExpectedMinor: c.TotalDueMinor}
	if _, err = s.createSharedPayment(ctx, borrower, c.ID, repay); err != nil {
		t.Fatal("repay:", err)
	}
	settled, err := store.GetCase(ctx, db, c.ID)
	if err != nil || settled.State != "settled" {
		t.Fatalf("settled=%+v err=%v", settled, err)
	}
	rows, err := store.Installments(ctx, db, c.ID)
	if err != nil || len(rows) != 1 || rows[0].PaidPrincipalMinor+rows[0].PaidInterestMinor != c.TotalDueMinor {
		t.Fatalf("paid=%+v err=%v", rows, err)
	}
}

func TestDatedLoanUsesActualContractDateAndFrozenRate(t *testing.T) {
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	days, due, _ := applicationSchedule(ApplicationInput{InstallmentCount: 1, RepaymentDate: "2026-10-31"}, created)
	quoted, _ := simpleAnnualInterest(365_000_00, 1000, 1, days)
	c := store.Case{PrincipalMinor: 365_000_00, InterestMinor: quoted, InterestRateBPS: 1000, InstallmentCount: 1, IntervalDays: days, CreatedAt: created, RequestedDueAt: due}
	verified := due.Add(48 * time.Hour) // Late synchronization must retain the actual deadline.
	funded, actualDue, actualDays, interest, err := fundedSchedule(c, "2026-10-11T03:00:00Z", verified)
	if err != nil || actualDays != 20 || interest != 200_000 || interest >= quoted || !actualDue.Equal(*due) || funded.Day() != 11 {
		t.Fatalf("terms=%v %v %d %d %v", funded, actualDue, actualDays, interest, err)
	}
	_, _, actualDays, _, err = fundedSchedule(c, "2026-10-31T03:00:00Z", verified)
	if err != nil || actualDays != 1 {
		t.Fatalf("same-day funding: days=%d err=%v", actualDays, err)
	}
	for _, completed := range []string{"", "invalid", "2026-09-30T23:59:59Z", due.Format(time.RFC3339), "2026-11-01T01:00:00Z"} {
		if _, _, _, _, err := fundedSchedule(c, completed, verified); err == nil {
			t.Fatalf("accepted invalid completion %q", completed)
		}
	}
	if _, _, _, _, err := fundedSchedule(c, "2026-10-11T03:00:00Z", created); err == nil {
		t.Fatal("accepted future contract completion")
	}
}

func TestRepaymentChoicesPersistAndFinalize(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	var account string
	if err := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	var poolID int64
	if err := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared) VALUES('corporation',98000001,'Test','open','{"min_principal_minor":100,"max_installments":12}',$1::uuid,true) RETURNING id`, account).Scan(&poolID); err != nil {
		t.Fatal(err)
	}
	service := &Service{Pool: db, Characters: func(context.Context, string) ([]Character, error) { return []Character{{ID: 2112345678}}, nil }}
	for _, term := range []struct {
		name        string
		count, days int
		date        string
	}{
		{"month", 1, 30, ""}, {"year", 1, 365, ""}, {"custom", 1, 45, ""}, {"installments", 3, 30, ""}, {"date", 1, 0, time.Now().In(loanCalendarZone).AddDate(0, 0, 30).Format(time.DateOnly)},
	} {
		t.Run(term.name, func(t *testing.T) {
			c, err := service.CreateApplication(ctx, account, ApplicationInput{PoolID: poolID, BorrowerCharacterID: 2112345678, PrincipalMinor: 365_000_00, InstallmentCount: term.count, IntervalDays: term.days, RepaymentDate: term.date})
			if err != nil {
				t.Fatal(err)
			}
			if c.FirstDueAt != nil || (c.RequestedDueAt != nil) != (term.date != "") {
				t.Fatalf("application dates: %+v", c)
			}
			stored, err := store.GetCase(ctx, db, c.ID)
			if err != nil || stored.InterestMinor != c.InterestMinor {
				t.Fatalf("read=%+v %v", stored, err)
			}
			for _, all := range []bool{false, true} {
				if _, err = store.ListCases(ctx, db, account, all); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = store.ListCasesForCorporations(ctx, db, account, []int64{98000001}); err != nil {
				t.Fatal(err)
			}
			if term.date != "" {
				snapshot, err := service.ApprovalSnapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range snapshot {
					if item.ID == c.ID {
						var payload map[string]any
						if err := json.Unmarshal(item.Payload, &payload); err != nil {
							t.Fatal(err)
						}
						found = payload["requested_due_at"] != nil
					}
				}
				if !found {
					t.Fatal("approval snapshot omitted agreed deadline")
				}
			}
			c, err = store.TransitionCase(ctx, db, c.ID, c.Version, "approved", account, "")
			if err != nil {
				t.Fatal(err)
			}
			funding := c.CreatedAt.Add(5 * 24 * time.Hour).Truncate(time.Second)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			active, err := activateFundedCase(ctx, tx, c, account, Contract{Completed: funding.Format(time.RFC3339)}, funding.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			rows, err := store.Installments(ctx, db, c.ID)
			if err != nil || len(rows) != term.count {
				t.Fatalf("schedule=%+v err=%v", rows, err)
			}
			var total int64
			for _, row := range rows {
				total += row.PrincipalMinor + row.InterestMinor
			}
			if total != active.TotalDueMinor || !rows[0].DueAt.Equal(*active.FirstDueAt) {
				t.Fatalf("schedule totals=%d case=%+v", total, active)
			}
			if term.date != "" && (active.InterestMinor >= c.InterestMinor || !active.FirstDueAt.Equal(*c.RequestedDueAt) || active.IntervalDays != 25) {
				t.Fatalf("dated final terms=%+v", active)
			}
			if term.date == "" && (active.InterestMinor != c.InterestMinor || !active.FirstDueAt.Equal(funding.Add(time.Hour).AddDate(0, 0, term.days))) {
				t.Fatalf("relative terms=%+v", active)
			}
			// A retry cannot create a duplicate schedule or finalize amounts again.
			retry, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = activateFundedCase(ctx, retry, c, account, Contract{Completed: funding.Format(time.RFC3339)}, funding.Add(2*time.Hour)); err == nil {
				t.Fatal("duplicate activation succeeded")
			}
			retry.Rollback(ctx)
		})
	}
}
