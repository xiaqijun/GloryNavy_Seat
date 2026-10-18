package feedback

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestScreenshotValidation(t *testing.T) {
	var p, j bytes.Buffer
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&p, src); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&j, src, nil); err != nil {
		t.Fatal(err)
	}
	good := ImageUpload{MIME: "image/png", Content: p.Bytes()}
	for _, images := range [][]ImageUpload{nil, {good}, {good, good, {MIME: "image/jpeg", Content: j.Bytes()}}} {
		if err := validateImages(images); err != nil {
			t.Fatal(err)
		}
	}
	// Valid PNG header with excessive dimensions must be rejected before decoding pixels.
	bomb := bytes.Clone(p.Bytes())
	binary.BigEndian.PutUint32(bomb[16:20], 100_000)
	binary.BigEndian.PutUint32(bomb[20:24], 100_000)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	for _, images := range [][]ImageUpload{{good, good, good, good}, {{MIME: "image/png"}}, {{MIME: "image/jpeg", Content: p.Bytes()}}, {{MIME: "image/svg+xml", Content: []byte(`<svg onload="alert(1)"/>`)}}, {{MIME: "image/png", Content: bytes.Repeat([]byte{0}, maxImageBytes+1)}}, {{MIME: "image/png", Content: p.Bytes()[:33]}}, {{MIME: "image/png", Content: bomb}}} {
		if err := validateImages(images); err != invalidImages {
			t.Fatal("invalid screenshot accepted", err)
		}
	}
}
