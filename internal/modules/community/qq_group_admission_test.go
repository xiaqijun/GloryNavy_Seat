package community

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/identity"
)

func TestQQGroupAdmissionRequiresCurrentMemberAndMatchingBinding(t *testing.T) {
	s, _, user, bindCode := qqBindingFixture(t)
	ctx := t.Context()
	const groupID = "official-group"
	const syncGroupID = "sync-official-group"
	const openID = "confirmed-member"
	var posts atomic.Int32
	var syncItem atomic.Pointer[OfficialQQGroupJoinRequest]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			page := OfficialQQGroupJoinRequestPage{List: []OfficialQQGroupJoinRequest{}}
			if r.URL.Path == "/v2/groups/sync-official-group/join_request_list" {
				if item := syncItem.Load(); item != nil {
					page.List = append(page.List, *item)
				}
			}
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		posts.Add(1)
		if r.Method != http.MethodPost || (r.URL.Path != "/v2/groups/official-group/approval_join_request/confirmed-member" &&
			r.URL.Path != "/v2/groups/sync-official-group/approval_join_request/confirmed-member") {
			t.Errorf("unexpected QQ request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("test-app", "test-secret", server.URL)
	bot.token = qqBotToken{AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}
	useOfficialQQTestBot(t, s, bot)
	if _, err := s.SaveQQGroupSettings(ctx, []QQGroupSetting{
		{GroupOpenID: groupID, Enabled: true}, {GroupOpenID: syncGroupID, Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	isMember := false
	s.QQGroupMember = func(_ context.Context, candidate string) (bool, error) {
		return isMember && candidate == user, nil
	}
	if _, err := s.CreateQQGroupApplication(ctx, user, groupID); !errors.Is(err, ErrQQGroupMemberRequired) {
		t.Fatalf("non-member application: %v", err)
	}
	isMember = true
	if err := s.ConfirmQQOfficial(ctx, "binding", openID, bindCode, []byte("binding"), bot.callbackSnapshot()); err != nil {
		t.Fatal(err)
	}
	application, err := s.CreateQQGroupApplication(ctx, user, groupID)
	if err != nil {
		t.Fatal(err)
	}
	request := OfficialQQGroupJoinRequest{
		GroupOpenID: groupID, JoinRequestID: "request", MemberOpenID: "different-member",
		VerifyInfo: officialQQVerifyInfo{VerifyMessage: application.Code},
	}
	handle := func(event string) {
		t.Helper()
		if err := s.ProcessQQGroupJoinRequest(ctx, event, request, []byte(event), bot.callbackSnapshot()); err != nil {
			t.Fatal(err)
		}
	}
	handle("mismatched-binding")
	request.MemberOpenID = openID
	isMember = false
	handle("membership-revoked")
	if pending, err := s.QQGroupApplication(ctx, user); err != nil || pending.Status != "pending" || posts.Load() != 0 {
		t.Fatalf("invalid request advanced application or called QQ: %+v, posts=%d, %v", pending, posts.Load(), err)
	}
	isMember = true
	handle("valid-application")
	handle("valid-application")
	if approved, err := s.QQGroupApplication(ctx, user); err != nil || approved.Status != "approved" || posts.Load() != 1 {
		t.Fatalf("valid request did not approve exactly once: %+v, posts=%d, %v", approved, posts.Load(), err)
	}
	event := OfficialQQGroupMemberEvent{GroupOpenID: groupID, MemberOpenID: openID}
	if err := s.ProcessQQGroupMemberAdded(ctx, "member-joined", event, []byte("member-joined"), bot.callbackSnapshot()); err != nil {
		t.Fatal(err)
	}
	if joined, err := s.QQGroupApplication(ctx, user); err != nil || joined.Status != "bound" {
		t.Fatalf("verified member event did not complete the application: %+v, %v", joined, err)
	}
	application, err = s.CreateQQGroupApplication(ctx, user, syncGroupID)
	if err != nil {
		t.Fatal(err)
	}
	syncItem.Store(&OfficialQQGroupJoinRequest{
		JoinRequestID: "listed-request", MemberOpenID: openID,
		VerifyInfo: officialQQVerifyInfo{VerifyMessage: application.Code},
	})
	for range 2 {
		if err = s.SyncQQGroupApplications(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if approved, err := s.QQGroupApplication(ctx, user); err != nil || approved.Status != "approved" || posts.Load() != 2 {
		t.Fatalf("list entry without group did not approve exactly once: %+v, posts=%d, %v", approved, posts.Load(), err)
	}
	if joined, err := s.qqGroupApplication(ctx, user, groupID); err != nil || joined.Status != "bound" {
		t.Fatalf("selected group shows another group's application: %+v, %v", joined, err)
	}
}

func TestQQGroupAdmissionBindsFirstQQOnMemberEvent(t *testing.T) {
	s, pool, user, unusedDirectCode := qqBindingFixture(t)
	ctx := t.Context()
	const group = "official-first-group"
	const secondGroup = "official-second-group"
	const member = "first-official-member"
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v2/groups/official-first-group/approval_join_request/first-official-member" {
			t.Errorf("unexpected QQ request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("test-app", "test-secret", server.URL)
	bot.token = qqBotToken{AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}
	useOfficialQQTestBot(t, s, bot)
	verified := bot.callbackSnapshot()
	if _, err := s.SaveQQGroupSettings(ctx, []QQGroupSetting{{GroupOpenID: group, Enabled: true}, {GroupOpenID: secondGroup, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	isMember := true
	s.QQGroupMember = func(_ context.Context, candidate string) (bool, error) { return isMember && candidate == user, nil }
	app, err := s.CreateQQGroupApplication(ctx, user, group)
	if err != nil {
		t.Fatal(err)
	}
	otherApp, err := s.CreateQQGroupApplication(ctx, user, secondGroup)
	if err != nil {
		t.Fatal(err)
	}
	apply := func(id, groupOpenID, memberOpenID, code string) {
		t.Helper()
		request := OfficialQQGroupJoinRequest{GroupOpenID: groupOpenID, JoinRequestID: id, MemberOpenID: memberOpenID, VerifyInfo: officialQQVerifyInfo{VerifyMessage: code}}
		if e := s.ProcessQQGroupJoinRequest(ctx, id, request, []byte(id), verified); e != nil {
			t.Fatal(e)
		}
	}
	join := func(id, groupOpenID, memberOpenID string) {
		t.Helper()
		event := OfficialQQGroupMemberEvent{GroupOpenID: groupOpenID, MemberOpenID: memberOpenID}
		if e := s.ProcessQQGroupMemberAdded(ctx, id, event, []byte(id), verified); e != nil {
			t.Fatal(e)
		}
	}
	assertStatus := func(want string) {
		t.Helper()
		p, e := s.Get(ctx, user)
		if e != nil || p.QQ.Confirmation != want {
			t.Fatalf("QQ confirmation = %q, want %q: %v", p.QQ.Confirmation, want, e)
		}
	}
	apply("wrong-group-code", secondGroup, member, app.Code)
	join("unsolicited-member", group, member)
	assertStatus("pending")
	apply("first-application", group, member, app.Code)
	apply("first-application", group, member, app.Code)
	apply("different-account-second-group", secondGroup, "different-member", otherApp.Code)
	if state, e := s.qqGroupApplication(ctx, user, group); e != nil || state.Status != "approved" || posts.Load() != 1 {
		t.Fatalf("first application state: %+v, QQ POSTs=%d, %v", state, posts.Load(), e)
	}
	assertStatus("pending")
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 102, "Second Pilot", "other-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Update(ctx, other.UserID, "987654321", "Second Pilot", "0"); err != nil {
		t.Fatal(err)
	}
	otherCode, _, err := s.CreateQQOfficialChallenge(ctx, other.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmQQOfficial(ctx, "other-binding", member, otherCode, []byte("other-binding"), verified); !errors.Is(err, ErrQQOfficialConflict) {
		t.Fatalf("other account could take the reserved identity: %v", err)
	}
	join("wrong-member", group, "different-member")
	join("wrong-group", secondGroup, member)
	isMember = false
	join("no-longer-member", group, member)
	assertStatus("pending")
	isMember = true
	join("joined", group, member)
	join("joined", group, member)
	assertStatus("confirmed")
	state, err := s.qqGroupApplication(ctx, user, group)
	if err != nil || state.Status != "bound" || state.ApprovedAt == nil || state.BoundAt == nil {
		t.Fatalf("membership was not recorded: %+v, %v", state, err)
	}
	var boundOpenID string
	if err = pool.QueryRow(ctx, "SELECT openid FROM community_qq_bindings WHERE user_id=$1", user).Scan(&boundOpenID); err != nil || boundOpenID != member {
		t.Fatalf("official binding OpenID=%q: %v", boundOpenID, err)
	}
	if err = s.ConfirmQQOfficial(ctx, "outdated-binding", "different-member", unusedDirectCode, []byte("outdated-binding"), verified); !errors.Is(err, ErrQQChallenge) {
		t.Fatalf("old direct code was still valid after joining: %v", err)
	}
}
