import { msg, getLocale } from "@/lib/i18n";
import { CheckCircle2, Clock3, ShieldCheck } from "lucide-react";
import * as api from "./api";
export function GrowthStatus({
  data,
  failed = false,
}: {
  data?: api.GrowthStatus;
  failed?: boolean;
}) {
  const state = failed ? "unknown" : data?.state;
  const label =
    state === "met"
      ? msg("达标")
      : state === "claimed"
        ? msg("已领取")
        : state === "pending"
          ? msg("申请处理中")
          : state === "closed"
            ? msg("未开放")
            : state === "missing"
              ? data?.remaining_sp != null
                ? msg(
                    "还需 {0} 技能点",
                    data.remaining_sp.toLocaleString(getLocale()),
                  )
                : msg("未达标")
              : msg("待检查");
  const Icon =
    state === "met" ? CheckCircle2 : state === "claimed" ? ShieldCheck : Clock3;
  return (
    <span className="welfare-growth-status" role="status">
      <Icon size={16} aria-hidden="true" />
      {label}
    </span>
  );
}
