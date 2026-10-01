package welfare

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (h Handler) activityProject(w http.ResponseWriter, r *http.Request) {
	var input ActivityProjectInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		respond(w, r, nil, ErrInvalid)
		return
	}
	p, err := h.Service.SaveActivityProject(r.Context(), h.User(r), input)
	respond(w, r, p, err)
}

func (h Handler) activityApply(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 7<<20)
	if err := r.ParseMultipartForm(7 << 20); err != nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	defer r.MultipartForm.RemoveAll()
	id, corp := number(r.FormValue("project_id")), number(r.FormValue("corporation_id"))
	values := append([]string(nil), r.MultipartForm.Value["character_id"]...)
	if len(values) == 0 {
		// Keep the original single-role form compatible with older clients.
		values = []string{r.FormValue("character_id")}
	}
	characters := make([]int64, 0, len(values))
	seen := map[int64]bool{}
	for _, value := range values {
		character := number(value)
		if character == 0 || seen[character] || len(characters) >= 20 {
			respond(w, r, nil, ErrInvalid)
			return
		}
		seen[character] = true
		characters = append(characters, character)
	}
	files := r.MultipartForm.File["images"]
	baseRequestKey := strings.TrimSpace(r.FormValue("request_key"))
	if id == 0 || len(characters) == 0 || corp == 0 || !validUUID(baseRequestKey) || len(files) < 1 || len(files) > 3 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	// Reject an invalid or out-of-scope batch before publishing any child case.
	// Execute performs the same check immediately before each insert; this early
	// pass prevents a typo in one selected role from leaving a partial batch.
	if h.Service.Characters == nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	bound, err := h.Service.Characters(r.Context(), h.User(r))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	allowed := map[int64]bool{}
	for _, character := range bound {
		if character.CorporationID == corp {
			allowed[character.ID] = true
		}
	}
	for _, character := range characters {
		if !allowed[character] {
			respond(w, r, nil, pgx.ErrNoRows)
			return
		}
	}
	images := make([]ActivityImage, 0, len(files))
	for _, file := range files {
		if file.Size < 1 || file.Size > 2<<20 {
			respond(w, r, nil, ErrInvalid)
			return
		}
		reader, err := file.Open()
		if err != nil {
			respond(w, r, nil, ErrInvalid)
			return
		}
		content, err := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
		reader.Close()
		if err != nil || len(content) > 2<<20 {
			respond(w, r, nil, ErrInvalid)
			return
		}
		images = append(images, ActivityImage{MIME: http.DetectContentType(content), Content: content})
	}
	outputs := make([]json.RawMessage, 0, len(characters))
	for _, character := range characters {
		command := Command{Action: "apply", RequestKey: activityChildRequestKey(baseRequestKey, character), CorporationID: corp, Kind: "activity_" + strconv.FormatInt(id, 10), Detail: Detail{CharacterID: character, Description: strings.TrimSpace(r.FormValue("description")), SupportingContractID: number(r.FormValue("contract_id")), ActivityBatchKey: baseRequestKey}, Images: images}
		out, executeErr := h.Service.Execute(r.Context(), h.User(r), command)
		if executeErr != nil {
			respond(w, r, nil, executeErr)
			return
		}
		outputs = append(outputs, out)
	}
	if len(outputs) == 1 {
		respond(w, r, outputs[0], nil)
		return
	}
	respond(w, r, map[string]any{"cases": outputs}, nil)
}

// activityChildRequestKey gives each role an idempotency slot while preserving
// one batch key across retries. The SHA-256-derived UUID is deterministic and
// carries the RFC 4122 version and variant bits expected by validUUID.
func activityChildRequestKey(batch string, character int64) string {
	sum := sha256.Sum256([]byte(batch + ":" + strconv.FormatInt(character, 10)))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func (h Handler) activityImage(w http.ResponseWriter, r *http.Request) {
	id, ordinal := number(chi.URLParam(r, "id")), number(chi.URLParam(r, "ordinal"))
	mime, content, err := h.Service.ActivityImage(r.Context(), h.User(r), id, int(ordinal))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
