package app

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/feedback"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFeedbackMergeWhenModuleDisabled(t *testing.T) {
	p := testutil.Database(t)
	ctx := context.Background()
	ids := identity.New(p)
	sourceToken, e := ids.SignIn(ctx, 101, "Source", "source-owner", "")
	if e != nil {
		t.Fatal(e)
	}
	source, _ := ids.Session(ctx, sourceToken)
	targetToken, e := ids.SignIn(ctx, 202, "Target", "target-owner", "")
	if e != nil {
		t.Fatal(e)
	}
	target, _ := ids.Session(ctx, targetToken)
	service := &feedback.Service{Pool: p, LockAccounts: ids.LockActiveAccounts, Administrator: func(context.Context, string) (bool, error) { return false, nil }}
	ticket, e := service.Create(ctx, feedback.Actor{ID: source.UserID, Name: "Source"}, feedback.Submission{RequestKey: "00000000-0000-4000-8000-000000000001", Kind: "feature", Title: "Suggestion", Body: "Keep this history"})
	if e != nil {
		t.Fatal(e)
	}
	proof, e := ids.ProveMerge(ctx, 101, "source-owner", targetToken, target.UserID, nil)
	if e != nil {
		t.Fatal(e)
	}
	h, e := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access"}, AuthConfig{Origin: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/identity/merges/"+proof, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: targetToken})
		r.Header.Set("Origin", "https://example.com")
		r.Header.Set("X-CSRF-Token", target.CSRFToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Data identity.MergePreview `json:"data"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	var summary struct {
		Tickets int `json:"tickets"`
	}
	if e = json.Unmarshal(out.Data.Data["feedback"], &summary); e != nil || summary.Tickets != 1 {
		t.Fatal(e, w.Body.String())
	}
	w = call("POST", `{"token":"`+out.Data.Token+`"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var owner, original string
	e = p.QueryRow(ctx, "SELECT account_id::text,original_account_id::text FROM feedback_tickets WHERE id=$1::bigint", ticket.ID).Scan(&owner, &original)
	if e != nil || owner != target.UserID || original != source.UserID {
		t.Fatal(owner, original, e)
	}
	if _, e = service.Create(ctx, feedback.Actor{ID: source.UserID, Name: "Old session"}, feedback.Submission{RequestKey: "00000000-0000-4000-8000-000000000002", Kind: "bug", Title: "Late", Body: "Late"}); e == nil {
		t.Fatal("retired account accepted")
	}
	if _, e = p.Exec(ctx, "INSERT INTO feedback_tickets(account_id,submitted_by,author_name,request_key,kind,title,body) VALUES($1,$1,'Old',gen_random_uuid(),'bug','Late','Late')", source.UserID); e == nil {
		t.Fatal("active-account trigger accepted retired account")
	}
}
