package welfare

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid welfare request")
var ErrGrowthUnmet = errors.New("growth skill requirements not met")
var ErrGrowthClaimed = errors.New("growth project already applied for or claimed")
var ErrActivityClaimed = errors.New("activity project claim limit reached")
var ErrConflict = errors.New("welfare record changed or entitlement occupied")
var ErrRule = errors.New("welfare policy or history needs confirmation")
var ErrAttendanceRequired = errors.New("confirmed attendance loss required")
var ErrLossQuota = errors.New("member reimbursement period cap exceeded")
var ErrLossDailyQuota = fmt.Errorf("daily: %w", ErrLossQuota)
var ErrLossWeeklyQuota = fmt.Errorf("weekly: %w", ErrLossQuota)
var ErrLossMonthlyQuota = fmt.Errorf("monthly: %w", ErrLossQuota)

type Case = store.Case
type Policy = store.Policy
type Member = store.Member
type Loss = eve.CharacterLoss
type Character struct {
	ID                int64  `json:"id,string"`
	Name              string `json:"name"`
	AccountID         string `json:"account_id"`
	MainCharacterName string `json:"main_character_name,omitempty"`
	CorporationID     int64  `json:"corporation_id,string"`
}
type Ship struct {
	ID   int64  `json:"id,string"`
	Name string `json:"name"`
}
type Corporation struct {
	ID     int64  `json:"id,string"`
	Name   string `json:"name"`
	Manage bool   `json:"can_manage"`
}
type LossQuotaPeriod struct {
	UsedMinor      int64  `json:"used_minor"`
	ReservedMinor  int64  `json:"reserved_minor"`
	CapMinor       *int64 `json:"cap_minor,omitempty"`
	RemainingMinor *int64 `json:"remaining_minor,omitempty"`
}
type LossQuota struct {
	Kind    string          `json:"kind"`
	Daily   LossQuotaPeriod `json:"daily"`
	Weekly  LossQuotaPeriod `json:"weekly"`
	Monthly LossQuotaPeriod `json:"monthly"`
}
type Config struct {
	LossRateBPS         *int64         `json:"loss_rate_bps,omitempty"`
	LossCapMinor        *int64         `json:"loss_cap_minor,omitempty"`
	LossDailyCapMinor   *int64         `json:"loss_daily_cap_minor,omitempty"`
	LossWeeklyCapMinor  *int64         `json:"loss_weekly_cap_minor,omitempty"`
	LossMonthlyCapMinor *int64         `json:"loss_monthly_cap_minor,omitempty"`
	SubsidyRateBPS      *int64         `json:"subsidy_rate_bps,omitempty"`
	RewardID            int64          `json:"reward_id,string,omitempty"`
	RewardVersion       int64          `json:"reward_version,string,omitempty"`
	ProjectName         string         `json:"project_name,omitempty"`
	ActivityClaimLimit  *int64         `json:"claim_limit,omitempty"`
	Rewards             *GrowthRewards `json:"rewards,omitempty"`
	Enabled             bool           `json:"enabled"`
	EffectiveAt         string         `json:"effective_at"`
	ShipTypeID          int64          `json:"ship_type_id,string"`
	FittingID           int64          `json:"fitting_id,string"`
	SkillPlanID         int64          `json:"skill_plan_id,string"`
	ReferenceMinor      int64          `json:"reference_minor"`
	DayZone             string         `json:"day_zone"`
	Note                string         `json:"note"`
}
type Detail struct {
	ImageCount       int                   `json:"image_count,omitempty"`
	Cancellation     *Cancellation         `json:"cancellation,omitempty"`
	PaymentStatus    string                `json:"payment_status,omitempty"`
	Purchase         *eve.DeliveryContract `json:"purchase,omitempty"`
	Rewards          *GrowthRewards        `json:"rewards,omitempty"`
	Valuation        *Valuation            `json:"valuation,omitempty"`
	PricingMode      string                `json:"pricing_mode,omitempty"`
	Delivery         *Delivery             `json:"delivery,omitempty"`
	SyncedLoss       bool                  `json:"synced_loss,omitempty"`
	LossEvidence     *Loss                 `json:"loss_evidence,omitempty"`
	CharacterID      int64                 `json:"character_id,string"`
	CharacterName    string                `json:"character_name"`
	ShipTypeID       int64                 `json:"ship_type_id,string"`
	KillmailID       int64                 `json:"killmail_id,string"`
	ContractID       int64                 `json:"contract_id,string"`
	SupportingContractID int64             `json:"supporting_contract_id,string,omitempty"`
	EventID          int64                 `json:"event_id,string"`
	OccurredAt       string                `json:"occurred_at"`
	Description      string                `json:"description"`
	Evidence         string                `json:"evidence"`
	Alliance         string                `json:"alliance"`
	BaseMinor        int64                 `json:"base_minor"`
	Discipline       bool                  `json:"discipline"`
	PolicyVersion    int64                 `json:"policy_version,string"`
	Rule             Config                `json:"rule"`
	SkillEvidence    json.RawMessage       `json:"skill_evidence,omitempty"`
	FittingEvidence  json.RawMessage       `json:"fitting_evidence,omitempty"`
	ActivityBatchKey string                `json:"activity_batch_key,omitempty"`
	Receipt          string                `json:"receipt"`
	Reviewer         string                `json:"reviewer"`
	Executor         string                `json:"executor"`
	ReviewNote       string                `json:"review_note"`
}
type GrantLine struct {
	AccountID string `json:"account_id"`
	Amount    int64  `json:"amount_minor"`
}
type Command struct {
	Images                []ActivityImage   `json:"-"`
	ConfirmedNotDelivered bool              `json:"confirmed_not_delivered,omitempty"`
	LossPolicyVersion     *int64            `json:"loss_policy_version,string,omitempty"`
	ManualPricing         bool              `json:"manual_pricing,omitempty"`
	Delivery              DeliverySelection `json:"delivery"`
	Action                string            `json:"action"`
	RequestKey            string            `json:"request_key"`
	ID                    int64             `json:"id,string"`
	CorporationID         int64             `json:"corporation_id,string"`
	Kind                  string            `json:"kind"`
	Version               int64             `json:"version,string"`
	AccountID             string            `json:"account_id"`
	Note                  string            `json:"note"`
	Token                 string            `json:"token"`
	Config                Config            `json:"config"`
	Profile               Member            `json:"profile"`
	Detail                Detail            `json:"detail"`
	Lines                 []GrantLine       `json:"lines"`
}

var kinds = []string{"srp", "alliance", "solo", "growth_gila", "growth_ishtar", "growth_loki", "growth_absolution", "capital", "supercarrier", "titan", "grant"}

func isGrowth(k string) bool   { return strings.HasPrefix(k, "growth_") }
func isActivity(k string) bool { return activityProjectID(k) > 0 }
func isLoss(k string) bool     { return k == "srp" || k == "alliance" || k == "solo" }
func validUUID(v string) bool {
	var id pgtype.UUID
	return id.Scan(v) == nil && id.Valid && strings.ToLower(v) == id.String()
}
func raw(v any) json.RawMessage        { b, _ := json.Marshal(v); return b }
func validText(v string, max int) bool { return strings.TrimSpace(v) != "" && len([]rune(v)) <= max }
func Calculate(kind string, base int64, discipline bool) (int64, error) {
	if base <= 0 || base > 100000000000000 {
		return 0, ErrInvalid
	}
	rate := int64(0)
	switch kind {
	case "srp", "alliance":
		rate = 80
		if discipline {
			rate = 40
		}
	case "solo":
		rate = 50
	case "capital", "supercarrier":
		rate = 10
	case "titan":
		rate = 5
	default:
		return 0, ErrInvalid
	}
	amount := base/100*rate + (base%100*rate)/100
	if kind == "solo" {
		amount = min(amount, 20000000000)
	}
	return amount, nil
}
func validateConfig(k string, c Config) error {
	if c.LossRateBPS != nil && (!cashLoss(k) || *c.LossRateBPS < 1 || *c.LossRateBPS > 10000) {
		return ErrInvalid
	}
	if c.LossCapMinor != nil && (!cashLoss(k) || *c.LossCapMinor < 0 || *c.LossCapMinor > 100000000000000) {
		return ErrInvalid
	}
	for _, cap := range []*int64{c.LossDailyCapMinor, c.LossWeeklyCapMinor, c.LossMonthlyCapMinor} {
		if cap != nil && (!cashLoss(k) || *cap < 0 || (*cap > 0 && *cap < 100) || *cap > 100000000000000) {
			return ErrInvalid
		}
	}
	if c.SubsidyRateBPS != nil && (!isSuper(k) || *c.SubsidyRateBPS < 1 || *c.SubsidyRateBPS > 10000) {
		return ErrInvalid
	}
	if len([]rune(c.Note)) > 1000 || c.ReferenceMinor < 0 || c.ReferenceMinor > 100000000000000 {
		return ErrInvalid
	}
	if cashLoss(k) {
		return nil
	}
	if !c.Enabled || isSuper(k) {
		return nil
	}
	if _, e := time.Parse(time.RFC3339, c.EffectiveAt); e != nil {
		return ErrInvalid
	}
	if k == "solo" {
		if c.DayZone != "UTC" && c.DayZone != "Asia/Shanghai" {
			return ErrRule
		}
	}
	if growthFittingID(k) > 0 {
		if c.FittingID != growthFittingID(k) || c.ShipTypeID <= 0 || c.SkillPlanID < 0 {
			return ErrRule
		}
		return validateGrowthRewards(c.Rewards)
	}
	if isGrowth(k) && (c.ShipTypeID <= 0 || c.FittingID <= 0 || c.SkillPlanID <= 0) {
		return ErrRule
	}
	return nil
}
