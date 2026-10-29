package loan

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func TestPartialGuaranteeAndCollateralDoNotExceedGap(t *testing.T) {
	f := newAutoSecurityFixture(t)
	const gap = int64(200_000_000_000)
	g, err := f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222, AmountMinor: gap, AllocationMinor: gap / 2})
	if err != nil || g.AmountMinor != gap/2 {
		t.Fatal(g, err)
	}
	security, err := f.s.caseSecurity(f.ctx, f.db, f.c, 0)
	if err != nil || security.PendingMinor != gap/2 || security.UnallocatedMinor != gap/2 {
		t.Fatal(security, err)
	}
	if err = f.approve(f.c); !errors.Is(err, ErrCoverage) {
		t.Fatal("pending guarantee counted", err)
	}
	input := f.input
	input.CoverageMinor = gap / 2
	input.AllocationMinor = gap / 2
	input.Items = []AssetSelection{{ItemID: 501, Quantity: 1}}
	request, err := f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, input)
	if err != nil || request.CoverageMinor != gap/2 {
		t.Fatal(request, err)
	}
	security, err = f.s.caseSecurity(f.ctx, f.db, f.c, 0)
	if err != nil || security.UnallocatedMinor != 0 || security.EffectiveMinor != 0 || security.PendingMinor != gap {
		t.Fatal(security, err)
	}
	if err = f.approve(f.c); !errors.Is(err, ErrCoverage) {
		t.Fatal("pending collateral counted", err)
	}
}

func TestPartialSecurityConcurrentUsesLockedGap(t *testing.T) {
	f := newAutoSecurityFixture(t)
	const gap = int64(200_000_000_000)
	in := f.input
	in.CoverageMinor = gap
	in.AllocationMinor = 150_000_000_000
	start := make(chan struct{})
	result := make(chan error, 2)
	go func() {
		<-start
		_, err := f.s.AddGuarantee(f.ctx, f.borrower, f.c.ID, GuaranteeInput{GuarantorCharacterID: 222, AmountMinor: gap, AllocationMinor: 150_000_000_000})
		result <- err
	}()
	go func() { <-start; _, err := f.s.CreateCollateralRequest(f.ctx, f.borrower, f.c.ID, in); result <- err }()
	close(start)
	a, b := <-result, <-result
	if a != nil {
		a, b = b, a
	}
	if a != nil || !errors.Is(b, ErrConflict) {
		t.Fatal("expected one success and stale conflict", a, b)
	}
	s, err := f.s.caseSecurity(f.ctx, f.db, f.c, 0)
	if err != nil || s.PendingMinor != 150_000_000_000 || s.UnallocatedMinor != 50_000_000_000 {
		t.Fatal(s, err)
	}
}

func TestOldPolicyClientCannotClearBorrowerCap(t *testing.T) {
	f := newSingleCreditFixture(t, 1_000_000)
	if _, err := f.db.Exec(f.ctx, `UPDATE loan_credit_policy SET borrower_limit_bps=3500 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	p, err := store.ReadCreditPolicy(f.ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	var old map[string]any
	if err = json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	delete(old, "borrower_limit_bps")
	raw, _ = json.Marshal(old)
	h := Handler{Service: f.s, User: func(*http.Request) string { return f.manager }}
	w := httptest.NewRecorder()
	h.policyUpdate(w, httptest.NewRequest("POST", "/settings/policy", bytes.NewReader(raw)))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	p, err = store.ReadCreditPolicy(f.ctx, f.db)
	if err != nil || p.BorrowerLimitBPS != 3500 {
		t.Fatal(p, err)
	}
}

func TestApprovalRiskAcknowledgementAndConcentration(t *testing.T) {
	f := newSingleCreditFixture(t, 1_000_000)
	borrower := f.account()
	f.credit(borrower, 1_000_000, "active", 0)
	c := f.application(borrower, 400_000)
	if _, err := f.s.Review(f.ctx, f.manager, c.ID, ReviewInput{State: "approved", Version: c.Version}); !errors.Is(err, ErrRepaymentRisk) {
		t.Fatal("missing acknowledgment", err)
	}
	e, err := store.LatestCreditEvaluation(f.ctx, f.db, borrower)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.ReadCreditPolicy(f.ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	risk := repaymentRisk(c, e, p, nil, false, time.Now().UTC())
	if risk.Status != "unknown" || risk.Token == "" {
		t.Fatal(risk)
	}
	if _, err = f.s.Review(f.ctx, f.manager, c.ID, ReviewInput{State: "approved", Version: c.Version, RiskAcknowledgement: "stale"}); !errors.Is(err, ErrRepaymentRisk) {
		t.Fatal("stale acknowledgment", err)
	}
	if _, err = f.s.Review(f.ctx, f.manager, c.ID, ReviewInput{State: "approved", Version: c.Version, RiskAcknowledgement: risk.Token}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE loan_credit_policy SET borrower_limit_bps=5000 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	second := f.application(borrower, 100_001)
	if err = f.approve(second); !errors.Is(err, ErrLimit) {
		t.Fatal("borrower cap bypassed", err)
	}
}
