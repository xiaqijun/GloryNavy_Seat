package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

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
	contentType := "application/json"
	call := func(method, path string, body []byte, login, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/feedback/"+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", contentType)
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
	// Multipart has no file/request size ceiling and does not retain upload files.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	contentType = writer.FormDataContentType()
	_ = writer.WriteField("payload", `{"request_key":"00000000-0000-4000-8000-000000000002","kind":"bug","title":"大截图","body":"超过旧文件和请求限制"}`)
	large := append(bytes.Clone(pngData.Bytes()), make([]byte, 12<<20)...)
	for range 3 {
		part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="image"; filename="screenshot.png"`}, "Content-Type": {"image/png"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(large); err != nil {
			t.Fatal(err)
		}
	}
	if e = writer.Close(); e != nil {
		t.Fatal(e)
	}
	w = call("POST", "tickets", multipartBody.Bytes(), true, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	w = call("GET", "tickets/"+result.Data.ID, nil, true, false)
	if e = json.Unmarshal(w.Body.Bytes(), &detail); e != nil || len(detail.Data.Ticket.Images) != 3 {
		t.Fatal(e, w.Body.String())
	}
	w = call("GET", "tickets/"+result.Data.ID+"/images/"+detail.Data.Ticket.Images[0].ID, nil, true, false)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), large) {
		t.Fatal(w.Code, "large image not preserved")
	}
	files, e := os.ReadDir(tmp)
	if e != nil || len(files) != 0 {
		t.Fatal("temporary upload leaked", files, e)
	}
	var chunks, maxChunk int
	if e = p.QueryRow(ctx, "SELECT count(*),max(octet_length(content)) FROM feedback_image_chunks").Scan(&chunks, &maxChunk); e != nil || chunks < 36 || maxChunk > 1<<20 {
		t.Fatal(chunks, maxChunk, e)
	}
	// Keep the ordinary host deadlines short to prove that the protected streaming
	// route overrides them through the real response-writer middleware chain.
	server := httptest.NewUnstartedServer(h)
	server.Config.ReadTimeout = 100 * time.Millisecond
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	defer server.Close()
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		_, e := pipeWriter.Write(multipartBody.Bytes()[:1024])
		if e == nil {
			time.Sleep(250 * time.Millisecond)
			_, e = pipeWriter.Write(multipartBody.Bytes()[1024:])
		}
		_ = pipeWriter.CloseWithError(e)
	}()
	request, e := http.NewRequest("POST", server.URL+"/api/v1/feedback/tickets", pipeReader)
	if e != nil {
		t.Fatal(e)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Origin", "https://example.com")
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
	response, e := server.Client().Do(request)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		b, _ := io.ReadAll(response.Body)
		t.Fatal(response.StatusCode, string(b))
	}
}
