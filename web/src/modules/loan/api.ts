import { apiFetch, getData } from "@/lib/http";

export type Pool = { id: string; lender_kind: "personal" | "corporation"; lender_user_id?: string; corporation_id?: string; name: string; state: string; config: Record<string, unknown>; version: number };
export type Credit = { account_id: string; score?: number; total_limit_minor: number; unsecured_limit_minor: number; state: string; rule_version: string; reason: string; version: number };
export type Case = { id: string; public_id: string; pool_id: string; pool_name: string; lender_kind: string; borrower_account_id: string; borrower_character_id: string; principal_minor: number; interest_minor: number; total_due_minor: number; installment_count: number; interval_days: number; first_due_at: string; state: string; version: number; created_at: string };
export type Context = { pools: Pool[]; credit: Credit; administrator: boolean };
const object = (v: unknown): v is Record<string, any> => typeof v === "object" && v !== null;
const contextData = (v: unknown): v is Context => object(v) && Array.isArray(v.pools) && object(v.credit) && typeof v.administrator === "boolean";
const casesData = (v: unknown): v is { items: Case[] } => object(v) && Array.isArray(v.items);
export const context = (signal?: AbortSignal) => getData("/api/v1/loan/context", contextData, signal);
export const cases = (signal?: AbortSignal) => getData("/api/v1/loan/cases", casesData, signal);
export async function createApplication(csrf: string, body: Record<string, unknown>) { const r = await apiFetch("/api/v1/loan/applications", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) }); const p = await r.json().catch(() => null); if (!r.ok) throw new Error(p?.error?.message ?? "贷款申请失败"); return p.data as Case; }
export async function review(csrf: string, id: string, body: { state: "approved" | "rejected"; version: number; note?: string }) { const r = await apiFetch(`/api/v1/loan/cases/${encodeURIComponent(id)}/review`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) }); const p = await r.json().catch(() => null); if (!r.ok) throw new Error(p?.error?.message ?? "贷款审核失败"); return p.data as Case; }
