package feedback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/feedback/internal/store"
)

var invalidImages = &Fault{400, "invalid_feedback_images", "截图最多 3 张，请选择有效的 PNG/JPG 图片"}

type ImageUpload struct {
	MIME     string `json:"mime"`
	Content  []byte `json:"content"`
	source   io.ReadSeeker
	prepared io.ReadSeeker
	size     int64
	digest   [32]byte
}
type Image struct {
	ID   string `json:"id"`
	MIME string `json:"mime"`
}

// Inspect the encoded stream without allocating a full-resolution pixel buffer.
func validateImages(images []ImageUpload) error {
	if len(images) > 3 {
		return invalidImages
	}
	for i := range images {
		in := &images[i]
		in.prepared = in.source
		if in.prepared == nil {
			in.prepared = bytes.NewReader(in.Content)
		}
		if _, err := in.prepared.Seek(0, io.SeekStart); err != nil {
			return err
		}
		prefix := make([]byte, 512)
		n, _ := in.prepared.Read(prefix)
		if (in.MIME != "image/png" && in.MIME != "image/jpeg") || http.DetectContentType(prefix[:n]) != in.MIME {
			return invalidImages
		}
		if _, err := in.prepared.Seek(0, io.SeekStart); err != nil {
			return err
		}
		cfg, format, err := image.DecodeConfig(in.prepared)
		if err != nil || "image/"+format != in.MIME || cfg.Width < 1 || cfg.Height < 1 {
			return invalidImages
		}
		if _, err = in.prepared.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if err = inspectEncodedImage(in.prepared, in.MIME); err != nil {
			return invalidImages
		}
		if _, err = in.prepared.Seek(0, io.SeekStart); err != nil {
			return err
		}
		h := sha256.New()
		in.size, err = io.Copy(h, in.prepared)
		if err != nil {
			return err
		}
		copy(in.digest[:], h.Sum(nil))
	}
	return nil
}

func saveImages(ctx context.Context, q *store.Queries, ticket int64, message pgtype.Int8, images []ImageUpload) error {
	buffer := make([]byte, 1<<20)
	for i, in := range images {
		id, err := q.InsertImage(ctx, store.InsertImageParams{TicketID: ticket, MessageID: message, Ordinal: int16(i + 1), Mime: in.MIME, Digest: in.digest[:], SizeBytes: in.size})
		if err != nil {
			return err
		}
		if _, err = in.prepared.Seek(0, io.SeekStart); err != nil {
			return err
		}
		for left, ordinal := in.size, int64(1); left > 0; ordinal++ {
			n := int(min(left, int64(len(buffer))))
			if _, err = io.ReadFull(in.prepared, buffer[:n]); err != nil {
				return err
			}
			if err = q.InsertImageChunk(ctx, store.InsertImageChunkParams{ImageID: id, Ordinal: ordinal, Content: buffer[:n]}); err != nil {
				return err
			}
			left -= int64(n)
		}
	}
	return nil
}
func sameImages(ctx context.Context, q *store.Queries, ticket int64, message pgtype.Int8, images []ImageUpload) error {
	digests, err := q.ImageDigests(ctx, store.ImageDigestsParams{TicketID: ticket, MessageID: message})
	if err != nil {
		return err
	}
	if len(digests) != len(images) {
		return changed
	}
	for i, in := range images {
		if !bytes.Equal(digests[i], in.digest[:]) {
			return changed
		}
	}
	return nil
}
func loadImages(ctx context.Context, q *store.Queries, id int64, out *Detail) error {
	rows, err := q.ListImages(ctx, id)
	if err != nil {
		return err
	}
	messages := make(map[int64]int, len(out.Messages))
	for i, m := range out.Messages {
		id, _ := strconv.ParseInt(m.ID, 10, 64)
		messages[id] = i
	}
	for _, r := range rows {
		v := Image{ID: strconv.FormatInt(r.ID, 10), MIME: r.Mime}
		if !r.MessageID.Valid {
			out.Ticket.Images = append(out.Ticket.Images, v)
		} else if i, ok := messages[r.MessageID.Int64]; ok {
			out.Messages[i].Images = append(out.Messages[i].Images, v)
		}
	}
	return nil
}

// Image checks current ownership in the content query, including after account merges.
func (s *Service) Image(ctx context.Context, actor string, ticket, id int64) (string, io.ReadCloser, int64, error) {
	admin, err := s.Administrator(ctx, actor)
	if err != nil {
		return "", nil, 0, err
	}
	v, err := store.New(s.Pool).ReadImage(ctx, store.ReadImageParams{TicketID: ticket, ID: id, Actor: uuid(actor), Administrator: admin})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, 0, missing
	}
	if err != nil {
		return "", nil, 0, err
	}
	if len(v.Content) > 0 {
		return v.Mime, io.NopCloser(bytes.NewReader(v.Content)), v.SizeBytes, nil
	}
	return v.Mime, &chunkReader{ctx: ctx, q: store.New(s.Pool), id: id, next: 1, left: v.SizeBytes}, v.SizeBytes, nil
}

type chunkReader struct {
	ctx            context.Context
	q              *store.Queries
	id, next, left int64
	buffer         *bytes.Reader
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.buffer == nil || r.buffer.Len() == 0 {
		if r.left == 0 {
			return 0, io.EOF
		}
		b, err := r.q.ReadImageChunk(r.ctx, store.ReadImageChunkParams{ImageID: r.id, Ordinal: r.next})
		if err != nil {
			return 0, err
		}
		if len(b) == 0 || int64(len(b)) > r.left {
			return 0, io.ErrUnexpectedEOF
		}
		r.next++
		r.buffer = bytes.NewReader(b)
	}
	n, err := r.buffer.Read(p)
	r.left -= int64(n)
	return n, err
}
func (r *chunkReader) Close() error { r.buffer = nil; r.left = 0; return nil }
