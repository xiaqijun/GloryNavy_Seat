import { describe, expect, it } from "vitest";
import { isAccountAccess } from "./api";
describe("authorization response contract", () => {
  const data = {
    administrator: false,
    site_roles: [],
    characters: [
      {
        character_id: "123",
        state: "ready",
        corporation: {
          id: "10",
          name: "Corp",
          alliance_id: "0",
          ceo_id: "124",
        },
        roles: ["Director"],
        roles_at_hq: [],
        roles_at_base: [],
        roles_at_other: [],
        synced_at: "2026-09-14T01:00:00Z",
        valid_until: "2026-09-14T02:00:00Z",
      },
    ],
  };
  it("preserves scopes and accepts empty role sets", () => {
    expect(isAccountAccess(data)).toBe(true);
    expect(isAccountAccess({ ...data, characters: [] })).toBe(true);
  });
  it("rejects malformed roles, state and dates", () => {
    for (const change of [
      { roles: "Director" },
      { state: "admin" },
      { valid_until: "invalid" },
      { corporation: { id: 10 } },
    ])
      expect(
        isAccountAccess({
          ...data,
          characters: [{ ...data.characters[0], ...change }],
        }),
      ).toBe(false);
  });
});
