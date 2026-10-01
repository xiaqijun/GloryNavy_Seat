import { assetSlotLabel } from "@/lib/eve-terminology";
import { msg, getLocale } from "@/lib/i18n";
import { useState, useRef } from "react";
import { StarLocation } from "./location";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronDown,
  Crosshair,
  RefreshCw,
  Rocket,
  Check,
  X,
} from "lucide-react";
import { EveImage } from "@/components/eve-image";
import { FormDialog } from "@/components/ui/form-dialog";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import {
  getBattle,
  isSaved,
  evidenceStates,
  type BattleItem,
  type Loss,
} from "./battle-api";
import { formatDate, write, type Entry, type Event } from "./api";

export function BattlePanel({
  user,
  csrf,
  event,
  entry,
}: {
  user: string;
  csrf: string;
  event: Event;
  entry: Entry;
}) {
  const [open, setOpen] = useState(false);
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ["attendance", "battle", user, event.id, entry.character_id],
    queryFn: ({ signal }) => getBattle(event.id, entry.character_id, signal),
    enabled: open,
    refetchInterval: open ? 10000 : false,
  });
  const refresh = useMutation({
    mutationFn: () =>
      write(
        `/events/${event.id}/characters/${entry.character_id}/battle/refresh`,
        csrf,
        { version: event.version, request_key: crypto.randomUUID() },
        isSaved,
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: ["attendance"] }),
  });
  const lossTask = q.data?.tasks.find((t) => t.kind === "losses");
  const target = `battle-${event.id}-${entry.character_id}`;
  return (
    <div className="attendance-battle">
      <button
        className="attendance-battle-toggle"
        aria-expanded={open}
        aria-controls={target}
        onClick={() => setOpen((v) => !v)}
      >
        <Rocket size={16} aria-hidden="true" />
        <span>{msg("舰船与损失")}</span>
        <ChevronDown
          size={16}
          aria-hidden="true"
          className={open ? "is-open" : ""}
        />
      </button>
      {open && (
        <div id={target} className="attendance-battle-content">
          {q.isPending ? (
            <p role="status">{msg("正在读取记录")}</p>
          ) : q.isError ? (
            <div role="alert">
              {q.error.message}
              <IconAction
                label={msg("重试读取记录")}
                onClick={() => void q.refetch()}
              >
                <RefreshCw />
              </IconAction>
            </div>
          ) : (
            <>
              <section aria-label={msg("{0} 的舰船快照", entry.name)}>
                <h3>
                  <Rocket size={17} aria-hidden="true" />
                  {msg("舰船快照")}{" "}
                </h3>
                {q.data.ships.length === 0 ? (
                  <p className="attendance-note">{msg("尚无舰船快照")}</p>
                ) : (
                  q.data.ships.map((s) => (
                    <details key={s.id} className="attendance-evidence">
                      <summary>
                        <EveImage
                          kind="type"
                          id={s.ship_type_id}
                          className="attendance-ship-icon"
                        />
                        <span>
                          <strong>{s.ship_name}</strong>
                          <small>{formatDate(s.observed_at)}</small>
                          <StarLocation
                            id={s.solar_system_id}
                            name={s.solar_system_name}
                          />
                        </span>
                        <span className="attendance-evidence-status">
                          {evidenceStates[s.state] ?? msg("采集异常")}
                        </span>
                      </summary>
                      <div className="attendance-evidence-body">
                        {s.joined_at && (
                          <p className="attendance-note">
                            {msg("入队")} {formatDate(s.joined_at)}
                          </p>
                        )}
                        {s.fitting ? (
                          <>
                            <p className="attendance-note">
                              {msg("资产快照")}{" "}
                              {formatDate(s.fitting.assets_observed_at)}{" "}
                              {msg("· 可能早于点名时的实际装配")}{" "}
                            </p>
                            <Items items={s.fitting.items} />
                          </>
                        ) : (
                          <p className="attendance-note">
                            {s.state === "pending"
                              ? msg("后台正在读取角色资产")
                              : (evidenceStates[s.state] ??
                                msg("暂无可用装配"))}
                          </p>
                        )}
                      </div>
                    </details>
                  ))
                )}
              </section>
              <section aria-label={msg("{0} 的活动损失", entry.name)}>
                <div className="attendance-toolbar">
                  <h3>
                    <Crosshair size={17} aria-hidden="true" />
                    {msg("损失记录")}{" "}
                  </h3>
                  {event.can_manage && (
                    <IconAction
                      label={msg("同步 {0} 的损失", entry.name)}
                      disabled={refresh.isPending}
                      onClick={() => refresh.mutate()}
                    >
                      <RefreshCw />
                    </IconAction>
                  )}
                </div>
                {refresh.isError && <p role="alert">{refresh.error.message}</p>}
                {lossTask && (
                  <p className="attendance-note">
                    {lossTask.reason
                      ? (evidenceStates[lossTask.reason] ?? msg("采集异常"))
                      : lossTask.checked_at
                        ? msg("最近检查 {0}", formatDate(lossTask.checked_at))
                        : msg("等待首次同步")}
                  </p>
                )}
                {q.data.losses.length === 0 ? (
                  <p className="attendance-note">
                    {lossTask?.checked_at && !lossTask.reason
                      ? msg("暂未发现活动时段内的损失")
                      : msg("暂无已采集的损失记录")}
                  </p>
                ) : (
                  q.data.losses.map((l) => (
                    <LossRecord key={l.id} loss={l} event={event} csrf={csrf} />
                  ))
                )}
              </section>
              {q.data.truncated && (
                <p className="attendance-note">
                  {msg("显示最近 20 次舰船快照与 100 条损失。")}{" "}
                </p>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
function Items({
  items,
  loss = false,
}: {
  items: BattleItem[];
  loss?: boolean;
}) {
  return items.length ? (
    <ul className="attendance-fit-items">
      {items.map((i, index) => (
        <li key={`${i.type_id}-${i.slot}-${index}`}>
          <EveImage
            kind="type"
            id={i.type_id}
            className="attendance-item-icon"
          />
          <span>
            <strong>{i.name}</strong>
            <small>{slotName(i.slot, loss)}</small>
          </span>
          <span className="attendance-item-quantity">
            {loss
              ? msg(
                  "毁 {0} · 掉 {1}",
                  i.destroyed.toLocaleString(getLocale()),
                  i.dropped.toLocaleString(getLocale()),
                )
              : `× ${i.quantity.toLocaleString(getLocale())}`}
          </span>
        </li>
      ))}
    </ul>
  ) : (
    <p className="attendance-note">{msg("报告未提供装备明细")}</p>
  );
}
function slotName(slot: string, loss: boolean) {
  return loss ? msg("物品位置 {0}", slot) : assetSlotLabel(slot, true);
}

function LossRecord({
  loss,
  event,
  csrf,
}: {
  loss: Loss;
  event: Event;
  csrf: string;
}) {
  const [review, setReview] = useState<"confirmed" | "rejected" | null>(null);
  const [reason, setReason] = useState("");
  const [requestKey, setRequestKey] = useState(() => crypto.randomUUID());
  const [version, setVersion] = useState(loss.version);
  const summary = useRef<HTMLElement>(null);
  const beginReview = (state: "confirmed" | "rejected") => {
    m.reset();
    setReason("");
    setReview(state);
    setVersion(loss.version);
    setRequestKey(crypto.randomUUID());
  };
  const client = useQueryClient();
  const m = useMutation({
    mutationFn: () =>
      write(
        `/events/${event.id}/losses/${loss.id}/review`,
        csrf,
        {
          version,
          request_key: requestKey,
          state: review,
          reason,
        },
        isSaved,
      ),
    onSuccess: () => {
      setReview(null);
      setReason("");
      summary.current?.focus();
      void client.invalidateQueries({ queryKey: ["attendance"] });
    },
  });
  return (
    <details className="attendance-evidence">
      <summary ref={summary}>
        <EveImage
          kind="type"
          id={loss.ship_type_id}
          className="attendance-ship-icon"
        />
        <span>
          <strong>{loss.ship_name}</strong>
          <small>{formatDate(loss.occurred_at)}</small>
          <StarLocation
            id={loss.solar_system_id}
            name={loss.solar_system_name}
            label={msg("损失星系")}
          />
        </span>
        <span className={`attendance-evidence-status loss-${loss.state}`}>
          {
            {
              candidate: msg("待确认"),
              confirmed: msg("活动损失"),
              rejected: msg("已排除"),
            }[loss.state]
          }
        </span>
      </summary>
      <div className="attendance-evidence-body">
        <p className="attendance-note">
          {msg("击毁报告 #")}
          {loss.id}
        </p>
        <Items items={loss.items} loss />
        {event.can_manage && (
          <div className="attendance-loss-review">
            <div className="attendance-toolbar">
              {loss.state !== "confirmed" && (
                <Button
                  variant="outline"
                  onClick={() => beginReview("confirmed")}
                >
                  <Check />
                  {msg("计入活动")}{" "}
                </Button>
              )}
              {loss.state !== "rejected" && (
                <Button variant="ghost" onClick={() => beginReview("rejected")}>
                  <X />
                  {msg("排除")}{" "}
                </Button>
              )}
            </div>
            {review && (
              <FormDialog
                title={review === "confirmed" ? msg("计入活动") : msg("排除")}
                close={() => setReview(null)}
                busy={m.isPending}
                disabled={!reason.trim()}
                destructive={review === "rejected"}
                submitLabel={
                  review === "confirmed" ? msg("确认计入") : msg("确认排除")
                }
                className="attendance-form"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!m.isPending) m.mutate();
                }}
              >
                <div className="attendance-event-copy">
                  <strong>{loss.ship_name}</strong>
                  <small>
                    {msg("击毁报告 #")}
                    {loss.id} · {formatDate(loss.occurred_at)}
                  </small>
                </div>
                <label>
                  {msg("确认原因")}{" "}
                  <textarea
                    rows={3}
                    required
                    maxLength={200}
                    value={reason}
                    onChange={(e) => {
                      setReason(e.target.value);
                      setRequestKey(crypto.randomUUID());
                    }}
                    disabled={m.isPending}
                    autoFocus={review !== "rejected"}
                  />
                </label>
                {m.isError && <p role="alert">{m.error.message}</p>}
              </FormDialog>
            )}
          </div>
        )}
      </div>
    </details>
  );
}
