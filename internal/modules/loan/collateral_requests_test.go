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

func TestCollateralRequestLifecycle(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	var borrower, manager string
	for _, v := range []*string{&borrower, &manager} {
		if err := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.RecordCreditEvaluation(ctx, db, store.CreditEvaluation{AccountID: borrower, PolicyVersion: creditPolicyRuleVersion(defaultCreditPolicy()), Score: 80, TotalLimitMinor: 30000, State: "active", FactorScores: json.RawMessage(`{}`), Evidence: json.RawMessage(`{}`), EvidenceCutoff: time.Now(), EvaluatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var poolID int64
	if err := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared,custodian_character_id) VALUES('corporation',98000001,'Test pool','open','{"collateral_haircut_bps":7000}',$1::uuid,true,333) RETURNING id`, manager).Scan(&poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE test_claims(id bigint PRIMARY KEY,ref bigint NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	newCase := func() store.Case {
		t.Helper()
		c, e := store.CreateCase(ctx, db, publicID("LN"), poolID, borrower, 111, 100000, 1000, 1200, 1, 30, nil, nil)
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	catalog := AssetCatalog{CharacterID: 222, ObservedAt: time.Now(), Items: []AssetChoice{{ItemID: 501, TypeID: 34, Quantity: 10, LocationID: 6001, Items: []AssetItem{{ItemID: 501, TypeID: 34, Quantity: 10, Name: "Tritanium", Included: true}}}, {ItemID: 502, TypeID: 35, Quantity: 1, LocationID: 6002, Items: []AssetItem{{ItemID: 502, TypeID: 35, Quantity: 1, Included: true}}}}}
	s := &Service{Pool: db, LockAccounts: func(context.Context, pgx.Tx, []string) error { return nil }, IsAdministrator: func(_ context.Context, a string) (bool, error) { return a == manager, nil }, AssetChoices: func(_ context.Context, a string, ch int64) (AssetCatalog, error) {
		if a != borrower || ch != 222 {
			return AssetCatalog{}, ErrForbidden
		}
		copy := catalog
		copy.Items = append([]AssetChoice(nil), catalog.Items...)
		return copy, nil
	}, CollateralValue: func(context.Context, json.RawMessage) (int64, error) { return 100000, nil }}
	setTestOfficers(s, Character{ID: 333, AccountID: manager, IsMain: true})
	input := CollateralRequestInput{CharacterID: 222, Items: []AssetSelection{{ItemID: 501, Quantity: 6}}}
	c := newCase()
	if _, e := s.CollateralAssets(ctx, manager, c.ID, 222); !errors.Is(e, ErrForbidden) {
		t.Fatal("foreign asset read", e)
	}
	if _, e := s.CreateCollateralRequest(ctx, manager, c.ID, input); !errors.Is(e, ErrForbidden) {
		t.Fatal("foreign create", e)
	}
	badInputs := []CollateralRequestInput{{CharacterID: 999, Items: input.Items}, {CharacterID: 222, Items: []AssetSelection{{501, 11}}}, {CharacterID: 222, Items: []AssetSelection{{501, 1}, {501, 1}}}, {CharacterID: 222, Items: []AssetSelection{{501, 1}, {502, 1}}}, {CharacterID: 222, Items: []AssetSelection{{999, 1}}}}
	for _, in := range badInputs {
		if _, e := s.CreateCollateralRequest(ctx, borrower, c.ID, in); e == nil {
			t.Fatalf("accepted invalid selection: %+v", in)
		}
	}
	// Two requests cannot reserve more than the authoritative asset quantity.
	other := newCase()
	start := make(chan struct{})
	result := make(chan error, 2)
	for _, id := range []int64{c.ID, other.ID} {
		go func(id int64) { <-start; _, e := s.CreateCollateralRequest(ctx, borrower, id, input); result <- e }(id)
	}
	close(start)
	a, b := <-result, <-result
	if !((a == nil && errors.Is(b, ErrConflict)) || (b == nil && errors.Is(a, ErrConflict))) {
		t.Fatal("concurrent over-reservation", a, b)
	}
	rows, e := store.ListCollateralRequests(ctx, db, c.ID)
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) == 0 {
		c = other
		rows, e = store.ListCollateralRequests(ctx, db, c.ID)
	}
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	request := rows[0]
	available, e := s.CollateralAssets(ctx, borrower, c.ID, 222)
	if e != nil || available.Items[0].Available != 4 {
		t.Fatal(available, e)
	}
	coverage, e := store.Coverage(ctx, db, c.ID)
	if e != nil || coverage != 0 {
		t.Fatal("pending counted as coverage", coverage, e)
	}
	if _, e = s.CancelCollateralRequest(ctx, manager, request.ID, request.Version); !errors.Is(e, ErrForbidden) {
		t.Fatal("foreign cancellation", e)
	}
	if _, e = s.CancelCollateralRequest(ctx, borrower, request.ID, request.Version+1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale cancellation", e)
	}
	contract := Contract{ID: 9001, OwnerKind: "character", OwnerID: 222, Type: "item_exchange", Status: "finished", Price: "0", Reward: "0", IssuerID: 222, AcceptorID: 333, ItemsReady: true, Items: json.RawMessage(`[{"type_id":34,"quantity":6,"included":true}]`), Completed: time.Now().UTC().Format(time.RFC3339)}
	s.Contracts = ContractAdapter{ReadFunc: func(context.Context, string, string, int64, int64) (Contract, error) { return contract, nil }, ClaimFunc: func(ctx context.Context, tx pgx.Tx, id int64, _ string, ref int64) error {
		_, e := tx.Exec(ctx, `INSERT INTO test_claims VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET ref=test_claims.ref WHERE test_claims.ref=EXCLUDED.ref`, id, ref)
		return e
	}}
	s.ReadContractTx = func(context.Context, pgx.Tx, string, string, int64, int64) (Contract, error) { return contract, nil }
	in := CollateralInput{RequestID: request.ID, OwnerAccountID: borrower, ContractKind: "character", ContractOwnerID: 222, ContractID: 9001, HaircutBPS: 7000}
	originalValue := s.CollateralValue
	s.CollateralValue = func(context.Context, json.RawMessage) (int64, error) { return 87499, nil }
	if _, err := s.AddCollateral(ctx, borrower, c.ID, in); !errors.Is(err, ErrCollateralValue) {
		t.Fatal("price fell below frozen target", err)
	}
	s.CollateralValue = originalValue
	valid := contract
	mutations := []func(*Contract){func(c *Contract) { c.IssuerID = 111 }, func(c *Contract) { c.AcceptorID = 444 }, func(c *Contract) { c.Price = "1" }, func(c *Contract) { c.Reward = "1" }, func(c *Contract) { c.Status = "outstanding" }, func(c *Contract) { c.Completed = request.CreatedAt.Add(-time.Hour).Format(time.RFC3339) }, func(c *Contract) { c.Items = json.RawMessage(`[{"type_id":34,"quantity":5,"included":true}]`) }, func(c *Contract) {
		c.Items = json.RawMessage(`[{"type_id":34,"quantity":6,"included":true},{"type_id":35,"quantity":1,"included":true}]`)
	}, func(c *Contract) { c.Items = json.RawMessage(`[{"type_id":34,"quantity":6,"included":false}]`) }, func(c *Contract) {
		c.Items = json.RawMessage(`[{"type_id":34,"quantity":6,"included":true,"raw_quantity":-2}]`)
	}}
	for _, mutate := range mutations {
		contract = valid
		mutate(&contract)
		if _, e := s.AddCollateral(ctx, borrower, c.ID, in); !errors.Is(e, ErrContract) {
			t.Fatal("accepted invalid delivery", e)
		}
	}
	contract = valid
	if _, e = db.Exec(ctx, `CREATE FUNCTION fail_delivery_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='collateral_delivered' THEN RAISE EXCEPTION 'test rollback'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_delivery BEFORE INSERT ON loan_audit FOR EACH ROW EXECUTE FUNCTION fail_delivery_audit()`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddCollateral(ctx, borrower, c.ID, in); e == nil {
		t.Fatal("audit failure did not abort")
	}
	current, e := store.GetCollateralRequest(ctx, db, request.ID)
	if e != nil || current.State != "awaiting_delivery" {
		t.Fatal(current, e)
	}
	var count int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM test_claims`).Scan(&count); e != nil || count != 0 {
		t.Fatal("claim not rolled back", count, e)
	}
	if _, e = db.Exec(ctx, `DROP TRIGGER fail_delivery ON loan_audit`); e != nil {
		t.Fatal(e)
	}
	col, e := s.AddCollateral(ctx, borrower, c.ID, in)
	if e != nil {
		t.Fatal(e)
	}
	if col.HaircutBPS != 8000 || col.CoveredMinor != 70000 {
		t.Fatal("new delivery did not use 80% and frozen target", col)
	}
	current, e = store.GetCollateralRequest(ctx, db, request.ID)
	if e != nil || current.State != "delivered" || current.CollateralID == nil || *current.CollateralID != col.ID {
		t.Fatal(current, e)
	}
	if _, e = s.AddCollateral(ctx, borrower, c.ID, in); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate delivery", e)
	}
	if _, e = s.CancelCollateralRequest(ctx, borrower, request.ID, current.Version); !errors.Is(e, ErrConflict) {
		t.Fatal("cancelled delivered request", e)
	}
	if _, e = s.DecideCollateral(ctx, borrower, col.ID, DecisionInput{Version: col.Version, State: "approved"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("self approval", e)
	}
	if _, e = s.DecideCollateral(ctx, manager, col.ID, DecisionInput{Version: col.Version, State: "approved"}); e != nil {
		t.Fatal("approval with different owned source character", e)
	}
	coverage, e = store.Coverage(ctx, db, c.ID)
	if e != nil || coverage != 70000 {
		t.Fatal(coverage, e)
	}
	next := newCase()
	pending, e := s.CreateCollateralRequest(ctx, borrower, next.ID, input)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CancelApplication(ctx, borrower, next.ID, next.Version); e != nil {
		t.Fatal(e)
	}
	pending, e = store.GetCollateralRequest(ctx, db, pending.ID)
	if e != nil || pending.State != "cancelled" {
		t.Fatal(pending, e)
	}
	// Account merges move pending ownership via the case and report it in preview.
	next = newCase()
	pending, e = s.CreateCollateralRequest(ctx, borrower, next.ID, input)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	preview, e := s.MergeAccountTx(ctx, tx, borrower, manager, true)
	if e != nil || !strings.Contains(string(preview), `"collateral_requests":3`) {
		t.Fatal(string(preview), e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Detail(ctx, borrower, next.ID); !errors.Is(e, ErrForbidden) {
		t.Fatal("old owner retained access", e)
	}
	if _, e = s.CancelCollateralRequest(ctx, manager, pending.ID, pending.Version); e != nil {
		t.Fatal("merged owner cannot cancel", e)
	}
}

func TestCollateralRequestRoutes(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	var borrower string
	if e := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(&borrower); e != nil {
		t.Fatal(e)
	}
	if _, err := store.RecordCreditEvaluation(ctx, db, store.CreditEvaluation{AccountID: borrower, PolicyVersion: creditPolicyRuleVersion(defaultCreditPolicy()), Score: 80, State: "active", FactorScores: json.RawMessage(`{}`), Evidence: json.RawMessage(`{}`), EvidenceCutoff: time.Now(), EvaluatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var poolID int64
	if e := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared,custodian_character_id) VALUES('corporation',98000001,'Pool','open','{}',$1::uuid,true,333) RETURNING id`, borrower).Scan(&poolID); e != nil {
		t.Fatal(e)
	}
	c, e := store.CreateCase(ctx, db, publicID("LN"), poolID, borrower, 111, 100000, 0, 0, 1, 30, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	s := &Service{Pool: db, CollateralValue: func(context.Context, json.RawMessage) (int64, error) { return 125000, nil }, LockAccounts: func(context.Context, pgx.Tx, []string) error { return nil }, AssetChoices: func(context.Context, string, int64) (AssetCatalog, error) {
		return AssetCatalog{CharacterID: 111, ObservedAt: time.Now(), Items: []AssetChoice{{ItemID: 1, TypeID: 34, Quantity: 1, LocationID: 60, Items: []AssetItem{{ItemID: 1, TypeID: 34, Quantity: 1, Included: true}}}}}, nil
	}}
	setTestOfficers(s, Character{ID: 333, IsMain: true})
	h := Handler{Service: s, User: func(*http.Request) string { return borrower }}
	router := chi.NewRouter()
	for _, route := range h.Module().Routes {
		router.Method(route.Method, route.Path, route.Handler)
	}
	for _, body := range []string{`{}`, `{"character_id":"111","items":[{"item_id":"1","quantity":"1"}],"valuation_minor":999}`, `{"character_id":111,"items":[]}`} {
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest("POST", fmt.Sprintf("/cases/%d/collateral-requests", c.ID), strings.NewReader(body)))
		if out.Code != 400 {
			t.Fatal(out.Code, out.Body)
		}
	}
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest("POST", fmt.Sprintf("/cases/%d/collateral-requests", c.ID), strings.NewReader(`{"character_id":"111","items":[{"item_id":"1","quantity":"1"}]}`)))
	if out.Code != 200 {
		t.Fatal(out.Code, out.Body)
	}
	out = httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest("GET", fmt.Sprintf("/cases/%d/collateral-assets?character_id=111", c.ID), nil))
	if out.Code != 200 || !strings.Contains(out.Body.String(), `"available":"0"`) {
		t.Fatal(out.Code, out.Body)
	}
}
