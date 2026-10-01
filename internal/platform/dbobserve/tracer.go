// Package dbobserve records slow application queries without SQL text or values.
package dbobserve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type queryKey struct{}
type queryStart struct {
	at  time.Time
	sql string
}

var sqlcName = regexp.MustCompile(`^-- name: ([A-Za-z][A-Za-z0-9_]{0,79}) :(?:exec|execrows|one|many)\s`)

type Tracer struct {
	logger    *slog.Logger
	threshold time.Duration
	now       func() time.Time
}

func New(logger *slog.Logger, threshold time.Duration) *Tracer {
	return &Tracer{logger: logger, threshold: threshold, now: time.Now}
}

func (t *Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if t.threshold <= 0 {
		return ctx
	}
	return context.WithValue(ctx, queryKey{}, queryStart{at: t.now(), sql: data.SQL})
}

func (t *Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(queryKey{}).(queryStart)
	if !ok || t.threshold <= 0 {
		return
	}
	elapsed := t.now().Sub(start.at)
	if elapsed < t.threshold {
		return
	}
	// Hash only slow statements. Arguments, raw SQL, and server error messages
	// can contain EVE tokens or member data and must never enter these logs.
	digest := sha256.Sum256([]byte(start.sql))
	name := "unnamed"
	if match := sqlcName.FindStringSubmatch(start.sql); match != nil {
		name = match[1]
	}
	result := "ok"
	var pgerr *pgconn.PgError
	attrs := []any{"query_name", name, "sql_hash", hex.EncodeToString(digest[:8]),
		"duration_ms", elapsed.Milliseconds(), "rows", data.CommandTag.RowsAffected()}
	switch {
	case errors.Is(data.Err, context.Canceled):
		result = "canceled"
	case errors.Is(data.Err, context.DeadlineExceeded):
		result = "timeout"
	case errors.As(data.Err, &pgerr):
		result = "database_error"
		// SQLSTATE is a server code, not the error's Message/Detail/Hint fields.
		attrs = append(attrs, "sqlstate", pgerr.Code)
	case data.Err != nil:
		result = "error"
	}
	attrs = append(attrs, "result", result)
	t.logger.WarnContext(ctx, "slow database query", attrs...)
}
