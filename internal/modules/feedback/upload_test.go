package feedback

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"os"
	"testing"
)

func TestMultipartRejectsMalformedAndCleansFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewGray(image.Rect(0, 0, 1, 1)))
	for _, c := range []struct {
		name, payload   string
		count           int
		extra, truncate bool
	}{
		{"valid", `{"kind":"bug","title":"a","body":"b"}`, 3, false, false},
		{"too many", `{}`, 4, false, false},
		{"unknown metadata", `{"unexpected":true}`, 1, false, false},
		{"unknown part", `{}`, 1, true, false},
		{"truncated", `{}`, 1, false, true},
		{"no payload", "", 1, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var body bytes.Buffer
			w := multipart.NewWriter(&body)
			for range c.count {
				p, e := w.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="image"; filename="../../x.png"`}, "Content-Type": {"image/png"}})
				if e != nil {
					t.Fatal(e)
				}
				_, _ = p.Write(picture.Bytes())
			}
			if c.payload != "" {
				_ = w.WriteField("payload", c.payload)
			}
			if c.extra {
				_ = w.WriteField("extra", "bad")
			}
			_ = w.Close()
			b := body.Bytes()
			if c.truncate {
				b = b[:len(b)-10]
			}
			r := httptest.NewRequest("POST", "/", bytes.NewReader(b))
			r.Header.Set("Content-Type", w.FormDataContentType())
			var in Submission
			cleanup, ok := readUploads(httptest.NewRecorder(), r, &in)
			if ok != (c.name == "valid") {
				t.Fatal(ok)
			}
			if ok {
				if err := validateImages(in.Images); err != nil {
					t.Fatal(err)
				}
			}
			cleanup()
			files, e := os.ReadDir(tmp)
			if e != nil || len(files) > 0 {
				t.Fatal(files, e)
			}
		})
	}
}
