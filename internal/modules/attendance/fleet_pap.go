package attendance

import (
	"context"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

// awardFleetPAP is called under the event and identity locks in the same
// transaction as the fresh fleet capture. Only roles in that snapshot are
// eligible, and an existing award row is never awarded a second time.
func (s *Service) awardFleetPAP(ctx context.Context, tx pgx.Tx, q *store.Queries, eventID int64, actor, key pgtype.UUID, points int32, eligible []store.AttendancePapAward) (int, error) {
	if len(eligible) == 0 {
		return 0, ErrInvalid
	}
	existing, err := q.PAPAwards(ctx, eventID)
	if err != nil {
		return 0, err
	}
	seen := make(map[int64]bool, len(existing))
	for _, a := range existing {
		seen[a.CharacterID] = true
	}
	slices.SortFunc(eligible, func(a, b store.AttendancePapAward) int { return intCmp(a.CharacterID, b.CharacterID) })
	newAwards := make([]store.AttendancePapAward, 0, len(eligible))
	accounts := make([]string, 0, len(eligible))
	for _, a := range eligible {
		if seen[a.CharacterID] {
			continue
		}
		seen[a.CharacterID] = true
		newAwards = append(newAwards, a)
		if account := a.AccountID.String(); !slices.Contains(accounts, account) {
			accounts = append(accounts, account)
		}
	}
	slices.Sort(accounts)
	for _, account := range accounts {
		if err := q.LockPAPAccount(ctx, account); err != nil {
			return 0, err
		}
	}
	const reason = "舰队点名发放集结分"
	if s.CoinAwards != nil && len(newAwards) > 0 {
		awards := make([]PAPCoinAward, 0, len(newAwards))
		for _, a := range newAwards {
			awards = append(awards, PAPCoinAward{Reference: strconv.FormatInt(eventID, 10) + "/" + strconv.FormatInt(a.CharacterID, 10), AccountID: a.AccountID.String(), Units: int64(points)})
		}
		if err := s.CoinAwards(ctx, tx, key.String(), reason, awards); err != nil {
			return 0, err
		}
	}
	for _, a := range newAwards {
		if err := q.SavePAPAward(ctx, store.SavePAPAwardParams{EventID: eventID, CharacterID: a.CharacterID, AccountID: a.AccountID, CharacterName: a.CharacterName, Points: points}); err != nil {
			return 0, err
		}
		if err := q.AddPAPLedger(ctx, store.AddPAPLedgerParams{EventID: eventID, CharacterID: a.CharacterID, AccountID: a.AccountID, ActorID: actor, RequestKey: key, Delta: points, Balance: points, Reason: reason}); err != nil {
			return 0, err
		}
	}
	if err := q.SetEventPAP(ctx, store.SetEventPAPParams{ID: eventID, PapPoints: points, PapIssued: true}); err != nil {
		return 0, err
	}
	return len(newAwards), nil
}

func intCmp(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
