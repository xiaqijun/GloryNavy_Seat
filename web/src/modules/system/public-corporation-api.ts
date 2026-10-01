import { getData } from "@/lib/http";

export interface PublicCorporation {
  corporation_id: number;
  name: string;
  ticker: string;
  member_count: number;
  date_founded: string;
  alliance?: {
    id: number;
    name: string;
    ticker: string;
    corporation_count: number | null;
  };
  updated_at: string;
  stale: boolean;
}
const validCount = (value: unknown) =>
  Number.isSafeInteger(value) && (value as number) >= 0;
export function isPublicCorporation(
  value: unknown,
): value is PublicCorporation {
  if (!value || typeof value !== "object") return false;
  const v = value as Partial<PublicCorporation>;
  return (
    v.corporation_id === 98530802 &&
    typeof v.name === "string" &&
    typeof v.ticker === "string" &&
    validCount(v.member_count) &&
    typeof v.date_founded === "string" &&
    Number.isFinite(Date.parse(v.date_founded)) &&
    typeof v.updated_at === "string" &&
    Number.isFinite(Date.parse(v.updated_at)) &&
    typeof v.stale === "boolean" &&
    (v.alliance === undefined ||
      (!!v.alliance &&
        Number.isSafeInteger(v.alliance.id) &&
        v.alliance.id > 0 &&
        typeof v.alliance.name === "string" &&
        typeof v.alliance.ticker === "string" &&
        (v.alliance.corporation_count === null ||
          validCount(v.alliance.corporation_count))))
  );
}
export const getPublicCorporation = (signal?: AbortSignal) =>
  getData("/api/v1/eve/public/corporation", isPublicCorporation, signal);
