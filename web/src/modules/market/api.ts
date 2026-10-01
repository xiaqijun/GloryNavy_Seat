import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";
export type Settings = { ratio_bps: number; version: number };
export type Context = { settings: Settings; administrator: boolean };
export type Amounts = { buy: string; mid: string; sell: string };
export type Line = {
  input: string;
  name: string;
  quantity: string;
  type_id: string;
  status: string;
  buy: string | null;
  mid: string | null;
  sell: string | null;
  observed_at: string | null;
};
export type Appraisal = {
  lines: Line[];
  totals: Amounts;
  adjusted: Amounts;
  ratio_bps: number;
  complete: boolean;
};
function object(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === "object";
}
function settings(v: unknown): v is Settings {
  return (
    object(v) &&
    Number.isSafeInteger(v.ratio_bps) &&
    Number.isSafeInteger(v.version)
  );
}
const money = (v: unknown) => typeof v === "string" && /^\d+\.\d{2}$/.test(v);
function amounts(v: unknown): v is Amounts {
  return object(v) && money(v.buy) && money(v.mid) && money(v.sell);
}
export function isAppraisal(v: unknown, maxLines = 100): v is Appraisal {
  return (
    object(v) &&
    Array.isArray(v.lines) &&
    v.lines.length <= maxLines &&
    v.lines.every(
      (l: unknown) =>
        object(l) &&
        typeof l.input === "string" &&
        typeof l.name === "string" &&
        typeof l.type_id === "string" &&
        /^\d+$/.test(l.type_id) &&
        typeof l.quantity === "string" &&
        /^\d+$/.test(l.quantity) &&
        [
          "ready",
          "invalid_quantity",
          "unknown_type",
          "unavailable",
          "missing_orders",
        ].includes(String(l.status)) &&
        [l.buy, l.mid, l.sell].every((x) => x === null || money(x)) &&
        (l.observed_at === null ||
          (typeof l.observed_at === "string" &&
            Number.isFinite(Date.parse(l.observed_at)))),
    ) &&
    amounts(v.totals) &&
    amounts(v.adjusted) &&
    Number.isSafeInteger(v.ratio_bps) &&
    typeof v.complete === "boolean"
  );
}
export const context = (signal?: AbortSignal) =>
  getData<Context>(
    "/api/v1/market/settings",
    (v): v is Context =>
      object(v) && settings(v.settings) && typeof v.administrator === "boolean",
    signal,
  );
async function post<T>(
  path: string,
  csrf: string,
  body: unknown,
  valid: (v: unknown) => v is T,
): Promise<T> {
  const r = await apiFetch(`/api/v1/market/${path}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(body),
  });
  const p = await r.json().catch(() => null);
  if (!r.ok)
    throw new APIError(p?.error?.message || msg("请求失败，请重试"), r.status);
  if (!valid(p?.data)) throw new Error(msg("服务响应格式异常"));
  return p.data;
}
export const estimate = (csrf: string, text: string) =>
  post("estimate", csrf, { text }, isAppraisal);
export const configure = (csrf: string, value: Settings) =>
  post("settings", csrf, value, settings);

export type ContractSide = "included" | "requested";
export const estimateContract = (
  csrf: string,
  owner: { kind: string; id: string },
  contract: string,
  side: ContractSide,
) =>
  post(
    "estimate-contract",
    csrf,
    { owner_kind: owner.kind, owner_id: owner.id, contract_id: contract, side },
    (v): v is Appraisal => isAppraisal(v, 2001),
  );
