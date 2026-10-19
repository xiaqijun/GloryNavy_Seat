package feedback

import (
	"bytes"
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
	var large bytes.Buffer
	if err := png.Encode(&large, image.NewGray(image.Rect(0, 0, 5000, 4000))); err != nil {
		t.Fatal(err)
	}
	if err := validateImages([]ImageUpload{{MIME: "image/png", Content: large.Bytes()}, {MIME: "image/png", Content: append(bytes.Clone(p.Bytes()), make([]byte, 12<<20)...)}}); err != nil {
		t.Fatal("large valid image rejected", err)
	}
	broken := bytes.Clone(p.Bytes())
	broken[40] ^= 1
	for _, images := range [][]ImageUpload{{good, good, good, good}, {{MIME: "image/png"}}, {{MIME: "image/jpeg", Content: p.Bytes()}}, {{MIME: "image/svg+xml", Content: []byte(`<svg onload="alert(1)"/>`)}}, {{MIME: "image/png", Content: p.Bytes()[:33]}}, {{MIME: "image/png", Content: broken}}, {{MIME: "image/jpeg", Content: j.Bytes()[:j.Len()-2]}}} {
		if err := validateImages(images); err != invalidImages {
			t.Fatal("invalid screenshot accepted", err)
		}
	}
}
