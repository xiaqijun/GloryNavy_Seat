package reminder

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
	"glorynavy.local/seat/internal/testutil"
	"sync"
	"sync/atomic"
	"testing"
)

func fixture(t *testing.T) (*Service, string, string) {
	t.Helper()
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 101, "Sender", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	admin, _ := users.Session(ctx, token)
	token, err = users.SignIn(ctx, 202, "Recipient", "recipient", "")
	if err != nil {
		t.Fatal(err)
	}
	recipient, _ := users.Session(ctx, token)
	s := &Service{Pool: pool, Enabled: true, Origin: "https://example.com", LockAccounts: users.LockActiveAccounts}
	s.Administrator = func(_ context.Context, u string) (bool, error) { return u == admin.UserID, nil }
	s.Own = func(_ context.Context, u string) ([]Character, error) {
		if u != admin.UserID {
			return []Character{}, nil
		}
		return []Character{{ID: 101, Name: "Sender", Owner: []byte("owner"), Ready: true, Generation: 1}}, nil
	}
	s.Main = func(_ context.Context, u string) (Character, error) {
		if u != recipient.UserID {
			return Character{}, pgx.ErrNoRows
		}
		return Character{ID: 202, Name: "Recipient", Owner: []byte("recipient")}, nil
	}
	s.Pending = func(context.Context, string) ([]string, error) { return []string{"pending-a", "cancel-b"}, nil }
	s.Send = func(context.Context, Character, int64, string, string) Result { return Result{State: "sent", ID: 999} }
	if _, err = s.Save(ctx, admin.UserID, SaveInput{Enabled: true, SenderID: 101, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	return s, admin.UserID, recipient.UserID
}
func TestDurableDedupConcurrentRestartAndNewPhase(t *testing.T) {
	s, _, u := fixture(t)
	ctx := context.Background()
	var sends atomic.Int32
	s.Send = func(_ context.Context, ch Character, id int64, subject, body string) Result {
		if id != 202 || ch.ID != 101 || subject != "待审批提醒" {
			t.Error("wrong recipient or sender")
		}
		sends.Add(1)
		return Result{State: "sent", ID: 999}
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.deliver(ctx, u); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if sends.Load() != 1 {
		t.Fatal("duplicate send", sends.Load())
	}
	restarted := *s
	if err := restarted.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	if sends.Load() != 1 {
		t.Fatal("restart replay")
	}
	s.Pending = func(context.Context, string) ([]string, error) { return nil, nil }
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	s.Pending = func(context.Context, string) ([]string, error) { return []string{"pending-a"}, nil }
	s.Pool.Exec(ctx, `UPDATE reminder_deliveries SET created_at=now()-interval '6 minutes'`)
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	if sends.Load() != 2 {
		t.Fatal("new phase missed")
	}
}
func TestUncertainDeliveryNeverRetriesAndInterruptedPersists(t *testing.T) {
	s, admin, u := fixture(t)
	ctx := context.Background()
	n := 0
	s.Send = func(context.Context, Character, int64, string, string) Result {
		n++
		return Result{State: "unknown", Reason: "confirmation_required"}
	}
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Deliveries(ctx, admin, "")
	if len(rows) != 1 || rows[0].State != "unknown" {
		t.Fatal(rows)
	}
	if err := s.Retry(ctx, admin, rows[0].ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unknown retried", err)
	}
	s.Pool.Exec(ctx, `UPDATE reminder_deliveries SET state='sending',created_at=now()-interval '6 minutes'`)
	if err := store.Maintain(ctx, s.Pool); err != nil {
		t.Fatal(err)
	}
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("crashed delivery replayed")
	}
	rows, _ = s.Deliveries(ctx, admin, "")
	if rows[0].State != "unknown" {
		t.Fatal(rows)
	}
}
func TestFailedDeliveryCanRetryWithCurrentContext(t *testing.T) {
	s, admin, u := fixture(t)
	ctx := context.Background()
	n := 0
	s.Send = func(context.Context, Character, int64, string, string) Result {
		n++
		if n == 1 {
			return Result{State: "failed", Reason: "mail_rejected"}
		}
		return Result{State: "sent", ID: 7}
	}
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Deliveries(ctx, admin, "")
	if err := s.Retry(ctx, u, rows[0].ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("non admin retry", err)
	}
	if err := s.Retry(ctx, admin, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	s.Pool.Exec(ctx, `UPDATE reminder_deliveries SET created_at=now()-interval '6 minutes'`)
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatal(n)
	}
}
func TestFinalBoundaryRejectsRevocationMainChangeAndSourceFailure(t *testing.T) {
	for _, kind := range []string{"revoked", "main_changed", "sender_changed", "source_failure", "pending_changed", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			s, _, u := fixture(t)
			ctx := context.Background()
			calls := 0
			sent := false
			own := s.Own
			main := s.Main
			s.Pending = func(context.Context, string) ([]string, error) {
				calls++
				if calls == 2 {
					switch kind {
					case "source_failure":
						return nil, errors.New("source unavailable")
					case "revoked":
						return nil, nil
					case "pending_changed":
						return []string{"new"}, nil
					}
				}
				return []string{"a"}, nil
			}
			s.Main = func(ctx context.Context, u string) (Character, error) {
				ch, e := main(ctx, u)
				if calls > 0 && kind == "main_changed" {
					ch.ID = 303
				}
				return ch, e
			}
			ownCalls := 0
			s.Own = func(ctx context.Context, u string) ([]Character, error) {
				ownCalls++
				ch, e := own(ctx, u)
				if ownCalls > 1 && kind == "sender_changed" {
					ch[0].Generation++
				}
				return ch, e
			}
			if kind == "disabled" {
				s.Enabled = false
			}
			s.Send = func(context.Context, Character, int64, string, string) Result {
				sent = true
				return Result{State: "sent", ID: 1}
			}
			if e := s.deliver(ctx, u); e != nil {
				t.Fatal(e)
			}
			if sent {
				t.Fatal("sent after boundary changed")
			}
		})
	}
}
func TestSourceFailureDoesNotEraseReceiptsAndMergeKeepsHistory(t *testing.T) {
	s, admin, u := fixture(t)
	ctx := context.Background()
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	s.Pending = func(context.Context, string) ([]string, error) { return nil, errors.New("offline") }
	if e := s.deliver(ctx, u); e == nil {
		t.Fatal("source failure hidden")
	}
	var n int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM reminder_receipts WHERE active`).Scan(&n)
	if n != 2 {
		t.Fatal(n)
	}
	tx, _ := s.Pool.Begin(ctx)
	if _, e := s.MergeAccountTx(ctx, tx, u, admin, true); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	var original, account string
	s.Pool.QueryRow(ctx, `SELECT original_account_id::text,account_id::text FROM reminder_deliveries`).Scan(&original, &account)
	if original != u || account != admin {
		t.Fatal(original, account)
	}
	tx, _ = s.Pool.Begin(ctx)
	if _, e := s.MergeAccountTx(ctx, tx, admin, u, true); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	c, e := store.ReadSettings(ctx, s.Pool, false)
	if e != nil || c.Enabled {
		t.Fatal("sender consent transferred", c, e)
	}
}
func TestSettingsRequireOwnScopeCurrentAdminAndRevision(t *testing.T) {
	s, admin, u := fixture(t)
	ctx := context.Background()
	if _, e := s.Save(ctx, u, SaveInput{Enabled: true, SenderID: 101, Revision: 2}); !errors.Is(e, ErrForbidden) {
		t.Fatal(e)
	}
	if _, e := s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 202, Revision: 2}); !errors.Is(e, ErrSender) {
		t.Fatal(e)
	}
	if _, e := s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 101, Revision: 1}); !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
	s.Own = func(context.Context, string) ([]Character, error) {
		return []Character{{ID: 101, Owner: []byte("owner")}}, nil
	}
	if _, e := s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 101, Revision: 2}); !errors.Is(e, ErrSender) {
		t.Fatal(e)
	}
	if _, e := s.Save(ctx, admin, SaveInput{Enabled: false, SenderID: 101, Revision: 2}); e != nil {
		t.Fatal("cannot pause unready sender", e)
	}
}
