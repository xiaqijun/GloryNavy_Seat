import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";

export type Route = { id: number; start: string; end: string; cost_per_m3: number; collateral_percentage: number; transport_type: string; transport_volume: number; paused: number; expiration: number; completion: number; minimum_cost: number };
export type Routes = { routes: Route[]; location_categories: Record<string, { category_name: string | null; category_priority: number }>; fetched_at: string };
export type Appraisal = { appraisal: { buy_price: number; sell_price: number; volume: number; failed_items: Record<string, number>; janice_url: string | null }; from_cache: boolean };
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const number = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v) && v >= 0 && v <= Number.MAX_SAFE_INTEGER;
const text = (v: unknown): v is string => typeof v === "string" && !!v.trim();
export function validRoutes(v: unknown): v is Routes {
  return object(v) && Array.isArray(v.routes) && v.routes.every(r => object(r) && Number.isSafeInteger(r.id) && Number(r.id) > 0 && [r.start, r.end, r.transport_type].every(text) && [r.cost_per_m3, r.collateral_percentage, r.transport_volume, r.minimum_cost].every(number) && (r.paused === 0 || r.paused === 1) && Number.isInteger(r.expiration) && Number(r.expiration) > 0 && Number.isInteger(r.completion) && Number(r.completion) > 0) && object(v.location_categories) && Object.values(v.location_categories).every(c => object(c) && (c.category_name === null || typeof c.category_name === "string") && Number.isInteger(c.category_priority)) && typeof v.fetched_at === "string" && Number.isFinite(Date.parse(v.fetched_at));
}
export function validAppraisal(v: unknown): v is Appraisal {
  if (!object(v) || !object(v.appraisal) || typeof v.from_cache !== "boolean") return false;
  const a = v.appraisal;
  if (![a.buy_price, a.sell_price, a.volume].every(number) || !object(a.failed_items) || !Object.values(a.failed_items).every(x => number(x) && x > 0)) return false;
  if (a.janice_url === null) return true;
  try { const u = new URL(String(a.janice_url)); return u.protocol === "https:" && u.host === "janice.e-351.com" && !u.username && !u.password; } catch { return false; }
}
export const routes = (signal?: AbortSignal) => getData("/api/v1/courier/routes", validRoutes, signal);
export async function appraise(csrf: string, appraisal_text: string): Promise<Appraisal> {
  const response = await apiFetch("/api/v1/courier/appraisal", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ appraisal_text }) });
  const body = await response.json().catch(() => null);
  if (!response.ok) throw new APIError(body?.error?.message || msg("快递服务暂不可用，请重试"), response.status);
  if (!validAppraisal(body?.data)) throw new Error(msg("服务响应格式异常"));
  return body.data;
}

export function quote(route: Route, volume: number, collateral: number) {
  if (![volume, collateral, route.cost_per_m3, route.collateral_percentage, route.minimum_cost, route.transport_volume].every(number)) return null;
  const volumeCost = volume * route.cost_per_m3;
  const collateralCost = collateral * route.collateral_percentage / 100;
  const reward = Math.max(volumeCost + collateralCost, route.minimum_cost);
  if (![volumeCost, collateralCost, reward].every(number)) return null;
  return { volume, collateral, volumeCost, collateralCost, reward, minimumApplied: reward > volumeCost + collateralCost, oversized: route.transport_volume > 0 && volume > route.transport_volume };
}
export function inputNumber(value: string): number {
  return /^\d+(\.\d+)?$/.test(value.trim()) ? Number(value) : NaN;
}
