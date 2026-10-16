package loan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func TestCancelApplication(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	accounts := make([]string, 4)
	for i := range accounts {
		if err := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(&accounts[i]); err != nil {
			t.Fatal(err)
		}
	}
	borrower, manager, guarantor, invited := accounts[0], accounts[1], accounts[2], accounts[3]
	var poolID int64
	if err := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared) VALUES('corporation',98000001,'Test pool','open','{"collateral_haircut_bps":7000}',$1::uuid,true) RETURNING id`, manager).Scan(&poolID); err != nil {
		t.Fatal(err)
	}
	s := &Service{Pool: db, LockAccounts: func(context.Context, pgx.Tx, []string) error { return nil }, IsAdministrator: func(_ context.Context, user string) (bool, error) { return user == manager, nil }}
	newCase := func() store.Case {
		t.Helper()
		c, err := store.CreateCase(ctx, db, publicID("LN"), poolID, borrower, 111, 100000, 1000, 1200, 1, 30, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c := newCase()
	g, err := store.AddGuarantee(ctx, db, c.ID, guarantor, 222, 10000, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DecideGuarantee(ctx, db, g.ID, g.Version, "accepted"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddGuarantee(ctx, db, c.ID, invited, 333, 10000, ""); err != nil {
		t.Fatal(err)
	}
	for i, state := range []string{"proposed", "approved"} {
		col, err := store.AddCollateral(ctx, db, c.ID, borrower, "character", 111, int64(9000+i), json.RawMessage(`[{"type_id":34,"quantity":1}]`), 10000, 7000, 7000)
		if err != nil {
			t.Fatal(err)
		}
		if state == "approved" {
			if _, err = store.DecideCollateral(ctx, db, col.ID, col.Version, state); err != nil {
				t.Fatal(err)
			}
		}
	}
	before, err := store.Summary(ctx, db, poolID)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{manager, guarantor} {
		if _, err := s.CancelApplication(ctx, user, c.ID, c.Version); !errors.Is(err, ErrForbidden) {
			t.Fatalf("non-owner cancellation: %v", err)
		}
	}
	if _, err := s.CancelApplication(ctx, borrower, c.ID, c.Version+1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version: %v", err)
	}
	// Any failed audit must also roll back the case and every security release.
	if _, err = db.Exec(ctx, `CREATE FUNCTION fail_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='application_cancelled' THEN RAISE EXCEPTION 'test rollback'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_cancel BEFORE INSERT ON loan_audit FOR EACH ROW EXECUTE FUNCTION fail_cancel_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelApplication(ctx, borrower, c.ID, c.Version); err == nil {
		t.Fatal("audit failure did not abort")
	}
	unchanged, err := store.GetCase(ctx, db, c.ID)
	if err != nil || unchanged.State != "submitted" || unchanged.Version != c.Version {
		t.Fatalf("rollback case=%+v err=%v", unchanged, err)
	}
	coverage, err := store.Coverage(ctx, db, c.ID)
	if err != nil || coverage != 17000 {
		t.Fatalf("rollback coverage=%d err=%v", coverage, err)
	}
	if _, err = db.Exec(ctx, `DROP TRIGGER fail_cancel ON loan_audit`); err != nil {
		t.Fatal(err)
	}
	// Exercise the registered route, strict request decoding and response contract.
	actor := borrower
	h := Handler{Service: s, User: func(*http.Request) string { return actor }}
	router := chi.NewRouter()
	for _, route := range h.Module().Routes {
		router.Method(route.Method, route.Path, route.Handler)
	}
	request := func(body string) *httptest.ResponseRecorder {
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest("POST", fmt.Sprintf("/cases/%d/cancel", c.ID), strings.NewReader(body)))
		return out
	}
	for _, body := range []string{`{}`, `{"version":0}`, `{"version":1,"state":"active"}`} {
		if out := request(body); out.Code != 400 {
			t.Fatalf("invalid request=%s status=%d", body, out.Code)
		}
	}
	actor = manager
	if out := request(`{"version":1}`); out.Code != 403 {
		t.Fatalf("admin status=%d", out.Code)
	}
	actor = borrower
	if out := request(`{"version":1}`); out.Code != 200 {
		t.Fatalf("cancel status=%d %s", out.Code, out.Body)
	}
	if out := request(`{"version":1}`); out.Code != 409 {
		t.Fatalf("duplicate status=%d", out.Code)
	}
	gs, err := store.ListGuarantees(ctx, db, c.ID)
	if err != nil || len(gs) != 2 {
		t.Fatal(gs, err)
	}
	for _, row := range gs {
		if row.State != "released" {
			t.Fatalf("guarantee not released: %+v", row)
		}
	}
	cols, err := store.ListCollateral(ctx, db, c.ID)
	if err != nil || len(cols) != 2 {
		t.Fatal(cols, err)
	}
	for _, row := range cols {
		if row.State != "released" {
			t.Fatalf("collateral not released: %+v", row)
		}
	}
	used, err := store.UsedResponsibility(ctx, db, guarantor)
	if err != nil || used != 0 {
		t.Fatalf("guarantor still liable: %d %v", used, err)
	}
	after, err := store.Summary(ctx, db, poolID)
	if err != nil || before != after {
		t.Fatalf("cash changed: before=%+v after=%+v err=%v", before, after, err)
	}
	var audits, payments int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM loan_audit WHERE case_id=$1 AND action='application_cancelled'),(SELECT count(*) FROM loan_payment_intents WHERE case_id=$1)`, c.ID).Scan(&audits, &payments); err != nil || audits != 1 || payments != 0 {
		t.Fatalf("audits=%d payments=%d err=%v", audits, payments, err)
	}
	for _, state := range []string{"awaiting_acceptance", "approved", "funding", "active", "settled", "rejected", "cancelled", "defaulted"} {
		row := newCase()
		if _, err := db.Exec(ctx, `UPDATE loan_cases SET state=$2 WHERE id=$1`, row.ID, state); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CancelApplication(ctx, borrower, row.ID, row.Version); !errors.Is(err, ErrConflict) {
			t.Fatalf("cancel %s: %v", state, err)
		}
	}
	for range 5 {
		row := newCase()
		start := make(chan struct{})
		errorsCh := make(chan error, 2)
		go func() { <-start; _, e := s.CancelApplication(ctx, borrower, row.ID, row.Version); errorsCh <- e }()
		go func() {
			<-start
			_, e := s.Review(ctx, manager, row.ID, ReviewInput{State: "rejected", Version: row.Version})
			errorsCh <- e
		}()
		close(start)
		a, b := <-errorsCh, <-errorsCh
		if !((a == nil && errors.Is(b, ErrConflict)) || (b == nil && errors.Is(a, ErrConflict))) {
			t.Fatalf("concurrent decisions: %v / %v", a, b)
		}
	}
	// Pause a collateral request after its initial state check, then withdraw.
	row := newCase()
	entered, resume := make(chan struct{}), make(chan struct{})
	s.Contracts = ContractAdapter{ReadFunc: func(context.Context, string, string, int64, int64) (Contract, error) {
		close(entered)
		<-resume
		return Contract{ID: 9900, OwnerKind: "character", OwnerID: 111, Type: "item_exchange", Status: "finished", ItemsReady: true, Items: json.RawMessage(`[{"type_id":34,"quantity":1}]`), IssuerID: 111, AcceptorID: 444, Price: "0", Reward: "0"}, nil
	}}
	s.CollateralValue = func(context.Context, json.RawMessage) (int64, error) { return 10000, nil }
	if _, err := db.Exec(ctx, `UPDATE loan_pools SET custodian_character_id=444 WHERE id=$1`, poolID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := s.AddCollateral(ctx, borrower, row.ID, CollateralInput{OwnerAccountID: borrower, ContractKind: "character", ContractOwnerID: 111, ContractID: 9900, HaircutBPS: 7000})
		result <- e
	}()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("collateral never reached evidence read: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("collateral timed out")
	}
	if _, err := s.CancelApplication(ctx, borrower, row.ID, row.Version); err != nil {
		t.Fatal(err)
	}
	close(resume)
	if err := <-result; !errors.Is(err, ErrConflict) {
		t.Fatalf("late collateral: %v", err)
	}
	cols, err = store.ListCollateral(ctx, db, row.ID)
	if err != nil || len(cols) != 0 {
		t.Fatalf("late collateral persisted: %+v %v", cols, err)
	}
}
