package attendance

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

func (s *Service) MergeAccountTx(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	return store.MergeAccount(ctx, tx, source, target, apply)
}
