package attendance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
	"testing"
	"time"
)

type fakeBattle struct {
	guard  error
	before func()
	more   bool
	calls  int
}

func TestAttendanceLocationUsesNewestObservationAndKeepsUnknown(t *testing.T) {
	s, _, event := battleFixture(t)
	ctx := context.Background()
	before, err := s.Detail(ctx, manager, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	var original time.Time
	for _, entry := range before.Entries {
		if entry.ID == 1 {
			if entry.SolarSystemID == nil || *entry.SolarSystemID != 30000142 || entry.LocationObservedAt == nil {
				t.Fatal("captured location missing", entry)
			}
			original = *entry.LocationObservedAt
		}
	}
	// A later insertion with an older ESI observation cannot replace the newer location.
	q := store.New(s.Pool)
	key, _ := uuid("98989898-9898-4898-8898-989898989898")
	_, err = q.SaveShip(ctx, store.SaveShipParams{EventID: event.ID, CharacterID: 1, RequestKey: key, ShipTypeID: 587, SolarSystemID: 30000143, ObservedAt: stamp(original.Add(-time.Minute))})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(ctx, manager, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range detail.Entries {
		if entry.ID == 1 && (entry.SolarSystemID == nil || *entry.SolarSystemID != 30000142 || !entry.LocationObservedAt.Equal(original)) {
			t.Fatal("older location replaced capture", entry)
		}
		if entry.ID == 2 && (entry.SolarSystemID != nil || entry.LocationObservedAt != nil) {
			t.Fatal("uncaptured location fabricated", entry)
		}
	}
}

func (f *fakeBattle) BattleCredential(_ context.Context, id int64, owner []byte, kind string) (eve.BattleProof, error) {
	return eve.BattleProof{CharacterID: id, Generation: 1, OwnerHash: owner}, nil
}
func (f *fakeBattle) GuardBattle(context.Context, pgx.Tx, eve.BattleProof) error { return f.guard }
func (f *fakeBattle) BattleFitting(_ context.Context, _ eve.BattleProof, ship int64, _ time.Time) (eve.BattleFitting, error) {
	if f.before != nil {
		f.before()
	}
	now := time.Now()
	return eve.BattleFitting{ShipItemID: 99, ShipTypeID: ship, ShipObservedAt: now, AssetsObservedAt: now.Add(-time.Hour), AssetsContentAt: now.Add(-time.Hour), Items: []eve.BattleItem{{TypeID: 34, Quantity: 1, Slot: "HiSlot0"}}}, nil
}
func (f *fakeBattle) BattleLosses(_ context.Context, p eve.BattleProof, page int, since, until time.Time) (eve.BattleLossPage, error) {
	f.calls++
	if f.before != nil {
		f.before()
	}
	return eve.BattleLossPage{More: f.more && page == 1, Next: time.Now().Add(time.Hour), Losses: []eve.BattleLoss{{ID: 100, CharacterID: p.CharacterID, ShipTypeID: 587, SolarSystemID: 30000142, At: until.Add(-time.Minute), Items: []eve.BattleItem{{TypeID: 34, Quantity: 3, Destroyed: 1, Dropped: 2}}}}}, nil
}
func battleFixture(t *testing.T) (*Service, *fakeBattle, Event) {
	s, roster := attendanceFixture(t)
	b := &fakeBattle{}
	s.Battle = b
	roster.fleet.Members[0].ShipTypeID = 587
	roster.fleet.Members[0].SolarSystemID = 30000142
	e := createTestEvent(t, s)
	r, err := s.Change(context.Background(), manager, e.ID, "capture", Change{Version: e.Version, RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", SourceID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return s, b, r.Event
}
func taskID(t *testing.T, s *Service, kind string) int64 {
	t.Helper()
	var id int64
	if err := s.Pool.QueryRow(context.Background(), "SELECT id FROM attendance_battle_tasks WHERE kind=$1 ORDER BY id LIMIT 1", kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestBattleSnapshotsWorkersScopeAndLossReview(t *testing.T) {
	s, b, e := battleFixture(t)
	ctx := context.Background()
	fitID := taskID(t, s, "fitting")
	if err := s.processBattle(ctx, fitID); err != nil {
		t.Fatal(err)
	}
	data, err := s.BattleDetail(ctx, manager, e.ID, 1)
	if err != nil || len(data.Ships) != 1 || data.Ships[0].Fitting == nil || data.Ships[0].Fitting.AssetsObservedAt.After(time.Now().Add(-50*time.Minute)) {
		t.Fatal(data, err)
	}
	if _, err = s.BattleDetail(ctx, member, e.ID, 1); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other member private evidence", err)
	}
	lossID := taskID(t, s, "losses")
	b.more = true
	if err = s.processBattle(ctx, lossID); err != nil {
		t.Fatal(err)
	}
	var progress string
	if err = s.Pool.QueryRow(ctx, "SELECT progress->>'page' FROM attendance_battle_tasks WHERE id=$1", lossID).Scan(&progress); err != nil || progress != "2" {
		t.Fatal(progress, err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE attendance_battle_tasks SET next_due_at=now() WHERE id=$1", lossID); err != nil {
		t.Fatal(err)
	}
	if err = s.processBattle(ctx, lossID); err != nil {
		t.Fatal(err)
	}
	data, err = s.BattleDetail(ctx, manager, e.ID, 1)
	if err != nil || len(data.Losses) != 1 || data.Losses[0].State != "candidate" {
		t.Fatal(data, err)
	}
	c := LossReview{Version: 1, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", State: "confirmed", Reason: "本次舰队战损"}
	if err = s.ReviewLoss(ctx, member, e.ID, 100, c); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("member reviewed loss", err)
	}
	if err = s.ReviewLoss(ctx, manager, e.ID, 100, c); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewLoss(ctx, manager, e.ID, 100, c); err != nil {
		t.Fatal("retry", err)
	}
	data, _ = s.BattleDetail(ctx, manager, e.ID, 1)
	if data.Losses[0].State != "confirmed" || data.Losses[0].Version != 2 {
		t.Fatal(data)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE attendance_battle_tasks SET next_due_at=now() WHERE id=$1", lossID); err != nil {
		t.Fatal(err)
	}
	if err = s.processBattle(ctx, lossID); err != nil {
		t.Fatal(err)
	}
	data, _ = s.BattleDetail(ctx, manager, e.ID, 1)
	if data.Losses[0].State != "confirmed" {
		t.Fatal("sync replaced review")
	}
}
func TestBattleStaleGrantBindingAndFencePreventPublication(t *testing.T) {
	for _, mode := range []string{"grant", "binding", "fence"} {
		t.Run(mode, func(t *testing.T) {
			s, b, e := battleFixture(t)
			ctx := context.Background()
			id := taskID(t, s, "fitting")
			switch mode {
			case "grant":
				b.guard = eve.ErrReauthorize
			case "binding":
				b.before = func() { s.Bindings = func(context.Context, pgx.Tx, []int64) ([]Binding, error) { return nil, nil } }
			case "fence":
				b.before = func() {
					_, err := s.Pool.Exec(ctx, "UPDATE attendance_battle_tasks SET fence=fence+1 WHERE id=$1", id)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := s.processBattle(ctx, id); err != nil {
				t.Fatal(err)
			}
			ships, err := store.New(s.Pool).ListShips(ctx, store.ListShipsParams{EventID: e.ID, CharacterID: 1})
			if err != nil || ships[0].FittingState == "ready" {
				t.Fatal(ships, err)
			}
		})
	}
}

func TestBattleLossCannotCountTwiceAndReopenKeepsSampling(t *testing.T) {
	s, b, e := battleFixture(t)
	ctx := context.Background()
	id := taskID(t, s, "losses")
	if err := s.processBattle(ctx, id); err != nil {
		t.Fatal(err)
	}
	review := LossReview{Version: 1, RequestKey: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", State: "confirmed", Reason: "确认活动损失"}
	if err := s.ReviewLoss(ctx, manager, e.ID, 100, review); err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, manager, Create{10, "Another event", time.Now().Add(-time.Hour), "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(ctx, manager, other.ID, "capture", Change{Version: 1, SourceID: 1, RequestKey: "ffffffff-ffff-4fff-8fff-ffffffffffff"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, "INSERT INTO attendance_losses(event_id,character_id,killmail_id,occurred_at,ship_type_id,solar_system_id,items) SELECT $1,character_id,killmail_id,occurred_at,ship_type_id,solar_system_id,items FROM attendance_losses WHERE event_id=$2", other.ID, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewLoss(ctx, manager, other.ID, 100, review); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate loss counted", err)
	}
	_, err = s.Pool.Exec(ctx, "UPDATE attendance_events SET state='closed',starts_at=now()-interval '48 hours',ends_at=now()-interval '25 hours' WHERE id=$1", e.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, "UPDATE attendance_battle_tasks SET next_due_at=now() WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	b.before = func() {
		_, err = s.Change(ctx, manager, e.ID, "reopen", Change{Version: e.Version, RequestKey: "abababab-abab-4bab-8bab-abababababab", Reason: "继续活动"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = s.processBattle(ctx, id); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = s.Pool.QueryRow(ctx, "SELECT state FROM attendance_battle_tasks WHERE id=$1", id).Scan(&state); err != nil || state != "pending" {
		t.Fatal("reopen stopped sampling", state, err)
	}
}
