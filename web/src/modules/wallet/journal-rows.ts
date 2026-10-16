import type { Row } from "./api";
import { hasWithheldTax } from "./tax";

export type JournalRow = Row & {
  display_kind?: "gross" | "tax";
  source?: Row;
};

function decimal(value?: string | null) {
  const m = /^(-?)(\d+)(?:\.(\d+))?(?:e([+-]?\d+))?$/i.exec(value || "");
  if (!m || Math.abs(Number(m[4] || 0)) > 100) return null;
  const scale = (m[3]?.length || 0) - Number(m[4] || 0);
  const coefficient = BigInt(m[1] + m[2] + (m[3] || ""));
  return scale < 0
    ? { coefficient: coefficient * 10n ** BigInt(-scale), scale: 0 }
    : { coefficient, scale };
}

// Reconstruct display-only gross income with decimal integer arithmetic.
// The source journal, actual balance and aggregate inputs remain untouched.
export function grossAmount(
  amount?: string | null,
  tax?: string | null,
): string | undefined {
  const net = decimal(amount),
    withheld = decimal(tax);
  if (!net || !withheld) return undefined;
  const scale = Math.max(net.scale, withheld.scale);
  const sum =
    net.coefficient * 10n ** BigInt(scale - net.scale) +
    withheld.coefficient * 10n ** BigInt(scale - withheld.scale);
  const digits = (sum < 0n ? -sum : sum).toString().padStart(scale + 1, "0");
  return `${sum < 0n ? "-" : ""}${scale ? `${digits.slice(0, -scale)}.${digits.slice(-scale)}` : digits}`;
}

export function journalRows(
  items: Row[],
  characterID?: string,
  direction?: string,
): JournalRow[] {
  if (!characterID) return items;
  return items.flatMap((row): JournalRow[] => {
    if (!hasWithheldTax(row.tax)) return direction === "tax" ? [] : [row];
    const gross: JournalRow = {
      ...row,
      source: row,
      display_kind: "gross",
      amount: grossAmount(row.amount, row.tax),
      balance: undefined,
    };
    const tax: JournalRow = {
      ...row,
      source: row,
      display_kind: "tax",
      amount: `-${row.tax}`,
      first_party_id: characterID,
      second_party_id: row.tax_receiver_id,
      reason: undefined,
    };
    if (direction === "tax") return [tax];
    if (direction === "out")
      return gross.amount?.startsWith("-") ? [gross, tax] : [tax];
    if (direction === "in")
      return gross.amount &&
        !gross.amount.startsWith("-") &&
        /[1-9]/.test(gross.amount)
        ? [gross]
        : [];
    return [gross, tax];
  });
}
