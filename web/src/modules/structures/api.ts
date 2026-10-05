import { getData } from "@/lib/http";

export type Service = { name: string; state: string };
export type Fuel = { type_id: string; name?: string; quantity: number };
export type Structure = {
  corporation_id: string;
  corporation_name: string;
  kind: "upwell" | "pos";
  id: string;
  name: string;
  type_id: string;
  type_name?: string;
  solar_system_id: string;
  solar_system_name?: string;
  state: string;
  fuel_expires?: string;
  profile_id?: string;
  unanchors_at?: string;
  services?: Service[];
  fuel?: Fuel[];
  observed_at: string;
  source_character_id: string;
};

const object = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null;
const id = (v: unknown): v is string => typeof v === "string" && /^[1-9][0-9]*$/.test(v);
const idOrZero = (v: unknown): v is string => typeof v === "string" && /^(0|[1-9][0-9]*)$/.test(v);
const isItem = (v: unknown): v is Structure =>
  object(v) && id(v.corporation_id) && typeof v.corporation_name === "string" && (v.kind === "upwell" || v.kind === "pos") &&
  id(v.id) && typeof v.name === "string" && id(v.type_id) && idOrZero(v.solar_system_id) &&
  typeof v.state === "string" && typeof v.observed_at === "string" &&
  Number.isFinite(Date.parse(v.observed_at));
const isData = (v: unknown): v is { items: Structure[] } =>
  object(v) && Array.isArray(v.items) && v.items.every(isItem);

export const structures = (corporationID = "", signal?: AbortSignal) =>
  getData(
    `/api/v1/structures/structures${corporationID ? `?corporation_id=${encodeURIComponent(corporationID)}` : ""}`,
    isData,
    signal,
  );
