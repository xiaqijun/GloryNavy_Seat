package community

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

// MergeAccountTx intentionally does not copy QQ/KOOK details. Account merge
// rules keep the target community profile and site permissions; source-only
// official openid bindings and pending challenges must be removed so they do
// not block a future explicit binding on the target account.
func (s *Service) MergeAccountTx(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	var sourceID pgtype.UUID
	if err := sourceID.Scan(source); err != nil {
		return nil, err
	}
	onebot, err := store.New(tx).CountOneBotBindings(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	notices, err := store.CancelAccountApprovalNotices(ctx, tx, source, apply)
	if err != nil {
		return nil, err
	}
	var bindings, challenges, groupApplications, groupBindings int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM community_qq_bindings WHERE user_id=$1`, source).Scan(&bindings); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM community_qq_link_challenges WHERE user_id=$1 AND used_at IS NULL`, source).Scan(&challenges); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM community_qq_group_applications WHERE user_id=$1`, source).Scan(&groupApplications); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM community_qq_group_bindings WHERE user_id=$1`, source).Scan(&groupBindings); err != nil {
		return nil, err
	}
	if apply {
		if err := store.New(tx).DeleteOneBotBindings(ctx, sourceID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM community_qq_link_challenges WHERE user_id=$1`, source); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM community_qq_bindings WHERE user_id=$1`, source); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM community_qq_group_bindings WHERE user_id=$1`, source); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM community_qq_group_applications WHERE user_id=$1`, source); err != nil {
			return nil, err
		}
	}
	return json.Marshal(map[string]any{"approval_notices_cancelled": notices, "onebot_bindings_removed": onebot, "qq_bindings_removed": bindings, "pending_challenges_removed": challenges, "qq_group_applications_removed": groupApplications, "qq_group_bindings_removed": groupBindings})
}
