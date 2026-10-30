package attendance

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

type FleetMOTDInput struct {
	SourceID   int64  `json:"source_character_id,string"`
	Version    int64  `json:"version,string"`
	Revision   int64  `json:"revision"`
	RequestKey string `json:"request_key"`
}

// QueueFleetNoticeMOTD records the authorized snapshot before any external write.
func (s *Service) QueueFleetNoticeMOTD(ctx context.Context, user string, eventID int64, in FleetMOTDInput) error {
	actor, err := uuid(user)
	if err != nil {
		return ErrInvalid
	}
	key, err := uuid(in.RequestKey)
	if err != nil || in.SourceID <= 0 || in.Version <= 0 || in.Revision <= 0 {
		return ErrInvalid
	}
	q := store.New(s.Pool)
	ev, err := q.GetEvent(ctx, eventID)
	if err != nil {
		return err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return err
	}
	if same, e := store.FleetNoticeMOTDReplay(ctx, s.Pool, eventID, key, actor, in.SourceID, in.Revision, in.Version); e == nil {
		if !same {
			return ErrConflict
		}
		return nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if ev.State != "open" || ev.Version != in.Version || s.EVE == nil {
		return ErrConflict
	}
	owned, err := s.Own(ctx, user)
	if err != nil {
		return err
	}
	var source *Binding
	for i := range owned {
		if owned[i].ID == in.SourceID {
			source = &owned[i]
			break
		}
	}
	if source == nil || len(source.OwnerHash) == 0 {
		return pgx.ErrNoRows
	}
	fleet, err := s.EVE.Fleet(ctx, source.ID, source.OwnerHash)
	if err != nil {
		return ErrUnavailable
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	bindings, err := s.Bindings(ctx, tx, []int64{source.ID})
	if err != nil {
		return err
	}
	if len(bindings) != 1 || bindings[0].ID != source.ID || bindings[0].UserID != user || subtle.ConstantTimeCompare(bindings[0].OwnerHash, fleet.OwnerHash) != 1 {
		return ErrConflict
	}
	if err = s.EVE.GuardFleet(ctx, tx, fleet); err != nil {
		return ErrConflict
	}
	q = store.New(tx)
	ev, err = q.LockEvent(ctx, eventID)
	if err != nil {
		return err
	}
	if same, e := store.FleetNoticeMOTDReplay(ctx, tx, eventID, key, actor, in.SourceID, in.Revision, in.Version); e == nil {
		if !same {
			return ErrConflict
		}
		return nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if ev.State != "open" || ev.Version != in.Version {
		return ErrConflict
	}
	n, err := store.FleetNoticeByEvent(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if n.Revision != in.Revision {
		return ErrConflict
	}
	busy, err := store.FleetMOTDOutstanding(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if busy {
		return ErrConflict
	}
	if err = store.InsertFleetNoticeMOTD(ctx, tx, eventID, key, actor, source.ID, fleet.FleetID, fleet.Generation, fleet.OwnerHash, in.Revision, in.Version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
