package feedback_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/png"
	"io"
	"strconv"
	"testing"

	"glorynavy.local/seat/internal/modules/feedback"
)

func screenshot(t *testing.T) feedback.ImageUpload {
	t.Helper()
	var b bytes.Buffer
	if e := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); e != nil {
		t.Fatal(e)
	}
	return feedback.ImageUpload{MIME: "image/png", Content: b.Bytes()}
}

func TestChunkFailureAndLegacyScreenshotRead(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	ticket, e := s.Create(ctx, a[0], input(1))
	if e != nil {
		t.Fatal(e)
	}
	id, _ := strconv.ParseInt(ticket.ID, 10, 64)
	old := screenshot(t)
	digest := sha256.Sum256(old.Content)
	var imageID int64
	e = p.QueryRow(ctx, `INSERT INTO feedback_images(ticket_id,ordinal,mime,content,digest,size_bytes) VALUES($1,1,$2,$3,$4,$5) RETURNING id`, id, old.MIME, old.Content, digest[:], len(old.Content)).Scan(&imageID)
	if e != nil {
		t.Fatal(e)
	}
	_, reader, size, e := s.Image(ctx, a[0].ID, id, imageID)
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(reader)
	_ = reader.Close()
	if e != nil || size != int64(len(b)) || !bytes.Equal(b, old.Content) {
		t.Fatal("legacy image", e)
	}
	_, e = p.Exec(ctx, `CREATE FUNCTION fail_feedback_chunk() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.ordinal=2 THEN RAISE EXCEPTION 'test chunk failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_feedback_chunk BEFORE INSERT ON feedback_image_chunks FOR EACH ROW EXECUTE FUNCTION fail_feedback_chunk()`)
	if e != nil {
		t.Fatal(e)
	}
	large := screenshot(t)
	large.Content = append(large.Content, make([]byte, 2<<20)...)
	_, e = s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(2).RequestKey, Revision: 1, Body: "large", Images: []feedback.ImageUpload{large}})
	if e == nil {
		t.Fatal("expected second chunk failure")
	}
	var n int
	_ = p.QueryRow(ctx, "SELECT count(*) FROM feedback_image_chunks").Scan(&n)
	if n != 0 {
		t.Fatal("orphan chunks", n)
	}
	_ = p.QueryRow(ctx, "SELECT count(*) FROM feedback_images").Scan(&n)
	if n != 1 {
		t.Fatal("orphan metadata", n)
	}
	d, e := s.Detail(ctx, a[0].ID, id)
	if e != nil || d.Ticket.Revision != 1 || len(d.Messages) != 0 {
		t.Fatal(d, e)
	}
}

func TestScreenshotsAtomicPrivateIdempotentAndMerged(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	in := input(1)
	in.Images = []feedback.ImageUpload{screenshot(t)}
	ticket, e := s.Create(ctx, a[0], in)
	if e != nil {
		t.Fatal(e)
	}
	id, _ := strconv.ParseInt(ticket.ID, 10, 64)
	retry, e := s.Create(ctx, a[0], in)
	if e != nil || retry.ID != ticket.ID {
		t.Fatal(retry, e)
	}
	without := in
	without.Images = nil
	_, e = s.Create(ctx, a[0], without)
	check(t, e, 409)
	d, e := s.Detail(ctx, a[0].ID, id)
	if e != nil || len(d.Ticket.Images) != 1 {
		t.Fatal(d, e)
	}
	imageID, _ := strconv.ParseInt(d.Ticket.Images[0].ID, 10, 64)
	for _, actor := range []feedback.Actor{a[0], a[2]} {
		mime, reader, size, e := s.Image(ctx, actor.ID, id, imageID)
		if e != nil {
			t.Fatal(e)
		}
		content, e := io.ReadAll(reader)
		_ = reader.Close()
		if e != nil || mime != "image/png" || size != int64(len(content)) || !bytes.Equal(content, in.Images[0].Content) {
			t.Fatal(mime, e)
		}
	}
	_, _, _, e = s.Image(ctx, a[1].ID, id, imageID)
	check(t, e, 404)
	_, _, _, e = s.Image(ctx, a[0].ID, id+1, imageID)
	check(t, e, 404)
	r := feedback.Reply{RequestKey: input(2).RequestKey, Revision: 1, Body: "补充截图", Images: in.Images}
	ticket, e = s.Reply(ctx, a[0], id, r)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Reply(ctx, a[0], id, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Images = nil
	_, e = s.Reply(ctx, a[0], id, r)
	check(t, e, 409)
	r.RequestKey = input(3).RequestKey
	r.Images = in.Images
	_, e = s.Reply(ctx, a[0], id, r)
	check(t, e, 409)
	d, e = s.Detail(ctx, a[2].ID, id)
	if e != nil || len(d.Messages) != 1 || len(d.Messages[0].Images) != 1 {
		t.Fatal(d, e)
	}
	var count int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM feedback_images").Scan(&count); e != nil || count != 2 {
		t.Fatal(count, e)
	}
	// Administrator revocation takes effect on the next image read.
	s.Administrator = func(context.Context, string) (bool, error) { return false, nil }
	_, _, _, e = s.Image(ctx, a[2].ID, id, imageID)
	check(t, e, 404)
	// Ownership comes exclusively from the parent ticket. No copied ACL survives a merge.
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.MergeAccountTx(ctx, tx, a[0].ID, a[1].ID, true); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	_, _, _, e = s.Image(ctx, a[0].ID, id, imageID)
	check(t, e, 404)
	_, reader, _, e := s.Image(ctx, a[1].ID, id, imageID)
	if e != nil {
		t.Fatal(e)
	}
	_ = reader.Close()
}

func TestScreenshotFailureRollsBackTicketAndReply(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	ticket, e := s.Create(ctx, a[0], input(1))
	if e != nil {
		t.Fatal(e)
	}
	id, _ := strconv.ParseInt(ticket.ID, 10, 64)
	// Force an attachment storage failure after the parent insert.
	_, e = p.Exec(ctx, `CREATE FUNCTION fail_feedback_image() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test image write failure'; END $$; CREATE TRIGGER fail_feedback_image BEFORE INSERT ON feedback_images FOR EACH ROW EXECUTE FUNCTION fail_feedback_image()`)
	if e != nil {
		t.Fatal(e)
	}
	in := input(2)
	in.Images = []feedback.ImageUpload{screenshot(t)}
	if _, e = s.Create(ctx, a[0], in); e == nil {
		t.Fatal("expected storage failure")
	}
	if _, e = s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(3).RequestKey, Revision: 1, Body: "截图", Images: in.Images}); e == nil {
		t.Fatal("expected reply storage failure")
	}
	var n int
	p.QueryRow(ctx, "SELECT count(*) FROM feedback_tickets").Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	d, e := s.Detail(ctx, a[0].ID, id)
	if e != nil || d.Ticket.Revision != 1 || len(d.Messages) != 0 {
		t.Fatal(d, e)
	}
}
