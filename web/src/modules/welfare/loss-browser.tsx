import { apiFetch } from "@/lib/http";
import { getLocale, msg } from "@/lib/i18n";
import { gameTerm } from "@/lib/eve-terminology";
import { useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  RefreshCw,
  ChevronRight,
  ArrowLeft,
  MapPin,
  CircleCheck,
  Clock3,
  ShieldPlus,
  CircleMinus,
  Crosshair,
} from "lucide-react";
import { Select } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import { getData } from "@/lib/http";
import * as api from "./api";

const unavailableReasons: Record<string, string> = {
  policy_missing: msg("补损规则未配置"),
  policy_disabled: msg("补损规则未启用"),
  policy_incomplete: msg("补损规则待完善"),
  policy_not_effective: msg("补损规则尚未生效"),
  loss_before_policy: msg("损失早于规则生效"),
  wrong_corporation: msg("损失时不属本军团"),
  future_loss: msg("损失时间异常"),
};

function LossStatus({ loss }: { loss: api.Loss }) {
  const r = loss.reimbursement;
  if (!r) return null;
  const Icon =
    r.status === "completed"
      ? CircleCheck
      : r.status === "processing"
        ? Clock3
        : r.status === "available"
          ? ShieldPlus
          : CircleMinus;
  const label =
    r.status === "completed"
      ? msg("已补损")
      : r.status === "processing"
        ? api.states[r.state] || msg("申请处理中")
        : r.status === "available"
          ? r.available_kinds.includes("srp")
            ? msg("可申请补损")
            : msg("可申请PVP补损")
          : unavailableReasons[r.reason || ""] || msg("暂不可申请");
  return (
    <span
      className={`loss-reimbursement loss-${r.status}`}
      title={
        r.kind
          ? api.kinds[r.kind]
          : r.status === "available"
            ? msg("已绑定角色可提交，由管理员人工审核")
            : label
      }
    >
      <Icon size={14} aria-hidden="true" />
      {label}
    </span>
  );
}

type LossItem = api.Loss["items"][number];
type SlotGroup = "high" | "medium" | "low" | "rig" | "subsystem" | "service" | "drone_bay" | "fighter_bay" | "cargo" | "other";

const slotOrder: SlotGroup[] = ["high", "medium", "low", "rig", "subsystem", "service", "drone_bay", "fighter_bay", "cargo", "other"];

// ESI killmail item flags are numeric; nested charges keep the parent module's group.
function lossSlotGroup(slot: string): SlotGroup {
  const root = slot.split("/")[0];
  const flag = Number(root);
  if (/^\d+$/.test(root)) {
    if (flag >= 27 && flag <= 34) return "high";
    if (flag >= 19 && flag <= 26) return "medium";
    if (flag >= 11 && flag <= 18) return "low";
    if (flag >= 92 && flag <= 99) return "rig";
    if (flag >= 125 && flag <= 132) return "subsystem";
    if (flag === 87) return "drone_bay";
    if (flag === 5) return "cargo";
  }
  if (/^HiSlot\d*$/.test(root)) return "high";
  if (/^MedSlot\d*$/.test(root)) return "medium";
  if (/^LoSlot\d*$/.test(root)) return "low";
  if (/^RigSlot\d*$/.test(root)) return "rig";
  if (/^SubSystemSlot\d*$/.test(root)) return "subsystem";
  if (/^ServiceSlot\d*$/.test(root)) return "service";
  if (root === "DroneBay") return "drone_bay";
  if (root === "FighterBay") return "fighter_bay";
  if (root === "Cargo" || root === "cargo") return "cargo";
  return "other";
}

function itemPrice(mid: string | null | undefined, item: LossItem, kind: "dropped" | "destroyed") {
  const parsed = mid && /^(\d+)\.(\d{2})$/.exec(mid);
  if (!parsed || item.quantity <= 0 || item.dropped + item.destroyed !== item.quantity) return "—";
  const total = BigInt(parsed[1]) * 100n + BigInt(parsed[2]);
  const dropped = total * BigInt(item.dropped) / BigInt(item.quantity);
  const cents = kind === "dropped" ? dropped : total - dropped;
  const whole = new Intl.NumberFormat(getLocale()).format(cents / 100n);
  const fraction = cents % 100n;
  return fraction ? `${whole}.${fraction.toString().padStart(2, "0")}` : whole;
}

export function LossItems({ loss, prices, priceError = false, priceLoading = false }: { loss: api.Loss; prices?: api.LossPrice[] | null; priceError?: boolean; priceLoading?: boolean }) {
  if (!loss.items.length) return null;
  const grouped = new Map<SlotGroup, { item: LossItem; index: number }[]>();
  for (const [index, item] of loss.items.entries()) {
    const group = lossSlotGroup(item.slot);
    const rows = grouped.get(group);
    if (rows) rows.push({ item, index });
    else grouped.set(group, [{ item, index }]);
  }
  const showPrices = prices !== undefined;
  return (
    <section className="welfare-km-items" aria-label={msg("掉落与损毁")}>
      <div className="welfare-km-items-head">
        <strong>{msg("掉落与损毁")}</strong>
        <span>{loss.items.length} {msg("项")}{priceError ? ` · ${msg("价格暂不可用")}` : priceLoading ? ` · ${msg("核价中")}` : ""}</span>
      </div>
      <div className="welfare-km-item-row welfare-columns" data-price={showPrices}>
        <span>{msg("物品")}</span>
        <span>{msg("数量")}</span>
        {showPrices && <span title={msg("吉他 4-4 中间价（ISK）")}>{msg("参考价")}</span>}
      </div>
      {slotOrder.filter((group) => grouped.has(group)).map((group) => (
        <div className="welfare-km-slot" key={group}>
          <div className="welfare-km-slot-title">
            {group === "other" ? msg("其他物品") : gameTerm("slots", group)}
          </div>
          {grouped.get(group)!.flatMap(({ item, index }) =>
            ([{ kind: "dropped" as const, quantity: item.dropped }, { kind: "destroyed" as const, quantity: item.destroyed }])
              .filter((part) => part.quantity > 0)
              .map((part) => (
                <div className={`welfare-km-item-row welfare-km-item-${part.kind}${item.slot.includes("/") ? " welfare-km-item-nested" : ""}`} data-price={showPrices} key={`${index}-${part.kind}`}>
                  <span className="welfare-km-item-name">
                    <EveImage kind="type" id={item.type_id} />
                    <span><span>{item.name}</span><small>{msg(part.kind === "dropped" ? "掉落" : "损毁")}</small></span>
                  </span>
                  <span className="welfare-km-item-quantity">{part.quantity.toLocaleString(getLocale())}</span>
                  {showPrices && <span className="welfare-km-item-price" title={prices?.[index]?.observed_at ? api.date(prices[index].observed_at!) : undefined}>{itemPrice(prices?.[index]?.mid, item, part.kind)}</span>}
                </div>
              )),
          )}
        </div>
      ))}
    </section>
  );
}

export function LossCombatAndItems({ loss, prices, priceError, priceLoading }: { loss: api.Loss; prices?: api.LossPrice[] | null; priceError?: boolean; priceLoading?: boolean }) {
  return (
    <div className="welfare-km-layout" data-dual={Boolean(loss.attackers && loss.items.length)}>
      <LossAttackers loss={loss} />
      <LossItems loss={loss} prices={prices} priceError={priceError} priceLoading={priceLoading} />
    </div>
  );
}

export function LossEvidence({ loss, pilotName, showHeader = true, prices, priceError, priceLoading }: { loss: api.Loss; pilotName?: string; showHeader?: boolean; prices?: api.LossPrice[] | null; priceError?: boolean; priceLoading?: boolean }) {
  return (
    <div className="welfare-loss-evidence">
      {showHeader && (
        <header className="welfare-km-header">
          <div className="welfare-km-portraits">
            <EveImage kind="character" id={loss.character_id} />
            <EveImage kind="type" id={loss.ship_type_id} variation="render" />
          </div>
          <div className="welfare-km-identity">
            <strong>{loss.ship_name}</strong>
            {pilotName && <span>{pilotName}</span>}
            <div className="welfare-km-facts">
              <span>KM #{loss.id}</span>
              <span><MapPin size={14} aria-hidden="true" />{loss.solar_system_name}</span>
              <time dateTime={loss.occurred_at}>{api.date(loss.occurred_at)}</time>
            </div>
          </div>
        </header>
      )}
      <LossCombatAndItems loss={loss} prices={prices} priceError={priceError} priceLoading={priceLoading} />
    </div>
  );
}

export function LossAttackers({ loss }: { loss: api.Loss }) {
  const [page, setPage] = useState({ id: loss.id, count: 5 });
  if (!loss.attackers) {
    return (
      <section className="welfare-km-attackers" aria-label={msg("参与者")}>
        <div className="welfare-km-attackers-head"><strong>{msg("参与者")}</strong></div>
        <p className="welfare-km-attackers-empty">{msg("参与者资料尚未同步")}</p>
      </section>
    );
  }
  const attackers = [...loss.attackers].sort((a, b) => b.damage_done - a.damage_done);
  const finalIndex = attackers.findIndex((a) => a.final_blow);
  if (finalIndex > 0) attackers.unshift(attackers.splice(finalIndex, 1)[0]);
  const count = page.id === loss.id ? page.count : 5;
  const visible = attackers.slice(0, count);
  const totalDamage = attackers.reduce((sum, attacker) => sum + attacker.damage_done, 0);
  return (
    <section className="welfare-km-attackers" aria-label={msg("参与者")}>
      <div className="welfare-km-attackers-head">
        <strong>{msg("参与者")} ({attackers.length})</strong>
        {loss.damage_taken !== undefined && (
          <span>{msg("承受伤害")} {loss.damage_taken.toLocaleString()}</span>
        )}
      </div>
      {visible.map((attacker, index) => {
        const name = attacker.name || (attacker.character_id ? `${msg("角色")} #${attacker.character_id}` : msg("NPC"));
        const share = totalDamage > 0 ? Math.min(100, Math.round(attacker.damage_done / totalDamage * 1000) / 10) : 0;
        return (
          <div className="welfare-km-attacker" key={`${attacker.character_id || attacker.corporation_id || "npc"}-${index}`}>
            <div className="welfare-km-attacker-portraits">
              <EveImage kind="character" id={attacker.character_id || ""} />
              {attacker.ship_type_id && <EveImage kind="type" id={attacker.ship_type_id} />}
            </div>
            <div className="welfare-km-attacker-body">
              <div className="welfare-km-attacker-title">
                <strong>{name}</strong>
                {attacker.final_blow && <span><Crosshair size={12} aria-hidden="true" />{msg("最后一击")}</span>}
              </div>
              {(attacker.corporation_name || attacker.alliance_name) && (
                <small>{[attacker.corporation_name, attacker.alliance_name].filter(Boolean).join(" · ")}</small>
              )}
              {(attacker.ship_name || attacker.weapon_name) && (
                <small>{[attacker.ship_name, attacker.weapon_name].filter(Boolean).join(" · ")}</small>
              )}
            </div>
            <span className="welfare-km-attacker-damage">
              <strong>{attacker.damage_done.toLocaleString()}</strong>
              <small>{share}%</small>
            </span>
          </div>
        );
      })}
      {attackers.length > 5 && (
        <div className="welfare-km-attackers-actions">
          {count < attackers.length && (
            <Button type="button" variant="outline" size="sm" onClick={() => setPage({ id: loss.id, count: Math.min(count + 20, attackers.length) })}>
              {msg("显示更多 · {0}/{1}", visible.length, attackers.length)}
            </Button>
          )}
          {count > 5 && (
            <Button type="button" variant="ghost" size="sm" onClick={() => setPage({ id: loss.id, count: 5 })}>
              {msg("收起")}
            </Button>
          )}
        </div>
      )}
    </section>
  );
}

export function LossBrowser({
  corp,
  csrf,
  characters,
  select,
  rulesAction,
}: {
  corp: string;
  csrf: string;
  characters: api.Character[];
  select: (v: api.Loss, kind: string) => void;
  rulesAction?: ReactNode;
}) {
  const chars = characters.filter((c) => c.corporation_id === corp);
  const [character, setCharacter] = useState(chars[0]?.id || "");
  const [before, setBefore] = useState("");
  const [id, setID] = useState("");
  const [kind, setKind] = useState("srp");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const q = useQuery({
    queryKey: ["welfare", "losses", corp, character, before, id],
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/welfare/losses?${new URLSearchParams({ corporation_id: corp, character_id: character, before: id ? "" : before, killmail_id: id })}`,
        api.isLossList,
        signal,
      ),
    enabled: !!character,
    refetchInterval: 15000,
  });
  const selected = id ? q.data?.items.find((i) => i.id === id) : undefined;
  const priceQuery = useQuery({
    queryKey: ["welfare", "loss-prices", corp, character, id, selected?.observed_at],
    queryFn: async ({ signal }) => {
      const quote = await getData(
        `/api/v1/welfare/losses/prices?${new URLSearchParams({ corporation_id: corp, character_id: character, killmail_id: id })}`,
        api.isLossPrices,
        signal,
      );
      if (quote.items.length !== selected?.items.length) throw new Error(msg("服务响应格式异常"));
      return quote;
    },
    enabled: Boolean(selected?.items.length),
    staleTime: 6 * 60 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: 1,
  });
  const eligibleKinds = (selected?.reimbursement?.available_kinds || []).filter(
    (k) => api.activeLossKinds.includes(k),
  );
  const selectedKind = eligibleKinds.includes(kind)
    ? kind
    : eligibleKinds[0] || "";
  const refresh = async () => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const r = await apiFetch(
        `/api/v1/eve/sync/characters/${character}/refresh`,
        {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
          body: JSON.stringify({ resource: "killmails" }),
        },
      );
      const v = await r.json();
      if (!r.ok) throw new Error(v.error?.message || msg("同步请求失败"));
      setMessage(msg("已请求同步"));
      await q.refetch();
    } catch (e) {
      setError(e instanceof Error ? e.message : msg("同步请求失败"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="welfare-loss-browser" aria-label={msg("损失记录")}>
      <div className="welfare-loss-toolbar">
        <Select
          label={msg("损失角色")}
          value={character}
          onValueChange={(v) => {
            setCharacter(v);
            setBefore("");
            setID("");
            setMessage("");
            setError("");
          }}
          options={chars.map((c) => ({ value: c.id, label: c.name }))}
        />
        <IconAction
          label={msg("同步击毁记录")}
          disabled={!character || busy}
          onClick={() => void refresh()}
        >
          <RefreshCw size={18} />
        </IconAction>
        {rulesAction}
      </div>
      {message && <small role="status">{message}</small>}
      {error && <p role="alert">{error}</p>}
      {q.isError ? (
        <p role="alert">
          {q.error.message}
          <Button variant="outline" onClick={() => void q.refetch()}>
            {msg("重试")}{" "}
          </Button>
        </p>
      ) : !character ? (
        <p>{msg("没有可用角色")}</p>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : id ? (
        <>
          <Button variant="outline" onClick={() => setID("")}>
            <ArrowLeft size={16} />
            {msg("返回列表")}{" "}
          </Button>
          {selected ? (
            <>
              <LossEvidence loss={selected} pilotName={chars.find((c) => c.id === selected.character_id)?.name} prices={priceQuery.data?.items ?? null} priceError={priceQuery.isError} priceLoading={priceQuery.isPending} />
              <LossStatus loss={selected} />
              {selected.reimbursement?.attendance_event_id &&
              selected.reimbursement.attendance_event_id !== "0" ? (
                <small>
                  {msg("关联出勤 #")}
                  {selected.reimbursement.attendance_event_id}
                </small>
              ) : null}
              {eligibleKinds.length > 0 ? (
                <div className="welfare-loss-apply">
                  <Select
                    label={msg("补损类型")}
                    value={selectedKind}
                    onValueChange={setKind}
                    options={eligibleKinds.map((value) => ({
                      value,
                      label: api.kinds[value],
                    }))}
                  />
                  <Button onClick={() => select(selected, selectedKind)}>
                    {msg("申请补损")}{" "}
                  </Button>
                </div>
              ) : null}
            </>
          ) : (
            <p>{msg("记录已不可用")}</p>
          )}
        </>
      ) : (
        <>
          {!q.data.items.length ? (
            <p>{msg("暂无已同步的损失记录")}</p>
          ) : (
            <div className="welfare-loss-list">
              {q.data.items.map((l) => (
                <button
                  className="welfare-loss-row"
                  key={l.id}
                  onClick={() => setID(l.id)}
                >
                  <EveImage kind="type" id={l.ship_type_id} variation="render" />
                  <span className="welfare-loss-row-content">
                    <span className="welfare-loss-row-heading">
                      <strong>{l.ship_name}</strong>
                      <small>KM #{l.id}</small>
                    </span>
                    <span className="welfare-loss-row-facts">
                      <small><MapPin size={13} aria-hidden="true" />{l.solar_system_name}</small>
                      <time dateTime={l.occurred_at}>{api.date(l.occurred_at)}</time>
                    </span>
                    <LossStatus loss={l} />
                  </span>
                  <ChevronRight size={16} />
                </button>
              ))}
            </div>
          )}
          {(before || q.data.next_cursor) && (
            <div className="welfare-paging">
              <Button
                variant="outline"
                disabled={!before}
                onClick={() => setBefore("")}
              >
                {msg("最新记录")}{" "}
              </Button>
              <Button
                variant="outline"
                disabled={!q.data.next_cursor}
                onClick={() => setBefore(q.data!.next_cursor)}
              >
                {msg("更早记录")}{" "}
              </Button>
            </div>
          )}
        </>
      )}
    </section>
  );
}
