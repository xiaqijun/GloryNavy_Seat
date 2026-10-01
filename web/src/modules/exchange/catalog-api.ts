import { getData } from "@/lib/http";
export type PhysicalContent = {
  isk_minor?: number;
  fittings: {
    fitting_id: string;
    quantity: number;
    name?: string;
    ship_type_id?: string;
    corporation_id?: string;
    version?: string;
    fit?: unknown;
  }[];
  items: { type_id: string; quantity: number; name?: string }[];
};
export type CatalogEntry = {
  id: string;
  version: string;
  name: string;
  archived: boolean;
  content: PhysicalContent;
};
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
const qty = (v: unknown, max: number) =>
  Number.isSafeInteger(v) && Number(v) > 0 && Number(v) <= max;
export const isPhysical = (v: unknown): v is PhysicalContent =>
  obj(v) &&
  !("coins_minor" in v) &&
  (v.isk_minor === undefined ||
    (Number.isSafeInteger(v.isk_minor) &&
      Number(v.isk_minor) >= 0 &&
      Number(v.isk_minor) <= 1e14)) &&
  Array.isArray(v.fittings) &&
  v.fittings.length <= 10 &&
  v.fittings.every((f) => obj(f) && id(f.fitting_id) && qty(f.quantity, 100)) &&
  Array.isArray(v.items) &&
  v.items.length <= 30 &&
  v.items.every((i) => obj(i) && id(i.type_id) && qty(i.quantity, 1000000));
export const isCatalog = (v: unknown): v is CatalogEntry =>
  obj(v) &&
  id(v.id) &&
  id(v.version) &&
  typeof v.name === "string" &&
  typeof v.archived === "boolean" &&
  isPhysical(v.content);
export async function getCatalog(
  signal?: AbortSignal,
): Promise<CatalogEntry[]> {
  const out: CatalogEntry[] = [];
  let after = "";
  do {
    const page = await getData(
      `/api/v1/exchange/catalog?${new URLSearchParams({ after })}`,
      (v): v is { items: CatalogEntry[]; next_cursor: string } =>
        obj(v) &&
        Array.isArray(v.items) &&
        v.items.every(isCatalog) &&
        typeof v.next_cursor === "string",
      signal,
    );
    out.push(...page.items);
    after = page.next_cursor;
  } while (after);
  return out;
}
