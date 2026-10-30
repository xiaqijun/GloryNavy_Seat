import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Modal } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { RewardSummary } from "@/components/reward-summary";
import { msg, getLocale } from "@/lib/i18n";
import { ContractInfo } from "./contract-info";
import { DecisionForm, DeliveryInfo } from "./rewards-panel";
import type { Order } from "./rewards-api";
import type { QueueItem } from "@/modules/approval/api";
import { isSaved, write } from "./api";
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
  const [reverting, setReverting] = useState(false);
  const order = item.payload as Order & {
    history?: { action: string; note: string; created_at: string }[];
  };
  if (reverting)
    return <RevertCancellation order={order} csrf={csrf} close={() => setReverting(false)} done={done} />;
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
        item.account_id !== user && item.actions.includes("revert_cancel_review") ? (
          <Button variant="outline" onClick={() => setReverting(true)}>{msg("回退审批")}</Button>
        ) : item.account_id !== user && item.actions.includes("cancelled") ? (
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
            {order.name} ·{" "}
            {msg("{0} 份", (order.units ?? 1).toLocaleString(getLocale()))}
          </strong>
          <span>
            #{order.id} · {order.recipient_name}
          </span>
        </div>
        <RewardSummary rewards={order.content} />
        {order.note && <p>{order.note}</p>}
        <DeliveryInfo order={order} copyReference={false} />
        {order.state === "pending" && item.account_id !== user && (
          <ContractInfo
            csrf={csrf}
            order={order}
            user={user}
            close={close}
            embedded
          />
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

function RevertCancellation({order, csrf, close, done}: {order: Order; csrf: string; close: () => void; done: () => void}) {
  const [note, setNote] = useState("");
  const [key] = useState(() => crypto.randomUUID());
  const mutation = useMutation({
    mutationFn: () => write(`/rewards/orders/${order.id}`, csrf, {
      version: order.version, request_key: key, state: "revert_cancel_review", note,
    }, isSaved),
    onSuccess: done,
  });
  return <Modal title={msg("回退审批")} close={close} busy={mutation.isPending} footer={<>
    <Button variant="outline" onClick={close}>{msg("取消")}</Button>
    <Button disabled={!note.trim() || mutation.isPending} onClick={() => mutation.mutate()}>{msg("确认回退")}</Button>
  </>}>
    <label>{msg("回退原因")}<textarea value={note} maxLength={200} onChange={(e) => setNote(e.target.value)} /></label>
    {mutation.isError && <p role="alert">{msg("操作失败，请刷新后重试")}</p>}
  </Modal>;
}
