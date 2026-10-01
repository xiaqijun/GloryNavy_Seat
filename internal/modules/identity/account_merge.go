package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
	"slices"
	"strconv"
)

var ErrMerge = errors.New("merge verification expired or accounts changed")

type MergeParticipant func(context.Context, pgx.Tx, string, string, bool) (json.RawMessage, error)
type MergePreview struct {
	ID         string                     `json:"id"`
	SourceMain Character                  `json:"source_main"`
	TargetMain Character                  `json:"target_main"`
	Characters []BoundCharacter           `json:"characters"`
	Data       map[string]json.RawMessage `json:"data"`
	Token      string                     `json:"token"`
	Completed  bool                       `json:"completed"`
}

func mergeHash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func rosterHash(chars []store.MergeCharacter) string {
	parts := []string{}
	for _, c := range chars {
		parts = append(parts, strconv.FormatInt(c.ID, 10), c.User, c.Owner, c.Status, strconv.FormatBool(c.Main), c.Name)
	}
	return mergeHash(parts)
}
func (s *Service) mergeSession(ctx context.Context, tx pgx.Tx, previous, target string) error {
	session, err := store.New(tx).LockSession(ctx, httpapi.Hash(previous))
	if err != nil || session.UserID.String() != target {
		return ErrSession
	}
	return nil
}

// ProveMerge is called only after trusted SSO verification; it never moves a character.
func (s *Service) ProveMerge(ctx context.Context, id int64, owner, previous, target string, save CredentialWrite) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	source, err := store.MergeSource(ctx, tx, id)
	if err != nil || source == target {
		return "", ErrMerge
	}
	chars, err := store.MergeCharacters(ctx, tx, source, target)
	if err != nil {
		return "", err
	}
	if len(chars) > 100 {
		return "", ErrMerge
	}
	before := rosterHash(chars)
	if err = store.LockMerge(ctx, tx, chars, source, target); err != nil {
		return "", ErrMerge
	}
	chars, err = store.MergeCharacters(ctx, tx, source, target)
	if err != nil {
		return "", err
	}
	if rosterHash(chars) != before {
		return "", ErrMerge
	}
	if err = s.mergeSession(ctx, tx, previous, target); err != nil {
		return "", err
	}
	found := false
	for _, c := range chars {
		if c.Status != "active" {
			return "", ErrMerge
		}
		if c.ID == id && c.User == source && c.Owner == hex.EncodeToString(httpapi.Hash(owner)) {
			found = true
		}
	}
	if !found {
		return "", ErrMerge
	}
	if save != nil {
		if err = save(ctx, tx); err != nil {
			return "", err
		}
	}
	request, err := store.NewMergeRequest(ctx, tx, source, target, httpapi.Hash(previous), id, before)
	if err != nil {
		return "", err
	}
	return request, tx.Commit(ctx)
}
func (s *Service) Merge(ctx context.Context, previous, id, token string, apply bool) (MergePreview, error) {
	var result MergePreview
	var valid pgtype.UUID
	if valid.Scan(id) != nil || !valid.Valid {
		return result, ErrMerge
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	proof, err := store.ReadMergeRequest(ctx, tx, id, httpapi.Hash(previous), false)
	if err != nil {
		return result, ErrMerge
	}
	if proof.Completed != nil {
		if err = s.mergeSession(ctx, tx, previous, proof.Target); err != nil {
			return result, err
		}
		if apply && token != proof.PreviewHash {
			return result, ErrMerge
		}
		if json.Unmarshal(proof.Preview, &result) != nil {
			return result, ErrMerge
		}
		result.Completed = true
		return result, nil
	}
	chars, err := store.MergeCharacters(ctx, tx, proof.Source, proof.Target)
	if err != nil {
		return result, err
	}
	if err = store.LockMerge(ctx, tx, chars, proof.Source, proof.Target); err != nil {
		return result, ErrMerge
	}
	if err = s.mergeSession(ctx, tx, previous, proof.Target); err != nil {
		return result, err
	}
	proof, err = store.ReadMergeRequest(ctx, tx, id, httpapi.Hash(previous), true)
	if err != nil {
		return result, ErrMerge
	}
	chars, err = store.MergeCharacters(ctx, tx, proof.Source, proof.Target)
	if err != nil {
		return result, err
	}
	if rosterHash(chars) != proof.Identity {
		return result, ErrMerge
	}
	result = MergePreview{ID: id, Characters: []BoundCharacter{}, Data: map[string]json.RawMessage{}}
	for _, c := range chars {
		if c.Status != "active" {
			return result, ErrMerge
		}
		ch := Character{ID: strconv.FormatInt(c.ID, 10), Name: c.Name}
		if c.User == proof.Source {
			result.Characters = append(result.Characters, BoundCharacter{ID: ch.ID, Name: ch.Name, Status: c.Status, IsMain: false})
			if c.Main {
				result.SourceMain = ch
			}
		} else if c.Main {
			result.TargetMain = ch
		}
	}
	if result.SourceMain.ID == "" || result.TargetMain.ID == "" || len(s.MergeParticipants) == 0 {
		return result, ErrMerge
	}
	names := []string{}
	for name := range s.MergeParticipants {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		data, e := s.MergeParticipants[name](ctx, tx, proof.Source, proof.Target, false)
		if e != nil {
			return result, e
		}
		result.Data[name] = data
	}
	result.Token = mergeHash(result)
	if apply {
		if token == "" || token != proof.PreviewHash || token != result.Token {
			return result, ErrMerge
		}
		for _, name := range names {
			if _, err = s.MergeParticipants[name](ctx, tx, proof.Source, proof.Target, true); err != nil {
				return result, err
			}
		}
		if err = store.FinishMerge(ctx, tx, proof, httpapi.Hash(previous)); err != nil {
			return result, err
		}
		result.Completed = true
	} else {
		data, _ := json.Marshal(result)
		if err = store.SetMergePreview(ctx, tx, id, result.Token, data); err != nil {
			return result, err
		}
	}
	return result, tx.Commit(ctx)
}
func (s *Service) CancelMerge(ctx context.Context, previous, id string) error {
	var valid pgtype.UUID
	if valid.Scan(id) != nil || !valid.Valid {
		return ErrMerge
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = store.CancelMerge(ctx, tx, id, httpapi.Hash(previous)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
