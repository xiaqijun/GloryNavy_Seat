import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Check } from "lucide-react";
import { Modal } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast-context";
import { getData } from "@/lib/http";
import { write } from "./api";
import { msg } from "@/lib/i18n";
import type { Order } from "./rewards-api";

type Handoff = {
  started?: boolean;
  can_start?: boolean;
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
  (v.started === undefined || typeof v.started === "boolean") &&
  (v.can_start === undefined || typeof v.can_start === "boolean") &&
  (v.amount === undefined ||
    (typeof v.amount === "string" && /^(0|[1-9]\d*)$/.test(v.amount))) &&
  typeof v.reference === "string" &&
  /^(?:GNV-EX-(?:[1-9]\d*|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})|EX-\d{8}-(?:\d{6,19}|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}))$/.test(
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
  const toast = useToast();
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
            toast.success(msg("已复制"));
          } catch {
            setStatus("failed");
          }
        }}
      >
        {status === "copied" ? <Check size={16} /> : <Copy size={16} />}
      </Button>
      {status === "failed" && (
        <small role="alert">{msg("复制失败，请手动复制")}</small>
      )}
    </span>
  );
}

export function ContractInfo({
  order,
  user,
  csrf,
  close,
  embedded = false,
}: {
  order: Order;
  user: string;
  csrf: string;
  close: () => void;
  embedded?: boolean;
}) {
  const client = useQueryClient();
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
  const start = useMutation({
    mutationFn: () =>
      write(
        `/rewards/orders/${order.id}/handoff`,
        csrf,
        { version: order.version },
        isHandoff,
      ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["exchange"] });
      void client.invalidateQueries({ queryKey: ["approval"] });
    },
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
      ) : data.state === "pending" && data.started === false ? (
        <div className="exchange-contract-info">
          {data.can_start && (
            <Button disabled={start.isPending} onClick={() => start.mutate()}>
              {msg(start.isPending ? "正在提交" : "开始发放")}
            </Button>
          )}
          {start.isError && <p role="alert">{start.error.message}</p>}
        </div>
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
