// Package esiclient owns ESI HTTP, encrypted response caching and shared budgets.
// It does not refresh SSO tokens, schedule jobs, or decide business visibility.
package esiclient

import (
	"crypto/cipher"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strconv"
	"time"
)

const CompatibilityDate = "2026-08-18"

type Fault struct {
	Reason    string
	Status    int
	Temporary bool
}

func (e Fault) Error() string { return e.Reason }

type RetryError struct{ Until time.Time }

func (e RetryError) Error() string                    { return "ESI retry deferred" }
func timestamp(t time.Time) pgtype.Timestamptz        { return pgtype.Timestamptz{Time: t, Valid: true} }
func New(pool *pgxpool.Pool, box cipher.AEAD) *Client { return &Client{pool: pool, box: box} }
func (s *Client) SetUserAgent(value string)           { s.userAgent = value }
func RetryUntil(h http.Header, now time.Time) time.Time {
	until := now
	for _, key := range []string{"Retry-After", "X-Esi-Error-Limit-Reset"} {
		if secs, err := strconv.ParseInt(h.Get(key), 10, 32); err == nil && secs >= 0 {
			candidate := now.Add(time.Duration(secs) * time.Second)
			if candidate.After(until) {
				until = candidate
			}
		}
	}
	if date, err := http.ParseTime(h.Get("Retry-After")); err == nil && date.After(until) {
		until = date
	}
	if until.Equal(now) && h.Get("Retry-After") != "0" && h.Get("X-Esi-Error-Limit-Reset") != "0" {
		return now.Add(time.Minute)
	}
	return until
}
