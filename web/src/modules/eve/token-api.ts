import { msg } from "@/lib/i18n";
import { getData } from "@/lib/http";
import { reasonLabel } from "./sync-api";

export const tokenStates = {
  valid: msg("有效期内"),
  refresh_due: msg("待刷新"),
  refresh_failed: msg("刷新异常"),
  reauthorize: msg("需重新授权"),
  unknown: msg("尚未观测"),
} as const;
export interface TokenObservation {
  character_id: string;
  name: string;
  state: keyof typeof tokenStates;
  generation: string;
  scopes: string[];
  observed_since: string | null;
  access_expires_at: string | null;
  last_used_at: string | null;
  reuse_count: number;
  last_refresh_attempt_at: string | null;
  last_refresh_success_at: string | null;
  last_refresh_reason: string;
  refresh_successes: number;
  refresh_failures: number;
  consecutive_failures: number;
  last_request_at: string | null;
  last_request_status: number;
  last_request_reason: string;
  network_requests: number;
  cache_hits: number;
  rate_limit_waits: number;
  request_failures: number;
}
export interface TokenList {
  tokens: TokenObservation[];
  next_cursor: string;
  observed_at: string;
}
export interface TokenEvent {
  id: string;
  generation: string;
  occurred_at: string;
  outcome: "authorized" | "refresh_success" | "refresh_failed";
  reason: string;
  duration_ms: number;
}
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const count = (v: unknown) =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
export function isTokenList(v: unknown): v is TokenList {
  return (
    record(v) &&
    date(v.observed_at) &&
    (v.next_cursor === "" || id(v.next_cursor)) &&
    Array.isArray(v.tokens) &&
    v.tokens.every(
      (t) =>
        record(t) &&
        id(t.character_id) &&
        id(t.generation) &&
        typeof t.name === "string" &&
        typeof t.state === "string" &&
        Object.hasOwn(tokenStates, t.state) &&
        Array.isArray(t.scopes) &&
        t.scopes.every((s) => typeof s === "string") &&
        [
          t.observed_since,
          t.access_expires_at,
          t.last_used_at,
          t.last_refresh_attempt_at,
          t.last_refresh_success_at,
          t.last_request_at,
        ].every((d) => d === null || date(d)) &&
        [
          t.reuse_count,
          t.refresh_successes,
          t.refresh_failures,
          t.consecutive_failures,
          t.network_requests,
          t.cache_hits,
          t.rate_limit_waits,
          t.request_failures,
        ].every(count) &&
        count(t.last_request_status) &&
        Number(t.last_request_status) <= 599 &&
        typeof t.last_refresh_reason === "string" &&
        typeof t.last_request_reason === "string",
    )
  );
}
export function isTokenEvents(v: unknown): v is { events: TokenEvent[] } {
  return (
    record(v) &&
    Array.isArray(v.events) &&
    v.events.every(
      (e) =>
        record(e) &&
        id(e.id) &&
        id(e.generation) &&
        date(e.occurred_at) &&
        ["authorized", "refresh_success", "refresh_failed"].includes(
          String(e.outcome),
        ) &&
        typeof e.reason === "string" &&
        count(e.duration_ms),
    )
  );
}
export const getTokens = (
  search: string,
  state: string,
  after: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/eve/sync/tokens?${new URLSearchParams({ search, state, after })}`,
    isTokenList,
    signal,
  );
export const getTokenEvents = (id: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/eve/sync/tokens/${encodeURIComponent(id)}/events`,
    isTokenEvents,
    signal,
  );
export const tokenReason = (reason: string) =>
  (
    ({
      invalid_grant: msg("授权已失效"),
      identity_mismatch: msg("角色身份不一致"),
      scope_lost: msg("授权范围减少"),
      sso_rate_limited: msg("SSO 限流等待"),
      sso_timeout: msg("刷新超时"),
      sso_unavailable: msg("SSO 请求或验证失败"),
      storage_error: msg("存储异常"),
      rotation_commit_uncertain: msg("刷新提交结果待核对"),
      rate_limited: msg("限流等待"),
    }) as Record<string, string>
  )[reason] ?? reasonLabel(reason);
