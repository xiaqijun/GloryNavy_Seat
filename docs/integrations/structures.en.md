# Structure management

The first structure-management slice is read-only. `/structures` and `GET /api/v1/structures/structures` show corporation Upwell structures and POS objects allowed by the signed-in account's current bindings and the `corporation.structure` object permission. Pass `corporation_id` to select one corporation.

The data comes from the ESI corporation structures and starbases endpoints and uses the shared ESI cache and rate limits. Upwell rows expose state, services, profile and `fuel_expires`; POS rows include fuel type IDs and quantities when the starbase detail is readable. Detail requests include the POS solar-system `system_id` required by ESI. `observed_at` is the response observation time and is not a real-time guarantee.

This slice does not implement rentals, billing, contracts, Access List/Profile edits, personal-entry controls, POS passwords, or other game-side writes. Those changes remain in EVE's Structure Browser. Missing authorization, changed corporation affiliation, or an unavailable ESI response is reported instead of being represented as fabricated empty data.

The required ESI scopes are `esi-corporations.read_structures.v1` and `esi-corporations.read_starbases.v1`; site administrators select a current CEO/Director grant as the source, while ordinary members use only their own valid binding. `esi-universe.read_structures.v1` remains available for a later static-name projection.
