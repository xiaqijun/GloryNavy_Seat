package welfare

import (
	"context"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"strconv"
	"time"
)

type Reimbursement struct {
	Status            string   `json:"status"`
	State             string   `json:"state"`
	Kind              string   `json:"kind"`
	Reason            string   `json:"reason"`
	AvailableKinds    []string `json:"available_kinds"`
	AttendanceEventID int64    `json:"attendance_event_id,string"`
}
type LossView struct {
	Loss
	Reimbursement Reimbursement `json:"reimbursement"`
}

// Annotate only after the raw loss reader has authorized this character.
func (s *Service) annotateLosses(ctx context.Context, actor string, corp, character int64, losses []Loss) ([]LossView, error) {
	out := make([]LossView, 0, len(losses))
	if len(losses) == 0 {
		return out, nil
	}
	admin, err := s.Administrator(ctx, actor)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(losses))
	for i, l := range losses {
		ids[i] = strconv.FormatInt(l.ID, 10)
	}
	states, err := store.LossStatuses(ctx, s.Pool, actor, admin, corp, character, ids)
	if err != nil {
		return nil, err
	}
	linked := map[int64]int64{}
	if s.AttendanceLosses != nil {
		kmIDs := make([]int64, len(losses))
		for i, l := range losses {
			kmIDs[i] = l.ID
		}
		linked, err = s.AttendanceLosses(ctx, corp, character, kmIDs)
		if err != nil {
			return nil, err
		}
	}
	for _, l := range losses {
		r := Reimbursement{Status: "available", AvailableKinds: []string{"solo"}, Reason: "attendance_unlinked", AttendanceEventID: linked[l.ID]}
		if r.AttendanceEventID > 0 {
			r.AvailableKinds = []string{"srp", "solo"}
			r.Reason = ""
		}
		if l.At.After(time.Now()) {
			r.Status = "unavailable"
			r.Reason = "future_loss"
			r.AvailableKinds = []string{}
		}
		for _, v := range states {
			if v.KillmailID == strconv.FormatInt(l.ID, 10) {
				r.Status = "processing"
				if v.State == "completed" {
					r.Status = "completed"
				}
				r.State = v.State
				r.Kind = v.Kind
				r.Reason = ""
				r.AvailableKinds = []string{}
				break
			}
		}
		out = append(out, LossView{Loss: l, Reimbursement: r})
	}
	return out, nil
}
