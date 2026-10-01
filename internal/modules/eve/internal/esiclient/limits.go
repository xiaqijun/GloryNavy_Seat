package esiclient

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

const reservationCost int64 = 5
const requestDeadline = 30 * time.Second

type reservation struct {
	ID  int64
	Key string
}

// Egress serializes short accounting transactions across workers. HTTP is
// outside the transaction. Always lock egress before any bucket.
func lockBudget(ctx context.Context, q *store.Queries, key string) (store.EveEsiLimit, error) {
	if err := q.EnsureESILimit(ctx, key); err != nil {
		return store.EveEsiLimit{}, err
	}
	return q.LockESILimit(ctx, key)
}

// Reserve preserves the entry point used to check global protection.
func (s *Client) Reserve(ctx context.Context, key string) error {
	_, err := s.reserve(ctx, key, routePolicy{})
	return err
}

func (s *Client) reserve(ctx context.Context, key string, policy routePolicy) (r reservation, err error) {
	r.Key = key
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	egress, err := lockBudget(ctx, q, "egress")
	if err != nil {
		return r, err
	}
	row, err := lockBudget(ctx, q, key)
	if err != nil {
		return r, err
	}
	now := time.Now()
	for _, until := range []time.Time{egress.BlockedUntil.Time, row.BlockedUntil.Time, row.NextRequestAt.Time} {
		if until.After(now) {
			return r, RetryError{until}
		}
	}
	// Learned capacity is authoritative; OpenAPI bootstraps the first request.
	if row.Capacity <= 0 && policy.Capacity > 0 {
		row.Capacity, row.WindowSeconds = policy.Capacity, policy.WindowSeconds
		if err = q.ConfigureESIBudget(ctx, store.ConfigureESIBudgetParams{LimitKey: key, Capacity: row.Capacity, WindowSeconds: row.WindowSeconds}); err != nil {
			return r, err
		}
	}
	if row.Capacity > 0 {
		usage, e := q.ESIBudgetUsage(ctx, key)
		if e != nil {
			return r, e
		}
		if row.Capacity-usage.Used < reservationCost {
			until := usage.NextExpiry.Time
			if !until.After(now) {
				until = now.Add(time.Duration(row.WindowSeconds) * time.Second)
			}
			return r, RetryError{until}
		}
		// A crashed process retains its charge through the HTTP deadline and window.
		r.ID, err = q.InsertESICharge(ctx, store.InsertESIChargeParams{LimitKey: key, Amount: reservationCost, ExpiresAt: timestamp(now.Add(time.Duration(row.WindowSeconds)*time.Second + requestDeadline)), Settled: false})
		if err != nil {
			return r, err
		}
		if err = q.SnapshotESIBudget(ctx, key); err != nil {
			return r, err
		}
	}
	if err = q.ReserveESILimit(ctx, store.ReserveESILimitParams{LimitKey: key, NextRequestAt: timestamp(now.Add(200 * time.Millisecond))}); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

// Settlement replaces the reservation, never charges twice. Unknown network
// outcomes keep five tokens. Higher stale snapshots cannot mint local tokens.
func (s *Client) settle(ctx context.Context, r reservation, route, identity string, res *http.Response) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if _, err = lockBudget(ctx, q, "egress"); err != nil {
		return err
	}
	key := r.Key
	o := rateObservation{}
	if res != nil {
		o.response(res)
		if o.group != "" {
			key = "group:" + o.group + ":" + identity
			if err = q.SetESIRoute(ctx, store.SetESIRouteParams{RouteKey: route, GroupName: o.group}); err != nil {
				return err
			}
		}
	}
	row, err := lockBudget(ctx, q, key)
	if err != nil {
		return err
	}
	if o.capacity.Valid && o.window.Valid {
		row.Capacity, row.WindowSeconds = o.capacity.Int64, o.window.Int64
		if err = q.ConfigureESIBudget(ctx, store.ConfigureESIBudgetParams{LimitKey: key, Capacity: row.Capacity, WindowSeconds: row.WindowSeconds}); err != nil {
			return err
		}
	}
	now := time.Now()
	if row.Capacity > 0 {
		cost := reservationCost
		if o.used.Valid {
			cost = o.used.Int64
		}
		expires := timestamp(now.Add(time.Duration(row.WindowSeconds) * time.Second))
		if r.ID != 0 {
			n, e := q.SettleESICharge(ctx, store.SettleESIChargeParams{ID: r.ID, LimitKey: key, Amount: cost, ExpiresAt: expires})
			if e != nil {
				return e
			}
			if n == 0 {
				return tx.Commit(ctx)
			} // idempotent completion
		} else {
			if _, err = q.InsertESICharge(ctx, store.InsertESIChargeParams{LimitKey: key, Amount: cost, ExpiresAt: expires, Settled: true}); err != nil {
				return err
			}
		}
		usage, e := q.ESIBudgetUsage(ctx, key)
		if e != nil {
			return e
		}
		if o.remaining.Valid && o.remaining.Int64 <= row.Capacity {
			debt := row.Capacity - usage.Used - o.remaining.Int64
			if debt > 0 {
				// Unattributed usage has no known timestamp: conservatively retain only
				// the missing amount for one window. Never extend existing charges.
				if _, err = q.InsertESICharge(ctx, store.InsertESIChargeParams{LimitKey: key, Amount: debt, ExpiresAt: expires, Settled: true}); err != nil {
					return err
				}
			}
		}
		if err = q.SnapshotESIBudget(ctx, key); err != nil {
			return err
		}
	}
	if r.ID != 0 && key != r.Key {
		if err = q.SnapshotESIBudget(ctx, r.Key); err != nil {
			return err
		}
	}
	if res != nil {
		remain, re := strconv.Atoi(res.Header.Get("X-Esi-Error-Limit-Remain"))
		if res.StatusCode == 420 || (re == nil && remain >= 0 && remain <= 5) {
			if err = q.BlockESILimit(ctx, store.BlockESILimitParams{LimitKey: "egress", BlockedUntil: timestamp(RetryUntil(res.Header, now))}); err != nil {
				return err
			}
		}
		if res.StatusCode == 429 {
			if err = q.BlockESILimit(ctx, store.BlockESILimitParams{LimitKey: key, BlockedUntil: timestamp(RetryUntil(res.Header, now))}); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (s *Client) policy(ctx context.Context, route string) (routePolicy, error) {
	p := catalog[route]
	group, err := store.New(s.pool).GetESIRoute(ctx, route)
	if err == pgx.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if group != "" && group != p.Group {
		p.Group = group
		p.Capacity, p.WindowSeconds = BucketPolicy(group)
	}
	return p, nil
}
