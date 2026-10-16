import { ArrowDownLeft, ArrowUpRight } from "lucide-react";
import { msg } from "@/lib/i18n";
import { money, type Row } from "./api";
import { hasWithheldTax } from "./tax";

export function JournalAmount({
  row,
  personal,
  taxFirst,
}: {
  row: Row;
  personal: boolean;
  taxFirst: boolean;
}) {
  const taxed = personal && hasWithheldTax(row.tax);
  const outgoing = row.amount?.startsWith("-");
  const amount = (
    <span
      className={
        row.amount == null ? undefined : outgoing ? "wallet-out" : "wallet-in"
      }
    >
      {row.amount != null &&
        (outgoing ? (
          <ArrowUpRight size={14} aria-hidden="true" />
        ) : (
          <ArrowDownLeft size={14} aria-hidden="true" />
        ))}
      {taxed && !outgoing
        ? msg("到账 {0}", money(row.amount))
        : money(row.amount)}
    </span>
  );
  const tax = taxed ? (
    <span className="wallet-withheld-tax wallet-out">
      {msg("代扣税 −{0}", money(row.tax))}
    </span>
  ) : null;
  return (
    <div className="wallet-amount-lines">
      {taxFirst && tax}
      {amount}
      {!taxFirst && tax}
    </div>
  );
}
