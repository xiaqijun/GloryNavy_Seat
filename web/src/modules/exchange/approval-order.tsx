import { useState } from "react";
import { Modal } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { RewardSummary } from "@/components/reward-summary";
import { msg, getLocale } from "@/lib/i18n";
import { ContractInfo } from "./contract-info";
import { DecisionForm, DeliveryInfo } from "./rewards-panel";
import type { Order } from "./rewards-api";
import type { QueueItem } from "@/modules/approval/api";
import "./exchange.css";

export function ApprovalOrder({
  item,
  user,
  csrf,
  close,
  done,
}: {
  item: QueueItem;
  user: string;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const [decision, setDecision] = useState<{
    order: Order;
    state: "cancelled" | "pending";
  } | null>(null);
  const order = item.payload as Order & {
    history?: { action: string; note: string; created_at: string }[];
  };
  if (decision)
    return (
      <DecisionForm
        csrf={csrf}
        decision={decision}
        close={() => setDecision(null)}
        done={done}
      />
    );
  return (
    <Modal
      title={msg("兑换记录")}
      close={close}
      footer={
        item.account_id !== user && item.actions.length > 0 ? (
          <>
            <Button onClick={() => setDecision({ order, state: "cancelled" })}>
              {msg("同意取消")}
            </Button>
            <Button
              variant="outline"
              onClick={() => setDecision({ order, state: "pending" })}
            >
              {msg("驳回取消")}
            </Button>
          </>
        ) : undefined
      }
    >
      <div className="approval-order">
        <div className="approval-order-heading">
          <strong>
            {order.name} × {order.quantity}
          </strong>
          <span>
            #{order.id} · {order.recipient_name}
          </span>
        </div>
        <RewardSummary rewards={order.content} />
        {order.note && <p>{order.note}</p>}
        <DeliveryInfo
          order={order}
          copyReference={order.state === "pending" && item.account_id !== user}
        />
        {order.state === "pending" && item.account_id !== user && (
          <ContractInfo order={order} user={user} close={close} embedded />
        )}
        {!!order.history?.length && (
          <details>
            <summary>{msg("处理记录")}</summary>
            {order.history.map((h, i) => (
              <div className="welfare-audit" key={i}>
                <time>
                  {new Date(h.created_at).toLocaleString(getLocale())}
                </time>
                <p>
                  {h.action === "delivery"
                    ? msg("系统核验")
                    : h.note || msg("提交申请")}
                </p>
              </div>
            ))}
          </details>
        )}
      </div>
    </Modal>
  );
}
