package attendance

import (
	"context"
	"errors"
	"html"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

type fleetDispatchArgs struct{}

func (fleetDispatchArgs) Kind() string { return "attendance.fleet-dispatch.v1" }

type fleetDispatchWorker struct {
	river.WorkerDefaults[fleetDispatchArgs]
	runtime *battleRuntime
}

func (w *fleetDispatchWorker) Timeout(*river.Job[fleetDispatchArgs]) time.Duration {
	return 5 * time.Minute
}

func (w *fleetDispatchWorker) Work(ctx context.Context, _ *river.Job[fleetDispatchArgs]) error {
	if !w.runtime.enabled {
		return river.JobSnooze(time.Hour)
	}
	s := w.runtime.service
	if err := store.MaintainFleetDeliveries(ctx, s.Pool); err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		d, err := store.ClaimFleetDelivery(ctx, s.Pool)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		bounded, cancel := context.WithTimeout(ctx, 55*time.Second)
		err = s.processFleetDelivery(bounded, d)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) processFleetDelivery(ctx context.Context, d store.FleetDelivery) error {
	finish := func(state, reason string) error {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := store.TransitionFleetDelivery(c, s.Pool, d.ID, "preparing", d.UpdatedAt, state, reason)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if s.FleetSend == nil {
		return finish("failed", "service_unavailable")
	}
	ev, err := store.New(s.Pool).GetEvent(ctx, d.EventID)
	if err != nil || ev.CorporationID != d.CorporationID {
		return finish("failed", "activity_unavailable")
	}
	if ev.State != "open" {
		return finish("skipped", "activity_closed")
	}
	actor := d.ActorID.String()
	if err := s.require(ctx, actor, d.CorporationID); err != nil {
		return finish("failed", "permission_changed")
	}
	account := ""
	if d.Channel == "email" {
		if !d.AccountID.Valid {
			return finish("skipped", "no_bound_members")
		}
		account = d.AccountID.String()
		if s.ReportBindings == nil {
			return finish("failed", "identity_unavailable")
		}
		bindings, err := s.ReportBindings(ctx, actor, "", d.CorporationID)
		if err != nil {
			return finish("failed", "identity_unavailable")
		}
		found := false
		for _, b := range bindings {
			if b.UserID == account {
				found = true
				break
			}
		}
		if !found {
			return finish("skipped", "membership_changed")
		}
	}
	if err := s.require(ctx, actor, d.CorporationID); err != nil {
		return finish("failed", "permission_changed")
	}
	sendingAt, err := store.TransitionFleetDelivery(ctx, s.Pool, d.ID, "preparing", d.UpdatedAt, "sending", "")
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	body := d.Body
	if d.Channel != "qq" {
		body = strings.ReplaceAll(html.EscapeString(body), "\n", "<br>")
	}
	r := s.FleetSend(ctx, d.Channel, d.CorporationID, d.TargetVersion, account, d.Subject, body)
	if r.State != "sent" && r.State != "failed" && r.State != "unknown" && r.State != "skipped" {
		r.State = "unknown"
		r.Reason = "confirmation_required"
	}
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = store.TransitionFleetDelivery(c, s.Pool, d.ID, "sending", sendingAt, r.State, r.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
