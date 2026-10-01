package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/internal/testutil"
	"sync"
	"testing"
	"time"
)

const adminID = "00000000-0000-4000-8000-000000000001"
const userID = "00000000-0000-4000-8000-000000000002"
const otherID = "00000000-0000-4000-8000-000000000003"

func key(n int) string { return fmt.Sprintf("00000000-0000-4000-9000-%012d", n) }
func fixture(t *testing.T) (*Service, *exchange.Service) {
	t.Helper()
	pool := testutil.Database(t)
	ctx := context.Background()
	for _, id := range []string{adminID, userID, otherID} {
		if _, e := pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1)`, id); e != nil {
			t.Fatal(e)
		}
	}
	coins := &exchange.Service{Pool: pool, AllowNew: true}
	s := &Service{Pool: pool, LockAccounts: identity.New(pool).LockActiveAccounts, Credit: coins.WelfareGrantTx}
	s.MatchReward = exchange.MatchRewardDelivery
	s.GrowthCheck = func(context.Context, string, int64, int64, Config) (json.RawMessage, json.RawMessage, error) {
		return json.RawMessage(`{"version":"1"}`), json.RawMessage(`{"state":"met","remaining_sp":0}`), nil
	}
	s.GuardGrowth = func(context.Context, pgx.Tx, int64, Config, json.RawMessage, json.RawMessage) error { return nil }
	s.GuardAttendanceLoss = func(context.Context, pgx.Tx, string, int64, int64, int64) (int64, error) { return 1, nil }
	s.AttendanceLosses = func(_ context.Context, _, _ int64, ids []int64) (map[int64]int64, error) {
		out := map[int64]int64{}
		for _, id := range ids {
			out[id] = 1
		}
		return out, nil
	}
	s.Administrator = func(_ context.Context, id string) (bool, error) { return id == adminID, nil }
	s.Scope = func(_ context.Context, id string, corp int64, manage bool) (bool, error) {
		return corp == 10 && (!manage || id == adminID), nil
	}
	s.Characters = func(_ context.Context, id string) ([]Character, error) {
		return []Character{{ID: 123, Name: "成员角色", AccountID: id, CorporationID: 10}}, nil
	}
	s.Members = func(_ context.Context, id string, corp int64) ([]Character, error) {
		if id != adminID || corp != 10 {
			return nil, pgx.ErrNoRows
		}
		return []Character{{ID: 123, Name: "成员角色", AccountID: userID, CorporationID: 10}, {ID: 456, Name: "另一角色", AccountID: otherID, CorporationID: 10}}, nil
	}
	return s, coins
}
func TestCalculate(t *testing.T) {
	for _, v := range []struct {
		k    string
		base int64
		d    bool
		want int64
	}{{"srp", 50000000000, false, 40000000000}, {"srp", 50000000000, true, 20000000000}, {"solo", 30000000000, false, 15000000000}, {"solo", 80000000000, false, 20000000000}, {"capital", 1000000, false, 100000}, {"titan", 1000000, false, 50000}} {
		n, e := Calculate(v.k, v.base, v.d)
		if e != nil || n != v.want {
			t.Fatalf("%+v: %d %v", v, n, e)
		}
	}
}
func TestWelfareGrantAtomicReplayAndReverse(t *testing.T) {
	s, coins := fixture(t)
	ctx := context.Background()
	c := Command{Action: "grant", RequestKey: key(1), CorporationID: 10, Note: "贡献奖励", Lines: []GrantLine{{userID, 1234}, {otherID, 999}}}
	q, e := s.Preview(ctx, adminID, c)
	if e != nil {
		t.Fatal(e)
	}
	c.Token = q.Token
	if _, e = s.Execute(ctx, userID, c); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("non-admin credit: %v", e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Execute(ctx, adminID, c); results <- e }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	var sum, count int64
	if e = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0),count(*) FROM exchange_coin_ledger`).Scan(&sum, &count); e != nil || sum != 2233 || count != 2 {
		t.Fatalf("double credit %d %d %v", sum, count, e)
	}
	c.Lines[0].Amount++
	if _, e = s.Execute(ctx, adminID, c); e == nil {
		t.Fatal("modified replay accepted")
	}
	dup := c
	dup.Lines = []GrantLine{{userID, 100}, {userID, 100}}
	if _, e = s.Preview(ctx, adminID, dup); e == nil {
		t.Fatal("duplicate member")
	}
	var id int64
	if e = s.Pool.QueryRow(ctx, `SELECT id FROM welfare_cases WHERE account_id=$1`, userID).Scan(&id); e != nil {
		t.Fatal(e)
	}
	rev := Command{Action: "reverse", ID: id, Version: 1, RequestKey: key(2), Note: "误发核实"}
	for range 2 {
		if _, e = s.Execute(ctx, adminID, rev); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE account_id=$1`, userID).Scan(&sum); e != nil || sum != 0 {
		t.Fatalf("reverse mismatch %d %v", sum, e)
	}
	// A disabled credit service must roll back the whole source transaction.
	coins.AllowNew = false
	c.RequestKey = key(3)
	c.Lines = []GrantLine{{userID, 888}}
	q, _ = s.Preview(ctx, adminID, c)
	c.Token = q.Token
	if _, e = s.Execute(ctx, adminID, c); e == nil {
		t.Fatal("disabled source issued")
	}
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_cases`).Scan(&count); e != nil || count != 2 {
		t.Fatal("partial publication", count, e)
	}
}
func TestWelfareClaimsAndWorkflow(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	policy := Config{Enabled: true, EffectiveAt: "2020-01-01T00:00:00Z"}
	if _, e := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "srp", Config: policy, RequestKey: key(10)}); e != nil {
		t.Fatal(e)
	}
	s.Losses = func(context.Context, string, int64, int64, int64, int64) ([]Loss, error) {
		return []Loss{{ID: 987, CharacterID: 123, CorporationID: 10, ShipTypeID: 34, At: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}}, nil
	}
	apply := Command{Action: "apply", CorporationID: 10, Kind: "srp", RequestKey: key(11), Detail: Detail{SyncedLoss: true, CharacterID: 123, ShipTypeID: 34, KillmailID: 987, OccurredAt: "2026-09-01T12:00:00Z", Description: "军团集结损失", Evidence: "KM 与合同证据"}}
	data, e := s.Execute(ctx, userID, apply)
	if e != nil {
		t.Fatal(e)
	}
	var first Case
	if e = json.Unmarshal(data, &first); e != nil {
		t.Fatal(e)
	}
	apply.RequestKey = key(12)
	data, e = s.Execute(ctx, otherID, apply)
	if e != nil {
		t.Fatal(e)
	}
	var second Case
	_ = json.Unmarshal(data, &second)
	approve := Command{Action: "approve", ID: first.ID, Version: 1, RequestKey: key(13), Note: "合同、舰队与损失已核对", Detail: Detail{BaseMinor: 50000000000}}
	if _, e = s.Execute(ctx, userID, approve); e == nil {
		t.Fatal("self approval")
	}
	data, e = s.Execute(ctx, adminID, approve)
	if e != nil {
		t.Fatal(e)
	}
	var approved Case
	_ = json.Unmarshal(data, &approved)
	if approved.Award != 50000000000 {
		t.Fatal(approved.Award)
	}
	approve.ID = second.ID
	approve.RequestKey = key(14)
	if _, e = s.Execute(ctx, adminID, approve); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate KM", e)
	}
	if _, e = s.Read(ctx, otherID, first.ID); e == nil {
		t.Fatal("cross-owner read")
	}
	if _, e = s.Execute(ctx, userID, Command{Action: "cancel", ID: first.ID, Version: 2, RequestKey: key(15)}); e == nil {
		t.Fatal("approved cannot be withdrawn by member")
	}
	for i, action := range []string{"void", "execute", "complete", "link_delivery"} {
		if _, e = s.Execute(ctx, adminID, Command{Action: action, ID: first.ID, Version: 2, RequestKey: key(18 + i), Note: "手动绕过", Detail: Detail{Receipt: "文字回执"}}); !errors.Is(e, ErrDelivery) {
			t.Fatalf("%s bypassed automatic contract verification: %v", action, e)
		}
	}
	var reserved bool
	if e = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM welfare_claims WHERE claim_key='km:987')`).Scan(&reserved); e != nil || !reserved {
		t.Fatal("approval lost reservation", reserved, e)
	}
}
func TestWelfareQualificationAndMerge(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = store.SaveProfile(ctx, tx, Member{AccountID: userID, Verified: true, History: map[string]string{"growth_gila": "unused"}, Months: []string{"2026-01", "2026-03", "2026-04"}}); e != nil {
		t.Fatal(e)
	}
	d := Detail{ShipTypeID: 17715, SkillEvidence: json.RawMessage(`{"state":"met"}`)}
	g := Case{AccountID: userID, CorporationID: 10, Kind: "growth_gila", State: "approved", Detail: raw(d), Keys: []string{}}
	keys, e := s.qualify(ctx, tx, g, d)
	if e != nil || len(keys) != 1 {
		t.Fatal(keys, e)
	}
	g.Keys = keys
	g, e = store.Save(ctx, tx, g)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Claim(ctx, tx, keys[0], g.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.qualify(ctx, tx, g, d); !errors.Is(e, ErrConflict) {
		t.Fatal("once duplicate", e)
	}
	capital := Case{AccountID: userID, Kind: "capital"}
	keys, e = s.qualify(ctx, tx, capital, Detail{ContractID: 1, ShipTypeID: 2})
	if e != nil || len(keys) != 4 {
		t.Fatal(keys, e)
	}
	if e = store.SaveProfile(ctx, tx, Member{AccountID: otherID, Verified: true, History: map[string]string{"growth_gila": "used"}, Months: []string{}}); e != nil {
		t.Fatal(e)
	}
	for _, pair := range [][2]string{{userID, otherID}, {otherID, userID}} {
		_, blocked := store.Merge(ctx, tx, pair[0], pair[1], false)
		var reason interface{ MergeBlockReason() string }
		if !errors.As(blocked, &reason) || reason.MergeBlockReason() == "" {
			t.Fatal("historical claim did not block conflicting merge", pair, blocked)
		}
	}
	// Fixture-only reset after verifying both directions; normal profile commands forbid this.
	if e = store.SaveProfile(ctx, tx, Member{AccountID: otherID, Verified: true, History: map[string]string{"growth_gila": "unused"}, Months: []string{}}); e != nil {
		t.Fatal(e)
	}
	if _, e = store.Merge(ctx, tx, userID, otherID, true); e != nil {
		t.Fatal(e)
	}
	moved, e := store.Read(ctx, tx, g.ID)
	if e != nil || moved.AccountID != otherID {
		t.Fatal(moved, e)
	}
	used, e := store.Claimed(ctx, tx, "once:"+otherID+":growth_ship_17715")
	if e != nil || !used {
		t.Fatal("lost qualification", e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
