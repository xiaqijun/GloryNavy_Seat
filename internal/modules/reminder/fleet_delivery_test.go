package reminder

import (
	"context"
	"regexp"
	"testing"
)

func TestFleetEmailRespectsVerificationOptOutAndRevision(t *testing.T) {
	s, _, user, body := emailFixture(t)
	ctx := context.Background()
	version, err := s.FleetTarget(ctx, "email")
	if err != nil {
		t.Fatal(err)
	}
	if r := s.FleetEmail(ctx, user, version, "Fleet", "body"); r.State != "skipped" {
		t.Fatal(r)
	}
	if err = s.RequestEmailCode(ctx, user, "recipient@example.com"); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`[0-9]{8}`).FindString(*body)
	account, err := s.ConfirmEmailCode(ctx, user, code)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.emailSend = func(context.Context, smtpConfig, string, string, string) Result {
		calls++
		return Result{State: "sent"}
	}
	if r := s.FleetEmail(ctx, user, version, "Fleet", "body"); r.State != "sent" || calls != 1 {
		t.Fatal(r, calls)
	}
	if _, err = s.SaveEmailAccount(ctx, user, account.Revision, false); err != nil {
		t.Fatal(err)
	}
	if r := s.FleetEmail(ctx, user, version, "Fleet", "body"); r.State != "skipped" || calls != 1 {
		t.Fatal(r, calls)
	}
	if r := s.FleetEmail(ctx, user, "stale", "Fleet", "body"); r.State != "failed" || calls != 1 {
		t.Fatal(r, calls)
	}
}
