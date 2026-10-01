import { msg } from "@/lib/i18n";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Check,
  Search,
  RefreshCw,
  Package,
  ArrowLeft,
  Link2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Modal } from "@/components/ui/dialog";
import { EveImage } from "@/components/eve-image";
import { contractStatuses } from "@/modules/eve/contracts-api";
import * as api from "./api";

export function DeliverySummary({
  contract,
  recipient,
  isk = false,
}: {
  contract: api.DeliveryContract;
  recipient: string;
  isk?: boolean;
}) {
  const c = contract;
  return (
    <div className="welfare-contract-summary">
      <div className="welfare-contract-heading">
        <Link2 size={18} aria-hidden="true" />
        <strong>
          {msg("合同 #")}
          {c.id}
        </strong>
        <span>{contractStatuses[c.status] || c.status}</span>
      </div>
      {c.title && <p>{c.title}</p>}
      <dl className="welfare-contract-values">
        <div>
          <dt>{msg("发放方")}</dt>
          <dd>
            {c.issuer_name ||
              (c.for_corporation
                ? msg("军团 #{0}", c.issuer_corporation_id)
                : msg("角色 #{0}", c.issuer_id))}
          </dd>
        </div>
        <div>
          <dt>{msg("接收人")}</dt>
          <dd>{recipient}</dd>
        </div>
        <div>
          <dt>{isk ? msg("补贴金额") : msg("合同价格")}</dt>
          <dd>{(isk ? c.reward : c.price) || "—"} ISK</dd>
        </div>
        <div>
          <dt>{msg("合同类型")}</dt>
          <dd>{msg("物品交换")}</dd>
        </div>
        <div>
          <dt>{msg("创建时间")}</dt>
          <dd>{api.date(c.issued)}</dd>
        </div>
        <div>
          <dt>{msg("同步时间")}</dt>
          <dd>{api.date(c.checked_at)}</dd>
        </div>
      </dl>
      {c.items.length > 0 && (
        <details open>
          <summary>
            {msg("交付物品 ·")} {c.items.length}
          </summary>
          <ul className="welfare-contract-items">
            {c.items.map((i, n) => (
              <li key={n}>
                <EveImage kind="type" id={i.type_id} />
                <span className="welfare-contract-item-text">
                  {i.name || msg("物品 #{0}", i.type_id)}
                  <small>
                    {i.included ? msg("交付") : msg("要求成员提供")}
                  </small>
                </span>
                <strong>× {i.quantity}</strong>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}
export function DeliveryContracts({
  item,
  csrf,
  close,
  done,
}: {
  item: api.Case;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const [text, setText] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<api.DeliveryContract | null>(null);
  const [checked, setChecked] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [requestKey, setRequestKey] = useState(() => crypto.randomUUID());
  const q = useQuery({
    queryKey: ["welfare", "delivery-candidates", item.id, search],
    queryFn: ({ signal }) => api.deliveryCandidates(item.id, search, signal),
  });
  const reset = () => {
    setRequestKey(crypto.randomUUID());
    setError("");
  };
  async function confirm() {
    if (!selected || !checked || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.post("commands", csrf, {
        action: "link_delivery",
        id: item.id,
        version: item.version,
        request_key: requestKey,
        note: note.trim() || msg("已核对合同收发双方与交付内容"),
        delivery: {
          owner_kind: selected.owner_kind,
          owner_id: selected.owner_id,
          contract_id: selected.id,
          content_token: selected.content_token,
        },
      });
      done();
    } catch (e) {
      setError(e instanceof Error ? e.message : msg("关联失败，请重试"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title={selected ? msg("确认交付合同") : msg("关联交付合同")}
      close={close}
      busy={busy}
      footer={
        <>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() =>
              selected
                ? (setSelected(null), setChecked(false), reset())
                : close()
            }
          >
            {selected ? (
              <>
                <ArrowLeft size={16} />
                {msg("返回")}{" "}
              </>
            ) : (
              msg("取消")
            )}
          </Button>
          {selected && (
            <Button disabled={busy || !checked} onClick={confirm}>
              {busy ? (
                msg("正在关联")
              ) : (
                <>
                  <Check size={16} />
                  {msg("确认关联")}{" "}
                </>
              )}
            </Button>
          )}
        </>
      }
    >
      {selected ? (
        <>
          <DeliverySummary
            contract={selected}
            recipient={item.detail.character_name}
            isk={["supercarrier", "titan"].includes(item.kind)}
          />
          <label className="welfare-check welfare-delivery-confirm">
            <input
              type="checkbox"
              checked={checked}
              disabled={busy}
              onChange={(e) => {
                setChecked(e.target.checked);
                reset();
              }}
            />
            {msg("已核对收发双方、金额及全部交付物品")}{" "}
          </label>
          <label className="welfare-field">
            <span>{msg("备注（选填）")}</span>
            <textarea
              maxLength={1000}
              disabled={busy}
              value={note}
              onChange={(e) => {
                setNote(e.target.value);
                reset();
              }}
            />
          </label>
          <p className="welfare-contract-hint">
            {msg("合同完成并确认领取角色后，自动标记已发放。")}{" "}
          </p>
        </>
      ) : (
        <>
          <form
            className="welfare-contract-search"
            onSubmit={(e) => {
              e.preventDefault();
              setSearch(text.trim());
            }}
          >
            <input
              aria-label={msg("合同 ID")}
              placeholder={msg("输入合同 ID")}
              value={text}
              inputMode="numeric"
              pattern="[0-9]*"
              onChange={(e) => setText(e.target.value)}
            />
            <IconAction
              label={msg("查找合同")}
              type="submit"
              disabled={q.isFetching}
            >
              <Search size={18} />
            </IconAction>
            <IconAction
              label={msg("刷新推荐")}
              onClick={() => {
                setText("");
                if (search) setSearch("");
                else void q.refetch();
              }}
              disabled={q.isFetching}
            >
              <RefreshCw size={18} />
            </IconAction>
          </form>
          {q.isPending && <p role="status">{msg("正在查找合同")}</p>}
          {q.isError && <p role="alert">{q.error.message}</p>}
          {!q.isError && q.data?.items.length === 0 && (
            <p className="welfare-contract-empty">
              <Package size={24} />
              {msg("暂无匹配合同，可输入已同步的合同 ID 查找。")}{" "}
            </p>
          )}
          {!q.isError && q.data && (
            <div className="welfare-contract-options">
              {q.data.items.map(({ contract: c, can_link, reason }) => (
                <div className="welfare-contract-option" key={c.id}>
                  <div>
                    <strong>{c.title || msg("物品交换 #{0}", c.id)}</strong>
                    <span>
                      #{c.id} · {contractStatuses[c.status] || c.status}
                    </span>
                    <span>
                      {api.date(c.issued)} · {c.items.length}{" "}
                      {msg("项物品")}{" "}
                    </span>
                    {reason && <p>{reason}</p>}
                  </div>
                  <Button
                    variant="outline"
                    disabled={!can_link}
                    onClick={() => {
                      setSelected(c);
                      setChecked(false);
                      reset();
                    }}
                  >
                    {msg("核对")}{" "}
                  </Button>
                </div>
              ))}
            </div>
          )}
        </>
      )}
      {error && <p role="alert">{error}</p>}
    </Modal>
  );
}
