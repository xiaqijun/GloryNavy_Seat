package feedback

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"time"
)

// readUploads spills each file directly to a private temporary file. The caller
// defers cleanup until the transaction completes; all error paths clean up too.
func readUploads(w http.ResponseWriter, r *http.Request, v any) (func(), bool) {
	files := []*os.File{}
	cleanup := func() {
		for _, f := range files {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "multipart/form-data" {
		return cleanup, read(w, r, v)
	}
	if err != nil {
		respond(w, r, nil, invalid)
		return cleanup, false
	}
	// The server's ordinary 15-second total deadlines do not apply to streaming
	// screenshots. The edge still enforces inactivity timeouts on this route.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Time{})
	reader, err := r.MultipartReader()
	if err != nil {
		respond(w, r, nil, invalid)
		return cleanup, false
	}
	images := []ImageUpload{}
	payload := false
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			respond(w, r, nil, invalid)
			return cleanup, false
		}
		switch part.FormName() {
		case "payload":
			if payload || part.FileName() != "" {
				respond(w, r, nil, invalid)
				return cleanup, false
			}
			b, e := io.ReadAll(io.LimitReader(part, 32769))
			if e != nil || len(b) > 32768 {
				respond(w, r, nil, invalid)
				return cleanup, false
			}
			d := json.NewDecoder(bytes.NewReader(b))
			d.DisallowUnknownFields()
			if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
				respond(w, r, nil, invalid)
				return cleanup, false
			}
			payload = true
		case "image":
			if len(images) == 3 || part.FileName() == "" {
				respond(w, r, nil, invalidImages)
				return cleanup, false
			}
			m, _, e := mime.ParseMediaType(part.Header.Get("Content-Type"))
			if e != nil || (m != "image/png" && m != "image/jpeg") {
				respond(w, r, nil, invalidImages)
				return cleanup, false
			}
			f, e := os.CreateTemp("", "feedback-upload-*")
			if e != nil {
				respond(w, r, nil, e)
				return cleanup, false
			}
			files = append(files, f)
			if _, e = io.Copy(f, part); e != nil {
				respond(w, r, nil, e)
				return cleanup, false
			}
			images = append(images, ImageUpload{MIME: m, source: f})
		default:
			respond(w, r, nil, invalid)
			return cleanup, false
		}
		_ = part.Close()
	}
	if !payload {
		respond(w, r, nil, invalid)
		return cleanup, false
	}
	switch in := v.(type) {
	case *Submission:
		if len(in.Images) > 0 {
			respond(w, r, nil, invalid)
			return cleanup, false
		}
		in.Images = images
	case *Reply:
		if len(in.Images) > 0 {
			respond(w, r, nil, invalid)
			return cleanup, false
		}
		in.Images = images
	default:
		respond(w, r, nil, invalid)
		return cleanup, false
	}
	return cleanup, true
}
