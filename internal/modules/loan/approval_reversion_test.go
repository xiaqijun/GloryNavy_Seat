package loan

import (
	"errors"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
	"testing"
)

func TestRevertLoanReviewKeepsHistoryAndBlocksPayments(t *testing.T) {
	f := newSingleCreditFixture(t, 10_000_000)
	borrower := f.account()
	original := f.application(borrower, 100000)
	approved, err := store.TransitionCase(f.ctx, f.db, original.ID, original.Version, "approved", f.manager, "approved")
	if err != nil {
		t.Fatal(err)
	}
	input := RevertReviewInput{Version: approved.Version, Note: "wrong decision", ConfirmedNotDisbursed: true}
	if _, err = f.s.RevertReview(f.ctx, borrower, approved.ID, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self: %v", err)
	}
	missing := input
	missing.ConfirmedNotDisbursed = false
	if _, err = f.s.RevertReview(f.ctx, f.manager, approved.ID, missing); !errors.Is(err, ErrInvalid) {
		t.Fatalf("confirmation: %v", err)
	}
	stale := input
	stale.Version--
	if _, err = f.s.RevertReview(f.ctx, f.manager, approved.ID, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	pending, err := f.s.RevertReview(f.ctx, f.manager, approved.ID, input)
	if err != nil || pending.State != "submitted" || pending.AcceptedAt != nil {
		t.Fatalf("reverted: %+v %v", pending, err)
	}
	var n int
	if err = f.db.QueryRow(f.ctx, `SELECT count(*) FROM loan_audit WHERE case_id=$1 AND action='revert_review'`, approved.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit %d %v", n, err)
	}
	rejected, err := f.s.Review(f.ctx, f.manager, pending.ID, ReviewInput{State: "rejected", Version: pending.Version, Note: "no"})
	if err != nil {
		t.Fatal(err)
	}
	pending, err = f.s.RevertReview(f.ctx, f.manager, rejected.ID, RevertReviewInput{Version: rejected.Version, Note: "wrong rejection"})
	if err != nil || pending.State != "submitted" {
		t.Fatalf("rejected rollback %+v %v", pending, err)
	}
	approved, err = store.TransitionCase(f.ctx, f.db, pending.ID, pending.Version, "approved", f.manager, "approved")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreatePayment(f.ctx, f.db, publicID("LNP"), approved.ID, "disbursement", f.manager, borrower, 111, 100000, "character", 111, 987654, f.manager)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RevertReview(f.ctx, f.manager, approved.ID, RevertReviewInput{Version: approved.Version, Note: "paid", ConfirmedNotDisbursed: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("payment guard: %v", err)
	}
}
