import { getLocale } from "@/lib/i18n";
import { getData } from "@/lib/http";
import type { Case } from "./api";

export type CapitalCheck = {
  reason?: string;
  state:
    | "closed"
    | "unbound"
    | "claimed"
    | "pending"
    | "contract_required"
    | "contract_invalid"
    | "contract_used"
    | "ready";
  policy_version: string;
  rate_bps: number;
  case_id?: string;
  purchase?: {
    contract_id: string;
    character_id: string;
    character_name: string;
    ship_type_id: string;
    ship_name: string;
    price_minor: number;
    award_minor: number;
    completed_at: string;
  };
};
const object = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null;
export function isCapitalCheck(v: unknown): v is CapitalCheck {
  if (
    !object(v) ||
    ![
      "closed",
      "unbound",
      "claimed",
      "pending",
      "contract_required",
      "contract_invalid",
      "contract_used",
      "ready",
    ].includes(String(v.state)) ||
    typeof v.policy_version !== "string" ||
    (v.reason !== undefined && typeof v.reason !== "string") ||
    !Number.isSafeInteger(v.rate_bps) ||
    (v.case_id !== undefined && typeof v.case_id !== "string")
  )
    return false;
  const p = v.purchase;
  if (v.state === "ready" && !p) return false;
  return (
    p === undefined ||
    (object(p) &&
      [
        "contract_id",
        "character_id",
        "character_name",
        "ship_type_id",
        "ship_name",
        "completed_at",
      ].every((k) => typeof p[k] === "string") &&
      Number.isSafeInteger(p.price_minor) &&
      Number(p.price_minor) > 0 &&
      Number.isSafeInteger(p.award_minor) &&
      Number(p.award_minor) >= 100)
  );
}
export const capitalCheck = (
  corp: string,
  kind: string,
  contract = "",
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/welfare/capital/check?${new URLSearchParams({ corporation_id: corp, kind, ...(contract ? { contract_id: contract } : {}) })}`,
    isCapitalCheck,
    signal,
  );

// Mirror the frozen integer calculation for display only; approval is server-owned.
export function capitalAmount(v: Case): number {
  if (v.award_minor > 0) return v.award_minor;
  const rate =
    v.detail.rule.subsidy_rate_bps ?? (v.kind === "titan" ? 500 : 1000);
  const base = v.detail.base_minor;
  if (
    !v.detail.purchase ||
    !Number.isSafeInteger(base) ||
    base <= 0 ||
    !Number.isSafeInteger(rate) ||
    rate < 1 ||
    rate > 10000
  )
    return 0;
  return Number((BigInt(base) * BigInt(rate)) / 10000n);
}

export function capitalMoney(minor: number): string {
  const amount = minor / 100;
  return Math.abs(amount) < 1_000_000
    ? amount.toLocaleString(getLocale(), { maximumFractionDigits: 2 })
    : amount.toLocaleString("en-US", {
        notation: "compact",
        compactDisplay: "short",
        maximumFractionDigits: 2,
      });
}
