export function formatCompactISK(minor: number, locale: string): string {
  const amount = minor / 100;
  const text = Math.abs(amount) < 1_000_000
    ? new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(amount)
    : new Intl.NumberFormat("en-US", { notation: "compact", compactDisplay: "short", maximumFractionDigits: 2 }).format(amount);
  return `${text} ISK`;
}
