import { money } from "@/modules/wallet/api";

// Keep ESI decimal strings exact, including scientific notation and large balances.
export function sumWalletBalances(
  values: (string | null | undefined)[],
): string | null {
  if (!values.length) return null;
  const parts = values.map((value) => money(value).replaceAll(",", ""));
  if (parts.some((value) => value === "—")) return null;
  const scale = Math.max(...parts.map((value) => value.split(".")[1].length));
  const total = parts.reduce((sum, value) => {
    const [whole, fraction] = value.split(".");
    return sum + BigInt(whole + fraction.padEnd(scale, "0"));
  }, 0n);
  const digits = (total < 0n ? -total : total)
    .toString()
    .padStart(scale + 1, "0");
  return `${total < 0n ? "-" : ""}${digits.slice(0, -scale)}.${digits.slice(-scale)}`;
}
