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
	if _, err := s.CreateQQGroupApplication(ctx, user, groupID); !errors.Is(err, ErrQQNotBound) {
		t.Fatalf("unbound application: %v", err)
	}
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
}
