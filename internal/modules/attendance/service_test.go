package attendance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/testutil"
	"sync"
	"testing"
	"time"
)

const manager = "11111111-1111-4111-8111-111111111111"
const member = "22222222-2222-4222-8222-222222222222"

type fakeEVE struct {
	fleet    eve.FleetSnapshot
	data     eve.OnlineData
	guard    error
	fleetErr error
	before   func()
}

func (f *fakeEVE) Fleet(context.Context, int64, []byte) (eve.FleetSnapshot, error) {
	if f.before != nil {
		f.before()
	}
	return f.fleet, f.fleetErr
}
func (f *fakeEVE) GuardFleet(context.Context, pgx.Tx, eve.FleetSnapshot) error { return f.guard }
func (f *fakeEVE) ActivityProfiles(_ context.Context, ids []int64) ([]eve.FleetMember, error) {
	return []eve.FleetMember{{ID: ids[0], Name: "Pilot", CorporationID: 10}}, nil
}
func (f *fakeEVE) OnlineDataTx(context.Context, pgx.Tx, []int64, time.Time, time.Time, ...int64) (eve.OnlineData, error) {
	return f.data, nil
}
func attendanceFixture(t *testing.T) (*Service, *fakeEVE) {
	pool := testutil.Database(t)
	f := &fakeEVE{fleet: eve.FleetSnapshot{CharacterID: 1, Generation: 1, OwnerHash: []byte("owner"), FleetID: 100, ObservedAt: time.Now(), Members: []eve.FleetMember{{ID: 1, Name: "Lead", CorporationID: 10}, {ID: 2, Name: "Alt", CorporationID: 10}, {ID: 3, Name: "Guest", CorporationID: 20}, {ID: 4, Name: "Pilot", CorporationID: 10}, {ID: 5, Name: "Unregistered", CorporationID: 10}}}}
	s := &Service{Pool: pool, EVE: f}
	s.Manage = func(_ context.Context, user string, corp int64) (bool, error) {
		return user == manager && corp == 10, nil
	}
	s.Corporations = func(_ context.Context, user string) ([]Corporation, error) {
		if user == manager {
			return []Corporation{{10, "Corp"}}, nil
		}
		return []Corporation{}, nil
	}
	all := []Binding{{ID: 1, Name: "Lead", UserID: manager, OwnerHash: []byte("owner")}, {ID: 2, Name: "Alt", UserID: manager}, {ID: 4, Name: "Pilot", UserID: member}}
	s.Bindings = func(_ context.Context, _ pgx.Tx, ids []int64) ([]Binding, error) {
		out := []Binding{}
		for _, b := range all {
			for _, id := range ids {
				if b.ID == id {
					out = append(out, b)
					break
				}
			}
		}
		return out, nil
	}
	s.Own = func(_ context.Context, user string) ([]Binding, error) {
		out := []Binding{}
		for _, b := range all {
			if b.UserID == user {
				out = append(out, b)
			}
		}
		return out, nil
	}
	s.ReportBindings = func(ctx context.Context, actor, target string, corp int64) ([]Binding, error) {
		return s.Own(ctx, actor)
	}
	return s, f
}
func createTestEvent(t *testing.T, s *Service) Event {
	t.Helper()
	e, err := s.Create(context.Background(), manager, Create{10, "Fleet", time.Now().Add(-time.Hour), "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestAttendanceCaptureManualOwnershipAndReplay(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	c := Change{Version: e.Version, RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", SourceID: 1}
	first, err := s.Change(ctx, manager, e.ID, "capture", c)
	if err != nil || first.Recorded != 3 || first.Excluded != 2 || first.ExcludedExternal == nil || *first.ExcludedExternal != 1 || first.ExcludedUnbound == nil || *first.ExcludedUnbound != 1 {
		t.Fatal(first, err)
	}
	replay, err := s.Change(ctx, manager, e.ID, "capture", c)
	if err != nil || replay.Event.Version != first.Event.Version || replay.ExcludedExternal == nil || *replay.ExcludedExternal != 1 || replay.ExcludedUnbound == nil || *replay.ExcludedUnbound != 1 {
		t.Fatal("replay", replay, err)
	}
	detail, err := s.Detail(ctx, manager, e.ID)
	if err != nil || detail.Event.Participants != 2 || len(detail.Entries) != 3 {
		t.Fatal("account union/guests", detail, err)
	}
	mine, err := s.Detail(ctx, member, e.ID)
	if err != nil || len(mine.Entries) != 1 || mine.Entries[0].ID != 4 {
		t.Fatal("member visibility", mine, err)
	}
	if _, err = s.Detail(ctx, "33333333-3333-4333-8333-333333333333", e.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("outsider", err)
	}
	c.SourceID = 4
	if _, err = s.Change(ctx, manager, e.ID, "capture", c); !errors.Is(err, ErrConflict) {
		t.Fatal("idempotency key reused with another body", err)
	}
	m := Change{Version: first.Event.Version, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", CharacterID: 4, Present: false, Reason: "误记"}
	if _, err = s.Change(ctx, manager, e.ID, "manual", m); !errors.Is(err, ErrConflict) {
		t.Fatal("open event accepted manual entry", err)
	}
	changed, err := s.Change(ctx, manager, e.ID, "close", Change{Version: first.Event.Version, RequestKey: "dddddddd-dddd-4ddd-8ddd-dddddddddddd"})
	if err != nil {
		t.Fatal(err)
	}
	m.Version = changed.Event.Version
	m.RequestKey = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	if _, err = s.Change(ctx, manager, e.ID, "manual", m); err != nil {
		t.Fatal(err)
	}
	detail, err = s.Detail(ctx, manager, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range detail.Entries {
		if r.ID == 4 && (r.Present || r.Source != "manual") {
			t.Fatal("capture replaced manual override")
		}
	}
	var audits int
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM attendance_audit").Scan(&audits); err != nil || audits != 3 {
		t.Fatal(audits, err)
	}
	// Historical attribution is immutable even if today's mapping changes.
	original := s.Bindings
	s.Bindings = func(ctx context.Context, tx pgx.Tx, ids []int64) ([]Binding, error) {
		b, err := original(ctx, tx, ids)
		for i := range b {
			if b[i].ID == 4 {
				b[i].UserID = manager
			}
		}
		return b, err
	}
	m.Version = detail.Event.Version
	m.RequestKey = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	m.Present = true
	if _, err = s.Change(ctx, manager, e.ID, "manual", m); err != nil {
		t.Fatal(err)
	}
	detail, _ = s.Detail(ctx, member, e.ID)
	if len(detail.Entries) != 1 || *detail.Entries[0].AccountID != member {
		t.Fatal("history transferred", detail)
	}
}
func TestAttendanceConcurrentVersionsAndGenerationFence(t *testing.T) {
	s, f := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	f.guard = eve.ErrReauthorize
	if _, err := s.Change(ctx, manager, e.ID, "capture", Change{Version: 1, SourceID: 1, RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	detail, _ := s.Detail(ctx, manager, e.ID)
	if len(detail.Entries) != 0 || detail.Event.Version != 1 {
		t.Fatal("stale grant published")
	}
	f.guard = nil
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for _, key := range []string{"cccccccc-cccc-4ccc-8ccc-cccccccccccc", "dddddddd-dddd-4ddd-8ddd-dddddddddddd"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			_, err := s.Change(ctx, manager, e.ID, "close", Change{Version: e.Version, RequestKey: k})
			out <- err
		}(key)
	}
	wg.Wait()
	close(out)
	success, conflicts := 0, 0
	for err := range out {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
	changed, err := s.Change(ctx, manager, e.ID, "manual", Change{Version: 2, CharacterID: 4, Present: true, Reason: "late", RequestKey: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"})
	if err != nil {
		t.Fatal("closed event did not accept manual entry", err)
	}
	if _, err := s.Change(ctx, member, e.ID, "reopen", Change{Version: changed.Event.Version, Reason: "late", RequestKey: "ffffffff-ffff-4fff-8fff-ffffffffffff"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("member wrote", err)
	}
}

func TestFleetDisbandAutomaticallyClosesEvent(t *testing.T) {
	s, f := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	f.fleetErr = eve.ErrFleetDisbanded
	changed, err := s.Change(ctx, manager, e.ID, "capture", Change{Version: e.Version, SourceID: 1, RequestKey: "abababab-abab-4bab-8bab-abababababab"})
	if err != nil || !changed.AutoClosed || changed.Event.State != "closed" || changed.Recorded != 0 {
		t.Fatal("fleet disband did not close event", changed, err)
	}
	detail, err := s.Detail(ctx, manager, e.ID)
	if err != nil || detail.Event.State != "closed" || detail.Event.EndsAt == nil {
		t.Fatal("closed event not persisted", detail, err)
	}
	audit, err := store.New(s.Pool).ListAudit(ctx, store.ListAuditParams{EventID: e.ID})
	if err != nil || len(audit) != 1 || audit[0].Action != "auto_close" {
		t.Fatal("missing auto-close audit", audit, err)
	}
	if _, err = s.Change(ctx, manager, e.ID, "manual", Change{Version: detail.Event.Version, CharacterID: 4, Present: true, Reason: "解散后补录", RequestKey: "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"}); err != nil {
		t.Fatal("closed event should accept manual entry", err)
	}

	// A transient ESI failure must remain retryable and must not close the event.
	s2, f2 := attendanceFixture(t)
	e2 := createTestEvent(t, s2)
	f2.fleetErr = errors.New("esi timeout")
	if _, err = s2.Change(ctx, manager, e2.ID, "capture", Change{Version: e2.Version, SourceID: 1, RequestKey: "dededede-dede-4ded-8ded-dededededede"}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("transient fleet failure was not returned as unavailable", err)
	}
	detail, err = s2.Detail(ctx, manager, e2.ID)
	if err != nil || detail.Event.State != "open" {
		t.Fatal("transient fleet failure closed event", detail, err)
	}
}

func TestOnlineAccountUnionCalendarAndUnknown(t *testing.T) {
	s, f := attendanceFixture(t)
	zone := time.FixedZone("CST", 8*3600)
	n := time.Now().In(zone)
	midnight := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, zone)
	f.data = eve.OnlineData{Spans: []eve.OnlineSpan{{CharacterID: 1, Start: midnight.Add(-120 * time.Second), End: midnight}, {CharacterID: 2, Start: midnight.Add(-60 * time.Second), End: midnight.Add(60 * time.Second)}}, Counts: []eve.SampleCount{{CharacterID: 1, Day: midnight.Add(-time.Minute).Format("2006-01-02"), Count: 2}}, Characters: []eve.OnlineCharacter{{ID: 1, State: "unknown"}, {ID: 2, State: "offline"}}}
	report, err := s.Report(context.Background(), manager, "", 0, 7)
	if err != nil {
		t.Fatal(err)
	}
	if report.Seconds != 180 || len(report.Members) != 1 || report.Members[0].Seconds != 180 || report.Days[5].Seconds != 120 || report.Days[6].Seconds != 60 {
		t.Fatal("overlap/day boundaries", report)
	}
	if report.Days[0].Samples != 0 || report.Members[0].Characters[0].State != "unknown" {
		t.Fatal("invented offline samples")
	}
}
