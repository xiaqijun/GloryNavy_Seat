import { apiFetch } from "@/lib/http";
import { msg, getLocale } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";

export interface SyncTarget {
  id: string;
  character_id: string;
  name: string;
  resource:
    | "wallet_balance"
    | "wallet_journal"
    | "wallet_transactions"
    | "corporation_wallet_balance"
    | "corporation_wallet_journal"
    | "corporation_wallet_transactions"
    | "corporation_wallet_divisions"
    | "online"
    | "fittings"
    | "skills"
    | "skillqueue"
    | "killmails"
    | "profile"
    | "authorization"
    | "character_contracts"
    | "corporation_contracts";
  pending_details?: number;
  failed_details?: number;
  state: "idle" | "queued" | "running" | "deferred" | "failed" | "blocked";
  reason: string;
  freshness: "never" | "fresh" | "stale";
  last_attempt_at: string | null;
  last_success_at: string | null;
  content_updated_at: string | null;
  next_due_at: string;
}
export interface SyncList {
  targets: SyncTarget[];
  available: boolean;
  next_cursor?: string;
}
export interface SyncRun {
  id: string;
  started_at: string;
  finished_at: string | null;
  outcome: string;
  reason: string;
  http_status: number;
}
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const optionalDate = (v: unknown) => v === null || date(v);
export function isSyncList(v: unknown): v is SyncList {
  return (
    record(v) &&
    typeof v.available === "boolean" &&
    (v.next_cursor === undefined || typeof v.next_cursor === "string") &&
    Array.isArray(v.targets) &&
    v.targets.every(
      (t) =>
        record(t) &&
        id(t.id) &&
        id(t.character_id) &&
        typeof t.name === "string" &&
        [
          "wallet_balance",
          "wallet_journal",
          "wallet_transactions",
          "corporation_wallet_balance",
          "corporation_wallet_journal",
          "corporation_wallet_transactions",
          "corporation_wallet_divisions",
          "online",
          "fittings",
          "skills",
          "skillqueue",
          "killmails",
          "profile",
          "authorization",
          "character_contracts",
          "corporation_contracts",
        ].includes(String(t.resource)) &&
        [t.pending_details, t.failed_details].every(
          (n) =>
            n === undefined ||
            (typeof n === "number" && Number.isSafeInteger(n) && n >= 0),
        ) &&
        ["idle", "queued", "running", "deferred", "failed", "blocked"].includes(
          String(t.state),
        ) &&
        typeof t.reason === "string" &&
        ["never", "fresh", "stale"].includes(String(t.freshness)) &&
        [t.last_attempt_at, t.last_success_at, t.content_updated_at].every(
          optionalDate,
        ) &&
        date(t.next_due_at),
    )
  );
}
export const getCharacterSync = (id: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/eve/sync/characters/${encodeURIComponent(id)}`,
    isSyncList,
    signal,
  );
export const getSyncTargets = (
  search: string,
  state: string,
  after: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/eve/sync/targets?${new URLSearchParams({ search, state, after })}`,
    isSyncList,
    signal,
  );
export const getSyncRuns = (id: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/eve/sync/targets/${encodeURIComponent(id)}/runs`,
    (v): v is { runs: SyncRun[] } =>
      record(v) &&
      Array.isArray(v.runs) &&
      v.runs.every(
        (r) =>
          record(r) &&
          idString(r.id) &&
          date(r.started_at) &&
          optionalDate(r.finished_at) &&
          typeof r.outcome === "string" &&
          typeof r.reason === "string" &&
          typeof r.http_status === "number",
      ),
    signal,
  );
const idString = id;
export async function refreshSync(
  id: string,
  csrf: string,
  resource?: string,
  admin = false,
) {
  const path = admin
    ? `/api/v1/eve/sync/targets/${encodeURIComponent(id)}/retry`
    : `/api/v1/eve/sync/characters/${encodeURIComponent(id)}/refresh`;
  const response = await apiFetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(resource ? { resource } : {}),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok)
    throw new APIError(
      payload?.error?.message ?? msg("同步请求失败，请重试"),
      response.status,
    );
  const results: unknown = payload?.data?.results;
  if (
    !Array.isArray(results) ||
    !results.every(
      (r) =>
        record(r) &&
        idString(r.target_id) &&
        ["queued", "already_pending", "deferred"].includes(String(r.outcome)) &&
        date(r.next_due_at),
    )
  )
    throw new APIError(msg("同步响应格式异常"), response.status);
  return results as {
    target_id: string;
    outcome: "queued" | "already_pending" | "deferred";
    next_due_at: string;
  }[];
}
export const resourceLabel = (resource: string) =>
  (
    ({
      wallet_balance: msg("个人钱包余额"),
      wallet_journal: msg("个人钱包流水"),
      wallet_transactions: msg("个人市场交易"),
      corporation_wallet_balance: msg("军团钱包余额"),
      corporation_wallet_journal: msg("军团钱包流水"),
      corporation_wallet_transactions: msg("军团市场交易"),
      corporation_wallet_divisions: msg("军团钱包分部名称"),
      online: msg("在线状态"),
      fittings: msg("舰船配置"),
      skills: msg("角色技能"),
      skillqueue: msg("训练队列"),
      killmails: msg("舰船损失"),
      profile: msg("基础资料"),
      authorization: msg("ESI 授权"),
      character_contracts: msg("个人合同"),
      corporation_contracts: msg("军团合同"),
    }) as Record<string, string>
  )[resource] ?? resource;
export const contractDetailLabel = (t: SyncTarget) =>
  t.failed_details
    ? msg("明细 {0} 项待处理", t.failed_details)
    : t.pending_details
      ? msg("明细 {0} 项待同步", t.pending_details)
      : "";
export const reasonLabel = (reason: string) =>
  (
    ({
      reauthorize: msg("需要重新授权"),
      shared_source: msg("由其他角色提供军团授权"),
      authorization_pending: msg("等待军团授权同步"),
      pagination: msg("继续同步后续分页"),
      pagination_changed: msg("分页变化，重新核对"),
      pagination_invalid: msg("分页信息异常"),
      access_token_rejected: msg("授权需要检查"),
      missing_scope: msg("缺少授权范围"),
      access_denied: msg("ESI 拒绝访问"),
      missing_role: msg("缺少游戏职务"),
      identity_changed: msg("角色身份已变化"),
      corporation_changed: msg("等待军团信息更新"),
      rate_limited: msg("等待 ESI 可用"),
      network_error: msg("连接失败"),
      upstream_unavailable: msg("ESI 暂不可用"),
      invalid_response: msg("数据格式异常"),
      response_too_large: msg("数据超出限制"),
      cache_invalid: msg("缓存需要检查"),
      resource_unavailable: msg("数据暂不可用"),
      worker_interrupted: msg("任务中断，等待重试"),
      module_disabled: msg("模块未启用"),
    }) as Record<string, string>
  )[reason] ?? (reason ? msg("同步需要检查") : "");
export const syncLabel = (t: SyncTarget) =>
  t.reason === "shared_source"
    ? msg("共享同步")
    : t.state === "idle"
      ? t.freshness === "fresh"
        ? t.failed_details
          ? msg("明细待处理")
          : t.pending_details
            ? msg("列表已更新")
            : msg("已更新")
        : t.freshness === "never"
          ? msg("待同步")
          : msg("待更新")
      : (
          {
            queued: msg("已排队"),
            running: msg("同步中"),
            deferred: msg("稍后重试"),
            failed: msg("同步失败"),
            blocked: msg("需要处理"),
          } as const
        )[t.state];
export const syncDate = (v: string | null) =>
  v
    ? new Date(v).toLocaleString(getLocale(), {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      })
    : msg("尚未同步");
