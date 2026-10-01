package esiclient

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

var validGroup = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,120}$`)

// Counts are based exclusively on X-Ratelimit-Used, never on the limiter's
// conservative reservation or on changes in Remaining across responses.
type rateObservation struct {
	route, group, identity            string
	character                         int64
	network, cache, localWait         bool
	status                            int
	at                                pgtype.Timestamptz
	capacity, window, remaining, used pgtype.Int8
	retry                             pgtype.Timestamptz
}

func headerCount(h http.Header, key string) pgtype.Int8 {
	n, err := strconv.ParseInt(h.Get(key), 10, 64)
	return pgtype.Int8{Int64: n, Valid: err == nil && n >= 0 && n < 10000000}
}

func (o *rateObservation) response(res *http.Response) {
	o.at = timestamp(time.Now())
	o.status = res.StatusCode
	group := res.Header.Get("X-Ratelimit-Group")
	if validGroup.MatchString(group) {
		o.group = group
		o.used = headerCount(res.Header, "X-Ratelimit-Used")
		o.remaining = headerCount(res.Header, "X-Ratelimit-Remaining")
		parts := strings.Split(res.Header.Get("X-Ratelimit-Limit"), "/")
		if len(parts) == 2 {
			n, err := strconv.ParseInt(parts[0], 10, 64)
			d, de := time.ParseDuration(parts[1])
			if err == nil && de == nil && n > 0 && n < 10000000 && d >= time.Second && d <= 24*time.Hour {
				o.capacity = pgtype.Int8{Int64: n, Valid: true}
				o.window = pgtype.Int8{Int64: int64(d / time.Second), Valid: true}
			}
		}
		if o.capacity.Valid && o.remaining.Int64 > o.capacity.Int64 {
			o.remaining.Valid = false
		}
	}
	if res.StatusCode == 429 {
		raw := res.Header.Get("Retry-After")
		if n, err := strconv.ParseInt(raw, 10, 32); err == nil && n >= 0 && n <= 86400 {
			o.retry = timestamp(o.at.Time.Add(time.Duration(n) * time.Second))
		} else if t, err := http.ParseTime(raw); err == nil && t.After(o.at.Time) && t.Before(o.at.Time.Add(24*time.Hour)) {
			o.retry = timestamp(t)
		}
	}
}

func (s *Client) recordRate(ctx context.Context, o rateObservation) {
	if !o.network && !o.cache && !o.localWait {
		return
	}
	// Short, best-effort metadata transaction; never turn a successful fetch into
	// a retry just because monitoring failed. No tokens, URL query or body saved.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.writeRate(ctx, o); err != nil {
		slog.Warn("ESI bucket observation unavailable")
	}
}

func (s *Client) writeRate(ctx context.Context, o rateObservation) error {
	// A route without a declared or observed group does not have a new bucket.
	if o.group == "" {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	key := "group:" + o.group + ":" + o.identity
	id, err := q.ObserveESIBucket(ctx, store.ObserveESIBucketParams{LimitKey: key, GroupName: o.group, CharacterID: o.character, HeaderAt: o.at, Capacity: o.capacity, WindowSeconds: o.window, Remaining: o.remaining, RetryAt: o.retry})
	if err != nil {
		return err
	}
	count := func(b bool) int64 {
		if b {
			return 1
		}
		return 0
	}
	used := int64(0)
	if o.used.Valid {
		used = o.used.Int64
	}
	err = q.ObserveESIRouteUsage(ctx, store.ObserveESIRouteUsageParams{BucketID: id, Route: o.route, NetworkRequests: count(o.network), CacheHits: count(o.cache), LocalWaits: count(o.localWait), UpstreamLimits: count(o.status == 429 || o.status == 420), UsedTokens: used, MeasuredResponses: count(o.used.Valid), UnmeasuredRequests: count(o.network && !o.used.Valid), LastStatus: int32(o.status), LastUsed: o.used, LastResponseAt: o.at})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
