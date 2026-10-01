import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Copy, Check } from "lucide-react";
import { Modal } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
import type { Order } from "./rewards-api";

type Handoff = {
  amount?: string;
  reference: string;
  recipient_id: string;
  recipient_name: string;
  state: Order["state"];
};
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const isHandoff = (v: unknown): v is Handoff =>
  obj(v) &&
  (v.amount === undefined ||
    (typeof v.amount === "string" && /^(0|[1-9]\d*)$/.test(v.amount))) &&
  typeof v.reference === "string" &&
  /^(?:GNV-EX-(?:[1-9]\d*|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})|EX-\d{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$/.test(
    v.reference,
  ) &&
  typeof v.recipient_id === "string" &&
  /^[1-9]\d*$/.test(v.recipient_id) &&
  typeof v.recipient_name === "string" &&
  ["pending", "cancel_requested", "fulfilled", "cancelled"].includes(
    String(v.state),
  );

function CopyButton({
  value,
  label,
  disabled = false,
}: {
  value: string;
  label: string;
  disabled?: boolean;
}) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  return (
    <span className="exchange-copy-action">
      <Button
        variant="outline"
        aria-label={label}
        title={label}
        disabled={disabled}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
            setStatus("copied");
          } catch {
            setStatus("failed");
          }
        }}
      >
        {status === "copied" ? <Check size={16} /> : <Copy size={16} />}
      </Button>
      {status === "copied" && <small role="status">{msg("已复制")}</small>}
      {status === "failed" && (
        <small role="alert">{msg("复制失败，请手动复制")}</small>
      )}
    </span>
  );
}

export function ContractInfo({
  order,
  user,
  close,
  embedded = false,
}: {
  order: Order;
  user: string;
  close: () => void;
  embedded?: boolean;
}) {
  const q = useQuery({
    queryKey: ["exchange", "handoff", user, order.id, order.version],
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/exchange/rewards/orders/${order.id}/handoff`,
        isHandoff,
        signal,
      ),
    staleTime: 0,
    gcTime: 0,
    refetchInterval: 30000,
  });
  const data = q.data;
  const content = (
    <>
      {q.isError ? (
        <div role="alert">
          {q.error.message}
          <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
        </div>
      ) : !data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <div className="exchange-contract-info">
          {data.state !== "pending" && (
            <p role="status">
              {msg("当前订单不处于待发放状态，请勿创建交付合同。")}
            </p>
          )}
          <div className="exchange-copy-field">
            <label>
              {msg("接收角色")}
              <input readOnly value={data.recipient_name} />
            </label>
            {data.state === "pending" && (
              <CopyButton
                value={data.recipient_name}
                label={msg("复制接收角色")}
              />
            )}
          </div>
          {data.amount && Number(data.amount) > 0 && (
            <div className="exchange-copy-field">
              <label>
                {msg("支付金额 / ISK")}
                <input readOnly value={data.amount} />
              </label>
              {data.state === "pending" && (
                <CopyButton value={data.amount} label={msg("复制支付金额")} />
              )}
            </div>
          )}
          <div className="exchange-copy-field">
            <label>
              {msg("合同结算 ID")}
              <input readOnly value={data.reference} />
            </label>
            {data.state === "pending" && (
              <CopyButton
                value={data.reference}
                label={msg("复制合同结算 ID")}
              />
            )}
          </div>
        </div>
      )}
    </>
  );
  return embedded ? (
    content
  ) : (
    <Modal title={msg("合同信息")} close={close} size="compact">
      {content}
    </Modal>
  );
}
