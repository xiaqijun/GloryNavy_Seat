package eve

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

// CleanSyncHistory drains the same bounded retention query used by dispatch.
// It never touches targets, unfinished runs, snapshots or business audits.
func CleanSyncHistory(ctx context.Context, pool *pgxpool.Pool, maxBatches int, pause time.Duration, report func(int, int64)) (int64, error) {
	if maxBatches < 1 || maxBatches > 100000 || pause < 10*time.Millisecond {
		return 0, errors.New("invalid cleanup bounds")
	}
	q := store.New(pool)
	var total int64
	for batch := 1; batch <= maxBatches; batch++ {
		n, err := q.CleanupSyncRuns(ctx)
		if err != nil {
			return total, err
		}
		total += n
		if report != nil && (batch%1000 == 0 || n == 0 || batch == maxBatches) {
			report(batch, total)
		}
		if n == 0 {
			return total, nil
		}
		timer := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			timer.Stop()
			return total, ctx.Err()
		case <-timer.C:
		}
	}
	return total, errors.New("batch limit reached; rerun to continue")
}
