package loan

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

type singleCreditFixture struct {
	t          *testing.T
	db         *pgxpool.Pool
	ctx        context.Context
	s          *Service
	manager    string
	poolID     int64
	contractID int64
}

func newSingleCreditFixture(t *testing.T, cash int64) *singleCreditFixture {
	f := &singleCreditFixture{t: t, db: loanTestDatabase(t), contractID: 9000}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	f.manager = f.account()
	if err := f.db.QueryRow(f.ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared) VALUES('corporation',98000001,'Credit test','open','{}',$1::uuid,true) RETURNING id`, f.manager).Scan(&f.poolID); err != nil {
		t.Fatal(err)
	}
	if err := store.PoolLedger(f.ctx, f.db, f.poolID, 0, 0, cash, "admin_deposit", f.manager); err != nil {
		t.Fatal(err)
	}
	f.s = &Service{Pool: f.db, LockAccounts: func(context.Context, pgx.Tx, []string) error { return nil }, IsAdministrator: func(_ context.Context, user string) (bool, error) { return user == f.manager, nil }}
	return f
}

func (f *singleCreditFixture) account() string {
	f.t.Helper()
	var account string
	if err := f.db.QueryRow(f.ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(&account); err != nil {
		f.t.Fatal(err)
	}
	return account
}

func (f *singleCreditFixture) credit(account string, limit int64, state string, age time.Duration) {
	f.t.Helper()
	now := time.Now().UTC().Add(-age)
	_, err := store.RecordCreditEvaluation(f.ctx, f.db, store.CreditEvaluation{
		AccountID: account, PolicyVersion: "capacity-v6-policy-1", Score: 80,
		TotalLimitMinor: limit, UnsecuredLimitMinor: limit / 5, AssetSupportLimitMinor: limit - limit/5,
		GuaranteeLimitMinor: limit, State: state, FactorScores: json.RawMessage(`{}`), Evidence: json.RawMessage(`{}`), EvidenceCutoff: now, EvaluatedAt: now,
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *singleCreditFixture) application(account string, amount int64) store.Case {
	f.t.Helper()
	c, err := store.CreateCase(f.ctx, f.db, publicID("LN"), f.poolID, account, 111, amount, 0, 0, 1, 30, nil, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}

func (f *singleCreditFixture) collateral(c store.Case, amount int64, state string) {
	f.t.Helper()
	f.contractID++
	row, err := store.AddCollateral(f.ctx, f.db, c.ID, c.BorrowerAccountID, "character", 111, f.contractID, json.RawMessage(`[{"type_id":34,"quantity":1}]`), amount, 10000, amount)
	if err != nil {
		f.t.Fatal(err)
	}
	if state != "proposed" {
		if _, err := store.DecideCollateral(f.ctx, f.db, row.ID, row.Version, state); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *singleCreditFixture) guarantee(c store.Case, guarantor string, amount int64, state string) store.Guarantee {
	f.t.Helper()
	g, err := store.AddGuarantee(f.ctx, f.db, c.ID, guarantor, 222, amount, "")
	if err != nil {
		f.t.Fatal(err)
	}
	if state != "invited" {
		g, err = store.DecideGuarantee(f.ctx, f.db, g.ID, g.Version, state)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	return g
}

func (f *singleCreditFixture) approve(c store.Case) error {
	_, err := f.s.Review(f.ctx, f.manager, c.ID, ReviewInput{State: "approved", Version: c.Version})
	return err
}

func TestSingleCreditApproval(t *testing.T) {
	f := newSingleCreditFixture(t, 10_000_000)
	for _, tc := range []struct {
		name                                    string
		limit, principal, collateral, guarantee int64
		collateralState, guaranteeState         string
		want                                    error
	}{
		{"within limit ignores historical unsecured component", 100000, 100000, 0, 0, "", "", nil},
		{"over limit needs coverage", 100000, 150000, 0, 0, "", "", ErrCoverage},
		{"exact collateral shortfall", 100000, 150000, 50000, 0, "approved", "", nil},
		{"one minor unit short", 100000, 150000, 49999, 0, "approved", "", ErrCoverage},
		{"accepted guarantee", 100000, 150000, 0, 50000, "", "accepted", nil},
		{"combined coverage", 100000, 150000, 30000, 20000, "approved", "accepted", nil},
		{"invitation is not coverage", 100000, 150000, 0, 50000, "", "invited", ErrCoverage},
		{"proposed collateral is not coverage", 100000, 150000, 50000, 0, "proposed", "", ErrCoverage},
		{"zero active limit permits fully secured loan", 0, 100000, 100000, 0, "approved", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			borrower := f.account()
			f.credit(borrower, tc.limit, "active", 0)
			c := f.application(borrower, tc.principal)
			if tc.collateral > 0 {
				f.collateral(c, tc.collateral, tc.collateralState)
			}
			if tc.guarantee > 0 {
				f.guarantee(c, f.account(), tc.guarantee, tc.guaranteeState)
			}
			if err := f.approve(c); !errors.Is(err, tc.want) {
				t.Fatalf("approve=%v want=%v", err, tc.want)
			}
			row, err := store.GetCase(f.ctx, f.db, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			state := "submitted"
			if tc.want == nil {
				state = "approved"
			}
			if row.State != state {
				t.Fatalf("state=%s want=%s", row.State, state)
			}
			if tc.want == nil {
				var evidence struct {
					Model    string
					Required int64 `json:"required_coverage_minor"`
					Coverage int64 `json:"approved_coverage_minor"`
				}
				var raw []byte
				if err := f.db.QueryRow(f.ctx, `SELECT after_state FROM loan_audit WHERE case_id=$1 AND action='credit_coverage_checked'`, c.ID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(raw, &evidence); err != nil {
					t.Fatal(err)
				}
				if evidence.Model != "single-credit-v1" || evidence.Required != max(int64(0), tc.principal-tc.limit) || evidence.Coverage < evidence.Required {
					t.Fatalf("audit=%s", raw)
				}
			}
		})
	}
	// Existing loans and guarantees consume the same personal limit; another
	// loan's collateral is never reused as security for this application.
	borrower := f.account()
	f.credit(borrower, 100000, "active", 0)
	existing := f.application(borrower, 60000)
	f.collateral(existing, 60000, "approved")
	if err := f.approve(existing); err != nil {
		t.Fatal(err)
	}
	f.guarantee(f.application(f.account(), 20000), borrower, 20000, "accepted")
	c := f.application(borrower, 50000)
	f.collateral(c, 29999, "approved")
	if err := f.approve(c); !errors.Is(err, ErrCoverage) {
		t.Fatalf("occupied limit: %v", err)
	}
	f.collateral(c, 1, "approved")
	if err := f.approve(c); err != nil {
		t.Fatalf("remaining 20000 + coverage 30000: %v", err)
	}
	// A failed audit must roll back both approval and its evaluation snapshot.
	borrower = f.account()
	f.credit(borrower, 100000, "active", 0)
	c = f.application(borrower, 100000)
	if _, err := f.db.Exec(f.ctx, `CREATE FUNCTION fail_credit_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='credit_coverage_checked' THEN RAISE EXCEPTION 'test rollback'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_credit BEFORE INSERT ON loan_audit FOR EACH ROW EXECUTE FUNCTION fail_credit_audit()`); err != nil {
		t.Fatal(err)
	}
	if err := f.approve(c); err == nil {
		t.Fatal("audit failure did not abort")
	}
	row, err := store.GetCase(f.ctx, f.db, c.ID)
	if err != nil || row.State != "submitted" || row.Version != c.Version {
		t.Fatalf("rollback case=%+v err=%v", row, err)
	}
	var count int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM loan_credit_evaluations WHERE account_id=$1::uuid`, borrower).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback evaluations=%d err=%v", count, err)
	}
}

func TestSingleCreditApprovalGuards(t *testing.T) {
	f := newSingleCreditFixture(t, 100000)
	for _, tc := range []struct {
		name, state string
		age         time.Duration
	}{
		{"missing", "", 0}, {"stale", "active", 100 * time.Hour}, {"suspended", "suspended", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			borrower := f.account()
			if tc.state != "" {
				f.credit(borrower, 100000, tc.state, tc.age)
			}
			c := f.application(borrower, 100000)
			f.collateral(c, 100000, "approved")
			if err := f.approve(c); !errors.Is(err, ErrRule) {
				t.Fatalf("credit guard: %v", err)
			}
		})
	}
	f.credit(f.manager, 100000, "active", 0)
	if err := f.approve(f.application(f.manager, 100000)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self review: %v", err)
	}
	borrower := f.account()
	f.credit(borrower, 1000000, "active", 0)
	if err := f.approve(f.application(borrower, 100001)); !errors.Is(err, ErrLimit) {
		t.Fatalf("pool limit: %v", err)
	}
	c := f.application(borrower, 100000)
	if _, err := f.s.Review(f.ctx, borrower, c.ID, ReviewInput{State: "approved", Version: c.Version}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unauthorized: %v", err)
	}
	f.s.LockAccounts = nil
	if err := f.approve(c); !errors.Is(err, ErrRule) {
		t.Fatalf("missing account fence: %v", err)
	}
}

// Both operations start together; database locks, rather than application
// timing, must decide which commitment gets the remaining capacity.
func assertSingleCreditRace(t *testing.T, a, b func() error, want error) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, run := range []func() error{a, b} {
		go func() { <-start; results <- run() }()
	}
	close(start)
	first, second := <-results, <-results
	if first != nil {
		first, second = second, first
	}
	if first != nil || !errors.Is(second, want) {
		t.Fatalf("race results=%v / %v, want one success and %v", first, second, want)
	}
}

func TestSingleCreditConcurrentLoans(t *testing.T) {
	f := newSingleCreditFixture(t, 1000000)
	borrower := f.account()
	f.credit(borrower, 100000, "active", 0)
	a, b := f.application(borrower, 70000), f.application(borrower, 70000)
	assertSingleCreditRace(t, func() error { return f.approve(a) }, func() error { return f.approve(b) }, ErrCoverage)
}

func TestSingleCreditConcurrentPoolReservations(t *testing.T) {
	f := newSingleCreditFixture(t, 100000)
	first, second := f.account(), f.account()
	f.credit(first, 100000, "active", 0)
	f.credit(second, 100000, "active", 0)
	a, b := f.application(first, 70000), f.application(second, 70000)
	assertSingleCreditRace(t, func() error { return f.approve(a) }, func() error { return f.approve(b) }, ErrLimit)
}

func TestSingleCreditConcurrentGuarantees(t *testing.T) {
	f := newSingleCreditFixture(t, 1000000)
	guarantor := f.account()
	f.credit(guarantor, 100000, "active", 0)
	a := f.guarantee(f.application(f.account(), 70000), guarantor, 70000, "invited")
	b := f.guarantee(f.application(f.account(), 70000), guarantor, 70000, "invited")
	accept := func(g store.Guarantee) error {
		_, err := f.s.DecideGuarantee(f.ctx, guarantor, g.ID, DecisionInput{State: "accepted", Version: g.Version})
		return err
	}
	assertSingleCreditRace(t, func() error { return accept(a) }, func() error { return accept(b) }, ErrLimit)
}

func TestSingleCreditConcurrentLoanAndGuarantee(t *testing.T) {
	f := newSingleCreditFixture(t, 1000000)
	account := f.account()
	f.credit(account, 100000, "active", 0)
	c := f.application(account, 70000)
	g := f.guarantee(f.application(f.account(), 70000), account, 70000, "invited")
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- f.approve(c) }()
	go func() {
		<-start
		_, err := f.s.DecideGuarantee(f.ctx, account, g.ID, DecisionInput{State: "accepted", Version: g.Version})
		results <- err
	}()
	close(start)
	first, second := <-results, <-results
	if first != nil {
		first, second = second, first
	}
	if first != nil || (!errors.Is(second, ErrLimit) && !errors.Is(second, ErrCoverage)) {
		t.Fatalf("loan/guarantee race=%v / %v", first, second)
	}
}

func TestRequiredCreditCoverageBoundaries(t *testing.T) {
	if got := requiredCreditCoverage(math.MaxInt64, math.MaxInt64-1, math.MaxInt64); got != math.MaxInt64-1 {
		t.Fatalf("overflow: %d", got)
	}
	if got := requiredCreditCoverage(100, 150, 50); got != 50 {
		t.Fatalf("existing over-limit responsibility: %d", got)
	}
}
