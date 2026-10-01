package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
)

var (
	ErrConflict  = errors.New("character belongs to another user")
	ErrSession   = errors.New("original session expired")
	ErrCharacter = errors.New("character is not available")
	ErrMain      = errors.New("select another main before unlinking")
	ErrLast      = errors.New("last active login character")
)

// LoginIntent is supplied only from the server-side, one-use SSO flow.
type LoginIntent struct {
	Kind, UserID string
	ExpectedID   int64
}
type CredentialWrite func(context.Context, pgx.Tx) error
type CharacterCleanup func(context.Context, pgx.Tx, int64, string) error

// Complete atomically applies verified identity, credentials and session changes.
// Lock order: character advisory lock/row, user row, session row, EVE credential.
func (s *Service) Complete(ctx context.Context, id int64, name, owner, previous string, intent LoginIntent, save CredentialWrite) (string, error) {
	if id <= 0 || name == "" || owner == "" {
		return "", ErrCharacter
	}
	if intent.Kind == "" {
		intent.Kind = "login"
	}
	if intent.Kind != "login" && intent.Kind != "link" && intent.Kind != "reauthorize" {
		return "", ErrCharacter
	}
	if intent.Kind == "reauthorize" && intent.ExpectedID != id {
		return "", ErrCharacter
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", id); err != nil {
		return "", err
	}
	q := store.New(tx)
	ch, err := q.GetCharacter(ctx, id)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var user pgtype.UUID
	if intent.Kind != "login" {
		if user.Scan(intent.UserID) != nil || !user.Valid {
			return "", ErrSession
		}
		if found && ch.UserID != user {
			return "", ErrConflict
		}
		if _, err = q.LockUser(ctx, user); err != nil {
			return "", ErrSession
		}
		session, e := q.LockSession(ctx, httpapi.Hash(previous))
		if e != nil || session.UserID != user {
			return "", ErrSession
		}
		if intent.Kind == "reauthorize" && !found {
			return "", ErrCharacter
		}
	} else if found {
		user = ch.UserID
		if _, err = q.LockUser(ctx, user); err != nil {
			return "", err
		}
	} else {
		user, err = q.CreateUser(ctx)
		if err != nil {
			return "", err
		}
	}
	ownerHash := httpapi.Hash(owner)
	if found && (ch.Status != "active" || subtle.ConstantTimeCompare(ch.OwnerHash, ownerHash) != 1) {
		if err = q.BlockCharacter(ctx, id); err != nil {
			return "", err
		}
		if err = q.RevokeCharacterSessions(ctx, id); err != nil {
			return "", err
		}
		if err = q.CharacterEvent(ctx, store.CharacterEventParams{UserID: user, CharacterID: id, Action: "ownership_blocked"}); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return "", ErrOwnership
	}
	if !found {
		if err = q.CreateCharacter(ctx, store.CreateCharacterParams{CharacterID: id, UserID: user, Name: name, OwnerHash: ownerHash}); err != nil {
			return "", err
		}
		if intent.Kind == "login" {
			if err = q.SetMain(ctx, store.SetMainParams{ID: user, MainCharacterID: pgtype.Int8{Int64: id, Valid: true}}); err != nil {
				return "", err
			}
		}
		if err = q.CharacterEvent(ctx, store.CharacterEventParams{UserID: user, CharacterID: id, Action: "linked"}); err != nil {
			return "", err
		}
	} else if err = q.UpdateCharacter(ctx, store.UpdateCharacterParams{CharacterID: id, Name: name}); err != nil {
		return "", err
	}
	if save != nil {
		if err = save(ctx, tx); err != nil {
			return "", err
		}
	}
	// Linking/reauthorizing proves ownership but does not change the authenticating character.
	if intent.Kind != "login" {
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return previous, nil
	}
	if err = q.DeleteSession(ctx, httpapi.Hash(previous)); err != nil {
		return "", err
	}
	if err = q.PruneSessions(ctx); err != nil {
		return "", err
	}
	if err = q.LimitUserSessions(ctx, user); err != nil {
		return "", err
	}
	token := rand.Text()
	err = q.InsertSession(ctx, store.InsertSessionParams{TokenHash: httpapi.Hash(token), UserID: user, CharacterID: id, CsrfToken: rand.Text(), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(SessionLifetime), Valid: true}})
	if err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return token, nil
}

type BoundCharacter struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	IsMain bool   `json:"is_main"`
}

func (s *Service) Characters(ctx context.Context, userID string) ([]BoundCharacter, error) {
	var user pgtype.UUID
	if err := user.Scan(userID); err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListCharacters(ctx, user)
	if err != nil {
		return nil, err
	}
	result := make([]BoundCharacter, 0, len(rows))
	for _, r := range rows {
		result = append(result, BoundCharacter{strconv.FormatInt(r.CharacterID, 10), r.Name, r.Status, r.IsMain})
	}
	return result, nil
}

// ChangeCharacter verifies ownership and the original live session inside the mutation transaction.
func (s *Service) ChangeCharacter(ctx context.Context, token string, id int64, unlink bool, cleanup CharacterCleanup) error {
	if id <= 0 {
		return ErrCharacter
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", id); err != nil {
		return err
	}
	q := store.New(tx)
	ch, err := q.GetCharacter(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCharacter
	}
	if err != nil {
		return err
	}
	user, err := q.LockUser(ctx, ch.UserID)
	if err != nil {
		return err
	}
	session, err := q.LockSession(ctx, httpapi.Hash(token))
	if err != nil {
		return ErrSession
	}
	if session.UserID != ch.UserID {
		return ErrCharacter
	}
	action := "main_changed"
	if unlink {
		if user.MainCharacterID.Int64 == id {
			return ErrMain
		}
		active, e := q.ActiveCharacters(ctx, ch.UserID)
		if e != nil {
			return e
		}
		others := 0
		for _, c := range active {
			if c.CharacterID != id {
				others++
			}
		}
		if others == 0 {
			return ErrLast
		}
		if cleanup == nil {
			return errors.New("character cleanup unavailable")
		}
		if err = cleanup(ctx, tx, id, ch.UserID.String()); err != nil {
			return err
		}
		if err = q.RevokeCharacterSessions(ctx, id); err != nil {
			return err
		}
		if err = q.DeleteCharacter(ctx, store.DeleteCharacterParams{CharacterID: id, UserID: ch.UserID}); err != nil {
			return err
		}
		action = "unlinked"
	} else {
		if ch.Status != "active" {
			return ErrCharacter
		}
		if err = q.SetMain(ctx, store.SetMainParams{ID: ch.UserID, MainCharacterID: pgtype.Int8{Int64: id, Valid: true}}); err != nil {
			return err
		}
	}
	if err = q.CharacterEvent(ctx, store.CharacterEventParams{UserID: ch.UserID, CharacterID: id, Action: action}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
