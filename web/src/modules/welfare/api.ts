import { gameTerm } from "@/lib/eve-terminology";
import { apiFetch } from "@/lib/http";
import { msg, getLocale } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";
import { isAppraisal, type Appraisal } from "@/modules/market/api";
export const activeLossKinds = ["srp", "solo"];
export type GrowthRewards = {
  isk_minor?: number;
  fittings: {
    fitting_id: string;
    quantity: number;
    name?: string;
    ship_type_id?: string;
    version?: string;
    fit?: unknown;
  }[];
  items: { type_id: string; quantity: number; name?: string }[];
  coins_minor: number;
};
export const projectLabel = (kind: string, config?: Config) =>
  config?.project_name || kinds[kind] || msg("成长项目");
export const caseLabel = (v: Case) => projectLabel(v.kind, v.detail.rule);
export const kinds: Record<string, string> = {
  srp: msg("军团补损"),
  alliance: msg("联盟补损"),
  solo: msg("PVP补损"),
  growth_gila: gameTerm("ships", "growth_gila"),
  growth_ishtar: gameTerm("ships", "growth_ishtar"),
  growth_loki: gameTerm("ships", "growth_loki"),
  growth_absolution: gameTerm("ships", "growth_absolution"),
  capital: msg("普通旗舰"),
  supercarrier: gameTerm("shipClasses", "supercarrier"),
  titan: gameTerm("shipClasses", "titan"),
  grant: msg("果壳币发放"),
  activity: msg("活动福利"),
};
export const states: Record<string, string> = {
  submitted: msg("待审核"),
  information: msg("待补充"),
  external: msg("待联盟结论"),
  approved: msg("待交付"),
  executing: msg("交付待核对"),
  completed: msg("已完成"),
  rejected: msg("已驳回"),
  cancelled: msg("已取消"),
  cancel_requested: msg("取消待审核"),
  reversed: msg("已冲正"),
};
export type Character = {
  id: string;
  name: string;
  account_id: string;
  corporation_id: string;
  main_character_name?: string;
};
export type Config = {
  loss_rate_bps?: number;
  loss_cap_minor?: number;
  loss_daily_cap_minor?: number;
  loss_weekly_cap_minor?: number;
  loss_monthly_cap_minor?: number;
  subsidy_rate_bps?: number;
  reward_id?: string;
  reward_version?: string;
  project_name?: string;
  claim_limit?: number;
  rewards?: GrowthRewards;
  enabled: boolean;
  effective_at: string;
  ship_type_id: string;
  fitting_id: string;
  skill_plan_id: string;
  reference_minor: number;
  day_zone: string;
  note: string;
};
export type Policy = {
  corporation_id: string;
  kind: string;
  version: string;
  config: Config;
};
export type Detail = {
  cancellation?: {
    reason: string;
    requested_at: string;
    decision?: string;
    reviewer?: string;
    review_note?: string;
    reviewed_at?: string;
  };
  payment_status?: string;
  purchase?: DeliveryContract;
  rewards?: GrowthRewards;
  valuation?: Valuation;
  pricing_mode?: "automatic" | "manual";
  delivery?: {
    automatic?: boolean;
    contract: DeliveryContract;
    confirmed_by: string;
    confirmed_at: string;
    verified_at?: string;
  };
  synced_loss?: boolean;
  loss_evidence?: Loss;
  character_id: string;
  character_name: string;
  ship_type_id: string;
  killmail_id: string;
  contract_id: string;
  supporting_contract_id?: string;
  event_id: string;
  occurred_at: string;
  description: string;
  evidence: string;
  image_count?: number;
  alliance: string;
  base_minor: number;
  discipline: boolean;
  policy_version: string;
  rule: Config;
  receipt: string;
  reviewer: string;
  executor: string;
  review_note: string;
  skill_evidence?: {
    state: string;
    met: number;
    total: number;
    observed_at: string | null;
  };
};
export type Loss = {
	attackers?: {
		character_id?: string;
		corporation_id?: string;
		alliance_id?: string;
		faction_id?: string;
		ship_type_id?: string;
		weapon_type_id?: string;
		damage_done: number;
		final_blow: boolean;
		security_status?: number;
		name?: string;
		corporation_name?: string;
		alliance_name?: string;
		ship_name?: string;
		weapon_name?: string;
	}[];
	damage_taken?: number;
	victim_alliance_id?: string;
  reimbursement?: {
    attendance_event_id?: string;
    reason?: string;
    status: "available" | "processing" | "completed" | "unavailable";
    state: string;
    kind: string;
    available_kinds: string[];
  };
  id: string;
  character_id: string;
  corporation_id: string;
  ship_type_id: string;
  ship_name: string;
  solar_system_id: string;
  solar_system_name: string;
  occurred_at: string;
  observed_at: string;
  items: {
    type_id: string;
    name: string;
    slot: string;
    quantity: number;
    destroyed: number;
    dropped: number;
  }[];
};
export type LossPrice = { mid: string | null; observed_at: string | null };
export const isLossPrices = (v: unknown): v is { items: LossPrice[] } =>
  obj(v) && Array.isArray(v.items) && v.items.length <= 2000 && v.items.every((item) =>
    obj(item) &&
    (item.mid === null || typeof item.mid === "string" && /^\d+\.\d{2}$/.test(item.mid)) &&
    (item.observed_at === null || typeof item.observed_at === "string" && Number.isFinite(Date.parse(item.observed_at))));
export const isLoss = (v: unknown): v is Loss =>
  obj(v) &&
  (v.damage_taken === undefined || Number.isSafeInteger(v.damage_taken) && Number(v.damage_taken) >= 0) &&
  (v.attackers === undefined || Array.isArray(v.attackers) && v.attackers.every((a) =>
    obj(a) && Number.isSafeInteger(a.damage_done) && Number(a.damage_done) >= 0 && typeof a.final_blow === "boolean")) &&
  (v.reimbursement === undefined ||
    (obj(v.reimbursement) &&
      (v.reimbursement.attendance_event_id === undefined ||
        str(v.reimbursement.attendance_event_id)) &&
      (v.reimbursement.reason === undefined || str(v.reimbursement.reason)) &&
      ["available", "processing", "completed", "unavailable"].includes(
        String(v.reimbursement.status),
      ) &&
      str(v.reimbursement.state) &&
      str(v.reimbursement.kind) &&
      Array.isArray(v.reimbursement.available_kinds) &&
      v.reimbursement.available_kinds.every((k) =>
        ["srp", "alliance", "solo"].includes(String(k)),
      ))) &&
  [
    v.id,
    v.character_id,
    v.corporation_id,
    v.ship_type_id,
    v.ship_name,
    v.solar_system_id,
    v.solar_system_name,
  ].every(str) &&
  typeof v.occurred_at === "string" &&
  Number.isFinite(Date.parse(v.occurred_at)) &&
  typeof v.observed_at === "string" &&
  Number.isFinite(Date.parse(v.observed_at)) &&
  Array.isArray(v.items) &&
  v.items.every(
    (i) =>
      obj(i) &&
      str(i.type_id) &&
      str(i.name) &&
      str(i.slot) &&
      [i.quantity, i.destroyed, i.dropped].every(
        (n) => Number.isSafeInteger(n) && Number(n) >= 0,
      ),
  );
export const isLossList = (
  v: unknown,
): v is { items: Loss[]; next_cursor: string } =>
  obj(v) &&
  Array.isArray(v.items) &&
  v.items.every(isLoss) &&
  str(v.next_cursor);
export type Case = {
  id: string;
  reference?: string;
  account_id: string;
  corporation_id: string;
  kind: string;
  state: string;
  version: string;
  detail: Detail;
  award_minor: number;
  created_at: string;
  updated_at: string;
};
export type Context = {
  corporations: { id: string; name: string; can_manage: boolean; can_compensate: boolean }[];
  characters: Character[];
  administrator: boolean;
  policies: Policy[];
  loss_quotas?: LossQuota[];
};
export type LossQuotaPeriod = {
  used_minor: number;
  reserved_minor: number;
  cap_minor?: number;
  remaining_minor?: number;
};
export type LossQuota = {
  kind: "srp" | "solo";
  daily: LossQuotaPeriod;
  weekly: LossQuotaPeriod;
  monthly: LossQuotaPeriod;
};
export type Profile = {
  account_id: string;
  verified: boolean;
  history: Record<string, string>;
  months: string[];
  version: string;
};
export type Quote = {
  token: string;
  lines: { account_id: string; amount_minor: number }[];
  total_minor: number;
};
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);
const str = (v: unknown) => typeof v === "string";
export const isGrowthRewards = (v: unknown): v is GrowthRewards =>
  obj(v) &&
  (v.isk_minor === undefined ||
    (Number.isSafeInteger(v.isk_minor) &&
      Number(v.isk_minor) >= 0 &&
      Number(v.isk_minor) <= 1e14)) &&
  Number.isSafeInteger(v.coins_minor) &&
  Number(v.coins_minor) >= 0 &&
  Number(v.coins_minor) <= 1e12 &&
  Array.isArray(v.fittings) &&
  v.fittings.length <= 10 &&
  v.fittings.every(
    (f) =>
      obj(f) &&
      typeof f.fitting_id === "string" &&
      /^[1-9]\d*$/.test(f.fitting_id) &&
      Number.isInteger(f.quantity) &&
      Number(f.quantity) >= 1 &&
      Number(f.quantity) <= 100,
  ) &&
  Array.isArray(v.items) &&
  v.items.length <= 30 &&
  v.items.every(
    (i) =>
      obj(i) &&
      typeof i.type_id === "string" &&
      /^[1-9]\d*$/.test(i.type_id) &&
      Number.isInteger(i.quantity) &&
      Number(i.quantity) >= 1 &&
      Number(i.quantity) <= 1000000,
  );
export type Valuation = {
  source: "market" | "purchase_contract";
  state: "pending" | "ready" | "incomplete" | "unavailable";
  reason: string;
  at: string;
  amount_minor: number;
  settings_version: string;
  market?: Appraisal;
  contract?: DeliveryContract;
};
export const valuationStateMessage = (
  state: Valuation["state"] | undefined,
): "待核价" | "报价不完整" | "核价失败" => {
  switch (state) {
    case "incomplete":
      return "报价不完整";
    case "unavailable":
      return "核价失败";
    default:
      return "待核价";
  }
};
export const isValuation = (v: unknown): v is Valuation =>
  obj(v) &&
  ["market", "purchase_contract"].includes(String(v.source)) &&
  ["pending", "ready", "incomplete", "unavailable"].includes(String(v.state)) &&
  str(v.reason) &&
  typeof v.at === "string" &&
  Number.isFinite(Date.parse(v.at)) &&
  Number.isSafeInteger(v.amount_minor) &&
  Number(v.amount_minor) >= 0 &&
  str(v.settings_version) &&
  (v.market === undefined || isAppraisal(v.market, 2001)) &&
  (v.contract === undefined || isDeliveryContract(v.contract));
export type DeliveryContract = {
  issuer_name?: string;
  id: string;
  owner_kind: string;
  owner_id: string;
  type: string;
  status: string;
  checked_at: string;
  title: string;
  issuer_id: string;
  issuer_corporation_id: string;
  for_corporation: boolean;
  assignee_id: string;
  acceptor_id: string;
  price: string;
  reward: string;
  issued: string;
  completed: string;
  items_ready: boolean;
  content_token: string;
  items: {
    type_id: string;
    quantity: string;
    included: boolean;
    name: string;
  }[];
};
export type DeliveryCandidate = {
  contract: DeliveryContract;
  can_link: boolean;
  reason: string;
};
export const isDeliveryContract = (v: unknown): v is DeliveryContract =>
  obj(v) &&
  [
    "id",
    "owner_kind",
    "owner_id",
    "type",
    "status",
    "checked_at",
    "title",
    "issuer_id",
    "issuer_corporation_id",
    "assignee_id",
    "acceptor_id",
    "price",
    "reward",
    "issued",
    "completed",
    "content_token",
  ].every((k) => str(v[k])) &&
  ["character", "corporation"].includes(String(v.owner_kind)) &&
  typeof v.for_corporation === "boolean" &&
  typeof v.items_ready === "boolean" &&
  Array.isArray(v.items) &&
  v.items.every(
    (i) =>
      obj(i) &&
      str(i.type_id) &&
      str(i.quantity) &&
      str(i.name) &&
      typeof i.included === "boolean",
  );
export const isDeliveryCandidates = (
  v: unknown,
): v is { items: DeliveryCandidate[] } =>
  obj(v) &&
  Array.isArray(v.items) &&
  v.items.every(
    (i) =>
      obj(i) &&
      isDeliveryContract(i.contract) &&
      typeof i.can_link === "boolean" &&
      str(i.reason),
  );
export const deliveryCandidates = (
  id: string,
  contract: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/welfare/cases/${id}/contracts${contract ? `?contract_id=${encodeURIComponent(contract)}` : ""}`,
    isDeliveryCandidates,
    signal,
  );
const char = (v: unknown): v is Character =>
  obj(v) &&
  str(v.id) &&
  str(v.name) &&
  str(v.account_id) &&
  (v.main_character_name === undefined || str(v.main_character_name)) &&
  str(v.corporation_id);
export const isCase = (v: unknown): v is Case =>
  obj(v) &&
  str(v.id) &&
  (v.reference === undefined || (str(v.reference) && v.reference.length > 0)) &&
      obj(v.detail) &&
      (v.detail.image_count === undefined || (Number.isSafeInteger(v.detail.image_count) && Number(v.detail.image_count) >= 1 && Number(v.detail.image_count) <= 3)) &&
  (v.detail.supporting_contract_id === undefined || str(v.detail.supporting_contract_id)) &&
  (v.detail.cancellation === undefined ||
    (obj(v.detail.cancellation) &&
      str(v.detail.cancellation.reason) &&
      str(v.detail.cancellation.requested_at))) &&
  (v.detail.rewards === undefined || isGrowthRewards(v.detail.rewards)) &&
  (v.detail.valuation === undefined || isValuation(v.detail.valuation)) &&
  (v.detail.purchase === undefined || isDeliveryContract(v.detail.purchase)) &&
  (v.detail.delivery === undefined ||
    (obj(v.detail.delivery) &&
      isDeliveryContract(v.detail.delivery.contract) &&
      str(v.detail.delivery.confirmed_by) &&
      str(v.detail.delivery.confirmed_at))) &&
  str(v.account_id) &&
  str(v.corporation_id) &&
  str(v.version) &&
  str(v.kind) &&
  (Object.hasOwn(kinds, String(v.kind)) ||
    /^growth_fitting_[1-9]\d*$/.test(String(v.kind)) ||
    /^activity_[1-9]\d*$/.test(String(v.kind))) &&
  str(v.state) &&
  String(v.state) in states &&
  Number.isSafeInteger(v.award_minor) &&
  obj(v.detail) &&
  str(v.detail.description) &&
  str(v.detail.character_name) &&
  str(v.detail.evidence) &&
  str(v.detail.ship_type_id) &&
  str(v.created_at) &&
  str(v.updated_at);
export const context = (corp: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/welfare/context?${new URLSearchParams({ corporation_id: corp })}`,
    (v): v is Context => {
      if (!obj(v) || typeof v.administrator !== "boolean") return false;
      if (!Array.isArray(v.characters) || !v.characters.every(char)) return false;
      if (
        !Array.isArray(v.corporations) ||
        !v.corporations.every(
          (x) =>
            obj(x) &&
            str(x.id) &&
            str(x.name) &&
            typeof x.can_manage === "boolean" &&
            typeof x.can_compensate === "boolean",
        )
      ) return false;
      if (
        !Array.isArray(v.policies) ||
        !v.policies.every(
          (x) =>
            obj(x) &&
            str(x.kind) &&
            str(x.version) &&
            obj(x.config) &&
            (x.config.rewards === undefined || isGrowthRewards(x.config.rewards)),
        )
      ) return false;
      const quotas = v.loss_quotas;
      return (
        quotas === undefined ||
        (Array.isArray(quotas) &&
          quotas.every(
            (x: unknown) =>
              obj(x) &&
              (x.kind === "srp" || x.kind === "solo") &&
              ["daily", "weekly", "monthly"].every((period) => {
                const p = x[period];
                return (
                  obj(p) &&
                  Number.isSafeInteger(p.used_minor) &&
                  Number(p.used_minor) >= 0 &&
                  Number.isSafeInteger(p.reserved_minor) &&
                  Number(p.reserved_minor) >= 0 &&
                  (p.cap_minor === undefined ||
                    (Number.isSafeInteger(p.cap_minor) && Number(p.cap_minor) > 0)) &&
                  (p.remaining_minor === undefined ||
                    (Number.isSafeInteger(p.remaining_minor) && Number(p.remaining_minor) >= 0))
                );
              }),
          ))
      );
    },
    signal,
  );
export const list = (
  corp: string,
  all: boolean,
  kind: string,
  before: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/welfare/cases?${new URLSearchParams({ corporation_id: corp, scope: all ? "all" : "mine", kind, before })}`,
    (v): v is { items: Case[]; next_cursor: string } =>
      obj(v) &&
      Array.isArray(v.items) &&
      v.items.every(isCase) &&
      str(v.next_cursor),
    signal,
  );
export const members = (corp: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/welfare/members?corporation_id=${corp}`,
    (v): v is { items: Character[] } =>
      obj(v) && Array.isArray(v.items) && v.items.every(char),
    signal,
  );
export const profile = (corp: string, member: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/welfare/profile?${new URLSearchParams({ corporation_id: corp, member })}`,
    (v): v is Profile =>
      obj(v) &&
      str(v.account_id) &&
      typeof v.verified === "boolean" &&
      obj(v.history) &&
      Object.values(v.history).every(str) &&
      Array.isArray(v.months) &&
      v.months.every(str) &&
      str(v.version),
    signal,
  );
export const detail = (id: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/welfare/cases/${id}`,
    (
      v,
    ): v is {
      item: Case;
      history: { action: string; note: string; created_at: string }[];
    } =>
      obj(v) &&
      isCase(v.item) &&
      Array.isArray(v.history) &&
      v.history.every(
        (x) => obj(x) && str(x.action) && str(x.note) && str(x.created_at),
      ),
    signal,
  );
export async function post(
  path: string,
  csrf: string,
  body: object,
): Promise<unknown> {
  const r = await apiFetch(`/api/v1/welfare/${path}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(body),
  });
  const v = await r.json().catch(() => null);
  if (!r.ok)
    throw new APIError(
      v?.error?.message ?? msg("操作失败，请重试"),
      r.status,
      v?.request_id,
    );
  if (!obj(v) || !("data" in v))
    throw new APIError(msg("返回格式异常，请刷新核对"), r.status);
  return v.data;
}
export function isQuote(v: unknown): v is Quote {
  return (
    obj(v) &&
    str(v.token) &&
    Number.isSafeInteger(v.total_minor) &&
    Array.isArray(v.lines) &&
    v.lines.every(
      (x) =>
        obj(x) && str(x.account_id) && Number.isSafeInteger(x.amount_minor),
    )
  );
}
export const money = (n: number) =>
  new Intl.NumberFormat(getLocale(), { maximumFractionDigits: 2 }).format(
    n / 100,
  );
export function minor(text: string): number {
  if (!/^\d+(\.\d{1,2})?$/.test(text))
    throw new Error(msg("金额最多保留两位小数"));
  const [a, b = ""] = text.split(".");
  const n = Number(a) * 100 + Number(b.padEnd(2, "0"));
  if (!Number.isSafeInteger(n)) throw new Error(msg("金额超出范围"));
  return n;
}
export const date = (s: string) =>
  s
    ? new Date(s).toLocaleString(getLocale(), {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";
export const capitalRate = (kind: string, config?: Config) =>
  (config?.subsidy_rate_bps ?? (kind === "titan" ? 500 : 1000)) / 100;
export function subsidyRate(text: string): number {
  if (
    !/^\d+(\.\d{1,2})?$/.test(text) ||
    Number(text) < 0.01 ||
    Number(text) > 100
  )
    throw new Error(msg("补贴比例须为 0.01%–100%，最多两位小数"));
  return minor(text);
}
export function lossAward(base: number, config?: Config): number {
  if (!Number.isSafeInteger(base) || base <= 0) return 0;
  const result =
    (BigInt(base) * BigInt(config?.loss_rate_bps ?? 10000)) / 10000n;
  const cap = BigInt(config?.loss_cap_minor || 0);
  return Number(cap > 0n && result > cap ? cap : result);
}
export function lossPreview(base: string | number, config?: Config): number {
  try {
    return lossAward(typeof base === "string" ? minor(base) : base, config);
  } catch {
    return 0;
  }
}
export function awardPreview(
  kind: string,
  base: string,
  discipline: boolean,
  rule?: Config,
): string {
  try {
    const n = BigInt(minor(base));
    const rate = ["srp", "alliance"].includes(kind)
      ? discipline
        ? 40n
        : 80n
      : kind === "solo"
        ? 50n
        : kind === "titan"
          ? 5n
          : 10n;
    let v = ["supercarrier", "titan"].includes(kind)
      ? (n *
          BigInt(rule?.subsidy_rate_bps ?? (kind === "titan" ? 500 : 1000))) /
        10000n
      : (n * rate) / 100n;
    if (kind === "solo" && v > 20000000000n) v = 20000000000n;
    return money(Number(v));
  } catch {
    return "—";
  }
}

export type GrowthStatus = {
  state: "met" | "missing" | "unknown" | "claimed" | "pending" | "closed";
  remaining_sp: number | null;
  observed_at: string | null;
};
export const isGrowthStatus = (v: unknown): v is GrowthStatus =>
  obj(v) &&
  ["met", "missing", "unknown", "claimed", "pending", "closed"].includes(
    String(v.state),
  ) &&
  (v.remaining_sp === null ||
    (Number.isSafeInteger(v.remaining_sp) && Number(v.remaining_sp) >= 0)) &&
  (v.observed_at === null || str(v.observed_at));
export const growthCheck = (
  corp: string,
  char: string,
  kind: string,
  signal?: AbortSignal,
) =>
  getData(
    "/api/v1/welfare/growth/check?" +
      new URLSearchParams({ corporation_id: corp, character_id: char, kind }),
    isGrowthStatus,
    signal,
  );
