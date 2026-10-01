// Package access owns site roles and corporation-scoped authorization.
package access

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Grant struct {
	Permission string `json:"permission"`
	// An empty filter is unrestricted, matching SeAT 4+. Never infer a default corporation.
	Corporations EntityIDs `json:"corporations"`
	Alliances    EntityIDs `json:"alliances"`
}

// Game IDs remain decimal strings at API and JSON storage boundaries.
type EntityIDs []int64

func (ids EntityIDs) MarshalJSON() ([]byte, error) {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, strconv.FormatInt(id, 10))
	}
	return json.Marshal(values)
}
func (ids *EntityIDs) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	result := make(EntityIDs, 0, len(values))
	for _, v := range values {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != v {
			return fmt.Errorf("invalid entity ID")
		}
		result = append(result, id)
	}
	*ids = result
	return nil
}

type Corporation struct {
	ID         int64  `json:"id,string"`
	Name       string `json:"name"`
	AllianceID int64  `json:"alliance_id,string"`
	CEOID      int64  `json:"ceo_id,string"`
}
type Fact struct {
	NeedsAuthorization bool        `json:"needs_authorization"`
	CharacterID        int64       `json:"character_id,string"`
	State              string      `json:"state"`
	Corporation        Corporation `json:"corporation"`
	Roles              []string    `json:"roles"`
	RolesAtHQ          []string    `json:"roles_at_hq"`
	RolesAtBase        []string    `json:"roles_at_base"`
	RolesAtOther       []string    `json:"roles_at_other"`
	SyncedAt           time.Time   `json:"synced_at"`
	ValidUntil         time.Time   `json:"valid_until"`
}
type Permission struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Scope string `json:"scope"`
}

// ManageableCatalog contains only delivered, user-facing features.
// Add business abilities here when their corresponding feature is shipped.
// Internal SeAT mappings and stored historical grants use Catalog independently.
func ManageableCatalog() []Permission {
	result := []Permission{{"corporation.welfare", "福利审核与交付", "corporation"}, {"access.manage", "权限管理", "global"}, {"eve.sync.manage", "ESI 同步管理", "global"}, {"corporation.contract", "军团合同", "corporation"}, {"corporation.attendance", "军团考勤与集结分", "corporation"}, {"corporation.skills", "军团技能要求", "corporation"}, {"corporation.journal", "钱包流水", "corporation"}, {"corporation.transaction", "钱包市场交易", "corporation"}}
	for i, n := range divisions {
		result = append(result, Permission{"corporation.wallet_" + n + "_division", fmt.Sprintf("钱包分部 %d", i+1), "corporation"})
	}
	return result
}

// SeAT-compatible ability names are kept stable for future business modules.
// Catalog entries authorize operations; they do not imply those data modules exist yet.
func Catalog() []Permission {
	result := []Permission{{"access.self", "本人权限", "self"}, {"access.manage", "权限管理", "global"}, {"eve.sync.manage", "ESI 同步管理", "global"}}
	result = append(result, Permission{"access.members.read", "成员数据", "administrator"})
	names := [][2]string{{"welfare", "福利审核与交付"}, {"skills", "军团技能要求"}, {"attendance", "军团考勤"}, {"summary", "军团概览"}, {"asset", "资产"}, {"customs_office", "海关"}, {"starbase", "母星基地"}, {"structure", "建筑"}, {"mining", "采矿"}, {"extraction", "卫星开采"}, {"industry", "工业"}, {"blueprint", "蓝图"}, {"contract", "合同"}, {"market", "市场"}, {"ledger", "采矿账本"}, {"journal", "钱包流水"}, {"transaction", "交易明细"}, {"contact", "联系人"}, {"standing", "声望"}, {"killmail", "击毁报告"}, {"security", "安全审查"}, {"tracking", "成员追踪"}, {"projects", "军团项目"}}
	for _, n := range names {
		result = append(result, Permission{"corporation." + n[0], n[1], "corporation"})
	}
	for i, n := range divisions {
		result = append(result, Permission{"corporation.wallet_" + n + "_division", fmt.Sprintf("钱包分部 %d", i+1), "corporation"}, Permission{"corporation.asset_" + n + "_division", fmt.Sprintf("资产分部 %d", i+1), "corporation"})
	}
	return result
}

var divisions = []string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh"}

func known(permission string) bool {
	for _, p := range Catalog() {
		if p.ID == permission {
			return true
		}
	}
	return false
}
func mapped(role, permission string) bool {
	var abilities []string
	switch role {
	case "Accountant":
		abilities = []string{"summary", "journal", "transaction"}
	case "Auditor", "Junior_Accountant":
		abilities = []string{"summary"}
	case "Contract_Manager":
		abilities = []string{"summary", "contract"}
	case "Diplomat":
		abilities = []string{"summary", "tracking"}
	case "Security_Officer":
		abilities = []string{"summary", "security"}
	case "Trader":
		abilities = []string{"summary", "market"}
	case "Project_Manager":
		abilities = []string{"projects"}
	}
	for _, a := range abilities {
		if permission == "corporation."+a {
			return true
		}
	}
	for i, n := range divisions {
		if role == fmt.Sprintf("Account_Take_%d", i+1) && permission == "corporation.wallet_"+n+"_division" {
			return true
		}
		if role == fmt.Sprintf("Container_Take_%d", i+1) && permission == "corporation.asset_"+n+"_division" {
			return true
		}
	}
	return false
}
func Fresh(f Fact, now time.Time) bool {
	return (f.State == "ready" || f.State == "retry") && !f.SyncedAt.IsZero() && !f.SyncedAt.After(now) && f.ValidUntil.After(now)
}

// Evaluate never treats location-specific or grantable roles as global roles.
// target must be resolved server-side; alliance IDs from a request are not trusted.
func Evaluate(admin bool, facts []Fact, grants []Grant, permission string, target Corporation, now time.Time) bool {
	if !known(permission) {
		return false
	}
	corp := strings.HasPrefix(permission, "corporation.")
	if corp && target.ID <= 0 {
		return false
	}
	if admin || permission == "access.self" {
		return true
	}
	if permission == "access.members.read" {
		return false
	}
	if corp && permission != "corporation.welfare" {
		for _, f := range facts {
			if !Fresh(f, now) || f.Corporation.ID != target.ID {
				continue
			}
			if f.CharacterID == target.CEOID || slices.Contains(f.Roles, "Director") {
				return true
			}
			for _, r := range f.Roles {
				if mapped(r, permission) {
					return true
				}
			}
		}
	}
	for _, g := range grants {
		if g.Permission != permission {
			continue
		}
		if len(g.Corporations) == 0 && len(g.Alliances) == 0 {
			return true
		}
		if corp && (slices.Contains(g.Corporations, target.ID) || (target.AllianceID > 0 && slices.Contains(g.Alliances, target.AllianceID))) {
			return true
		}
	}
	return false
}
func ValidateGrants(grants []Grant) error {
	if len(grants) > 100 {
		return fmt.Errorf("too many grants")
	}
	seen := map[string]bool{}
	for _, g := range grants {
		if !known(g.Permission) || g.Permission == "access.self" || g.Permission == "access.members.read" || seen[g.Permission] {
			return fmt.Errorf("invalid or duplicate permission")
		}
		seen[g.Permission] = true
		if !strings.HasPrefix(g.Permission, "corporation.") && (len(g.Corporations) > 0 || len(g.Alliances) > 0) {
			return fmt.Errorf("global permission cannot have entity filters")
		}
		if len(g.Corporations)+len(g.Alliances) > 100 {
			return fmt.Errorf("too many filters")
		}
		for _, ids := range [][]int64{g.Corporations, g.Alliances} {
			for _, id := range ids {
				if id <= 0 {
					return fmt.Errorf("invalid entity ID")
				}
			}
		}
	}
	return nil
}
