import { getData } from "@/lib/http";

export interface ScopeOption { id: string; name: string }
export interface ScopeOptions { corporations: ScopeOption[]; alliances: ScopeOption[] }
const options = (value: unknown): value is ScopeOption[] => Array.isArray(value) && value.every((item) =>
  !!item && typeof item === "object" && typeof item.id === "string" && /^[1-9]\d*$/.test(item.id) &&
  BigInt(item.id) <= 9223372036854775807n && typeof item.name === "string",
);
export function isScopeOptions(value: unknown): value is ScopeOptions {
  if (!value || typeof value !== "object") return false;
  const data = value as Record<string, unknown>;
  return options(data.corporations) && options(data.alliances);
}
export const getScopeOptions = (signal?: AbortSignal) =>
  getData("/api/v1/access/scope-options", isScopeOptions, signal);
