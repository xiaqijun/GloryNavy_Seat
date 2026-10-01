package system

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/system/internal/store"
)

// New receives only this module's infrastructure dependencies from the host.
// Generated SQL and persistence models remain under the module's internal tree.
func New(pool *pgxpool.Pool, version string) Service {
	return Service{Reader: store.New(pool), Version: version}
}
