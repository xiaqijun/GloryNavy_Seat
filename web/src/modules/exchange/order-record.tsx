import { useId, useState, type ReactNode } from "react";
import { ChevronDown, ChevronUp, Coins } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EveImage } from "@/components/eve-image";
import { RewardSummary } from "@/components/reward-summary";
import { getLocale, msg } from "@/lib/i18n";
import { formatDate } from "./api";
import type { Order } from "./rewards-api";

export function OrderRecord({
  order,
  delivery,
  children,
}: {
  order: Order;
  delivery: ReactNode;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const detailsId = useId();
  const amount = order.coins_minor / 100;
  const [divisor, suffix] =
    amount >= 1e12
      ? [1e12, "T"]
      : amount >= 1e9
        ? [1e9, "B"]
        : amount >= 1e6
          ? [1e6, "M"]
          : [1, ""];
  const total = amount.toLocaleString(getLocale(), {
    maximumFractionDigits: 2,
  });
  const state = {
    pending: msg("待发放"),
    cancel_requested: msg("取消待审核"),
    fulfilled: msg("已发放"),
    cancelled: msg("已取消"),
  }[order.state];
  const progress: Record<string, string> = {
    waiting_items: msg("等待合同明细"),
    awaiting_acceptance: msg("等待领取合同"),
    mismatch: msg("合同内容不符"),
    multiple_contracts: msg("存在多个交付合同"),
    issuer_unverified: msg("发放人未通过核验"),
    contract_claimed: msg("合同已用于其他发放"),
    evidence_unavailable: msg("合同证据暂不可用"),
  };
  const deliveryStatus = order.delivery?.status ?? "";
  const notice =
    (order.state === "pending" || order.state === "cancel_requested") &&
    progress[deliveryStatus];
  return (
    <article
      className="exchange-order-row exchange-record"
      aria-label={msg("兑换记录 #{0}", order.id)}
    >
      <div className="exchange-record-heading">
        <EveImage kind="type" id={order.type_id} className="exchange-avatar" />
        <div className="exchange-record-identity">
          <div className="exchange-record-title">
            <strong>{order.name || `#${order.type_id}`}</strong>
            <span>
              {msg("{0} 份", (order.units ?? 1).toLocaleString(getLocale()))}
            </span>
          </div>
          <div className="exchange-record-meta">
            <span>{order.recipient_name || `#${order.recipient_id}`}</span>
            <time dateTime={order.created_at}>
              {formatDate(order.created_at)}
            </time>
          </div>
        </div>
        <div
          className="exchange-record-amount"
          title={`${total} ${msg("果壳币")}`}
        >
          <Coins size={16} aria-hidden="true" />
          <strong>
            {(amount / divisor).toLocaleString(getLocale(), {
              maximumFractionDigits: 2,
            })}
            {suffix}
          </strong>
          <span>{msg("果壳币")}</span>
        </div>
        <span className="exchange-record-state" data-state={order.state}>
          #{order.id} · {state}
        </span>
      </div>
      {notice && <p className="exchange-record-notice">{notice}</p>}
      <div className="exchange-record-tools">
        <Button
          variant="ghost"
          aria-expanded={open}
          aria-controls={detailsId}
          onClick={() => setOpen(!open)}
        >
          {msg("兑换详情")}
          {open ? (
            <ChevronUp aria-hidden="true" />
          ) : (
            <ChevronDown aria-hidden="true" />
          )}
        </Button>
        <div className="exchange-record-actions">{children}</div>
      </div>
      <div id={detailsId} className="exchange-record-details" hidden={!open}>
        <div className="exchange-record-evidence">
          {delivery}
          <span>
            {msg("果壳币")} · {total}
          </span>
          {order.note && <p>{order.note}</p>}
        </div>
        {order.content && <RewardSummary rewards={order.content} compact />}
      </div>
    </article>
  );
}
