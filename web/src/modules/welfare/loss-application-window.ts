import { useEffect, useState } from "react";
import { msg } from "@/lib/i18n";

export function applicationDays(value: string): number {
  const days = Number(value);
  if (!Number.isInteger(days) || days < 1 || days > 3650) {
    throw new Error(msg("请填写 1–3650 天的申请时限"));
  }
  return days;
}

export function lossApplicationExpired(occurred: string, days = 30, now = Date.now()): boolean {
  return now > Date.parse(occurred) + days * 86_400_000;
}

// Refresh the warning if a form stays open across its eligibility deadline.
export function useLossApplicationExpired(occurred: string, days = 30): boolean {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const deadline = Date.parse(occurred) + days * 86_400_000;
    if (!Number.isFinite(deadline) || now > deadline) return;
    const remaining = deadline - Date.now() + 1;
    const timer = window.setTimeout(() => setNow(Date.now()), Math.max(0, Math.min(remaining, 2_147_483_647)));
    return () => window.clearTimeout(timer);
  }, [occurred, days, now]);
  return lossApplicationExpired(occurred, days, now);
}
