package welfare

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLossApplicationWindowBoundaries(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	boundary := now.Add(-30 * 24 * time.Hour)
	for _, tc := range []struct {
		at   time.Time
		want error
	}{{boundary, nil}, {boundary.Add(time.Second), nil}, {boundary.Add(-time.Second), ErrLossBeforeStart}, {now, nil}, {now.Add(time.Second), ErrRule}} {
		for _, zone := range []*time.Location{time.UTC, time.FixedZone("CST", 8*3600)} {
			if err := checkLossStart(Config{}, tc.at.In(zone).Format(time.RFC3339), now); !errors.Is(err, tc.want) {
				t.Fatalf("%s: %v, want %v", tc.at, err, tc.want)
			}
		}
	}
	if err := checkLossStart(Config{}, "not-a-time", now); !errors.Is(err, ErrRule) {
		t.Fatal("malformed loss", err)
	}
	for _, days := range []int64{-1, 0, 1, 30, 3650, 3651} {
		cfg := Config{LossApplicationDays: &days}
		valid := days >= 1 && days <= 3650
		for _, kind := range []string{"srp", "solo", "growth_gila"} {
			if (validateConfig(kind, cfg) == nil) != (valid && cashLoss(kind)) {
				t.Fatal("config bounds", kind, days)
			}
		}
		if valid {
			at := now.Add(-time.Duration(days) * 24 * time.Hour)
			if err := checkLossStart(cfg, at.Format(time.RFC3339), now); err != nil {
				t.Fatal("configured inclusive boundary", days, err)
			}
			if err := checkLossStart(cfg, at.Add(-time.Second).Format(time.RFC3339), now); !errors.Is(err, ErrLossBeforeStart) {
				t.Fatal("configured expiry", days, err)
			}
		} else if err := checkLossStart(cfg, now.Format(time.RFC3339), now); !errors.Is(err, ErrRule) {
			t.Fatal("invalid persisted rule", days, err)
		}
	}
	legacy := Config{LossOccurredAfter: "2099-01-01T00:00:00Z"}
	if err := checkLossStart(legacy, now.Add(-time.Hour).Format(time.RFC3339), now); err != nil {
		t.Fatal("obsolete cutoff still active", err)
	}
	effective := effectiveLossConfig(legacy)
	if effective.LossOccurredAfter != "" || effective.LossApplicationDays == nil || *effective.LossApplicationDays != 30 {
		t.Fatal("default rule", effective)
	}
}

func TestLossApplicationDefaultWindowWithoutPolicy(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	loss := Loss{ID: 9451, CharacterID: 123, CorporationID: 10, ShipTypeID: 34, At: time.Now().Add(-31 * 24 * time.Hour)}
	s.Losses = func(context.Context, string, int64, int64, int64, int64) ([]Loss, error) { return []Loss{loss}, nil }
	for i, kind := range []string{"srp", "solo"} {
		cmd := Command{Action: "apply", CorporationID: 10, Kind: kind, RequestKey: key(9451 + i), Detail: Detail{SyncedLoss: true, CharacterID: 123, KillmailID: loss.ID, Description: "loss", Evidence: "ESI", OccurredAt: time.Now().Format(time.RFC3339)}}
		if _, err := s.Execute(ctx, userID, cmd); !errors.Is(err, ErrLossBeforeStart) {
			t.Fatal("missing policy bypass", kind, err)
		}
	}
	var cases, claims int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_cases`).Scan(&cases); err != nil || cases != 0 {
		t.Fatal("expired application persisted", cases, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_claims`).Scan(&claims); err != nil || claims != 0 {
		t.Fatal("expired application reserved entitlement", claims, err)
	}
}
