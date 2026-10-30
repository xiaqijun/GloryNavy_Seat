package attendance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
)

type fakeFleetWriter struct {
	fleet    eve.FleetSnapshot
	motd     string
	result   eve.FleetMOTDResult
	writes   int
	failRead bool
}

func (f *fakeFleetWriter) ReadFleetMOTD(context.Context, int64, []byte) (eve.FleetSnapshot, string, error) {
	if f.failRead {
		return eve.FleetSnapshot{}, "", errors.New("unavailable")
	}
	return f.fleet, f.motd, nil
}
func (f *fakeFleetWriter) UpdateFleetMOTD(_ context.Context, _ int64, _ []byte, _, _ int64, old, next string) eve.FleetMOTDResult {
	f.writes++
	if old != f.motd {
		return eve.FleetMOTDResult{State: "failed", Reason: "fleet_changed"}
	}
	f.motd = next
	return f.result
}

func TestFleetMOTDMergePreservesOtherContent(t *testing.T) {
	old := "舰队指令<br>[GNV PAP] old [/GNV PAP]<br>注意事项"
	got, err := mergeFleetPAP(old, 2)
	if err != nil || !strings.Contains(got, "舰队指令<br>") || !strings.Contains(got, "<br>注意事项") || !strings.Contains(got, "每角色 2 PAP") {
		t.Fatal(got, err)
	}
	if _, err = mergeFleetPAP("[GNV PAP] missing end", 1); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestFleetMOTDOutboxSentAndUncertain(t *testing.T) {
	s, fleet := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	writer := &fakeFleetWriter{fleet: fleet.fleet, motd: "原有公告", result: eve.FleetMOTDResult{State: "sent"}}
	s.FleetWriter = writer
	input := Change{Version: e.Version, RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", SourceID: 1, PAPPoints: 1}
	first, err := s.Change(ctx, manager, e.ID, "capture_pap", input)
	if err != nil {
		t.Fatal(err)
	}
	u, err := store.ClaimFleetMOTD(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processFleetMOTD(ctx, u); err != nil {
		t.Fatal(err)
	}
	latest, err := store.LatestFleetMOTD(ctx, s.Pool, e.ID)
	if err != nil || latest.State != "sent" || writer.writes != 1 || !strings.Contains(writer.motd, "原有公告") {
		t.Fatal(latest, writer.writes, err)
	}
	input.Version = first.Event.Version
	input.RequestKey = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	if _, err = s.Change(ctx, manager, e.ID, "capture_pap", input); err != nil {
		t.Fatal(err)
	}
	writer.result = eve.FleetMOTDResult{State: "unknown", Reason: "confirmation_required"}
	u, err = store.ClaimFleetMOTD(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processFleetMOTD(ctx, u); err != nil {
		t.Fatal(err)
	}
	latest, err = store.LatestFleetMOTD(ctx, s.Pool, e.ID)
	if err != nil || latest.State != "unknown" || writer.writes != 2 {
		t.Fatal(latest, writer.writes, err)
	}
	if err = store.MaintainFleetMOTD(ctx, s.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimFleetMOTD(ctx, s.Pool); err == nil {
		t.Fatal("unknown retried")
	}
}

func TestFleetNoticeMOTDSyncAndPAP(t *testing.T) {
	s, fleet := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	writer := &fakeFleetWriter{fleet: fleet.fleet, motd: "已有指令", result: eve.FleetMOTDResult{State: "sent"}}
	s.FleetWriter = writer
	n, err := s.SaveFleetNotice(ctx, manager, e.ID, FleetNoticeInput{SolarSystem: "Jita", Doctrine: "<战舰>", Commander: "Lead", Links: &FleetNoticeLinks{CommanderID: "1", FireAnchor: "Alt", FireAnchorID: "2", FittingDNA: "587:34;2::"}, Details: "[GNV PAP]", RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	in := FleetMOTDInput{SourceID: 1, Version: e.Version, Revision: n.Revision, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc"}
	if err = s.QueueFleetNoticeMOTD(ctx, manager, e.ID, in); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueFleetNoticeMOTD(ctx, manager, e.ID, in); err != nil {
		t.Fatal("replay", err)
	}
	changed := in
	changed.Revision++
	if err = s.QueueFleetNoticeMOTD(ctx, manager, e.ID, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("key reuse", err)
	}
	u, err := store.ClaimFleetMOTD(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processFleetMOTD(ctx, u); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"已有指令", "星系：Jita", "&lt;战舰&gt;", "&#91;GNV PAP&#93;", "未发放", `href="showinfo:1377//1"`, `href="showinfo:1377//2"`, `href="fitting:587:34;2::"`} {
		if !strings.Contains(writer.motd, want) {
			t.Fatalf("missing %s in %s", want, writer.motd)
		}
	}
	res, err := s.Change(ctx, manager, e.ID, "capture_pap", Change{Version: e.Version, SourceID: 1, PAPPoints: 1, RequestKey: "dddddddd-dddd-4ddd-8ddd-dddddddddddd"})
	if err != nil {
		t.Fatal(err)
	}
	u, err = store.ClaimFleetMOTD(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processFleetMOTD(ctx, u); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(writer.motd, "每角色 1 PAP") || strings.Count(writer.motd, "[GNV NOTICE]") != 1 || !strings.Contains(writer.motd, "已有指令") {
		t.Fatal(writer.motd)
	}
	in.Version = res.Event.Version
	in.RequestKey = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	if err = s.QueueFleetNoticeMOTD(ctx, manager, e.ID, in); err != nil {
		t.Fatal(err)
	}
	u, err = store.ClaimFleetMOTD(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	writer.result = eve.FleetMOTDResult{State: "unknown", Reason: "confirmation_required"}
	if err = s.processFleetMOTD(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err = store.MaintainFleetMOTD(ctx, s.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimFleetMOTD(ctx, s.Pool); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unknown automatically retried", err)
	}
	in.RequestKey = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	if err = s.QueueFleetNoticeMOTD(ctx, manager, e.ID, in); err != nil {
		t.Fatal("explicit retry", err)
	}
}

func TestFleetNoticeMOTDRejectsChangedTemplate(t *testing.T) {
	s, fleet := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	writer := &fakeFleetWriter{fleet: fleet.fleet, result: eve.FleetMOTDResult{State: "sent"}}
	s.FleetWriter = writer
	n, err := s.SaveFleetNotice(ctx, manager, e.ID, FleetNoticeInput{SolarSystem: "Jita", Doctrine: "护卫舰", RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.QueueFleetNoticeMOTD(ctx, manager, e.ID, FleetMOTDInput{SourceID: 1, Version: e.Version, Revision: n.Revision, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveFleetNotice(ctx, manager, e.ID, FleetNoticeInput{SolarSystem: "Amarr", Doctrine: "护卫舰", Revision: n.Revision, RequestKey: "dddddddd-dddd-4ddd-8ddd-dddddddddddd"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := store.ClaimFleetMOTD(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processFleetMOTD(ctx, u); err != nil {
		t.Fatal(err)
	}
	latest, err := store.LatestFleetMOTD(ctx, s.Pool, e.ID)
	if err != nil || latest.State != "obsolete" || writer.writes != 0 {
		t.Fatal(latest, writer.writes, err)
	}
}
