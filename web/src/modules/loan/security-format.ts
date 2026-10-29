import { getLocale } from "@/lib/i18n";

export const securityMoney = (minor: number) => `${new Intl.NumberFormat(getLocale(), { maximumFractionDigits: 2 }).format(minor / 100)} ISK`;

export function parseSecurityAmount(value: string): number | null {
  const match = /^(\d+)(?:\.(\d{1,2}))?$/.exec(value.trim());
  if (!match) return null;
  const minor = BigInt(match[1]) * 100n + BigInt((match[2] ?? "").padEnd(2, "0"));
  return minor > 0n && minor <= BigInt(Number.MAX_SAFE_INTEGER) ? Number(minor) : null;
}
