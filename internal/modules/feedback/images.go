package feedback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/feedback/internal/store"
)

const maxImageBytes = 2_000_000

var invalidImages = &Fault{400, "invalid_feedback_images", "截图最多 3 张，仅支持 PNG/JPG，每张不超过 2 MB，且不超过 1600 万像素"}

type ImageUpload struct {
	MIME    string `json:"mime"`
	Content []byte `json:"content"`
}
type Image struct {
	ID   string `json:"id"`
	MIME string `json:"mime"`
}

// DecodeConfig bounds allocation before the full decode rejects damaged images.
func validateImages(images []ImageUpload) error {
	if len(images) > 3 {
		return invalidImages
	}
	for _, in := range images {
		if len(in.Content) == 0 || len(in.Content) > maxImageBytes || (in.MIME != "image/png" && in.MIME != "image/jpeg") || http.DetectContentType(in.Content) != in.MIME {
			return invalidImages
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(in.Content))
		if err != nil || "image/"+format != in.MIME || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
			return invalidImages
		}
		if _, _, err = image.Decode(bytes.NewReader(in.Content)); err != nil {
			return invalidImages
		}
	}
	return nil
}

func saveImages(ctx context.Context, q *store.Queries, ticket int64, message pgtype.Int8, images []ImageUpload) error {
	for i, in := range images {
		digest := sha256.Sum256(in.Content)
		if err := q.InsertImage(ctx, store.InsertImageParams{TicketID: ticket, MessageID: message, Ordinal: int16(i + 1), Mime: in.MIME, Content: in.Content, Digest: digest[:]}); err != nil {
			return err
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
		digest := sha256.Sum256(in.Content)
		if !bytes.Equal(digests[i], digest[:]) {
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
func (s *Service) Image(ctx context.Context, actor string, ticket, id int64) (string, []byte, error) {
	admin, err := s.Administrator(ctx, actor)
	if err != nil {
		return "", nil, err
	}
	v, err := store.New(s.Pool).ReadImage(ctx, store.ReadImageParams{TicketID: ticket, ID: id, Actor: uuid(actor), Administrator: admin})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, missing
	}
	return v.Mime, v.Content, err
}
