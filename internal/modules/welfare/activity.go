package welfare

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

// Activity projects are welfare policies. The sequence gives each project a
// stable identity even when an administrator disables or edits its reward.
func activityProjectID(kind string) int64 {
	value, ok := strings.CutPrefix(kind, "activity_")
	if !ok {
		return 0
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0
	}
	return id
}

type ActivityImage struct {
	MIME    string
	Content []byte
}

type ActivityProjectInput struct {
	CorporationID int64  `json:"corporation_id,string"`
	ProjectID     int64  `json:"project_id,string"`
	Version       int64  `json:"version,string"`
	RequestKey    string `json:"request_key"`
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	RewardID      int64  `json:"reward_id,string"`
	RewardVersion int64  `json:"reward_version,string"`
	CoinsMinor    int64  `json:"coins_minor"`
	ClaimLimit    int64  `json:"claim_limit"`
}

func (s *Service) SaveActivityProject(ctx context.Context, actor string, input ActivityProjectInput) (Policy, error) {
	if !validUUID(actor) || !validUUID(input.RequestKey) || input.CorporationID <= 0 || !validText(input.Name, 100) || input.ProjectID < 0 || input.Version < 0 || input.RewardID < 0 || input.RewardVersion < 0 || input.CoinsMinor < 0 || input.CoinsMinor > 1000000000000 || input.ClaimLimit < 0 || input.ClaimLimit > 1000 || (input.RewardID == 0) != (input.RewardVersion == 0) || (input.Enabled || input.ProjectID == 0) && input.RewardID == 0 && input.CoinsMinor == 0 {
		return Policy{}, ErrInvalid
	}
	if err := s.admin(ctx, actor); err != nil {
		return Policy{}, err
	}
	if err := s.allowed(ctx, actor, input.CorporationID, true); err != nil {
		return Policy{}, err
	}
	fingerprint := hash(input)
	if replayFP, replay, replayErr := store.Replay(ctx, s.Pool, actor, input.RequestKey); replayErr == nil {
		if replayFP != fingerprint {
			return Policy{}, ErrConflict
		}
		var p Policy
		if err := json.Unmarshal(replay, &p); err != nil {
			return Policy{}, err
		}
		return p, nil
	} else if replayErr != pgx.ErrNoRows {
		return Policy{}, replayErr
	}
	rewards := &GrowthRewards{Fittings: []GrowthFitting{}, Items: []GrowthItem{}, Coins: input.CoinsMinor}
	if !input.Enabled && input.ProjectID > 0 {
		previous, err := store.ActivityProject(ctx, s.Pool, input.CorporationID, fmt.Sprintf("activity_%d", input.ProjectID))
		if err != nil {
			return Policy{}, err
		}
		var old Config
		if err = json.Unmarshal(previous.Config, &old); err != nil {
			return Policy{}, err
		}
		rewards, input.RewardID, input.RewardVersion = old.Rewards, old.RewardID, old.RewardVersion
	} else if input.RewardID > 0 {
		if s.LibraryReward == nil {
			return Policy{}, ErrRule
		}
		resolved, err := s.LibraryReward(ctx, actor, input.CorporationID, input.RewardID, input.RewardVersion)
		if err != nil {
			return Policy{}, err
		}
		rewards = resolved
		rewards.Coins = input.CoinsMinor
	}
	if err := validateGrowthRewards(rewards); err != nil {
		return Policy{}, err
	}
	cfg := Config{ProjectName: strings.TrimSpace(input.Name), RewardID: input.RewardID, RewardVersion: input.RewardVersion, Rewards: rewards, Enabled: input.Enabled}
	if input.ClaimLimit > 0 {
		cfg.ActivityClaimLimit = &input.ClaimLimit
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(context.Background())
	if err = s.LockAccounts(ctx, tx, []string{actor}); err != nil {
		return Policy{}, err
	}
	if err = store.Lock(ctx, tx); err != nil {
		return Policy{}, err
	}
	if err = s.admin(ctx, actor); err != nil {
		return Policy{}, err
	}
	if err = s.allowed(ctx, actor, input.CorporationID, true); err != nil {
		return Policy{}, err
	}
	replayFP, replay, err := store.Replay(ctx, tx, actor, input.RequestKey)
	if err == nil {
		if replayFP != fingerprint {
			return Policy{}, ErrConflict
		}
		var p Policy
		if err = json.Unmarshal(replay, &p); err != nil {
			return Policy{}, err
		}
		return p, nil
	}
	if err != pgx.ErrNoRows {
		return Policy{}, err
	}
	id := input.ProjectID
	if id == 0 {
		if input.Version != 0 {
			return Policy{}, ErrInvalid
		}
		id, err = store.NextActivityProject(ctx, tx)
		if err != nil {
			return Policy{}, err
		}
	} else if _, err = store.ActivityProject(ctx, tx, input.CorporationID, fmt.Sprintf("activity_%d", id)); err != nil {
		return Policy{}, err
	}
	p, err := store.SavePolicy(ctx, tx, Policy{CorporationID: input.CorporationID, Kind: fmt.Sprintf("activity_%d", id), Version: input.Version, Config: raw(cfg)})
	if err != nil {
		return Policy{}, normalize(err)
	}
	if err = store.Audit(ctx, tx, actor, input.RequestKey, fingerprint, "activity_project", cfg.ProjectName, 0, p); err != nil {
		return Policy{}, normalize(err)
	}
	return p, tx.Commit(ctx)
}

func (s *Service) ActivityImage(ctx context.Context, actor string, caseID int64, ordinal int) (string, []byte, error) {
	if caseID <= 0 || ordinal < 1 || ordinal > 3 {
		return "", nil, ErrInvalid
	}
	c, err := s.Read(ctx, actor, caseID)
	if err != nil {
		return "", nil, err
	}
	if !isActivity(c.Kind) {
		return "", nil, pgx.ErrNoRows
	}
	return store.ActivityImage(ctx, s.Pool, caseID, ordinal)
}

func validateActivityImages(images []ActivityImage) error {
	if len(images) < 1 || len(images) > 3 {
		return ErrInvalid
	}
	for _, image := range images {
		if len(image.Content) < 1 || len(image.Content) > 2<<20 || image.MIME != http.DetectContentType(image.Content) || image.MIME != "image/jpeg" && image.MIME != "image/png" && image.MIME != "image/webp" {
			return ErrInvalid
		}
	}
	return nil
}

func activityFingerprint(c Command) string {
	hashes := make([][32]byte, 0, len(c.Images))
	for _, image := range c.Images {
		hashes = append(hashes, sha256.Sum256(image.Content))
	}
	return hash(struct {
		Command Command
		Images  [][32]byte
	}{c, hashes})
}
