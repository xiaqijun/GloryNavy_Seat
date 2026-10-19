package exchange

import "encoding/json"

// Keep API money exactly representable by the browser as integer minor units.
const maxRewardTotal int64 = 9007199254740991
const maxRewardUnits int32 = 1000000

// Catalog limits apply to one pack. An immutable order can contain many packs;
// its expanded item totals must still fit the contract matcher.
func physicalSnapshotValid(r PhysicalReward) bool {
	if len(r.Fittings) > 10 || len(r.Items) > 30 || r.ISKMinor < 0 || r.ISKMinor > maxRewardTotal || (r.ISKMinor > 0 && r.ISKMinor < 100) || (len(r.Fittings)+len(r.Items) == 0 && r.ISKMinor == 0) {
		return false
	}
	seen := map[int64]bool{}
	for _, f := range r.Fittings {
		if f.ID < 1 || f.Quantity < 1 || f.Quantity > 1000000000 || seen[f.ID] {
			return false
		}
		seen[f.ID] = true
	}
	seen = map[int64]bool{}
	for _, i := range r.Items {
		if i.ID < 1 || i.Quantity < 1 || i.Quantity > 1000000000 || seen[i.ID] {
			return false
		}
		seen[i.ID] = true
	}
	return true
}

func rewardTotal(unit int64, units int32) (int64, error) {
	if unit < 0 || units < 1 || units > maxRewardUnits || unit > maxRewardTotal/int64(units) {
		return 0, ErrInvalid
	}
	return unit * int64(units), nil
}

func expandReward(raw json.RawMessage, units int32) (json.RawMessage, error) {
	if units == 1 {
		return raw, nil
	}
	var content PhysicalReward
	if json.Unmarshal(raw, &content) != nil || !physicalValid(content) {
		return nil, ErrInvalid
	}
	var err error
	if content.ISKMinor, err = rewardTotal(content.ISKMinor, units); err != nil {
		return nil, err
	}
	for i := range content.Fittings {
		if content.Fittings[i].Quantity, err = rewardTotal(content.Fittings[i].Quantity, units); err != nil {
			return nil, err
		}
	}
	for i := range content.Items {
		if content.Items[i].Quantity, err = rewardTotal(content.Items[i].Quantity, units); err != nil {
			return nil, err
		}
	}
	if _, err = rewardSnapshotItems(content); err != nil {
		return nil, err
	}
	return json.Marshal(content)
}
