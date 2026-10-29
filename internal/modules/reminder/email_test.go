package reminder

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/modules/reminder/internal/store"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"time"
)

func emailFixture(t *testing.T) (*Service, string, string, *string) {
	t.Helper()
	s, admin, user := fixture(t)
	if err := s.SetEmailKey(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))); err != nil {
		t.Fatal(err)
	}
	_, err := s.SaveEmailConfiguration(context.Background(), admin, EmailSettingsInput{Enabled: true, Host: "smtp.example.com", Port: 465, ImplicitTLS: true, Username: "sender@example.com", Sender: "sender@example.com", Secret: "test-secret", Events: []string{"approval_pending"}, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	var body string
	s.emailSend = func(_ context.Context, _ smtpConfig, _ string, _ string, b string) Result {
		body = b
		return Result{State: "sent"}
	}
	return s, admin, user, &body
}

func TestEmailFinalRecheckAndMerge(t *testing.T) {
	s, admin, user, body := emailFixture(t)
	ctx := context.Background()
	if err := s.RequestEmailCode(ctx, user, "recipient@example.com"); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`[0-9]{8}`).FindString(*body)
	if _, err := s.ConfirmEmailCode(ctx, user, code); err != nil {
		t.Fatal(err)
	}
	prev := s.Details["approval_pending"]
	s.Details["approval_pending"] = func(ctx context.Context, u string, since time.Time) ([]remindermessage.Item, error) {
		items, err := prev(ctx, u, since)
		if err != nil {
			return nil, err
		}
		_, err = s.Pool.Exec(ctx, `UPDATE reminder_email_accounts SET enabled=false,revision=revision+1 WHERE account_id=$1`, user)
		return items, err
	}
	calls := 0
	s.emailSend = func(context.Context, smtpConfig, string, string, string) Result {
		calls++
		return Result{State: "sent"}
	}
	if err := s.deliverEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("sent after account opt-out")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.MergeAccountTx(ctx, tx, user, admin, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var original string
	err = s.Pool.QueryRow(ctx, `SELECT original_account_id::text FROM reminder_email_deliveries WHERE account_id=$1`, admin).Scan(&original)
	if err != nil || original != user {
		t.Fatalf("delivery history lost: %q %v", original, err)
	}
	var enabled bool
	if err = s.Pool.QueryRow(ctx, `SELECT enabled FROM reminder_email_accounts WHERE account_id=$1`, user).Scan(&enabled); err != nil || enabled {
		t.Fatalf("source email remains enabled: %v %v", enabled, err)
	}
	target, err := s.EmailAccount(ctx, admin)
	if err != nil || target.Verified || target.Enabled {
		t.Fatalf("source email copied to target: %#v %v", target, err)
	}
}

func TestEmailVerificationAndIndependentDelivery(t *testing.T) {
	s, admin, user, body := emailFixture(t)
	ctx := context.Background()
	if err := s.RequestEmailCode(ctx, user, "recipient@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestEmailCode(ctx, user, "recipient@example.com"); !errors.Is(err, ErrEmailRate) {
		t.Fatalf("rate limit: %v", err)
	}
	code := regexp.MustCompile(`[0-9]{8}`).FindString(*body)
	if code == "" {
		t.Fatal("verification code missing")
	}
	if _, err := s.ConfirmEmailCode(ctx, user, "0000000x"); !errors.Is(err, ErrEmailCode) {
		t.Fatalf("invalid code: %v", err)
	}
	a, err := s.ConfirmEmailCode(ctx, user, code)
	if err != nil || !a.Verified || !a.Enabled || a.Email != "recipient@example.com" {
		t.Fatalf("verify: %#v %v", a, err)
	}
	if _, err = s.ConfirmEmailCode(ctx, user, code); !errors.Is(err, ErrEmailCode) {
		t.Fatalf("replay: %v", err)
	}
	if err = s.deliver(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err = s.deliverEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	var eveCount, emailCount int
	if err = s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM reminder_deliveries WHERE account_id=$1),(SELECT count(*) FROM reminder_email_deliveries WHERE account_id=$1)`, user).Scan(&eveCount, &emailCount); err != nil {
		t.Fatal(err)
	}
	if eveCount != 1 || emailCount != 1 {
		t.Fatalf("channels must be independent: eve=%d email=%d", eveCount, emailCount)
	}
	if !strings.Contains(*body, "测试事项") {
		t.Fatalf("business details missing from email: %q", *body)
	}
	if err = s.deliverEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM reminder_email_deliveries WHERE account_id=$1`, user).Scan(&emailCount); err != nil || emailCount != 1 {
		t.Fatalf("duplicate: %d %v", emailCount, err)
	}
	if _, err = s.SaveEmailConfiguration(ctx, user, EmailSettingsInput{Revision: 2, Port: 465}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin: %v", err)
	}
	c, err := s.EmailConfiguration(ctx, admin)
	if err != nil || !c.SecretConfigured {
		t.Fatalf("configuration: %#v %v", c, err)
	}
	if _, err = s.SaveEmailAccount(ctx, user, a.Revision-1, false); !errors.Is(err, ErrVersion) {
		t.Fatalf("stale account revision: %v", err)
	}
}

func TestEmailUnknownDoesNotRetry(t *testing.T) {
	s, _, user, body := emailFixture(t)
	ctx := context.Background()
	if err := s.RequestEmailCode(ctx, user, "recipient@example.com"); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`[0-9]{8}`).FindString(*body)
	if _, err := s.ConfirmEmailCode(ctx, user, code); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.emailSend = func(context.Context, smtpConfig, string, string, string) Result {
		calls++
		return Result{State: "unknown", Reason: "confirmation_required"}
	}
	if err := s.deliverEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE reminder_email_deliveries SET created_at=now()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}
	if err := s.deliverEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("ambiguous delivery resent %d times", calls)
	}
	d, err := store.ListEmail(ctx, s.Pool)
	if err != nil || len(d) != 1 || d[0].State != "unknown" {
		t.Fatalf("delivery: %#v %v", d, err)
	}
}

func TestEmailSettingsSecretAndRevision(t *testing.T) {
	s, admin, _, _ := emailFixture(t)
	ctx := context.Background()
	var encrypted []byte
	if err := s.Pool.QueryRow(ctx, `SELECT secret_ciphertext FROM reminder_email_settings`).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), "test-secret") {
		t.Fatal("SMTP credential stored in plaintext")
	}
	if _, err := s.SaveEmailConfiguration(ctx, admin, EmailSettingsInput{Port: 465, Revision: 1}); !errors.Is(err, ErrVersion) {
		t.Fatalf("stale revision: %v", err)
	}
	c, err := s.SaveEmailConfiguration(ctx, admin, EmailSettingsInput{Host: "new.example.com", Port: 465, ImplicitTLS: true, Username: "new@example.com", Sender: "new@example.com", Enabled: false, Revision: 2, Events: []string{"approval_pending"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.SecretConfigured {
		t.Fatal("old secret retained for changed host")
	}
	_, err = s.SaveEmailConfiguration(ctx, admin, EmailSettingsInput{Host: "new.example.com", Port: 465, ImplicitTLS: true, Username: "new@example.com", Sender: "new@example.com", Enabled: true, Revision: c.Revision, Events: []string{"approval_pending"}})
	if !errors.Is(err, ErrEmailConfig) {
		t.Fatalf("enabled without secret: %v", err)
	}
}

func TestEmailRetryOnlyExplicitFailures(t *testing.T) {
	s, admin, user, body := emailFixture(t)
	ctx := context.Background()
	if err := s.RequestEmailCode(ctx, user, "recipient@example.com"); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`[0-9]{8}`).FindString(*body)
	if _, err := s.ConfirmEmailCode(ctx, user, code); err != nil {
		t.Fatal(err)
	}
	s.emailSend = func(context.Context, smtpConfig, string, string, string) Result {
		return Result{State: "failed", Reason: "mail_rejected"}
	}
	if err := s.deliverEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListEmail(ctx, s.Pool)
	if err != nil || len(rows) != 1 {
		t.Fatalf("records: %#v %v", rows, err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = store.RetryEmail(ctx, tx, rows[0].ID, admin); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE reminder_email_deliveries SET created_at=now()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.emailSend = func(context.Context, smtpConfig, string, string, string) Result {
		calls++
		return Result{State: "unknown"}
	}
	if err = s.deliverEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("retry calls %d", calls)
	}
	rows, err = store.ListEmail(ctx, s.Pool)
	if err != nil || len(rows) != 2 {
		t.Fatalf("retry records: %#v %v", rows, err)
	}
	tx, err = s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = store.RetryEmail(ctx, tx, rows[0].ID, admin); err == nil {
		t.Fatal("unknown outcome must not be retried")
	}
}

func TestEmailSettingsCanDisableWithMissingSource(t *testing.T) {
	s, admin, _, _ := emailFixture(t)
	s.Pending = nil
	_, err := s.SaveEmailConfiguration(context.Background(), admin, EmailSettingsInput{Host: "smtp.example.com", Port: 465, ImplicitTLS: true, Username: "sender@example.com", Sender: "sender@example.com", Enabled: false, Revision: 2, Events: []string{"approval_pending"}})
	if err != nil {
		t.Fatalf("disable while source unavailable: %v", err)
	}
}

func TestEmailCodeRejectsFurtherAttemptsAfterFiveFailures(t *testing.T) {
	s, _, user, body := emailFixture(t)
	ctx := context.Background()
	if err := s.RequestEmailCode(ctx, user, "recipient@example.com"); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`[0-9]{8}`).FindString(*body)
	wrong := "00000000"
	if code == wrong {
		wrong = "11111111"
	}
	for i := 0; i < 5; i++ {
		if _, err := s.ConfirmEmailCode(ctx, user, wrong); !errors.Is(err, ErrEmailCode) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	if _, err := s.ConfirmEmailCode(ctx, user, code); !errors.Is(err, ErrEmailCode) {
		t.Fatalf("locked code accepted: %v", err)
	}
}
