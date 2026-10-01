import { getData } from "@/lib/http";

export interface CombatMonth { month: string; kills: number | null; value: number | null }
export interface PublicActivity {
  corporation_id: number;
  combat: { months: CombatMonth[]; updated_at: string; stale: boolean } | null;
  online: { characters: number | null; covered_characters: number; bound_characters: number; updated_at: string; expires_at: string } | null;
}
const count = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) >= 0;
const date = (v: unknown): v is string => typeof v === "string" && Number.isFinite(Date.parse(v));
export function isPublicActivity(value: unknown): value is PublicActivity {
  if (!value || typeof value !== "object") return false;
  const v = value as PublicActivity;
  return v.corporation_id === 98530802 &&
    (v.combat === null || (!!v.combat && Array.isArray(v.combat.months) && v.combat.months.length === 6 &&
      v.combat.months.every((m, i, rows) => m && typeof m.month === "string" && /^\d{4}-(0[1-9]|1[0-2])$/.test(m.month) && (i === 0 || m.month > rows[i-1].month) &&
        (m.kills === null || count(m.kills)) && (m.value === null || (typeof m.value === "number" && Number.isFinite(m.value) && m.value >= 0))) &&
      date(v.combat.updated_at) && typeof v.combat.stale === "boolean")) &&
    (v.online === null || (!!v.online && count(v.online.covered_characters) && count(v.online.bound_characters) && v.online.covered_characters <= v.online.bound_characters &&
      (v.online.characters === null ? v.online.covered_characters === 0 : count(v.online.characters) && v.online.covered_characters > 0 && v.online.characters <= v.online.covered_characters) &&
      date(v.online.updated_at) && date(v.online.expires_at) && Date.parse(v.online.expires_at) >= Date.parse(v.online.updated_at)));
}
export const getPublicActivity = (signal?: AbortSignal) => getData("/api/v1/eve/public/activity", isPublicActivity, signal);
