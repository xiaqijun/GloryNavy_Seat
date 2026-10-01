package access

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"net/http"
)

type MemberCharacter struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	IsMain bool   `json:"is_main"`
}
type MemberBinding struct {
	Value        string `json:"value"`
	Confirmation string `json:"confirmation"`
}
type MemberCommunity struct {
	QQ   MemberBinding `json:"qq"`
	KOOK MemberBinding `json:"kook"`
}
type MemberData struct {
	UserID     string            `json:"user_id"`
	Characters []MemberCharacter `json:"characters"`
	Access     Account           `json:"access"`
	Community  *MemberCommunity  `json:"community"`
}

func (h Handler) memberData(w http.ResponseWriter, r *http.Request) {
	// Recheck before resolving the supplied target; no caller-controlled identity
	// is accepted by the ordinary account or community mutation routes.
	allowed, err := h.Service.IsAdministrator(r.Context(), h.User(r))
	if err != nil {
		outcome(w, r, err)
		return
	}
	if !allowed {
		httpapi.Failure(w, r, 403, "forbidden", "仅站点管理员可查看成员数据")
		return
	}
	target := r.PathValue("user")
	if _, err := uuid(target); err != nil {
		httpapi.Failure(w, r, 400, "invalid_member", "成员 ID 无效")
		return
	}
	if h.MemberData == nil {
		outcome(w, r, errors.New("member data unavailable"))
		return
	}
	data, err := h.MemberData(r.Context(), target)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "member_unavailable", "成员不存在或已不可用")
		return
	}
	if err != nil {
		outcome(w, r, err)
		return
	}
	httpapi.Respond(w, r, 200, data)
}
