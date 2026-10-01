package eve

import "slices"

// SeAT web cd11287006bddbf00c48cf13fd1fa704b0ea25d6, default sso_scopes.
// Like SeAT's SsoController, omit publicData when private scopes are requested.
// The old chat-channel scope is absent from ESI's 2026-08-18 OpenAPI catalog.
var seatDefaultScopes = []string{
	"esi-alliances.read_contacts.v1",
	"esi-assets.read_assets.v1",
	"esi-assets.read_corporation_assets.v1",
	"esi-calendar.read_calendar_events.v1",
	"esi-characters.read_agents_research.v1",
	"esi-characters.read_blueprints.v1",
	"esi-characters.read_contacts.v1",
	CorporationRolesScope,
	"esi-characters.read_fatigue.v1",
	"esi-characters.read_fw_stats.v1",
	"esi-characters.read_loyalty.v1",
	"esi-characters.read_medals.v1",
	"esi-characters.read_notifications.v1",
	"esi-characters.read_standings.v1",
	"esi-characters.read_titles.v1",
	"esi-clones.read_clones.v1",
	"esi-clones.read_implants.v1",
	"esi-contracts.read_character_contracts.v1",
	"esi-contracts.read_corporation_contracts.v1",
	"esi-corporations.read_blueprints.v1",
	"esi-corporations.read_contacts.v1",
	"esi-corporations.read_container_logs.v1",
	"esi-corporations.read_corporation_membership.v1",
	"esi-corporations.read_divisions.v1",
	"esi-corporations.read_facilities.v1",
	"esi-corporations.read_fw_stats.v1",
	"esi-corporations.read_medals.v1",
	"esi-corporations.read_standings.v1",
	"esi-corporations.read_starbases.v1",
	"esi-corporations.read_structures.v1",
	"esi-corporations.read_titles.v1",
	"esi-corporations.track_members.v1",
	"esi-fittings.read_fittings.v1",
	"esi-fleets.read_fleet.v1",
	"esi-industry.read_character_jobs.v1",
	"esi-industry.read_character_mining.v1",
	"esi-industry.read_corporation_jobs.v1",
	"esi-industry.read_corporation_mining.v1",
	"esi-killmails.read_corporation_killmails.v1",
	"esi-killmails.read_killmails.v1",
	"esi-location.read_location.v1",
	"esi-location.read_online.v1",
	"esi-location.read_ship_type.v1",
	"esi-mail.read_mail.v1",
	"esi-markets.read_character_orders.v1",
	"esi-markets.read_corporation_orders.v1",
	"esi-markets.structure_markets.v1",
	"esi-planets.manage_planets.v1",
	"esi-planets.read_customs_offices.v1",
	"esi-search.search_structures.v1",
	"esi-skills.read_skillqueue.v1",
	"esi-skills.read_skills.v1",
	"esi-ui.open_window.v1",
	"esi-universe.read_structures.v1",
	"esi-wallet.read_character_wallet.v1",
	"esi-wallet.read_corporation_wallets.v1",
	"esi-corporations.read_projects.v1",
}

func SeatDefaultScopes() []string { return slices.Clone(seatDefaultScopes) }
func hasScopes(granted, required []string) bool {
	for _, scope := range required {
		if !slices.Contains(granted, scope) {
			return false
		}
	}
	return true
}
func (c *Client) EnableSeatDefaultScopes() { c.roles = true; c.scopes = SeatDefaultScopes() }

// Optional delivered write capability, separate from SeAT's 57-scope baseline.
func (c *Client) EnableFittingsWriteScope() {
	if !slices.Contains(c.scopes, FittingsWriteScope) {
		c.scopes = append(c.scopes, FittingsWriteScope)
	}
}
func (c *Client) RequestedScopes() []string {
	if len(c.scopes) > 0 {
		return slices.Clone(c.scopes)
	}
	if c.roles {
		return []string{CorporationRolesScope}
	}
	return []string{}
}
