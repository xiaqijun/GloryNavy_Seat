package attendance

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
)

type fleetMOTDArgs struct{}

func (fleetMOTDArgs) Kind() string { return "attendance.fleet-motd.v1" }

type fleetMOTDWorker struct {
	river.WorkerDefaults[fleetMOTDArgs]
	runtime *battleRuntime
}

func (w *fleetMOTDWorker) Timeout(*river.Job[fleetMOTDArgs]) time.Duration { return 5 * time.Minute }

func (w *fleetMOTDWorker) Work(ctx context.Context, _ *river.Job[fleetMOTDArgs]) error {
	if !w.runtime.enabled {
		return river.JobSnooze(time.Hour)
	}
	s := w.runtime.service
	if err := store.MaintainFleetMOTD(ctx, s.Pool); err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		u, err := store.ClaimFleetMOTD(ctx, s.Pool)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		bounded, cancel := context.WithTimeout(ctx, 55*time.Second)
		err = s.processFleetMOTD(bounded, u)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

const fleetPAPStart = "[GNV PAP]"
const fleetPAPEnd = "[/GNV PAP]"

func mergeFleetPAP(motd string, points int32) (string, error) {
	start, end := strings.Index(motd, fleetPAPStart), strings.Index(motd, fleetPAPEnd)
	block := fmt.Sprintf("%s 已发放 · 每角色 %d PAP %s", fleetPAPStart, points, fleetPAPEnd)
	if points == 0 {
		block = fleetPAPStart + " 未发放 " + fleetPAPEnd
	}
	if start < 0 && end < 0 {
		if motd == "" {
			return block, nil
		}
		return motd + "<br>" + block, nil
	}
	if start < 0 || end < start || strings.Count(motd, fleetPAPStart) != 1 || strings.Count(motd, fleetPAPEnd) != 1 {
		return "", ErrConflict
	}
	return motd[:start] + block + motd[end+len(fleetPAPEnd):], nil
}

func mergeFleetNotice(motd, body string) (string, error) {
	const startMarker, endMarker = "[GNV NOTICE]", "[/GNV NOTICE]"
	start, end := strings.Index(motd, startMarker), strings.Index(motd, endMarker)
	block := startMarker + "<br>" + body + "<br>" + endMarker
	if start < 0 && end < 0 {
		if motd == "" {
			return block, nil
		}
		return motd + "<br>" + block, nil
	}
	if start < 0 || end < start || strings.Count(motd, startMarker) != 1 || strings.Count(motd, endMarker) != 1 {
		return "", ErrConflict
	}
	return motd[:start] + block + motd[end+len(endMarker):], nil
}

func (s *Service) processFleetMOTD(ctx context.Context, u store.FleetMOTDUpdate) error {
	finish := func(to, reason string) error {
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := store.TransitionFleetMOTD(saveCtx, s.Pool, u.ID, "preparing", u.UpdatedAt, to, reason)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if s.FleetWriter == nil {
		return finish("failed", "service_unavailable")
	}
	ev, err := store.New(s.Pool).GetEvent(ctx, u.EventID)
	if err != nil {
		return finish("failed", "activity_unavailable")
	}
	if ev.State != "open" || (u.PAPPoints > 0 && (!ev.PapIssued || ev.PapPoints != u.PAPPoints)) {
		return finish("obsolete", "activity_changed")
	}
	latest, err := store.LatestFleetMOTD(ctx, s.Pool, u.EventID)
	if err != nil {
		return finish("failed", "activity_unavailable")
	}
	if latest.ID != u.ID {
		return finish("obsolete", "newer_update")
	}
	actor := u.ActorID.String()
	if err = s.require(ctx, actor, ev.CorporationID); err != nil {
		return finish("failed", "permission_changed")
	}
	own, err := s.Own(ctx, actor)
	if err != nil {
		return finish("failed", "identity_unavailable")
	}
	valid := false
	for _, c := range own {
		if c.ID == u.SourceID && len(c.OwnerHash) > 0 && subtle.ConstantTimeCompare(c.OwnerHash, u.OwnerHash) == 1 {
			valid = true
			break
		}
	}
	if !valid {
		return finish("failed", "identity_changed")
	}
	fleet, current, err := s.FleetWriter.ReadFleetMOTD(ctx, u.SourceID, u.OwnerHash)
	if err != nil {
		if errors.Is(err, eve.ErrReauthorize) {
			return finish("failed", "reauthorize")
		}
		if !eve.ESIRetryTime(err).IsZero() {
			return finish("failed", "rate_limited")
		}
		return finish("failed", "fleet_unavailable")
	}
	if fleet.FleetID != u.FleetID || fleet.Generation != u.Generation {
		return finish("failed", "fleet_changed")
	}
	next := current
	n, noticeErr := store.FleetNoticeByEvent(ctx, s.Pool, u.EventID)
	if noticeErr != nil && !errors.Is(noticeErr, pgx.ErrNoRows) {
		return finish("failed", "activity_unavailable")
	}
	if u.NoticeRevision > 0 && (noticeErr != nil || n.Revision != u.NoticeRevision) {
		return finish("obsolete", "activity_changed")
	}
	if noticeErr == nil {
		body := strings.ReplaceAll(html.EscapeString(fleetNoticeText(event(ev), fleetNoticeDTO(n))), "\n", "<br>")
		// User text must not be able to impersonate either managed block marker.
		body = strings.NewReplacer("[", "&#91;", "]", "&#93;").Replace(body)
		next, err = mergeFleetNotice(next, body)
		if err != nil {
			return finish("failed", "motd_format_changed")
		}
	}
	points := int32(0)
	if ev.PapIssued {
		points = ev.PapPoints
	}
	next, err = mergeFleetPAP(next, points)
	if err != nil {
		return finish("failed", "motd_format_changed")
	}
	if err = s.require(ctx, actor, ev.CorporationID); err != nil {
		return finish("failed", "permission_changed")
	}
	latest, err = store.LatestFleetMOTD(ctx, s.Pool, u.EventID)
	if err != nil || latest.ID != u.ID {
		return finish("obsolete", "newer_update")
	}
	check, err := store.New(s.Pool).GetEvent(ctx, u.EventID)
	if err != nil || check.State != "open" || check.PapIssued != ev.PapIssued || check.PapPoints != ev.PapPoints {
		return finish("obsolete", "activity_changed")
	}
	currentNotice, checkErr := store.FleetNoticeByEvent(ctx, s.Pool, u.EventID)
	if (noticeErr == nil && (checkErr != nil || currentNotice.Revision != n.Revision)) || (errors.Is(noticeErr, pgx.ErrNoRows) && !errors.Is(checkErr, pgx.ErrNoRows)) {
		return finish("obsolete", "activity_changed")
	}
	sendingAt, err := store.TransitionFleetMOTD(ctx, s.Pool, u.ID, "preparing", u.UpdatedAt, "sending", "")
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	result := s.FleetWriter.UpdateFleetMOTD(ctx, u.SourceID, u.OwnerHash, u.Generation, u.FleetID, current, next)
	if result.State != "sent" && result.State != "failed" && result.State != "unknown" {
		result.State = "unknown"
		result.Reason = "confirmation_required"
	}
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = store.TransitionFleetMOTD(saveCtx, s.Pool, u.ID, "sending", sendingAt, result.State, result.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
