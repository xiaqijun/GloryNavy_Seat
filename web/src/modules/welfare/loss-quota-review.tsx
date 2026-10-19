import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { money } from "./api";
import "./loss-quota-review.css";

type Period = { used_minor: number; reserved_minor: number; cap_minor?: number; remaining_minor?: number };
export type QuotaReview = { policy_version: string; case_version: string; checked_at: string; quota: { kind: string; daily: Period; weekly: Period; monthly: Period } };
const obj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const amount = (v: unknown): v is number => typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
function period(v: unknown): v is Period {
  return obj(v) && amount(v.used_minor) && amount(v.reserved_minor) &&
    (v.cap_minor === undefined ? v.remaining_minor === undefined : amount(v.cap_minor) && v.cap_minor > 0 && amount(v.remaining_minor) && v.remaining_minor <= v.cap_minor);
}
export function isQuotaReview(v: unknown): v is QuotaReview {
  return obj(v) && typeof v.policy_version === "string" && /^\d+$/.test(v.policy_version) && typeof v.case_version === "string" && /^\d+$/.test(v.case_version) && typeof v.checked_at === "string" && Number.isFinite(Date.parse(v.checked_at)) && obj(v.quota) && ["srp", "solo"].includes(String(v.quota.kind)) && period(v.quota.daily) && period(v.quota.weekly) && period(v.quota.monthly);
}
export function quotaLevel(q: QuotaReview["quota"], value: number) {
  const exceeds = (p: Period) => p.remaining_minor !== undefined && value > p.remaining_minor;
  if (exceeds(q.monthly)) return "blocked";
  if (exceeds(q.weekly)) return "monthly";
  if (exceeds(q.daily)) return "weekly";
  return "";
}
export function useLossQuotaReview(id: string, version: string, policyVersion: string, value: number, enabled: boolean) {
  const [consentKey, setConsentKey] = useState("");
  const query = useQuery({ queryKey: ["welfare", "loss-quota", id, version], queryFn: ({ signal }) => getData(`/api/v1/welfare/cases/${id}/loss-quota`, isQuotaReview, signal), enabled, retry: false, refetchInterval: enabled ? 30_000 : false, staleTime: 0 });
  const level = query.data ? quotaLevel(query.data.quota, value) : "";
  const key = JSON.stringify([id, version, policyVersion, value, query.data?.quota]);
  const confirmed = consentKey === key;
  const mismatch = !!query.data && (query.data.case_version !== version || query.data.policy_version !== policyVersion);
  const blocked = enabled && (value <= 0 || !query.data || query.isError || query.isFetching || mismatch || level === "blocked" || !!level && !confirmed);
  return { query, value, level, confirmed, mismatch, blocked, consent: confirmed && level !== "blocked" ? level : "", clear: () => setConsentKey(""), confirm: (checked: boolean) => setConsentKey(checked ? key : ""), retry: () => { setConsentKey(""); void query.refetch(); } };
}
export function LossQuotaConfirmation({ review }: { review: ReturnType<typeof useLossQuotaReview> }) {
  const { query, level, value, mismatch } = review;
  if (query.isError) return <div className="wide welfare-quota-confirm" role="alert">{query.error.message}<Button variant="outline" onClick={review.retry}>{msg("重试")}</Button></div>;
  if (!query.data) return <p className="wide" role="status">{msg("正在核对补损额度")}</p>;
  if (mismatch) return <p className="wide welfare-warning" role="alert">{msg("申请或设置已更新，请关闭后重新打开审批")}</p>;
  if (!level) return null;
  const q = query.data.quota;
  const source = level === "weekly" ? q.daily : level === "monthly" ? q.weekly : q.monthly;
  const target = level === "weekly" ? q.weekly : q.monthly;
  const label = level === "weekly" ? msg("日额度不足") : level === "monthly" ? msg("周额度不足") : msg("月额度不足，无法批准");
  return <section className="wide welfare-quota-confirm" aria-label={msg("额度确认")}>
    <strong>{label}</strong>
    <div className="welfare-quota-confirm-values">
      <span>{msg("剩余额度")}<b>{money(source.remaining_minor || 0)} ISK</b></span>
      <span>{msg("额度缺口")}<b>{money(Math.max(0, value - (source.remaining_minor || 0)))} ISK</b></span>
      {level !== "blocked" && <span>{level === "weekly" ? msg("本周剩余") : msg("本月剩余")}<b>{target.remaining_minor === undefined ? msg("不限制") : `${money(target.remaining_minor)} ISK`}</b></span>}
    </div>
    {level !== "blocked" && <label><input type="checkbox" checked={review.confirmed} onChange={e => review.confirm(e.target.checked)} />{level === "weekly" ? msg("确认使用周额度") : msg("确认使用月额度")}</label>}
  </section>;
}
