import { describe, expect, it } from "vitest";
import { isTokenList, isTokenEvents } from "./token-api";

const token = {
  character_id: "123",
  generation: "1",
  name: "舰长",
  state: "unknown",
  scopes: [],
  observed_since: null,
  access_expires_at: null,
  last_used_at: null,
  reuse_count: 0,
  last_refresh_attempt_at: null,
  last_refresh_success_at: null,
  last_refresh_reason: "",
  refresh_successes: 0,
  refresh_failures: 0,
  consecutive_failures: 0,
  last_request_at: null,
  last_request_status: 0,
  last_request_reason: "",
  network_requests: 0,
  cache_hits: 0,
  rate_limit_waits: 0,
  request_failures: 0,
};
describe("token observation contract", () => {
  it("preserves unknown expiry rather than treating it as a valid token", () => {
    expect(
      isTokenList({
        tokens: [token],
        next_cursor: "",
        observed_at: "2026-09-14T00:00:00Z",
      }),
    ).toBe(true);
  });
  it.each([
    { access_expires_at: "bad" },
    { cache_hits: -1 },
    { network_requests: "2" },
    { state: "working" },
    { scopes: [null] },
    { last_request_status: 900 },
  ])("rejects malformed observations %o", (change) => {
    expect(
      isTokenList({
        tokens: [{ ...token, ...change }],
        next_cursor: "",
        observed_at: "2026-09-14T00:00:00Z",
      }),
    ).toBe(false);
  });
  it("rejects malformed event durations and unknown outcomes", () => {
    const event = {
      id: "1",
      generation: "1",
      occurred_at: "2026-09-14T00:00:00Z",
      outcome: "refresh_success",
      reason: "",
      duration_ms: 250,
    };
    expect(isTokenEvents({ events: [event] })).toBe(true);
    expect(isTokenEvents({ events: [{ ...event, outcome: "secret" }] })).toBe(
      false,
    );
    expect(isTokenEvents({ events: [{ ...event, duration_ms: -1 }] })).toBe(
      false,
    );
  });
});
