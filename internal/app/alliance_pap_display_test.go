package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/attendance"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestAlliancePAPBoundInventoryAndRetainedSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testutil.Database(t)
	accounts := identity.New(pool)
	token, err := accounts.SignIn(ctx, 101, "Main", "main-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := accounts.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	for id := int64(102); id <= 105; id++ {
		token, err = accounts.Complete(ctx, id, fmt.Sprintf("Alt %d", id), fmt.Sprintf("owner-%d", id), token,
			identity.LoginIntent{Kind: "link", UserID: session.UserID}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if _, err = pool.Exec(ctx, `INSERT INTO access_administrators(user_id) VALUES($1)`, session.UserID); err != nil {
		t.Fatal(err)
	}
	// A stale formerly-owned character must not leak into the visible total.
	if _, err = pool.Exec(ctx, `INSERT INTO attendance_alliance_pap_snapshot
		(month,character_id,character_name,pap,account_id)
		VALUES($1,102,'Alt 102',9.5,$2),($1,999,'Detached',80,$2)`, month, session.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO attendance_alliance_pap_sync_history
		(month,state,complete,records_total,last_synced_at,version) VALUES($1,'ready',true,2,now(),1)`, month); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE attendance_alliance_pap_sync
		SET month=$1,state='error',complete=false,records_total=2,last_synced_at=now() WHERE singleton`, month); err != nil {
		t.Fatal(err)
	}
	handler, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test",
		[]string{"system", "identity", "eve", "access", "attendance"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	read := func(path string, data any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("status %d: %s", response.Code, response.Body)
		}
		if err := json.Unmarshal(response.Body.Bytes(), data); err != nil {
			t.Fatal(err)
		}
	}
	var personal struct{ Data attendance.AlliancePAPReport }
	read("/api/v1/attendance/alliance-pap", &personal)
	if !personal.Data.Available || personal.Data.Complete || personal.Data.State != "error" ||
		personal.Data.Points != 9.5 || len(personal.Data.BoundCharacters) != 5 ||
		len(personal.Data.Characters) != 1 || personal.Data.LastSyncedAt == nil {
		t.Fatalf("retained personal data = %+v", personal.Data)
	}
	var members struct {
		Data attendance.AlliancePAPMembersReport
	}
	read("/api/v1/attendance/alliance-pap/members", &members)
	if len(members.Data.Members) != 1 || members.Data.Members[0].Points != 9.5 ||
		len(members.Data.Members[0].BoundCharacters) != 5 || len(members.Data.Members[0].Characters) != 1 {
		t.Fatalf("member data = %+v", members.Data)
	}
	// Do not let the previous month's record count prove this month's completeness.
	if _, err = pool.Exec(ctx, `DELETE FROM attendance_alliance_pap_sync_history WHERE month=$1`, month); err != nil {
		t.Fatal(err)
	}
	read("/api/v1/attendance/alliance-pap", &personal)
	if personal.Data.Available || personal.Data.Points != 0 || len(personal.Data.Characters) != 0 {
		t.Fatalf("unpublished month exposed = %+v", personal.Data)
	}
	// A successful full empty snapshot is known data, not a synchronization error.
	if _, err = pool.Exec(ctx, `DELETE FROM attendance_alliance_pap_snapshot WHERE month=$1`, month); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE attendance_alliance_pap_sync
		SET state='ready',complete=true,records_total=0 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	read("/api/v1/attendance/alliance-pap", &personal)
	if !personal.Data.Available || !personal.Data.Complete || personal.Data.Points != 0 ||
		len(personal.Data.Characters) != 0 || len(personal.Data.BoundCharacters) != 5 {
		t.Fatalf("empty snapshot = %+v", personal.Data)
	}
	if _, err = pool.Exec(ctx, `UPDATE attendance_alliance_pap_sync
		SET month=$1 WHERE singleton`, month.AddDate(0, -1, 0)); err != nil {
		t.Fatal(err)
	}
	read("/api/v1/attendance/alliance-pap", &personal)
	if personal.Data.Available || personal.Data.Month != month.Format("2006-01") {
		t.Fatalf("previous month exposed = %+v", personal.Data)
	}
}
