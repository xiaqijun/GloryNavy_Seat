package reminder

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
)

type emailScanArgs struct{}

func (emailScanArgs) Kind() string { return "reminder.email-scan.v1" }

type emailSendArgs struct {
	Account string `json:"account"`
}

func (emailSendArgs) Kind() string { return "reminder.email-send.v1" }

type emailScanWorker struct {
	river.WorkerDefaults[emailScanArgs]
	runtime *Jobs
}
type emailSendWorker struct {
	river.WorkerDefaults[emailSendArgs]
	runtime *Jobs
}

func (w *emailScanWorker) Work(ctx context.Context, _ *river.Job[emailScanArgs]) error {
	s := w.runtime.Service
	if !s.Enabled {
		return river.JobSnooze(time.Hour)
	}
	if err := store.MaintainEmail(ctx, s.Pool); err != nil {
		return err
	}
	c, err := store.ReadEmailSettings(ctx, s.Pool, false)
	if err != nil || !c.Enabled {
		return err
	}
	users, err := store.EmailUsers(ctx, s.Pool)
	if err != nil {
		return err
	}
	for _, id := range users {
		if _, err = w.runtime.queue.Insert(ctx, emailSendArgs{Account: id}, &river.InsertOpts{Queue: "reminder", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); err != nil {
			return err
		}
	}
	return nil
}
func (w *emailSendWorker) Work(ctx context.Context, j *river.Job[emailSendArgs]) error {
	if !w.runtime.Service.Enabled {
		return river.JobSnooze(time.Hour)
	}
	return w.runtime.Service.deliverEmail(ctx, j.Args.Account)
}

func (s *Service) deliverEmail(ctx context.Context, account string) error {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	locked, err := store.Lock(ctx, conn, "email:"+account)
	if err != nil || !locked {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if store.Unlock(c, conn, "email:"+account) != nil {
			_ = conn.Conn().Close(c)
		}
	}()
	c, err := store.ReadEmailSettings(ctx, conn, false)
	if err != nil || !c.Enabled {
		return err
	}
	a, err := store.ReadEmailAccount(ctx, conn, account, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil || !a.Enabled || !a.Verified {
		return err
	}
	secret, err := s.openEmailSecret(c.Secret)
	if err != nil {
		return err
	}
	order, err := store.EmailEventOrder(ctx, conn, account, c.Events)
	if err != nil {
		return err
	}
	type snapshot struct {
		event Event
		keys  []string
	}
	var snapshots []snapshot
	var sourceError error
	for _, kind := range order {
		event, ok := eventByID(kind)
		source := s.source(kind)
		if !ok || source == nil {
			continue
		}
		keys, e := readSource(ctx, source, account, kind, c.Started[kind])
		if e != nil {
			sourceError = errors.Join(sourceError, fmt.Errorf("email reminder source %s unavailable", kind))
			continue
		}
		snapshots = append(snapshots, snapshot{event, keys})
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return ErrForbidden
	}
	if err = s.LockAccounts(ctx, tx, []string{account}); err != nil {
		return err
	}
	current, err := store.ReadEmailSettings(ctx, tx, true)
	if err != nil {
		return err
	}
	if !current.Enabled || current.Revision != c.Revision {
		return nil
	}
	currentAccount, err := store.ReadEmailAccount(ctx, tx, account, true)
	if err != nil {
		return err
	}
	if !currentAccount.Enabled || !currentAccount.Verified || currentAccount.Revision != a.Revision || currentAccount.Email != a.Email {
		return nil
	}
	var event Event
	var due, keys []string
	for _, snap := range snapshots {
		pending, e := store.ReconcileEmail(ctx, tx, account, snap.keys, snap.event.ID)
		if e != nil {
			return e
		}
		if len(due) == 0 && len(pending) > 0 {
			event, keys, due = snap.event, snap.keys, pending
		}
	}
	limited, err := store.EmailRateLimited(ctx, tx, account)
	if err != nil {
		return err
	}
	if len(due) == 0 || limited {
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return sourceError
	}
	if len(due) > mailBatchSize {
		due = due[:mailBatchSize]
	}
	id, err := store.ClaimEmail(ctx, tx, account, a.Email, event.ID, due)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	result := Result{State: "failed", Reason: "context_changed"}
	body, detailErr := s.message(ctx, account, event, c.Started[event.ID], due)
	current, err = store.ReadEmailSettings(ctx, conn, false)
	currentAccount, accountErr := store.ReadEmailAccount(ctx, conn, account, false)
	if detailErr == nil && err == nil && accountErr == nil && current.Enabled && current.Revision == c.Revision && currentAccount.Enabled && currentAccount.Verified && currentAccount.Revision == a.Revision && currentAccount.Email == a.Email {
		fresh, e := readSource(ctx, s.source(event.ID), account, event.ID, c.Started[event.ID])
		if e == nil && slices.Equal(keys, fresh) {
			result = s.sendEmail(ctx, smtpConfig{Host: c.Host, Port: c.Port, ImplicitTLS: c.ImplicitTLS, Username: c.Username, Password: secret, From: c.Sender}, a.Email, event.Name, body)
		}
	}
	finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return store.FinishEmail(finish, conn, id, result.State, result.Reason)
}
