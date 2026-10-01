package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestWelfareAllianceNewApplicationsPaused(t *testing.T) {
	s := &Service{}
	for _, actor := range []string{userID, adminID} {
		for _, synced := range []bool{false, true} {
			_, err := s.Execute(context.Background(), actor, Command{Action: "apply", Kind: "alliance", CorporationID: 10, RequestKey: key(450), Detail: Detail{SyncedLoss: synced}})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("alliance application accepted: %v", err)
			}
		}
	}
}

func TestWelfareManualReviewWithoutRulesOrVerification(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	// The loss predates the current corporation and no policy/profile exists.
	s.Losses = func(_ context.Context, _ string, _ int64, char, _, id int64) ([]Loss, error) {
		return []Loss{{ID: id, CharacterID: char, CorporationID: 20, ShipTypeID: 17715, At: time.Date(2019, 1, 1, 1, 0, 0, 0, time.UTC)}}, nil
	}
	for i, kind := range []string{"srp", "solo", "solo"} {
		c := Command{Action: "apply", CorporationID: 10, Kind: kind, RequestKey: key(400 + i), Detail: Detail{CharacterID: 123, KillmailID: int64(800 + i), SyncedLoss: true, Description: "交人工审核", Evidence: "ESI 证据", Alliance: "unknown"}}
		b, err := s.Execute(ctx, userID, c)
		if err != nil {
			t.Fatalf("%s apply: %v", kind, err)
		}
		var v Case
		if err = json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		approval := Command{Action: "approve", ID: v.ID, Version: 1, RequestKey: key(410 + i), Note: "人工核定实际金额", ManualPricing: true, Detail: Detail{BaseMinor: -1}}
		if _, err = s.Execute(ctx, adminID, approval); err == nil {
			t.Fatal("negative amount accepted")
		}
		approval.Detail.BaseMinor = 12345
		b, err = s.Execute(ctx, adminID, approval)
		if err != nil {
			t.Fatalf("%s approve: %v", kind, err)
		}
		if err = json.Unmarshal(b, &v); err != nil || v.Award != 12345 || len(v.Keys) != 1 {
			t.Fatalf("manual amount or daily gate: %+v %v", v, err)
		}
	}
	s.Characters = func(context.Context, string) ([]Character, error) { return []Character{}, nil }
	if _, err := s.Execute(ctx, userID, Command{Action: "apply", CorporationID: 10, Kind: "solo", RequestKey: key(430), Detail: Detail{CharacterID: 123, SyncedLoss: true}}); err == nil {
		t.Fatal("unbound character accepted")
	}
}
