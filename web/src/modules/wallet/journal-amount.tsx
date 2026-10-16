import { ArrowDownLeft, ArrowUpRight } from "lucide-react";
import { money, type Row } from "./api";

export function JournalAmount({ row }: { row: Row }) {
  const outgoing = row.amount?.startsWith("-");
  return (
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
      {row.amount != null && !outgoing ? "+" : ""}
      {money(row.amount)}
    </span>
  );
}
