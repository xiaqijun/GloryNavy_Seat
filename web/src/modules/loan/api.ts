import { apiFetch, getData } from "@/lib/http";

export type Pool = { id: string; is_shared: boolean; lender_kind: "personal" | "corporation"; lender_user_id?: string; corporation_id?: string; custodian_character_id?: string; name: string; state: string; config: Record<string, unknown>; version: number };
export type PoolSummary = { pending_minor: number; funded_minor: number; cash_minor: number; reserved_minor: number; available_minor: number };
export type Contribution = { id: string; pool_id: string; account_id: string; lender_kind: "personal" | "corporation"; source_character_id: string; corporation_id?: string; amount_minor: number; state: "pending" | "funded" | "cancelled"; contract_id?: string; version: number; created_at: string; funded_at?: string };
export type Credit = { account_id: string; score?: number; total_limit_minor: number; unsecured_limit_minor: number; state: string; rule_version: string; reason: string; version: number; repayment_points?: number; leverage_points?: number; security_points?: number; pap_points?: number; asset_points?: number; evidence?: string };
export type Case = { id: string; public_id: string; pool_id: string; pool_name: string; lender_kind: string; borrower_account_id: string; borrower_character_id: string; principal_minor: number; interest_minor: number; total_due_minor: number; installment_count: number; interval_days: number; first_due_at: string; state: string; version: number; created_at: string };
export type Guarantee = { id: string; case_id: string; guarantor_account_id: string; guarantor_character_id?: string; amount_minor: number; state: string; version: number; case_public_id?: string; borrower_account_id?: string; borrower_character_id?: string; principal_minor?: number };
export type Collateral = { id: string; case_id: string; owner_account_id: string; contract_kind: string; contract_owner_id: string; contract_id: string; valuation_minor: number; haircut_bps: number; covered_minor: number; state: string; version: number; items?: unknown[] };
export type CaseDetail = { case: Case; installments: unknown[]; guarantees: Guarantee[]; collateral: Collateral[] };
export type Context = { pools: Pool[]; pool_summary: PoolSummary; credit: Credit; administrator: boolean };
const object = (v: unknown): v is Record<string, any> => typeof v === "object" && v !== null;
const contextData = (v: unknown): v is Context => object(v) && Array.isArray(v.pools) && object(v.credit) && typeof v.administrator === "boolean";
const casesData = (v: unknown): v is { items: Case[] } => object(v) && Array.isArray(v.items);
export const context = (signal?: AbortSignal) => getData("/api/v1/loan/context", contextData, signal);
export const cases = (signal?: AbortSignal) => getData("/api/v1/loan/cases", casesData, signal);
const detailData = (v: unknown): v is CaseDetail => object(v) && object(v.case) && Array.isArray(v.guarantees) && Array.isArray(v.collateral);
export const detail = (id: string, signal?: AbortSignal) => getData(`/api/v1/loan/cases/${encodeURIComponent(id)}`, detailData, signal);
const contributionsData = (v: unknown): v is { items: Contribution[] } => object(v) && Array.isArray(v.items);
export const contributions = (signal?: AbortSignal) => getData("/api/v1/loan/contributions", contributionsData, signal);
const guaranteesData = (v: unknown): v is { items: Guarantee[] } => object(v) && Array.isArray(v.items);
export const guarantees = (signal?: AbortSignal) => getData("/api/v1/loan/guarantees", guaranteesData, signal);
export async function createApplication(csrf: string, body: Record<string, unknown>) { const r = await apiFetch("/api/v1/loan/applications", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) }); const p = await r.json().catch(() => null); if (!r.ok) throw new Error(p?.error?.message ?? "贷款申请失败"); return p.data as Case; }
async function mutate<T>(csrf: string, path: string, method: "POST" | "PUT", body: Record<string, unknown>, fallback: string) { const r = await apiFetch(path, { method, credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) }); const p = await r.json().catch(() => null); if (!r.ok) throw new Error(p?.error?.message ?? fallback); return p.data as T; }
export const createContribution = (csrf: string, body: Record<string, unknown>) => mutate<Contribution>(csrf, "/api/v1/loan/contributions", "POST", body, "出借额度提交失败");
export const depositContribution = (csrf: string, id: string, body: Record<string, unknown>) => mutate<Contribution>(csrf, `/api/v1/loan/contributions/${encodeURIComponent(id)}/deposit`, "POST", body, "入金合同核验失败");
export const cancelContribution = (csrf: string, id: string, body: Record<string, unknown>) => mutate<Contribution>(csrf, `/api/v1/loan/contributions/${encodeURIComponent(id)}/cancel`, "POST", body, "出借额度取消失败");
export async function review(csrf: string, id: string, body: { state: "approved" | "rejected"; version: number; note?: string }) { const r = await apiFetch(`/api/v1/loan/cases/${encodeURIComponent(id)}/review`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) }); const p = await r.json().catch(() => null); if (!r.ok) throw new Error(p?.error?.message ?? "贷款审核失败"); return p.data as Case; }
export const createGuarantee = (csrf: string, id: string, body: Record<string, unknown>) => mutate<Guarantee>(csrf, `/api/v1/loan/cases/${encodeURIComponent(id)}/guarantees`, "POST", body, "担保邀请失败");
export const decideGuarantee = (csrf: string, id: string, body: Record<string, unknown>) => mutate<Guarantee>(csrf, `/api/v1/loan/guarantees/${encodeURIComponent(id)}/decision`, "POST", body, "担保决定失败");
export const createCollateral = (csrf: string, id: string, body: Record<string, unknown>) => mutate<Collateral>(csrf, `/api/v1/loan/cases/${encodeURIComponent(id)}/collateral`, "POST", body, "抵押登记失败");
export const decideCollateral = (csrf: string, id: string, body: Record<string, unknown>) => mutate<Collateral>(csrf, `/api/v1/loan/collateral/${encodeURIComponent(id)}/decision`, "POST", body, "抵押审批失败");
