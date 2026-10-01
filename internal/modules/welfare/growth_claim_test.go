package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"sync"
	"testing"
	"time"
)

func TestGrowthAllowanceSharedByCharactersAndConcurrentApplications(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.Characters = func(_ context.Context, actor string) ([]Character, error) {
		return []Character{{ID: 123, AccountID: actor, CorporationID: 10}, {ID: 456, AccountID: actor, CorporationID: 10}}, nil
	}
	s.GrowthFitting = func(_ context.Context, _ string, corp, id int64) (GrowthFitting, error) {
		return GrowthFitting{ID: id, Name: "Training", ShipTypeID: 587, Version: 1, Fit: json.RawMessage(`{"ship_type_id":"587","items":[]}`)}, nil
	}
	for i, kind := range []string{"growth_fitting_1", "growth_fitting_2"} {
		_, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: kind, RequestKey: key(950 + i), Config: Config{Enabled: true, EffectiveAt: time.Now().Add(-time.Hour).Format(time.RFC3339), FittingID: int64(i + 1), Rewards: &GrowthRewards{Coins: 100}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	commands := []Command{
		{Action: "apply", CorporationID: 10, Kind: "growth_fitting_1", RequestKey: key(952), Detail: Detail{CharacterID: 123}},
		{Action: "apply", CorporationID: 10, Kind: "growth_fitting_2", RequestKey: key(953), Detail: Detail{CharacterID: 456}},
	}
	results := make([]json.RawMessage, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range commands {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = s.Execute(ctx, userID, commands[i]) }(i)
	}
	wg.Wait()
	winner := -1
	for i, err := range errs {
		if err == nil {
			if winner != -1 {
				t.Fatal("both characters submitted")
			}
			winner = i
		} else if !errors.Is(err, ErrGrowthClaimed) {
			t.Fatal(err)
		}
	}
	if winner == -1 {
		t.Fatal("no application accepted")
	}
	var first Case
	json.Unmarshal(results[winner], &first)
	replayed, err := s.Execute(ctx, userID, commands[winner])
	var replay Case
	if err != nil || json.Unmarshal(replayed, &replay) != nil || replay.ID != first.ID || replay.Version != first.Version {
		t.Fatal("idempotent retry failed", err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_cases WHERE account_id=$1 AND kind LIKE 'growth_%'`, userID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate rows", count, err)
	}
	// A different project for the same hull shares the allowance.
	if _, err = s.Execute(ctx, userID, Command{Action: "apply", CorporationID: 10, Kind: "growth_fitting_2", RequestKey: key(954), Detail: Detail{CharacterID: 456}}); !errors.Is(err, ErrGrowthClaimed) {
		t.Fatal("same hull duplicate accepted", err)
	}
	// Withdrawal before delivery allows a fresh application with another character.
	if _, err = s.Execute(ctx, userID, Command{Action: "cancel", ID: first.ID, Version: first.Version, RequestKey: key(955)}); err != nil {
		t.Fatal(err)
	}
	commands[1-winner].RequestKey = key(956)
	if _, err = s.Execute(ctx, userID, commands[1-winner]); err != nil {
		t.Fatal("withdrawal did not release application", err)
	}
	// Imported/manual historical claims also block first-time system applications.
	if _, err = s.Execute(ctx, adminID, Command{Action: "profile", CorporationID: 10, AccountID: otherID, RequestKey: key(957), Note: "核实历史", Profile: Member{Verified: true, History: map[string]string{"growth_fitting_1": "used"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(ctx, otherID, Command{Action: "apply", CorporationID: 10, Kind: "growth_fitting_1", RequestKey: key(958), Detail: Detail{CharacterID: 123}}); !errors.Is(err, ErrGrowthClaimed) {
		t.Fatal("historical claim bypass", err)
	}
}

func TestGrowthAutomaticQualificationAndHullHistory(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	state := "missing"
	stale := false
	s.GrowthCheck = func(context.Context, string, int64, int64, Config) (json.RawMessage, json.RawMessage, error) {
		return raw(map[string]string{"version": "1"}), raw(map[string]any{"state": state, "remaining_sp": 100}), nil
	}
	s.GuardGrowth = func(context.Context, pgx.Tx, int64, Config, json.RawMessage, json.RawMessage) error {
		if stale {
			return ErrConflict
		}
		return nil
	}
	s.GrowthFitting = func(_ context.Context, _ string, corp, id int64) (GrowthFitting, error) {
		ship := int64(587)
		if id == 3 {
			ship = 588
		}
		return GrowthFitting{ID: id, Name: "Training", ShipTypeID: ship, Version: 1}, nil
	}
	configure := func(id, n int) {
		t.Helper()
		_, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: fmt.Sprintf("growth_fitting_%d", id), RequestKey: key(n), Config: Config{Enabled: true, EffectiveAt: "2020-01-01T00:00:00Z", FittingID: int64(id), Rewards: &GrowthRewards{Coins: 100}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	configure(1, 1100)
	apply := Command{Action: "apply", CorporationID: 10, Kind: "growth_fitting_1", RequestKey: key(1101), Detail: Detail{CharacterID: 123}}
	for _, v := range []string{"missing", "unknown"} {
		state = v
		if _, err := s.Execute(ctx, userID, apply); !errors.Is(err, ErrGrowthUnmet) {
			t.Fatal("unqualified applied", v, err)
		}
	}
	status, err := s.GrowthStatus(ctx, userID, 10, 123, apply.Kind)
	if err != nil || status.State != "unknown" {
		t.Fatal(status, err)
	}
	if _, err = s.GrowthStatus(ctx, userID, 10, 999, apply.Kind); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign character", err)
	}
	state = "met"
	stale = true
	if _, err = s.Execute(ctx, userID, apply); !errors.Is(err, ErrConflict) {
		t.Fatal("changed fitting accepted", err)
	}
	stale = false
	data, err := s.Execute(ctx, userID, apply)
	if err != nil {
		t.Fatal(err)
	}
	var v Case
	json.Unmarshal(data, &v)
	for i, action := range []string{"approve"} {
		data, err = s.Execute(ctx, adminID, Command{Action: action, ID: v.ID, Version: v.Version, RequestKey: key(1102 + i), Note: "核对", Detail: Detail{Receipt: "游戏内已交付"}})
		if err != nil {
			t.Fatal(action, err)
		}
		json.Unmarshal(data, &v)
	}
	// Removing the project and recreating a different fitting/skill plan cannot erase the case's original hull.
	if _, err = s.Pool.Exec(ctx, "DELETE FROM welfare_policies WHERE kind='growth_fitting_1'"); err != nil {
		t.Fatal(err)
	}
	configure(2, 1110)
	apply.Kind = "growth_fitting_2"
	apply.RequestKey = key(1111)
	status, err = s.GrowthStatus(ctx, userID, 10, 123, apply.Kind)
	if err != nil || status.State != "claimed" {
		t.Fatal("lost receipt", status, err)
	}
	if _, err = s.Execute(ctx, userID, apply); !errors.Is(err, ErrGrowthClaimed) {
		t.Fatal("recreated hull allowed", err)
	}
	configure(3, 1112)
	apply.Kind = "growth_fitting_3"
	apply.RequestKey = key(1113)
	if _, err = s.Execute(ctx, userID, apply); err != nil {
		t.Fatal("different hull blocked", err)
	}
	// Audit snapshots retain manual legacy history even if its policy is removed.
	if _, err = s.Execute(ctx, adminID, Command{Action: "profile", CorporationID: 10, AccountID: otherID, RequestKey: key(1114), Note: "历史已领取", Profile: Member{History: map[string]string{"growth_fitting_1": "used"}}}); err != nil {
		t.Fatal(err)
	}
	apply.Kind = "growth_fitting_2"
	apply.RequestKey = key(1115)
	if _, err = s.Execute(ctx, otherID, apply); !errors.Is(err, ErrGrowthClaimed) {
		t.Fatal("manual history lost after deletion", err)
	}
	// Cross-project hull holdings participate in account merge fencing.
	apply.Kind = "growth_fitting_3"
	apply.RequestKey = key(1116)
	data, err = s.Execute(ctx, otherID, apply)
	if err != nil {
		t.Fatal(err)
	}
	var other Case
	json.Unmarshal(data, &other)
	for _, pair := range [][2]string{{userID, otherID}, {otherID, userID}} {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Merge(ctx, tx, pair[0], pair[1], false)
		tx.Rollback(ctx)
		var conflict interface{ MergeBlockReason() string }
		if !errors.As(err, &conflict) {
			t.Fatal("same hull merge conflict lost", err)
		}
	}
	if _, err = s.Execute(ctx, otherID, Command{Action: "cancel", ID: other.ID, Version: other.Version, RequestKey: key(1117)}); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = store.Merge(ctx, tx, userID, otherID, true); err != nil {
		t.Fatal(err)
	}
	if state, err := store.GrowthState(ctx, tx, otherID, "growth_fitting_2", 587, 0); err != nil || state != "claimed" {
		t.Fatal("merged hull receipt lost", state, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	legacy, err := store.Save(ctx, s.Pool, Case{AccountID: otherID, CorporationID: 10, Kind: "growth_fitting_2", State: "approved", Detail: raw(Detail{CharacterID: 123, ShipTypeID: 587, Rewards: &GrowthRewards{Coins: 100}}), Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(ctx, adminID, Command{Action: "execute", ID: legacy.ID, Version: legacy.Version, RequestKey: key(1120), Note: "旧单发放"}); !errors.Is(err, ErrGrowthClaimed) {
		t.Fatal("legacy approved bypass", err)
	}
	legacy.State = "executing"
	legacy, err = store.Save(ctx, s.Pool, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(ctx, adminID, Command{Action: "complete", ID: legacy.ID, Version: legacy.Version, RequestKey: key(1121), Note: "旧单确认", Detail: Detail{Receipt: "回执"}}); !errors.Is(err, ErrGrowthClaimed) {
		t.Fatal("legacy delivery bypass", err)
	}

}
