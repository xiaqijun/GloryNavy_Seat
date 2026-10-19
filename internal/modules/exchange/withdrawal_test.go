package exchange

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

func TestOwnerWithdrawalRefundAndReplay(t *testing.T) {
	for _, requested := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "previous cancellation request"}[requested], func(t *testing.T) {
			s, _, sh := rewardFixture(t)
			ctx := context.Background()
			c := quote(sh, 9000)
			c.Units = unitCount(2)
			id, e := s.ClaimReward(ctx, manager, c)
			if e != nil {
				t.Fatal(e)
			}
			version := int64(1)
			if requested {
				requestCancel(t, s, manager, id, 9001)
				version++
			}
			orders, e := s.RewardOrders(ctx, manager, false, 0)
			if e != nil || !orders.Items[0].CanWithdraw {
				t.Fatal(orders, e)
			}
			decision := OrderDecision{Version: version, RequestKey: rewardKey(9002), State: "withdrawn"}
			if e = s.DecideOrder(ctx, reviewer, id, decision); !errors.Is(e, pgx.ErrNoRows) {
				t.Fatal("another account withdrew", e)
			}
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for range 2 {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- s.DecideOrder(ctx, manager, id, decision) }()
			}
			wg.Wait()
			close(errs)
			for e := range errs {
				if e != nil {
					t.Fatal(e)
				}
			}
			assertShop(t, s, 10, 0, 0, 10, 2)
			var n int
			var sum int64
			if e = s.Pool.QueryRow(ctx, `SELECT count(*),sum(delta) FROM exchange_coin_ledger WHERE kind='refund' AND reference=$1`, strconv.FormatInt(id, 10)).Scan(&n, &sum); e != nil || n != 1 || sum != 10 {
				t.Fatal(n, sum, e)
			}
			if _, e = s.StartDelivery(ctx, reviewer, id, version); !errors.Is(e, ErrConflict) {
				t.Fatal("started withdrawn order", e)
			}
			tx, e := s.Pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			if e = s.StartSettlementTx(ctx, tx, reviewer, manager, id); !errors.Is(e, ErrConflict) {
				t.Fatal("batched withdrawn order", e)
			}
		})
	}
}

func TestOwnerWithdrawalGuards(t *testing.T) {
	for _, kind := range []string{"started", "batch", "live contract", "lost evidence", "read failure", "catalog changed", "rollback"} {
		t.Run(kind, func(t *testing.T) {
			s, _, sh := rewardFixture(t)
			ctx := context.Background()
			c := quote(sh, 9010)
			c.Units = unitCount(2)
			id, e := s.ClaimReward(ctx, manager, c)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "started":
				if _, e = s.StartDelivery(ctx, manager, id, 1); !errors.Is(e, pgx.ErrNoRows) {
					t.Fatal("self handling", e)
				}
				for range 2 {
					h, e := s.StartDelivery(ctx, reviewer, id, 1)
					if e != nil || !h.Started {
						t.Fatal(h, e)
					}
				}
			case "batch":
				tx, e := s.Pool.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				if e = s.StartSettlementTx(ctx, tx, reviewer, manager, id); e != nil {
					t.Fatal(e)
				}
				if e = tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
			case "live contract":
				installDelivery(t, s, id, 4, "outstanding")
			case "lost evidence":
				if e = store.SaveDelivery(ctx, s.Pool, id, "awaiting_acceptance", []eve.DeliveryContract{{ID: 99, Status: "outstanding"}}); e != nil {
					t.Fatal(e)
				}
			case "read failure":
				s.Contracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
					return nil, ErrUnavailable
				}
			case "catalog changed":
				if _, e = s.Pool.Exec(ctx, `UPDATE exchange_rewards SET catalog_version=catalog_version+1,stock=0 WHERE id=$1`, sh.Rewards[0].ID); e != nil {
					t.Fatal(e)
				}
			case "rollback":
				if _, e = s.Pool.Exec(ctx, `CREATE FUNCTION reject_refund() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='refund' THEN RAISE EXCEPTION 'refund fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_refund BEFORE INSERT ON exchange_coin_ledger FOR EACH ROW EXECUTE FUNCTION reject_refund()`); e != nil {
					t.Fatal(e)
				}
			}
			e = s.DecideOrder(ctx, manager, id, OrderDecision{Version: 1, RequestKey: rewardKey(9011), State: "withdrawn"})
			if kind == "catalog changed" {
				if e != nil {
					t.Fatal(e)
				}
				assertShop(t, s, 10, 0, 0, 10, 0)
			} else {
				if e == nil {
					t.Fatal("unsafe withdrawal")
				}
				assertShop(t, s, 10, 10, 0, 0, 0)
			}
		})
	}
}

func TestOwnerWithdrawalRacesStart(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "handoff", true: "batch"}[batch], func(t *testing.T) {
			s, _, sh := rewardFixture(t)
			ctx := context.Background()
			id, e := s.ClaimReward(ctx, manager, quote(sh, 9020))
			if e != nil {
				t.Fatal(e)
			}
			begin := make(chan struct{})
			errs := make(chan error, 2)
			go func() {
				<-begin
				errs <- s.DecideOrder(ctx, manager, id, OrderDecision{Version: 1, RequestKey: rewardKey(9021), State: "withdrawn"})
			}()
			go func() {
				<-begin
				if !batch {
					_, e := s.StartDelivery(ctx, reviewer, id, 1)
					errs <- e
					return
				}
				tx, e := s.Pool.Begin(ctx)
				if e == nil {
					e = s.StartSettlementTx(ctx, tx, reviewer, manager, id)
					if e == nil {
						e = tx.Commit(ctx)
					}
					tx.Rollback(ctx)
				}
				errs <- e
			}()
			close(begin)
			a, b := <-errs, <-errs
			if (a == nil) == (b == nil) {
				t.Fatal("exactly one operation must succeed", a, b)
			}
			row, e := store.New(s.Pool).Redemption(ctx, id)
			if e != nil {
				t.Fatal(e)
			}
			d, e := store.ReadDelivery(ctx, s.Pool, id)
			if e != nil {
				t.Fatal(e)
			}
			if (row.State == "cancelled") == d.Started {
				t.Fatal("both withdrawal and dispatch committed", row.State, d)
			}
		})
	}
}
