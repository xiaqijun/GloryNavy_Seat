package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/attendance"
	"glorynavy.local/seat/internal/modules/community"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestAlliancePAPMemberNicknameThroughHost(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	accounts := identity.New(pool)
	adminToken, err := accounts.SignIn(ctx, 101, "Main Pilot", "owner-main", "")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := accounts.Session(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err = accounts.Complete(ctx, 102, "Alt Pilot", "owner-alt", adminToken, identity.LoginIntent{Kind: "link", UserID: admin.UserID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	memberToken, err := accounts.SignIn(ctx, 201, "Fallback Pilot", "owner-fallback", "")
	if err != nil {
		t.Fatal(err)
	}
	member, err := accounts.Session(ctx, memberToken)
	if err != nil {
		t.Fatal(err)
	}
	profiles := community.New(pool)
	if _, err = profiles.Update(ctx, admin.UserID, "12345678", " 舰队长 ", "0"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	previous := month.AddDate(0, -1, 0)
	for _, snapshotMonth := range []time.Time{month, previous} {
		if _, err = pool.Exec(ctx, `
			INSERT INTO attendance_alliance_pap_snapshot(month,character_id,character_name,pap,account_id)
			VALUES ($1,101,'Main Pilot',2.50,$2),($1,102,'Alt Pilot',1.50,$2),
			       ($1,201,'Fallback Pilot',2.00,$3)`, snapshotMonth, admin.UserID, member.UserID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE attendance_alliance_pap_sync
		SET month=$1,state='ready',complete=true,records_total=3,last_synced_at=now() WHERE singleton`, month); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO attendance_alliance_pap_sync_history
		(month,state,complete,records_total,last_synced_at,version)
		VALUES($1,'ready',true,3,now(),1)`, previous); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO access_administrators(user_id) VALUES($1)`, admin.UserID); err != nil {
		t.Fatal(err)
	}
	handler, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test",
		[]string{"system", "identity", "eve", "access", "attendance"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	read := func(token string, selected time.Time) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/attendance/alliance-pap/members?month="+selected.Format("2006-01"), nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if response := read("", month); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", response.Code)
	}
	if response := read(memberToken, month); response.Code != http.StatusNotFound {
		t.Fatalf("non-admin status = %d", response.Code)
	}
	check := func(selected time.Time, nickname string) {
		t.Helper()
		response := read(adminToken, selected)
		if response.Code != http.StatusOK {
			t.Fatalf("report status = %d: %s", response.Code, response.Body.String())
		}
		var result struct {
			Data attendance.AlliancePAPMembersReport `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !result.Data.Available || len(result.Data.Members) != 2 {
			t.Fatalf("report = %+v", result.Data)
		}
		for _, row := range result.Data.Members {
			switch row.UserID {
			case admin.UserID:
				if row.Name != nickname || row.Points != 4 || !row.Achieved || len(row.Characters) != 2 {
					t.Fatalf("nickname account = %+v", row)
				}
				for _, character := range row.Characters {
					expected := map[int64]string{101: "Main Pilot", 102: "Alt Pilot"}
					if character.CharacterName != expected[character.CharacterID] {
						t.Fatalf("source character name changed: %+v", character)
					}
				}
			case member.UserID:
				if row.Name != "Fallback Pilot" || row.Points != 2 || row.Achieved {
					t.Fatalf("nickname fallback = %+v", row)
				}
			default:
				t.Fatalf("unexpected account = %s", row.UserID)
			}
		}
	}
	check(month, "舰队长")
	check(previous, "舰队长")
	if _, err = profiles.Update(ctx, admin.UserID, "12345678", "新昵称", "1"); err != nil {
		t.Fatal(err)
	}
	check(month, "新昵称")
	check(previous, "新昵称")
	// Legacy blank profile values also use the main-character fallback.
	if _, err = pool.Exec(ctx, `UPDATE community_profiles SET kook_name='   ' WHERE user_id=$1`, admin.UserID); err != nil {
		t.Fatal(err)
	}
	check(month, "Main Pilot")
	if _, err = pool.Exec(ctx, `DELETE FROM access_administrators WHERE user_id=$1`, admin.UserID); err != nil {
		t.Fatal(err)
	}
	if response := read(adminToken, month); response.Code != http.StatusNotFound {
		t.Fatalf("revoked administrator status = %d", response.Code)
	}
}
