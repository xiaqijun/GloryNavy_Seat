package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/feedback"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestFeedbackScreenshotHTTPBoundary(t *testing.T) {
	p := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(p)
	token, e := accounts.SignIn(ctx, 101, "Pilot", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	session, e := accounts.Session(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	h, e := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "feedback"}, AuthConfig{Origin: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path string, body []byte, login, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/feedback/"+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		if csrf {
			r.Header.Set("Origin", "https://example.com")
			r.Header.Set("X-CSRF-Token", session.CSRFToken)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	var pngData bytes.Buffer
	if e = png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); e != nil {
		t.Fatal(e)
	}
	// Each legal max-size image plus base64 and 6000 CJK characters fits the 8 MiB edge/API cap.
	content := append(pngData.Bytes(), make([]byte, 2_000_000-pngData.Len())...)
	in := feedback.Submission{RequestKey: "00000000-0000-4000-8000-000000000001", Kind: "bug", Title: "截图", Body: strings.Repeat("截图", 3000)}
	for range 3 {
		in.Images = append(in.Images, feedback.ImageUpload{MIME: "image/png", Content: content})
	}
	body, e := json.Marshal(in)
	if e != nil || len(body) >= 8<<20 {
		t.Fatal(len(body), e)
	}
	if w := call("POST", "tickets", body, true, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := call("POST", "tickets", body, true, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct{ Data feedback.Ticket }
	if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	w = call("GET", "tickets/"+result.Data.ID, nil, true, false)
	var detail struct{ Data feedback.Detail }
	if e = json.Unmarshal(w.Body.Bytes(), &detail); e != nil || len(detail.Data.Ticket.Images) != 3 {
		t.Fatal(e, w.Body.String())
	}
	path := "tickets/" + result.Data.ID + "/images/" + detail.Data.Ticket.Images[0].ID
	if w = call("GET", path, nil, false, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w = call("GET", path, nil, true, false)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(w.Body.Bytes(), content) {
		t.Fatal(w.Code, w.Header())
	}
	if w = call("GET", "tickets/999/images/"+detail.Data.Ticket.Images[0].ID, nil, true, false); w.Code != 404 {
		t.Fatal(w.Code)
	}
	body = append(body, bytes.Repeat([]byte{' '}, (8<<20)-len(body)+1)...)
	if w = call("POST", "tickets", body, true, true); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
