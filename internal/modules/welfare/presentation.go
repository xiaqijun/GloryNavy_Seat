package welfare

import (
	"context"
	"encoding/json"
	"strconv"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/platform/locale"
)

// presentCases projects known display fields after authorization. It does not
// modify stored evidence, audit entries, content tokens, amounts or user text.
// RawMessage preserves unknown fields and exact numbers in older snapshots.
func (s *Service) presentCases(ctx context.Context, cases []Case) []Case {
	out := append([]Case{}, cases...)
	type field struct {
		object map[string]json.RawMessage
		key    string
		id     int64
		system bool
	}
	fields := []field{}
	details := make([]map[string]json.RawMessage, len(out))
	type nested struct {
		parent map[string]json.RawMessage
		key    string
		object map[string]json.RawMessage
	}
	children := []nested{}
	var child func(map[string]json.RawMessage, string) map[string]json.RawMessage
	child = func(parent map[string]json.RawMessage, key string) map[string]json.RawMessage {
		var v map[string]json.RawMessage
		if json.Unmarshal(parent[key], &v) != nil || v == nil {
			return nil
		}
		children = append(children, nested{parent, key, v})
		return v
	}
	arrayItems := []struct {
		parent map[string]json.RawMessage
		key    string
		items  []map[string]json.RawMessage
	}{}
	add := func(v map[string]json.RawMessage, idKey, nameKey string, system bool) {
		var id string
		if json.Unmarshal(v[idKey], &id) != nil {
			return
		}
		n, _ := strconv.ParseInt(id, 10, 64)
		if n > 0 {
			fields = append(fields, field{v, nameKey, n, system})
		}
	}
	items := func(v map[string]json.RawMessage, key string) {
		var rows []map[string]json.RawMessage
		if json.Unmarshal(v[key], &rows) != nil || rows == nil {
			return
		}
		for _, r := range rows {
			add(r, "type_id", "name", false)
		}
		arrayItems = append(arrayItems, struct {
			parent map[string]json.RawMessage
			key    string
			items  []map[string]json.RawMessage
		}{v, key, rows})
	}
	for i, c := range out {
		if json.Unmarshal(c.Detail, &details[i]) != nil {
			continue
		}
		d := details[i]
		if purchase := child(d, "purchase"); purchase != nil {
			items(purchase, "items")
		}
		if rewards := child(d, "rewards"); rewards != nil {
			items(rewards, "items")
		}
		if rule := child(d, "rule"); rule != nil {
			if rewards := child(rule, "rewards"); rewards != nil {
				items(rewards, "items")
			}
		}
		if loss := child(d, "loss_evidence"); loss != nil {
			add(loss, "ship_type_id", "ship_name", false)
			add(loss, "solar_system_id", "solar_system_name", true)
			items(loss, "items")
		}
		if valuation := child(d, "valuation"); valuation != nil {
			var reason string
			if json.Unmarshal(valuation["reason"], &reason) == nil {
				valuation["reason"] = raw(locale.Message(ctx, reason))
			}
			if market := child(valuation, "market"); market != nil {
				items(market, "lines")
			}
			if contract := child(valuation, "contract"); contract != nil {
				items(contract, "items")
			}
		}
		if delivery := child(d, "delivery"); delivery != nil {
			if contract := child(delivery, "contract"); contract != nil {
				items(contract, "items")
			}
		}
	}
	types, systems := []int64{}, []int64{}
	for _, f := range fields {
		if f.system {
			systems = append(systems, f.id)
		} else {
			types = append(types, f.id)
		}
	}
	names, places := map[int64]eve.StaticTypeName{}, map[int64]eve.StaticTypeName{}
	if s.Names != nil {
		// Keep evidence readable if the optional display-name lookup fails.
		if len(types) > 0 {
			names, _ = s.Names.TypeNames(ctx, types)
		}
		if len(systems) > 0 {
			places, _ = s.Names.SolarSystemNames(ctx, systems)
		}
	}
	for _, f := range fields {
		n := names[f.id]
		if f.system {
			n = places[f.id]
		}
		if n.Name != "" {
			f.object[f.key] = raw(n.Name)
		}
	}
	for _, a := range arrayItems {
		a.parent[a.key] = raw(a.items)
	}
	for i := len(children) - 1; i >= 0; i-- {
		c := children[i]
		c.parent[c.key] = raw(c.object)
	}
	for i, d := range details {
		if d != nil {
			out[i].Detail = raw(d)
		}
	}
	return out
}

// Enrich only the authorized detail response. The application snapshot, pricing
// evidence and audit remain immutable; a newer EVE cache adds display fields.
func (s *Service) enrichCaseLoss(ctx context.Context, c Case) Case {
	if s.LatestLoss == nil {
		return c
	}
	var detail map[string]json.RawMessage
	if json.Unmarshal(c.Detail, &detail) != nil {
		return c
	}
	var loss map[string]json.RawMessage
	if json.Unmarshal(detail["loss_evidence"], &loss) != nil || loss == nil {
		return c
	}
	if _, ok := loss["attackers"]; ok {
		return c
	}
	var d Detail
	if json.Unmarshal(c.Detail, &d) != nil || d.CharacterID <= 0 || d.KillmailID <= 0 {
		return c
	}
	latest, err := s.LatestLoss(ctx, c.AccountID, d.CharacterID, d.KillmailID)
	if err != nil || latest == nil || latest.ID != d.KillmailID || latest.CharacterID != d.CharacterID || len(latest.Attackers) == 0 {
		return c
	}
	loss["attackers"] = raw(latest.Attackers)
	loss["damage_taken"] = raw(latest.DamageTaken)
	if latest.VictimAllianceID > 0 {
		loss["victim_alliance_id"] = raw(strconv.FormatInt(latest.VictimAllianceID, 10))
	}
	detail["loss_evidence"] = raw(loss)
	c.Detail = raw(detail)
	return c
}

// Apply the same response-only projection to policy reward names.
func (s *Service) presentPolicies(ctx context.Context, policies []Policy) []Policy {
	out := append([]Policy{}, policies...)
	cases := make([]Case, len(out))
	for i, p := range out {
		cases[i].Detail = raw(map[string]json.RawMessage{"rule": p.Config})
	}
	presented := s.presentCases(ctx, cases)
	for i, c := range presented {
		var v map[string]json.RawMessage
		if json.Unmarshal(c.Detail, &v) == nil && v["rule"] != nil {
			out[i].Config = v["rule"]
		}
	}
	return out
}
