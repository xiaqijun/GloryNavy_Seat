import { msg } from "@/lib/i18n";
import { getData } from "@/lib/http";
export interface RateBucket {
  id: string;
  group: string;
  character_id: string;
  name: string;
  observed_since: string;
  header_at: string | null;
  capacity: number | null;
  window_seconds: number | null;
  policy_source: "openapi" | "response" | "unknown";
  remaining: number | null;
  retry_at: string | null;
  local_remaining: number | null;
  local_recovery_at: string | null;
  local_blocked_until: string | null;
  egress_blocked_until: string | null;
  used_tokens: number;
  network_requests: number;
  unmeasured_requests: number;
}
export interface RateRoute {
  route: string;
  network_requests: number;
  cache_hits: number;
  local_waits: number;
  upstream_limits: number;
  used_tokens: number;
  measured_responses: number;
  unmeasured_requests: number;
  last_status: number;
  last_used: number | null;
  last_response_at: string | null;
}
export interface RateList {
  buckets: RateBucket[];
  next_cursor: string;
  observed_at: string;
}
export interface RouteList {
  routes: RateRoute[];
  next_cursor: string;
}
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const count = (v: unknown) =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
export function isRateList(v: unknown): v is RateList {
  return (
    record(v) &&
    date(v.observed_at) &&
    (v.next_cursor === "" || id(v.next_cursor)) &&
    Array.isArray(v.buckets) &&
    v.buckets.every(
      (b) =>
        record(b) &&
        id(b.id) &&
        (b.character_id === "" || id(b.character_id)) &&
        typeof b.group === "string" &&
        b.group.length > 0 &&
        ["openapi", "response", "unknown"].includes(
          b.policy_source as string,
        ) &&
        typeof b.name === "string" &&
        date(b.observed_since) &&
        [
          b.header_at,
          b.retry_at,
          b.local_recovery_at,
          b.local_blocked_until,
          b.egress_blocked_until,
        ].every((d) => d === null || date(d)) &&
        [b.capacity, b.window_seconds, b.remaining, b.local_remaining].every(
          (n) => n === null || count(n),
        ) &&
        [b.used_tokens, b.network_requests, b.unmeasured_requests].every(count),
    )
  );
}
export function isRouteList(v: unknown): v is RouteList {
  return (
    record(v) &&
    typeof v.next_cursor === "string" &&
    Array.isArray(v.routes) &&
    v.routes.every(
      (r) =>
        record(r) &&
        typeof r.route === "string" &&
        [
          r.network_requests,
          r.cache_hits,
          r.local_waits,
          r.upstream_limits,
          r.used_tokens,
          r.measured_responses,
          r.unmeasured_requests,
          r.last_status,
        ].every(count) &&
        (r.last_used === null || count(r.last_used)) &&
        (r.last_response_at === null || date(r.last_response_at)),
    )
  );
}
export const getRateLimits = (
  search: string,
  after: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/eve/sync/rate-limits?${new URLSearchParams({ search, after })}`,
    isRateList,
    signal,
  );
export const getRateRoutes = (
  id: string,
  after: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/eve/sync/rate-limits/${encodeURIComponent(id)}/routes?${new URLSearchParams({ after })}`,
    isRouteList,
    signal,
  );
export function bucketState(b: RateBucket, now: number) {
  if (
    [b.retry_at, b.local_blocked_until, b.egress_blocked_until].some(
      (t) => t && Date.parse(t) > now,
    )
  )
    return msg("限流等待");
  if (
    b.local_remaining !== null &&
    b.local_remaining < 5 &&
    b.local_recovery_at &&
    Date.parse(b.local_recovery_at) > now
  )
    return msg("本地预算等待");
  if (!b.header_at || b.remaining === null || !b.window_seconds)
    return msg("尚未计量");
  if (Date.parse(b.header_at) + b.window_seconds * 1000 < now)
    return msg("快照已过期");
  return msg("已观测");
}
