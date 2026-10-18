package app

import (
	"context"
	"glorynavy.local/seat/internal/modules/attendance"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/feedback"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
	"glorynavy.local/seat/internal/modules/reminder"
	"glorynavy.local/seat/internal/modules/sentry"
	"glorynavy.local/seat/internal/modules/welfare"
	"glorynavy.local/seat/internal/testutil"
	"testing"
	"time"
)

func TestReminderSourceOwnershipCurrentStateAndWiring(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	users := identity.New(db)
	tok, e := users.SignIn(ctx, 101, "Owner", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	one, _ := users.Session(ctx, tok)
	tok, e = users.SignIn(ctx, 202, "Other", "other", "")
	if e != nil {
		t.Fatal(e)
	}
	two, _ := users.Session(ctx, tok)
	admin := false
	checkAdmin := func(context.Context, string) (bool, error) { return admin, nil }
	w := &welfare.Service{Pool: db}
	ex := &exchange.Service{Pool: db}
	lo := &loan.Service{Pool: db, IsAdministrator: checkAdmin}
	at := &attendance.Service{Pool: db}
	fe := &feedback.Service{Pool: db, Administrator: checkAdmin}
	se := &sentry.Service{Pool: db}
	s := &reminder.Service{}
	enabled := []string{"welfare", "exchange", "loan", "attendance", "feedback", "sentry"}
	registerReminderSources(s, enabled, "welfare", w, "welfare_result")
	registerReminderSources(s, enabled, "exchange", ex, "exchange_result", "coin_change")
	registerReminderSources(s, enabled, "loan", lo, "loan_result", "guarantee_pending", "loan_due", "loan_overdue", "collateral_pending", "contribution_result")
	registerReminderSources(s, enabled, "attendance", at, "pap_change")
	registerReminderSources(s, enabled, "feedback", fe, "feedback_update", "feedback_pending")
	registerReminderSources(s, enabled, "sentry", se, "sentry_alert")
	since := time.Now().Add(-time.Hour)
	for kind, source := range s.Sources {
		if keys, e := source(ctx, one.UserID, since); e != nil || len(keys) != 0 {
			t.Fatalf("%s %v %v", kind, keys, e)
		}
	}
	execute := func(sql string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	execute(`INSERT INTO welfare_cases(account_id,corporation_id,kind,state,detail) VALUES($1,98530802,'srp','approved','{}')`, one.UserID)
	execute(`INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','test',gen_random_uuid(),100,'test')`, one.UserID)
	execute(`INSERT INTO loan_pools(lender_kind,lender_user_id,name,state,created_by) VALUES('personal',$1,'test','open',$1)`, two.UserID)
	execute(`INSERT INTO loan_cases(public_id,pool_id,borrower_account_id,borrower_character_id,principal_minor,interest_minor,total_due_minor,installment_count,interval_days,first_due_at,state) SELECT 'test',id,$1,101,100,0,100,2,30,now()+interval '1 day','active' FROM loan_pools`, one.UserID)
	execute(`INSERT INTO loan_installments(case_id,sequence,due_at,principal_minor,interest_minor) SELECT id,1,now()+interval '1 day',50,0 FROM loan_cases`)
	execute(`INSERT INTO loan_installments(case_id,sequence,due_at,principal_minor,interest_minor) SELECT id,2,now()-interval '1 day',50,0 FROM loan_cases`)
	execute(`INSERT INTO loan_guarantees(case_id,guarantor_account_id,amount_minor) SELECT id,$1,100 FROM loan_cases`, one.UserID)
	execute(`INSERT INTO loan_collateral_requests(public_id,case_id,source_character_id,recipient_character_id,selections,items,observed_at) SELECT 'test',id,101,202,'[]','[]',now() FROM loan_cases`)
	execute(`INSERT INTO loan_contributions(pool_id,account_id,lender_kind,source_character_id,amount_minor,state,funded_at) SELECT id,$1,'personal',101,100,'funded',now() FROM loan_pools`, one.UserID)
	execute(`INSERT INTO feedback_tickets(account_id,submitted_by,author_name,request_key,kind,title,body) VALUES($1,$1,'Owner',gen_random_uuid(),'bug','test','test')`, one.UserID)
	execute(`INSERT INTO feedback_messages(ticket_id,actor,author_name,staff,request_key,body) SELECT id,$1,'Other',true,gen_random_uuid(),'reply' FROM feedback_tickets`, two.UserID)
	execute(`INSERT INTO sentry_keys(id,account_id,operation_id,name,key_prefix,key_hash,permissions,status,updated_at) VALUES(gen_random_uuid(),$1,gen_random_uuid(),'test','test',decode(repeat('00',32),'hex'),'{read}','sync_error',now()-interval '20 minutes')`, one.UserID)
	for _, kind := range []string{"welfare_result", "coin_change", "loan_result", "loan_due", "loan_overdue", "guarantee_pending", "collateral_pending", "contribution_result", "feedback_update", "sentry_alert"} {
		source := s.Sources[kind]
		if keys, e := source(ctx, one.UserID, since); e != nil || len(keys) != 1 {
			t.Fatalf("own %s %v %v", kind, keys, e)
		}
		if keys, e := source(ctx, two.UserID, since); e != nil || len(keys) != 0 {
			t.Fatalf("cross-account %s %v %v", kind, keys, e)
		}
	}
	for _, kind := range []string{"welfare_result", "coin_change", "loan_result", "contribution_result", "feedback_update"} {
		if keys, e := s.Sources[kind](ctx, one.UserID, time.Now().Add(time.Hour)); e != nil || len(keys) != 0 {
			t.Fatalf("historical %s %v %v", kind, keys, e)
		}
	}
	if keys, e := fe.ReminderKeys(ctx, two.UserID, "feedback_pending", since); e != nil || len(keys) != 0 {
		t.Fatal("nonadmin feedback", keys, e)
	}
	admin = true
	if keys, e := fe.ReminderKeys(ctx, two.UserID, "feedback_pending", since); e != nil || len(keys) != 1 {
		t.Fatal("admin feedback", keys, e)
	}
	admin = false
	execute(`UPDATE loan_cases SET state='approved'`)
	if keys, e := lo.FundingReminderKeys(ctx, two.UserID, since); e != nil || len(keys) != 1 {
		t.Fatal("pool owner funding", keys, e)
	}
	if keys, e := lo.FundingReminderKeys(ctx, one.UserID, since); e != nil || len(keys) != 0 {
		t.Fatal("self funding", keys, e)
	}
	execute(`UPDATE loan_cases SET state='settled'`)
	for _, kind := range []string{"loan_due", "loan_overdue", "guarantee_pending", "collateral_pending"} {
		if keys, e := s.Sources[kind](ctx, one.UserID, since); e != nil || len(keys) != 0 {
			t.Fatalf("resolved %s %v %v", kind, keys, e)
		}
	}
	empty := &reminder.Service{}
	registerReminderSources(empty, []string{}, "feedback", fe, "feedback_update")
	if len(empty.Sources) != 0 {
		t.Fatal("disabled module registered")
	}
}
