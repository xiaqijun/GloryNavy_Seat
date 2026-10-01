import { describe, it, expect } from "vitest";
import {
  bucketState,
  isRateList,
  isRouteList,
  type RateBucket,
} from "./rate-api";
const b: RateBucket = {
  id: "1",
  group: "contracts",
  character_id: "123",
  name: "Pilot",
  observed_since: "2026-09-14T12:00:00Z",
  header_at: "2026-09-14T12:00:00Z",
  capacity: 600,
  policy_source: "response",
  remaining: 590,
  window_seconds: 900,
  retry_at: null,
  local_remaining: 585,
  local_recovery_at: "2026-09-14T12:15:00Z",
  local_blocked_until: null,
  egress_blocked_until: null,
  used_tokens: 3,
  network_requests: 1,
  unmeasured_requests: 0,
};
describe("rate budget snapshots", () => {
  it("distinguishes expired snapshot, unknown metadata and local budget exhaustion", () => {
    expect(bucketState(b, Date.parse("2026-09-14T12:01:00Z"))).toBe("已观测");
    expect(bucketState(b, Date.parse("2026-09-14T12:16:00Z"))).toBe(
      "快照已过期",
    );
    expect(
      bucketState({ ...b, remaining: null }, Date.parse(b.header_at!)),
    ).toBe("尚未计量");
    expect(
      bucketState({ ...b, local_remaining: 0 }, Date.parse(b.header_at!)),
    ).toBe("本地预算等待");
    expect(
      bucketState(
        { ...b, retry_at: "2026-09-14T12:01:00Z" },
        Date.parse(b.header_at!),
      ),
    ).toBe("限流等待");
  });
  it("preserves zero versus missing consumption and rejects malformed counters", () => {
    const list = { buckets: [b], next_cursor: "", observed_at: b.header_at };
    expect(isRateList(list)).toBe(true);
    expect(isRateList({ ...list, buckets: [{ ...b, remaining: -1 }] })).toBe(
      false,
    );
    expect(
      isRateList({
        ...list,
        buckets: [{ ...b, remaining: null, capacity: null }],
      }),
    ).toBe(true);
    const route = {
      route: "GET /status/",
      network_requests: 1,
      cache_hits: 0,
      local_waits: 0,
      upstream_limits: 0,
      used_tokens: 0,
      measured_responses: 1,
      unmeasured_requests: 0,
      last_status: 500,
      last_used: 0,
      last_response_at: b.header_at,
    };
    expect(isRouteList({ routes: [route], next_cursor: "" })).toBe(true);
    expect(
      isRouteList({ routes: [{ ...route, last_used: null }], next_cursor: "" }),
    ).toBe(true);
    expect(
      isRouteList({
        routes: [{ ...route, used_tokens: "0" }],
        next_cursor: "",
      }),
    ).toBe(false);
  });
});
