import { getLocale } from "@/lib/i18n";

import { formatCompactISK } from "./amount-format";

export function LoanAmount({ minor }: { minor: number }) {
  const locale = getLocale();
  const exact = `${new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(minor / 100)} ISK`;
  return <span title={exact}><span aria-hidden="true">{formatCompactISK(minor, locale)}</span><span className="sr-only">{exact}</span></span>;
}
