package store

import "context"

type RewardSettings struct {
	Amount  int64 `json:"amount_minor"`
	Version int64 `json:"version"`
}

func ReadRewardSettings(ctx context.Context, db DBTX, lock bool) (v RewardSettings, err error) {
	query := `SELECT amount_minor,version FROM feedback_reward_settings WHERE singleton`
	if lock {
		query += ` FOR UPDATE`
	}
	err = db.QueryRow(ctx, query).Scan(&v.Amount, &v.Version)
	return
}
func RewardConfigReplay(ctx context.Context, db DBTX, actor, key string) (v RewardSettings, err error) {
	err = db.QueryRow(ctx, `SELECT amount_minor,version FROM feedback_reward_config_audit WHERE actor_id=$1 AND request_key=$2`, actor, key).Scan(&v.Amount, &v.Version)
	return
}
func SaveRewardSettings(ctx context.Context, db DBTX, actor, key string, amount int64) (v RewardSettings, err error) {
	err = db.QueryRow(ctx, `UPDATE feedback_reward_settings SET amount_minor=$1,version=version+1 WHERE singleton RETURNING amount_minor,version`, amount).Scan(&v.Amount, &v.Version)
	if err == nil {
		_, err = db.Exec(ctx, `INSERT INTO feedback_reward_config_audit(actor_id,request_key,amount_minor,version) VALUES($1,$2,$3,$4)`, actor, key, v.Amount, v.Version)
	}
	return
}
func AdoptReward(ctx context.Context, db DBTX, id int64, actor string, amount int64) error {
	_, err := db.Exec(ctx, `UPDATE feedback_tickets SET accepted_at=now(),accepted_by=$2,reward_minor=$3 WHERE id=$1 AND accepted_at IS NULL`, id, actor, amount)
	return err
}
