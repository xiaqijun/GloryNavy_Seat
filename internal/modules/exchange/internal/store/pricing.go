package store

import (
	"context"
	"encoding/json"
	"time"
)

type RewardPricing struct {
	Automatic bool       `json:"automatic"`
	Revision  int64      `json:"-"`
	NextAt    time.Time  `json:"next_at"`
	CheckedAt *time.Time `json:"checked_at"`
	QuotedAt  *time.Time `json:"quoted_at"`
	Status    string     `json:"status"`
}
type PricingSnapshot struct {
	RewardPricing
	ID, Version int64
	Content     json.RawMessage
}

func (q *Queries) ResetRewardPricing(ctx context.Context, id int64) error {
	_, err := q.db.Exec(ctx, `INSERT INTO exchange_reward_pricing(reward_id) VALUES($1) ON CONFLICT(reward_id) DO UPDATE SET revision=exchange_reward_pricing.revision+1,next_at=now(),status=CASE WHEN exchange_reward_pricing.automatic THEN 'pending' ELSE 'manual' END`, id)
	return err
}
func (q *Queries) SetRewardPricing(ctx context.Context, id int64, automatic bool) error {
	_, err := q.db.Exec(ctx, `INSERT INTO exchange_reward_pricing(reward_id,automatic,status) VALUES($1,$2,CASE WHEN $2 THEN 'pending' ELSE 'manual' END) ON CONFLICT(reward_id) DO UPDATE SET automatic=$2,revision=exchange_reward_pricing.revision+1,next_at=now(),status=CASE WHEN $2 THEN 'pending' ELSE 'manual' END`, id, automatic)
	return err
}
func (q *Queries) RewardPrices(ctx context.Context, ids []int64) (map[int64]RewardPricing, error) {
	out := map[int64]RewardPricing{}
	rows, err := q.db.Query(ctx, `SELECT reward_id,automatic,revision,next_at,checked_at,quoted_at,status FROM exchange_reward_pricing WHERE reward_id=ANY($1::bigint[])`, ids)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p RewardPricing
		if err = rows.Scan(&id, &p.Automatic, &p.Revision, &p.NextAt, &p.CheckedAt, &p.QuotedAt, &p.Status); err != nil {
			return out, err
		}
		out[id] = p
	}
	return out, rows.Err()
}
func (q *Queries) DuePrices(ctx context.Context) ([]int64, error) {
	rows, err := q.db.Query(ctx, `SELECT p.reward_id FROM exchange_reward_pricing p JOIN exchange_rewards r ON r.id=p.reward_id WHERE p.automatic AND NOT r.archived AND p.next_at<=now() ORDER BY p.next_at,p.reward_id LIMIT 25`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (q *Queries) PriceSnapshot(ctx context.Context, id int64) (PricingSnapshot, error) {
	var p PricingSnapshot
	err := q.db.QueryRow(ctx, `SELECT r.id,r.version,r.content,p.revision FROM exchange_rewards r JOIN exchange_reward_pricing p ON p.reward_id=r.id WHERE r.id=$1 AND NOT r.archived AND p.automatic AND p.next_at<=now()`, id).Scan(&p.ID, &p.Version, &p.Content, &p.Revision)
	return p, err
}

// Caller holds the reward lock. Revision prevents a late response from overriding a manual edit.
func (q *Queries) PublishPrice(ctx context.Context, p PricingSnapshot, value int64, status string, quoted *time.Time) error {
	tag, err := q.db.Exec(ctx, `UPDATE exchange_reward_pricing SET checked_at=now(),quoted_at=CASE WHEN $3='ready' THEN $4 ELSE quoted_at END,status=$3,next_at=now()+interval '6 hours',revision=revision+1 WHERE reward_id=$1 AND revision=$2 AND automatic AND next_at<=now()`, p.ID, p.Revision, status, quoted)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if status == "ready" {
		_, err = q.db.Exec(ctx, `UPDATE exchange_rewards SET isk_value=$2,version=version+1 WHERE id=$1`, p.ID, value)
	}
	return err
}
