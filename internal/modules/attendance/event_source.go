package attendance

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

type SourceSelection struct {
	SourceID string `json:"source_character_id"`
	Revision string `json:"revision"`
	Invalid  bool   `json:"invalid"`
}

type SourceChange struct {
	SourceID int64 `json:"source_character_id,string"`
	Revision int64 `json:"revision,string"`
}

// EventSource is private to the operator and never exposes another account's role.
func (s *Service) EventSource(ctx context.Context, user string, eventID int64) (SourceSelection, error) {
	result := SourceSelection{Revision: "0"}
	actor, err := uuid(user)
	if err != nil {
		return result, ErrInvalid
	}
	ev, err := store.New(s.Pool).GetEvent(ctx, eventID)
	if err != nil {
		return result, err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return result, err
	}
	selection, err := store.GetEventSource(ctx, s.Pool, eventID, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.Revision = strconv.FormatInt(selection.Revision, 10)
	owned, err := s.Own(ctx, user)
	if err != nil {
		return SourceSelection{}, err
	}
	for _, character := range owned {
		if character.ID == selection.SourceID && character.UserID == user {
			result.SourceID = strconv.FormatInt(character.ID, 10)
			return result, nil
		}
	}
	result.Invalid = true
	return result, nil
}

// SetEventSource rechecks binding and active account inside the write transaction.
func (s *Service) SetEventSource(ctx context.Context, user string, eventID int64, change SourceChange) (SourceSelection, error) {
	actor, err := uuid(user)
	if err != nil || change.SourceID <= 0 || change.Revision < 0 {
		return SourceSelection{}, ErrInvalid
	}
	ev, err := store.New(s.Pool).GetEvent(ctx, eventID)
	if err != nil {
		return SourceSelection{}, err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return SourceSelection{}, err
	}
	if s.Bindings == nil || s.AllianceLockAccounts == nil {
		return SourceSelection{}, ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceSelection{}, err
	}
	defer tx.Rollback(context.Background())
	bindings, err := s.Bindings(ctx, tx, []int64{change.SourceID})
	if err != nil {
		return SourceSelection{}, err
	}
	if len(bindings) != 1 || bindings[0].ID != change.SourceID || bindings[0].UserID != user {
		return SourceSelection{}, pgx.ErrNoRows
	}
	if err = s.AllianceLockAccounts(ctx, tx, []string{user}); err != nil {
		return SourceSelection{}, err
	}
	locked, err := store.New(tx).LockEvent(ctx, eventID)
	if err != nil {
		return SourceSelection{}, err
	}
	if locked.CorporationID != ev.CorporationID {
		return SourceSelection{}, ErrConflict
	}
	revision, err := store.SaveEventSource(ctx, tx, eventID, actor, change.SourceID, change.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceSelection{}, ErrConflict
	}
	if err != nil {
		return SourceSelection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SourceSelection{}, err
	}
	return SourceSelection{SourceID: strconv.FormatInt(change.SourceID, 10), Revision: strconv.FormatInt(revision, 10)}, nil
}
