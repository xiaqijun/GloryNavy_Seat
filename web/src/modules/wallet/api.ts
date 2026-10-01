import { journalTypes } from "./labels";
export { contextLabel } from "./labels";
import { getData } from "@/lib/http";
export type Owner = {
  kind: "character" | "corporation";
  id: string;
  name: string;
  divisions: number[];
  journal: boolean;
  transactions: boolean;
};
export type Row = {
  entry_key?: string;
  id: string;
  division: number;
  observed_at: string;
  date?: string;
  amount?: string | null;
  balance?: string | null;
  unit_price?: string;
  ref_type?: string;
  description?: string;
  reason?: string;
  first_party_id?: string;
  second_party_id?: string;
  client_id?: string;
  context_id?: string;
  context_id_type?: string;
  journal_ref_id?: string;
  tax?: string;
  tax_receiver_id?: string;
  location_id?: string;
  type_id?: string;
  type_name?: string;
  quantity?: string;
  is_buy?: boolean;
  is_personal?: boolean;
  name?: string;
};
export type Records = {
  items: Row[];
  names: Record<string, string>;
  next_cursor: string;
};
export type Summary = {
  items: {
    owner_id: string;
    balance: string | null;
    observed_at: string | null;
    income: string;
    expense: string;
  }[];
};
export type CorporationSummary = Summary;
export type IncomeTrendPoint = {
  period: string;
  income: string;
  expense: string;
  active_members: number;
};
export type FinanceTrendPoint = {
  period: string;
  income: string;
  expense: string;
  tax: string;
  net: string;
};
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^\d+$/.test(v);
export function isContext(v: unknown): v is { owners: Owner[] } {
  return (
    object(v) &&
    Array.isArray(v.owners) &&
    v.owners.every(
      (o) =>
        object(o) &&
        ["character", "corporation"].includes(String(o.kind)) &&
        id(o.id) &&
        typeof o.name === "string" &&
        typeof o.journal === "boolean" &&
        typeof o.transactions === "boolean" &&
        Array.isArray(o.divisions) &&
        o.divisions.every(
          (d) => Number.isInteger(d) && Number(d) >= 0 && Number(d) <= 7,
        ),
    )
  );
}
export function isRecords(v: unknown): v is Records {
  return (
    object(v) &&
    object(v.names) &&
    Object.values(v.names).every((n) => typeof n === "string") &&
    typeof v.next_cursor === "string" &&
    (v.next_cursor === "" || /^\d+(?:\.[a-f0-9]{32})?$/.test(v.next_cursor)) &&
    Array.isArray(v.items) &&
    v.items.every(
      (r) =>
        object(r) &&
        id(r.id) &&
        Number.isInteger(r.division) &&
        typeof r.observed_at === "string" &&
        Number.isFinite(Date.parse(r.observed_at)) &&
        [
          "amount",
          "balance",
          "unit_price",
          "quantity",
          "first_party_id",
          "second_party_id",
          "client_id",
          "type_id",
          "journal_ref_id",
        ].every((k) => r[k] == null || typeof r[k] === "string"),
    )
  );
}
export function isSummary(v: unknown): v is Summary {
  return object(v) && Array.isArray(v.items) && v.items.every((row) =>
    object(row) && id(row.owner_id) &&
    (row.balance === null || typeof row.balance === "string") &&
    (row.observed_at === null || (typeof row.observed_at === "string" && Number.isFinite(Date.parse(row.observed_at)))) &&
    typeof row.income === "string" && typeof row.expense === "string",
  );
}
export const context = (member: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/wallet/context?${new URLSearchParams({ member })}`,
    isContext,
    signal,
  );
export const summary = (from: string, signal?: AbortSignal) =>
  getData(`/api/v1/wallet/summary?${new URLSearchParams({ from })}`, isSummary, signal);
export const corporationSummary = (ownerID: string, from: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/wallet/corporation-summary?${new URLSearchParams({ owner_id: ownerID, from })}`,
    isSummary,
    signal,
  );
export const corporationFinanceTrend = (ownerID: string, from: string, until: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/wallet/corporation-finance-trend?${new URLSearchParams({ owner_id: ownerID, from, until })}`,
    (value: unknown): value is { items: FinanceTrendPoint[] } =>
      object(value) && Array.isArray(value.items) && value.items.every((row) =>
        object(row) && typeof row.period === "string" && /^\d{4}-\d{2}$/.test(row.period) &&
        typeof row.income === "string" && typeof row.expense === "string" &&
        typeof row.tax === "string" && typeof row.net === "string",
      ),
    signal,
  );
export const corporationPersonalSummary = (corporationID: string, from: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/wallet/personal-corporation-summary?${new URLSearchParams({ corporation_id: corporationID, from })}`,
    isSummary,
    signal,
  );
export const corporationPersonalIncomeTrend = (corporationID: string, from: string, until: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/wallet/personal-corporation-income-trend?${new URLSearchParams({ corporation_id: corporationID, from, until })}`,
    (value: unknown): value is { items: IncomeTrendPoint[] } =>
      object(value) && Array.isArray(value.items) && value.items.every((row) =>
        object(row) && typeof row.period === "string" && /^\d{4}-\d{2}$/.test(row.period) &&
        typeof row.income === "string" && typeof row.expense === "string" &&
        typeof row.active_members === "number" && Number.isInteger(row.active_members) && row.active_members >= 0,
      ),
    signal,
  );
export const records = (
  owner: Owner,
  division: number,
  part: string,
  filters: Record<string, string>,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/wallet/records?${new URLSearchParams({ owner_kind: owner.kind, owner_id: owner.id, division: String(division), part, ...filters })}`,
    isRecords,
    signal,
  );

// Dashboard summaries need the complete selected period rather than the
// first page of the wallet table. Keep pagination inside the wallet boundary
// so callers cannot accidentally calculate a rate from a partial page.
export async function journal(
  owner: Owner,
  division: number,
  filters: Record<string, string>,
  signal?: AbortSignal,
): Promise<Records> {
  const items: Row[] = [];
  let before = "";
  for (let page = 0; page < 200; page += 1) {
    const current = await records(
      owner,
      division,
      "journal",
      before ? { ...filters, before } : filters,
      signal,
    );
    items.push(...current.items);
    if (!current.next_cursor) {
      return { items, names: {}, next_cursor: "" };
    }
    before = current.next_cursor;
  }
  throw new Error("wallet journal pagination limit exceeded");
}
// Exact string formatting also accepts scientific notation emitted by ESI.
export function money(value?: string | null): string {
  if (value == null) return "—";
  const m = /^(-?)(\d+)(?:\.(\d+))?(?:e([+-]?\d+))?$/i.exec(value);
  if (!m) return "—";
  const exponent = Number(m[4] || 0);
  if (Math.abs(exponent) > 100) return "—";
  const digits = m[2] + (m[3] || ""),
    point = m[2].length + exponent;
  const whole = (
    point <= 0 ? "0" : digits.slice(0, point).padEnd(point, "0")
  ).replace(/^0+(?=\d)/, "");
  const fraction =
    point < 0 ? "0".repeat(-point) + digits : digits.slice(point);
  return `${m[1]}${whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",")}.${fraction.padEnd(2, "0")}`;
}
export const refLabels = journalTypes;
export const refLabel = (ref?: string) =>
  ref ? (Object.hasOwn(refLabels, ref) ? refLabels[ref] : ref) : "—";
