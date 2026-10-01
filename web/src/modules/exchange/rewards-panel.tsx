import { ContractInfo } from "./contract-info";
import { useApprovalEnabled, approvalLink } from "@/modules/approval/api";
import { Link } from "react-router-dom";
import { RewardSummary } from "@/components/reward-summary";
import { msg, getLocale } from "@/lib/i18n";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Gift, Coins, Package, Copy, Check, RefreshCw } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Modal } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { EveImage } from "@/components/eve-image";
import {
  getShop,
  getOrders,
  isClaimed,
  type Reward,
  type Shop,
  type Order,
} from "./rewards-api";
import { isSaved } from "./api";
import { formatDate, write, type Context } from "./api";
const number = (n: number) => n.toLocaleString(getLocale());
const coin = (n: number) => number(n / 100);

export function RewardsPanel({
  user,
  csrf,
  context,
}: {
  user: string;
  csrf: string;
  context: Context;
}) {
  const client = useQueryClient();
  const approvalEnabled = useApprovalEnabled();
  const [after, setAfter] = useState("");
  const [before, setBefore] = useState("");
  const [all, setAll] = useState(false);
  const [handoff, setHandoff] = useState<Order | null>(null);
  const [claim, setClaim] = useState<{ reward: Reward; shop: Shop } | null>(
    null,
  );
  const [decision, setDecision] = useState<{
    order: Order;
    state: "cancelled" | "cancel_requested" | "pending";
  } | null>(null);
  const q = useQuery({
    queryKey: ["exchange", "shop", user, after],
    queryFn: ({ signal }) => getShop(after, signal),
    refetchInterval: 30000,
  });
  const orders = useQuery({
    queryKey: ["exchange", "orders", user, all && !approvalEnabled, before],
    queryFn: ({ signal }) => getOrders(all && !approvalEnabled, before, signal),
    refetchInterval: 30000,
  });
  // Orders and the reward catalog have separate endpoints and permissions.
  // Reading orders does not need to wait for the catalog response.
  useEffect(() => {
    if (!orders.data?.next_cursor) return;
    const next = orders.data.next_cursor;
    void client.prefetchQuery({
      queryKey: ["exchange", "orders", user, all && !approvalEnabled, next],
      queryFn: ({ signal }) => getOrders(all && !approvalEnabled, next, signal),
      staleTime: 30_000,
    });
  }, [client, orders.data?.next_cursor, user, all, approvalEnabled]);
  useEffect(() => {
    if (!q.data?.next_cursor) return;
    const next = q.data.next_cursor;
    void client.prefetchQuery({
      queryKey: ["exchange", "shop", user, next],
      queryFn: ({ signal }) => getShop(next, signal),
      staleTime: 30_000,
    });
  }, [client, q.data?.next_cursor, user]);
  const opener = useRef<HTMLElement | null>(null);
  const remember = () => {
    opener.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
  };
  const close = () => {
    setClaim(null);
    setDecision(null);
    opener.current?.focus();
  };
  const done = () => {
    close();
    void client.invalidateQueries({ queryKey: ["exchange"] });
    void client.invalidateQueries({ queryKey: ["approval"] });
  };
  if (q.isError)
    return (
      <div role="alert">
        {q.error.message}
        <Button variant="outline" onClick={() => void q.refetch()}>
          {msg("重试")}{" "}
        </Button>
      </div>
    );
  if (!q.data) return <p role="status">{msg("正在读取")}</p>;
  const shop = q.data;
  const formOpen = !!(claim || decision || handoff);
  return (
    <div className="exchange-section">
      {handoff && (
        <ContractInfo
          user={user}
          order={orders.data?.items.find((o) => o.id === handoff.id) ?? handoff}
          close={() => setHandoff(null)}
        />
      )}
      <div className="exchange-toolbar">
        <span className="exchange-note">
          {shop.isk_per_coin
            ? msg("1 果壳币 = {0} ISK", number(shop.isk_per_coin))
            : msg("管理员尚未配置果壳币价值")}
        </span>
      </div>
      <div className="exchange-metrics">
        {[
          {
            icon: <Coins />,
            label: msg("可用果壳币"),
            value: Math.max(0, shop.available_minor),
          },
          {
            icon: <Package />,
            label: msg("待发放占用"),
            value: shop.reserved_minor,
          },
          { icon: <Gift />, label: msg("已兑换"), value: shop.spent_minor },
        ].map((v) => (
          <Card key={v.label} className="exchange-card exchange-metric">
            <span className="exchange-metric-icon" aria-hidden="true">
              {v.icon}
            </span>
            <div>
              <small>{v.label}</small>
              <strong>{coin(v.value)}</strong>
            </div>
          </Card>
        ))}
      </div>
      {shop.available_minor < 0 && (
        <p role="alert">
          {msg("果壳币欠额")} {coin(-shop.available_minor)}
          {msg("，补足前暂不可兑换。")}{" "}
        </p>
      )}
      {claim && (
        <ClaimForm
          csrf={csrf}
          quote={claim}
          characters={context.characters}
          done={done}
          close={close}
        />
      )}
      {decision && (
        <DecisionForm
          csrf={csrf}
          decision={decision}
          done={done}
          close={close}
        />
      )}
      <div className="exchange-rewards-grid">
        {shop.rewards.map((r) => (
          <Card className="exchange-card exchange-reward" key={r.id}>
            <div className="exchange-toolbar">
              {!r.content && (
                <EveImage
                  kind="type"
                  id={r.type_id}
                  className="exchange-reward-icon"
                />
              )}
              <div className="exchange-event-copy">
                <strong>{r.name || `#${r.type_id}`}</strong>
                <small>
                  {msg("每份")} {number(r.quantity)} {msg("个 · 库存")}{" "}
                  {number(r.stock)} {msg("份")}{" "}
                  {!r.enabled ? msg(" · 未上架") : ""}
                </small>
              </div>
            </div>
            <RewardSummary rewards={r.content} compact />
            <div className="exchange-toolbar exchange-reward-footer">
              <div className="exchange-event-copy">
                <strong className="exchange-reward-price">
                  {shop.isk_per_coin
                    ? msg("{0} 币", coin(r.coins_minor))
                    : msg("待定价")}
                </strong>
                <small>
                  {number(r.isk_value)} {msg("ISK / 份")}
                </small>
              </div>
              <Button
                disabled={
                  formOpen ||
                  !r.enabled ||
                  r.stock < 1 ||
                  shop.isk_per_coin === 0 ||
                  shop.available_minor < r.coins_minor ||
                  context.characters.length === 0
                }
                onClick={() => {
                  remember();
                  close();
                  setClaim({ reward: r, shop });
                }}
              >
                {msg("兑换")}{" "}
              </Button>
            </div>
          </Card>
        ))}
      </div>
      {!shop.rewards.length && (
        <Card className="exchange-card">
          <p className="exchange-note">{msg("暂无可兑换奖励")}</p>
        </Card>
      )}
      {(after || shop.next_cursor) && (
        <div className="exchange-toolbar">
          <Button
            variant="outline"
            disabled={!after}
            onClick={() => setAfter("")}
          >
            {msg("返回首批")}{" "}
          </Button>
          <Button
            variant="outline"
            disabled={!shop.next_cursor}
            onClick={() => setAfter(shop.next_cursor)}
          >
            {msg("更多奖励")}{" "}
          </Button>
        </div>
      )}
      <Card className="exchange-card">
        <div className="exchange-card-heading">
          <h2>{msg("兑换记录")}</h2>
          <div className="exchange-toolbar">
            {" "}
            {shop.admin && !approvalEnabled && (
              <Select
                label={msg("兑换记录范围")}
                value={all ? "all" : "mine"}
                onValueChange={(v) => {
                  setAll(v === "all");
                  setBefore("");
                }}
                options={[
                  { value: "mine", label: msg("我的兑换") },
                  { value: "all", label: msg("全部兑换") },
                ]}
              />
            )}{" "}
            <Button
              variant="outline"
              aria-label={msg("刷新兑换记录")}
              title={msg("刷新兑换记录")}
              disabled={orders.isFetching}
              onClick={() => {
                void orders.refetch();
                void q.refetch();
              }}
            >
              <RefreshCw size={16} />
            </Button>
          </div>
        </div>
        {shop.admin && !approvalEnabled && (
          <p className="exchange-note">
            {msg(
              "在游戏中创建零 ISK 物品交换合同，描述填写兑换单号；完成后自动核对。",
            )}
          </p>
        )}
        {orders.isError ? (
          <div role="alert">
            {orders.error.message}
            <Button onClick={() => void orders.refetch()}>{msg("重试")}</Button>
          </div>
        ) : !orders.data ? (
          <p role="status">{msg("正在读取")}</p>
        ) : (
          <>
            {!orders.data.items.length && (
              <p className="exchange-note">{msg("暂无兑换记录")}</p>
            )}
            {orders.data.items.map((o) => (
              <div className="exchange-order-row" key={o.id}>
                <EveImage
                  kind="type"
                  id={o.type_id}
                  className="exchange-avatar"
                />
                <div className="exchange-event-copy">
                  <strong>
                    {o.name || `#${o.type_id}`} × {o.quantity}
                  </strong>
                  <small>
                    {o.recipient_name} · {coin(o.coins_minor)} {msg("币 ·")}{" "}
                    {formatDate(o.created_at)}
                  </small>
                  {o.note && <small>{o.note}</small>}
                  <DeliveryInfo order={o} />
                  {shop.admin && approvalEnabled && (
                    <Link to={approvalLink("exchange", o.id)}>
                      {msg("前往审批中心")}
                    </Link>
                  )}
                  {shop.admin && !approvalEnabled && (
                    <Button
                      className="exchange-contract-trigger"
                      variant="outline"
                      disabled={formOpen}
                      onClick={() => setHandoff(o)}
                    >
                      {msg("合同信息")}
                    </Button>
                  )}
                  {shop.admin && o.content && (
                    <details className="exchange-delivery-details">
                      <summary>{msg("奖励内容")}</summary>
                      <RewardSummary rewards={o.content} />
                    </details>
                  )}
                  <small>
                    #{o.id} ·{" "}
                    {
                      {
                        pending: msg("待发放"),
                        cancel_requested: msg("取消待审核"),
                        fulfilled: msg("已发放"),
                        cancelled: msg("已取消"),
                      }[o.state]
                    }
                  </small>
                </div>
                {(o.state === "pending" || o.state === "cancel_requested") && (
                  <div className="exchange-toolbar">
                    {o.state === "pending" &&
                      (o.can_request_cancel ?? !all) && (
                        <Button
                          variant="outline"
                          disabled={formOpen}
                          onClick={() => {
                            remember();
                            close();
                            setDecision({
                              order: o,
                              state: "cancel_requested",
                            });
                          }}
                        >
                          {msg("申请取消")}
                        </Button>
                      )}
                    {shop.admin &&
                      !approvalEnabled &&
                      o.state === "cancel_requested" && (
                        <>
                          <Button
                            variant="outline"
                            disabled={formOpen}
                            onClick={() => {
                              remember();
                              close();
                              setDecision({ order: o, state: "cancelled" });
                            }}
                          >
                            {msg("同意取消")}
                          </Button>
                          <Button
                            variant="outline"
                            disabled={formOpen}
                            onClick={() => {
                              remember();
                              close();
                              setDecision({ order: o, state: "pending" });
                            }}
                          >
                            {msg("驳回取消")}
                          </Button>
                        </>
                      )}
                  </div>
                )}
              </div>
            ))}
            {(before || orders.data.next_cursor) && (
              <div className="exchange-toolbar">
                <Button
                  variant="outline"
                  disabled={!before}
                  onClick={() => setBefore("")}
                >
                  {msg("返回最新")}{" "}
                </Button>
                <Button
                  variant="outline"
                  disabled={!orders.data.next_cursor}
                  onClick={() => setBefore(orders.data!.next_cursor)}
                >
                  {msg("更早记录")}{" "}
                </Button>
              </div>
            )}
          </>
        )}
      </Card>
    </div>
  );
}
function ClaimForm({
  csrf,
  quote,
  characters,
  done,
  close,
}: {
  csrf: string;
  quote: { reward: Reward; shop: Shop };
  characters: Context["characters"];
  done: () => void;
  close: () => void;
}) {
  const formId = useId();
  const focusRecipient = useCallback((node: HTMLFormElement | null) => {
    node?.querySelector<HTMLButtonElement>('[role="combobox"]')?.focus();
  }, []);
  const [recipient, setRecipient] = useState(characters[0]?.id ?? "");
  const [key, setKey] = useState(() => crypto.randomUUID());
  const m = useMutation({
    mutationFn: () =>
      write(
        "/rewards/claim",
        csrf,
        {
          reward_id: quote.reward.id,
          reward_version: quote.reward.version,
          rate_version: quote.shop.version,
          recipient_id: recipient,
          request_key: key,
        },
        isClaimed,
      ),
    onSuccess: done,
  });
  return (
    <Modal
      title={msg("确认兑换")}
      size="compact"
      busy={m.isPending}
      close={close}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            disabled={m.isPending}
            onClick={close}
          >
            {msg("取消")}
          </Button>
          <Button
            type="submit"
            form={formId}
            disabled={m.isPending || !recipient}
            aria-busy={m.isPending}
          >
            {m.isPending ? msg("正在提交") : msg("确认兑换")}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        className="exchange-form"
        ref={focusRecipient}
        aria-label={msg("确认兑换")}
        onSubmit={(e) => {
          e.preventDefault();
          if (!m.isPending && recipient) m.mutate();
        }}
      >
        <div className="exchange-decision-summary">
          <strong>
            {quote.reward.name || `#${quote.reward.type_id}`} ×{" "}
            {quote.reward.quantity}
          </strong>
        </div>
        <RewardSummary rewards={quote.reward.content} compact />
        <div className="exchange-refund-summary">
          {msg("本次扣占")}{" "}
          <strong>
            {coin(quote.reward.coins_minor)} {msg("币")}
          </strong>
        </div>
        <div className="exchange-recipient-field">
          <span>{msg("接收角色")}</span>
          <Select
            disabled={m.isPending}
            label={msg("接收角色")}
            value={recipient}
            onValueChange={(v) => {
              setRecipient(v);
              setKey(crypto.randomUUID());
            }}
            options={characters.map((c) => ({ value: c.id, label: c.name }))}
          />
        </div>
        <small>{msg("取消需管理员审核，同意后退回果壳币。")}</small>
        {m.isError && <p role="alert">{m.error.message}</p>}
      </form>
    </Modal>
  );
}
export function DecisionForm({
  csrf,
  decision,
  done,
  close,
}: {
  csrf: string;
  decision: {
    order: Order;
    state: "cancelled" | "cancel_requested" | "pending";
  };
  done: () => void;
  close: () => void;
}) {
  const formId = useId();
  const [note, setNote] = useState("");
  const [undelivered, setUndelivered] = useState(false);
  const action = {
    cancel_requested: {
      title: msg("申请取消"),
      label: msg("取消原因"),
      submit: msg("提交申请"),
    },
    cancelled: {
      title: msg("同意取消"),
      label: msg("核对说明"),
      submit: msg("确认取消"),
    },
    pending: {
      title: msg("驳回取消"),
      label: msg("驳回原因"),
      submit: msg("确认驳回"),
    },
  }[decision.state];
  const [key, setKey] = useState(() => crypto.randomUUID());
  const m = useMutation({
    mutationFn: () =>
      write(
        `/rewards/orders/${decision.order.id}`,
        csrf,
        {
          version: decision.order.version,
          request_key: key,
          state: decision.state,
          ...(decision.state === "cancelled"
            ? { undelivered_confirmed: undelivered }
            : {}),
          note,
        },
        isSaved,
      ),
    onSuccess: done,
  });
  return (
    <Modal
      title={action.title}
      size="compact"
      busy={m.isPending}
      close={close}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            autoFocus={decision.state === "cancelled"}
            disabled={m.isPending}
            onClick={close}
          >
            {msg("取消")}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant={decision.state === "cancelled" ? "destructive" : "default"}
            disabled={
              m.isPending ||
              !note.trim() ||
              (decision.state === "cancelled" && !undelivered)
            }
            aria-busy={m.isPending}
          >
            {m.isPending ? msg("正在提交") : action.submit}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        className="exchange-form exchange-decision-form"
        aria-label={msg("处理兑换")}
        onSubmit={(e) => {
          e.preventDefault();
          if (
            !m.isPending &&
            note.trim() &&
            (decision.state !== "cancelled" || undelivered)
          )
            m.mutate();
        }}
      >
        <div className="exchange-decision-summary">
          <strong>
            {decision.order.name || `#${decision.order.type_id}`} ×{" "}
            {decision.order.quantity}
          </strong>
          <span>
            #{decision.order.id} · {decision.order.recipient_name}
          </span>
        </div>
        {decision.state === "cancelled" && (
          <div className="exchange-refund-summary">
            <span>{msg("退还果壳币")}</span>
            <strong>{coin(decision.order.coins_minor)}</strong>
          </div>
        )}
        <label>
          {action.label}
          <textarea
            autoFocus={decision.state !== "cancelled"}
            rows={3}
            required
            maxLength={200}
            value={note}
            disabled={m.isPending}
            onChange={(e) => {
              setNote(e.target.value);
              setKey(crypto.randomUUID());
            }}
          />
        </label>
        {decision.state === "cancel_requested" && (
          <small>{msg("审核期间保留果壳币占用和库存预留。")}</small>
        )}
        {decision.state === "cancelled" && (
          <label className="exchange-cancel-check">
            <input
              type="checkbox"
              required
              checked={undelivered}
              disabled={m.isPending}
              onChange={(e) => {
                setUndelivered(e.target.checked);
                setKey(crypto.randomUUID());
              }}
            />
            {msg("已核实未交付，相关合同已取消或不存在")}
          </label>
        )}
        {m.isError && <p role="alert">{m.error.message}</p>}
      </form>
    </Modal>
  );
}

export function DeliveryInfo({
  order,
  copyReference = true,
}: {
  order: Order;
  copyReference?: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  const reference = order.reference ?? `GNV-EX-${order.id}`;
  const labels: Record<string, string> = {
    waiting_contract: msg("等待交付合同"),
    waiting_items: msg("等待合同明细"),
    awaiting_acceptance: msg("等待领取合同"),
    fulfilled: msg("合同已核对"),
    mismatch: msg("合同内容不符"),
    multiple_contracts: msg("存在多个交付合同"),
    issuer_unverified: msg("发放人未通过核验"),
    contract_claimed: msg("合同已用于其他发放"),
    evidence_unavailable: msg("合同证据暂不可用"),
  };
  return (
    <div className="exchange-delivery-info">
      {copyReference ? (
        <Button
          variant="ghost"
          title={msg("复制合同结算 ID")}
          aria-label={msg("复制合同结算 ID")}
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(reference);
              setCopied(true);
              setFailed(false);
            } catch {
              setFailed(true);
            }
          }}
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
          <code>{reference}</code>
        </Button>
      ) : (
        <code>{reference}</code>
      )}
      {copied && <span role="status">{msg("已复制")}</span>}
      {failed && <span role="alert">{msg("复制失败，请手动复制")}</span>}
      {order.state !== "cancelled" && order.delivery && (
        <small>{labels[order.delivery.status] ?? msg("等待核对")}</small>
      )}
      {!!order.delivery?.contracts.length && (
        <small>
          {msg("合同")}{" "}
          {order.delivery.contracts.map((c) => `#${c.id}`).join(" · ")}
        </small>
      )}
    </div>
  );
}
