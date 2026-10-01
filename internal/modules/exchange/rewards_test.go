package exchange

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/testutil"
)

type rewardNames struct{}

func (rewardNames) TypeNames(_ context.Context, ids []int64) (map[int64]eve.StaticTypeName, error) {
	out := map[int64]eve.StaticTypeName{}
	for _, id := range ids {
		if id == 34 {
			out[id] = eve.StaticTypeName{ID: id, Name: "三钛合金", Source: "sde"}
		}
	}
	return out, nil
}
func (rewardNames) SolarSystemNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return nil, nil
}
func rewardKey(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

const reviewer = "33333333-3333-4333-8333-333333333333"

func rewardFixture(t *testing.T) (*Service, testEvent, Shop) {
	t.Helper()
	s := rewardTestService(t)
	e := testEvent{ID: 1, Version: 1}
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1)`, reviewer); err != nil {
		t.Fatal(err)
	}
	s.Names = rewardNames{}
	s.LockAccounts = func(context.Context, pgx.Tx, []string) error { return nil }
	s.Contracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	s.Administrator = func(_ context.Context, u string) (bool, error) { return u == manager || u == reviewer, nil }
	if err := s.SetPAP(ctx, manager, e.ID, testPAPChange{Version: e.Version, RequestKey: rewardKey(1), Units: 5, Reason: "集结"}); err != nil {
		t.Fatal(err)
	}
	e.Version++
	if err := s.EditShop(ctx, manager, "rate", ShopEdit{Version: 1, RequestKey: rewardKey(2), Rate: 10000}); err != nil {
		t.Fatal(err)
	}
	if err := s.EditShop(ctx, manager, "reward", ShopEdit{RequestKey: rewardKey(3), TypeID: 34, Quantity: 10, Value: 401, Stock: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	sh, err := s.Shop(ctx, manager, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s, e, sh
}
func quote(sh Shop, key int) ClaimReward {
	return ClaimReward{RewardID: sh.Rewards[0].ID, RewardVersion: sh.Rewards[0].Version, RateVersion: sh.Version, RecipientID: 1, RequestKey: rewardKey(key)}
}
func assertShop(t *testing.T, s *Service, earned, reserved, spent, available int64, stock int32) Shop {
	t.Helper()
	sh, err := s.Shop(context.Background(), manager, 0)
	if err != nil || sh.Earned != earned || sh.Reserved != reserved || sh.Spent != spent || sh.Available != available || sh.Rewards[0].Stock != stock {
		t.Fatalf("balance %+v: %v", sh, err)
	}
	var journal int64
	if err := s.Pool.QueryRow(context.Background(), "SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE account_id=$1", manager).Scan(&journal); err != nil || journal != available {
		t.Fatalf("wallet journal mismatch: %d != %d, %v", journal, available, err)
	}
	return sh
}
func TestRewardsSnapshotReplayDebtAndCancel(t *testing.T) {
	s, e, sh := rewardFixture(t)
	ctx := context.Background()
	c := quote(sh, 10)
	id, err := s.ClaimReward(ctx, manager, c)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := s.ClaimReward(ctx, manager, c); err != nil || again != id {
		t.Fatal("replay", again, err)
	}
	altered := c
	altered.RecipientID = 2
	if _, err = s.ClaimReward(ctx, manager, altered); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay", err)
	}
	assertShop(t, s, 10, 5, 0, 5, 1)
	if err = s.EditShop(ctx, manager, "rate", ShopEdit{Version: sh.Version, RequestKey: rewardKey(11), Rate: 20000}); err != nil {
		t.Fatal(err)
	}
	orders, err := s.RewardOrders(ctx, manager, false, 0)
	if err != nil || len(orders.Items) != 1 || orders.Items[0].CoinsMinor != 5 || orders.Items[0].Rate != 10000 || orders.Items[0].Value != 401 {
		t.Fatal(orders, err)
	}
	if err = s.SetPAP(ctx, manager, e.ID, testPAPChange{Version: e.Version, RequestKey: rewardKey(12), Units: 1, Reason: "更正"}); err != nil {
		t.Fatal(err)
	}
	sh = assertShop(t, s, 2, 5, 0, -3, 1)
	if _, err = s.ClaimReward(ctx, manager, quote(sh, 13)); !errors.Is(err, ErrInsufficientCoinsMinor) {
		t.Fatal("debt allowed spend", err)
	}
	requestCancel(t, s, manager, id, 9001)
	d := OrderDecision{Version: 2, RequestKey: rewardKey(14), State: "cancelled", UndeliveredConfirmed: true, Note: "不再需要"}
	if err = s.DecideOrder(ctx, member, id, d); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other cancelled", err)
	}
	if err = s.DecideOrder(ctx, reviewer, id, d); err != nil {
		t.Fatal(err)
	}
	if err = s.DecideOrder(ctx, reviewer, id, d); err != nil {
		t.Fatal("cancel replay", err)
	}
	assertShop(t, s, 2, 0, 0, 2, 2)
	d.RequestKey = rewardKey(15)
	d.State = "fulfilled"
	if err = s.DecideOrder(ctx, reviewer, id, d); !errors.Is(err, ErrInvalid) {
		t.Fatal("manual fulfillment allowed", err)
	}
}
func TestRewardsPermissionsAndFulfillment(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	if err := s.EditShop(ctx, member, "rate", ShopEdit{Version: sh.Version, RequestKey: rewardKey(20), Rate: 1}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	c := quote(sh, 21)
	if _, err := s.ClaimReward(ctx, member, c); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign recipient", err)
	}
	c.RecipientID = 4
	id, err := s.ClaimReward(ctx, member, c)
	if err != nil {
		t.Fatal(err)
	}
	mine, err := s.RewardOrders(ctx, manager, false, 0)
	if err != nil || len(mine.Items) != 0 {
		t.Fatal(mine, err)
	}
	if _, err = s.RewardOrders(ctx, member, true, 0); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("member read all", err)
	}
	d := OrderDecision{Version: 1, RequestKey: rewardKey(22), State: "fulfilled", Note: "合同已交付"}
	for _, user := range []string{member, manager} {
		if err = s.DecideOrder(ctx, user, id, d); !errors.Is(err, ErrInvalid) {
			t.Fatal("manual fulfillment", err)
		}
	}
	installDelivery(t, s, id, 1, "finished")
	if err = s.CheckDelivery(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckDelivery(ctx, id); err != nil {
		t.Fatal(err)
	}
	sh, err = s.Shop(ctx, member, 0)
	if err != nil || sh.Available != 0 || sh.Reserved != 0 || sh.Spent != 5 {
		t.Fatal(sh, err)
	}

}
func TestRewardsConcurrentSpendAndLastStock(t *testing.T) {
	for _, sameAccount := range []bool{true, false} {
		t.Run(fmt.Sprint(sameAccount), func(t *testing.T) {
			s, _, sh := rewardFixture(t)
			ctx := context.Background()
			if sameAccount {
				// Two independent products each cost 6 points, against a shared 10-point account.
				_, err := s.Pool.Exec(ctx, "UPDATE exchange_rewards SET isk_value=501; INSERT INTO exchange_rewards(type_id,quantity,isk_value,stock,enabled) VALUES(34,1,501,2,true)")
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.Pool.Exec(ctx, "UPDATE exchange_rewards SET stock=1"); err != nil {
					t.Fatal(err)
				}
			}
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					c := quote(sh, 30+i)
					user := manager
					if i == 1 {
						if sameAccount {
							c.RewardID = 2
							c.RecipientID = 2
						} else {
							user = member
							c.RecipientID = 4
						}
					}
					_, err := s.ClaimReward(ctx, user, c)
					errs <- err
				}(i)
			}
			wg.Wait()
			close(errs)
			success := 0
			for err := range errs {
				if err == nil {
					success++
				} else if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrInsufficientCoinsMinor) && !errors.Is(err, ErrRewardUnavailable) {
					t.Fatal(err)
				}
			}
			if success != 1 {
				t.Fatal("double spend/stock", success)
			}
		})
	}
}
func TestRewardsDecisionRaceAndRollback(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	id, err := s.ClaimReward(ctx, manager, quote(sh, 40))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	requestCancel(t, s, manager, id, 9002)
	for i, state := range []string{"cancelled", "cancelled"} {
		wg.Add(1)
		go func(i int, state string) {
			defer wg.Done()
			errs <- s.DecideOrder(ctx, reviewer, id, OrderDecision{Version: 2, RequestKey: rewardKey(41 + i), State: state, UndeliveredConfirmed: true, Note: "处理"})
		}(i, state)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("two terminal decisions", success)
	}
	sh, err = s.Shop(ctx, manager, 0)
	if err != nil || sh.Reserved != 0 || sh.Available != 10-sh.Spent || sh.Rewards[0].Stock != int32(2-sh.Spent/5) {
		t.Fatal(sh, err)
	}
	// Audit failure must roll back both cancellation and inventory restoration.
	id, err = s.ClaimReward(ctx, manager, quote(sh, 43))
	if err != nil {
		t.Fatal(err)
	}
	sh, err = s.Shop(ctx, manager, 0)
	if err != nil {
		t.Fatal(err)
	}
	requestCancel(t, s, manager, id, 9003)
	_, err = s.Pool.Exec(ctx, `CREATE FUNCTION reject_shop() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test'; END$$; CREATE TRIGGER reject_shop BEFORE INSERT ON exchange_shop_audit FOR EACH ROW EXECUTE FUNCTION reject_shop()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DecideOrder(ctx, reviewer, id, OrderDecision{Version: 2, RequestKey: rewardKey(44), State: "cancelled", UndeliveredConfirmed: true, Note: "取消"}); err == nil {
		t.Fatal("failure swallowed")
	}
	assertShop(t, s, sh.Earned, sh.Reserved, sh.Spent, sh.Available, sh.Rewards[0].Stock)
}

const manager = "11111111-1111-4111-8111-111111111111"
const member = "22222222-2222-4222-8222-222222222222"

type testEvent struct{ ID, Version int64 }
type testPAPChange struct {
	Version    int64
	RequestKey string
	Units      int32
	Reason     string
}

func rewardTestService(t *testing.T) *Service {
	s := &Service{Pool: testutil.Database(t), Sources: map[string]string{"pap": "PAP"}, AllowNew: true}
	s.Administrator = func(_ context.Context, user string) (bool, error) { return user == manager, nil }
	s.Bindings = func(_ context.Context, _ pgx.Tx, ids []int64) ([]Binding, error) {
		out := []Binding{}
		for _, id := range ids {
			if id == 1 || id == 2 {
				out = append(out, Binding{ID: id, Name: "Lead", UserID: manager})
			}
			if id == 4 {
				out = append(out, Binding{ID: id, Name: "Pilot", UserID: member})
			}
		}
		return out, nil
	}
	if err := s.EditSource(context.Background(), manager, SourceEdit{ID: "pap", Mode: "automatic", MinorPerUnit: 1, Version: 1, RequestKey: rewardKey(99)}); err != nil {
		t.Fatal(err)
	}
	return s
}
func (s *Service) SetPAP(ctx context.Context, _ string, _ int64, c testPAPChange) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = s.ReconcileTx(ctx, tx, "pap", c.RequestKey, c.Reason, []Award{{"1/1", manager, 0, int64(c.Units)}, {"1/2", manager, 0, int64(c.Units)}, {"1/4", member, 0, int64(c.Units)}})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func TestCurrencyAutomaticCreditUsesOriginalRate(t *testing.T) {
	s := rewardTestService(t)
	ctx := context.Background()
	s.Names = rewardNames{}
	apply := func(key string, units int64) error {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err = s.ReconcileTx(ctx, tx, "pap", key, "集结", []Award{{"event/character", manager, 0, units}}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := apply(rewardKey(60), 5); err != nil {
		t.Fatal(err)
	}
	if err := apply(rewardKey(60), 5); err != nil {
		t.Fatal("replay", err)
	}
	if err := s.EditSource(ctx, manager, SourceEdit{ID: "pap", MinorPerUnit: 200, Version: 2, RequestKey: rewardKey(61)}); err != nil {
		t.Fatal(err)
	}
	if err := apply(rewardKey(62), 2); err != nil {
		t.Fatal(err)
	}
	sh, err := s.Shop(ctx, manager, 0)
	if err != nil || sh.Earned != 2 {
		t.Fatal("reprice changed old credits", sh, err)
	}
	s.AllowNew = false
	if err := apply(rewardKey(63), 0); err != nil {
		t.Fatal(err)
	}
	sh, err = s.Shop(ctx, manager, 0)
	if err != nil || sh.Earned != 0 {
		t.Fatal("disabled module lost correction", sh, err)
	}
	var count, sum int64
	if err = s.Pool.QueryRow(ctx, "SELECT count(*),sum(delta) FROM exchange_coin_ledger").Scan(&count, &sum); err != nil || count != 3 || sum != 0 {
		t.Fatal(count, sum, err)
	}
}
