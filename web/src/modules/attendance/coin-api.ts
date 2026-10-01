type Quote = {
  token: string;
  mode: "manual" | "automatic";
  points: number;
  converted: number;
  pending: number;
  coins_minor: number;
  characters: number;
  unit_scale?: number;
};
export const isCoinQuote = (v: unknown): v is Quote => {
  if (!v || typeof v !== "object") return false;
  const r = v as Record<string, unknown>;
  return (
    typeof r.token === "string" &&
    /^[a-f0-9]{64}$/.test(r.token) &&
    ["manual", "automatic"].includes(String(r.mode)) &&
    ["points", "converted", "pending", "coins_minor", "characters"].every(
      (k) => Number.isSafeInteger(r[k]) && Number(r[k]) >= 0,
    ) &&
    (r.unit_scale == null || (Number.isSafeInteger(r.unit_scale) && Number(r.unit_scale) > 0))
  );
};

export function formatPAPUnits(value: number, scale = 1, locale = "zh-CN") {
  return (value / scale).toLocaleString(locale, {
    minimumFractionDigits: scale > 1 ? 2 : 0,
    maximumFractionDigits: scale > 1 ? 2 : 0,
  });
}
