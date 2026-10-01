import { msg, getLocale } from "@/lib/i18n";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Award,
  History,
  ArrowRight,
  UsersRound,
  CalendarCheck2,
} from "lucide-react";
import { FormDialog } from "@/components/ui/form-dialog";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { EveImage } from "@/components/eve-image";
import {
  formatDate,
  getPendingPAP,
  write,
  type Event,
  type Entry,
  type Context,
} from "./api";
import { getPAP, getPAPHistory, isSaved } from "./pap-api";
import { CoinConversion } from "./coin-conversion";
import { PAPBatchIssue } from "./pap-batch";

export function PAPEvent({
  user,
  csrf,
  event,
  entries,
}: {
  user: string;
  csrf: string;
  event: Event;
  entries: Entry[];
}) {
  const client = useQueryClient();
  const [editing, setEditing] = useState<null | {
    version: string;
    key: string;
    points: number;
    reason: string;
    revoke: boolean;
    supplement: boolean;
  }>(null);
  const [history, setHistory] = useState(false);
  const [after, setAfter] = useState("");
  const action = useRef<HTMLButtonElement>(null);
  const eligible = entries.filter(
    (e) => e.present && e.account_id !== null,
  ).length;
  const pendingSupplement =
    event.pap_issued === true &&
    (event.pap_points ?? 0) > 0 &&
    entries.some(
      (e) =>
        e.present &&
        e.account_id !== null &&
        (e.pap_points ?? 0) < (event.pap_points ?? 0),
    );
  const q = useQuery({
    queryKey: ["attendance", "pap-history", user, event.id, after],
    queryFn: ({ signal }) => getPAPHistory(event.id, after, signal),
    enabled: history,
  });
  const m = useMutation({
    mutationFn: () =>
      write(
        `/events/${event.id}/pap`,
        csrf,
        {
          version: editing!.version,
          request_key: editing!.key,
          points: editing!.points,
          reason: editing!.reason,
          revoke: editing!.revoke,
          supplement: editing!.supplement,
        },
        isSaved,
      ),
    onError: () => {
      void client.invalidateQueries({
        queryKey: ["attendance", "event", user, event.id],
      });
    },
    onSuccess: () => {
      setEditing(null);
      setAfter("");
      void client.invalidateQueries({ queryKey: ["attendance"] });
      void client.invalidateQueries({ queryKey: ["exchange"] });
    },
  });
  const begin = (revoke: boolean, supplement = false) => {
    m.reset();
    setEditing({
      version: event.version,
      key: crypto.randomUUID(),
      points: event.pap_points ?? 1,
      reason: supplement ? msg("补录后补发集结分") : "",
      revoke,
      supplement,
    });
  };
  const update = (values: Partial<NonNullable<typeof editing>>) =>
    setEditing((v) => (v ? { ...v, ...values, key: crypto.randomUUID() } : v));
  return (
    <Card className="attendance-card attendance-pap">
      <div className="attendance-toolbar">
        <span className="attendance-mark">
          <Award aria-hidden="true" />
        </span>
        <div className="attendance-event-copy">
          <strong>{msg("军团 PAP")}</strong>
          <small>
            {event.pap_issued
              ? msg("已发放 · 每角色 {0} 分", event.pap_points)
              : event.state === "open"
                ? msg("结束活动后发分")
                : msg("未发放")}
          </small>
        </div>
        {event.can_manage && event.state === "closed" && (
          <>
            {event.pap_issued && pendingSupplement && (
              <Button
                variant="outline"
                onClick={() => begin(false, true)}
                disabled={m.isPending}
              >
                {msg("补发集结分")}
              </Button>
            )}
            <Button
              ref={action}
              variant="outline"
              onClick={() => begin(false, false)}
              disabled={m.isPending}
            >
              {event.pap_issued ? msg("更正分值") : msg("发放军团 PAP")}
            </Button>
            {event.pap_issued && (
              <Button
                variant="outline"
                disabled={m.isPending}
                onClick={() => begin(true, false)}
              >
                {msg("撤销军团 PAP")}{" "}
              </Button>
            )}
          </>
        )}
        <Button
          variant="ghost"
          aria-expanded={history}
          onClick={() => setHistory((v) => !v)}
        >
          <History />
          {msg("明细")}{" "}
        </Button>
      </div>
      {editing && (
        <FormDialog
          close={() => setEditing(null)}
          busy={m.isPending}
          disabled={
            !editing.reason.trim() || (!editing.revoke && eligible === 0)
          }
          destructive={editing.revoke}
          title={
            editing.revoke
              ? msg("撤销军团 PAP")
              : editing.supplement
                ? msg("补发集结分")
              : event.pap_issued
                ? msg("更正分值")
                : msg("发放军团 PAP")
          }
          submitLabel={
            editing.revoke
              ? msg("确认撤销")
              : editing.supplement
                ? msg("确认补发")
              : event.pap_issued
                ? msg("确认更正")
                : msg("确认发放")
          }
          className="attendance-form attendance-pap-form"
          formLabel={
            editing.revoke
              ? msg("撤销军团 PAP")
              : editing.supplement
                ? msg("补发集结分")
                : msg("发放军团 PAP")
          }
          onSubmit={(e) => {
            e.preventDefault();
            if (!m.isPending) m.mutate();
          }}
        >
          <strong>{event.title}</strong>
          {editing.supplement && (
            <p className="attendance-note">
              {msg("仅补发给本次补录且尚未获得本活动集结分的角色。")}
            </p>
          )}
          {!editing.revoke && (
            <label>
              {msg("每角色分值")}{" "}
              <input
                aria-label={msg("每角色分值")}
                type="number"
                min={1}
                max={10000}
                step={1}
                value={editing.points}
                disabled={m.isPending}
                required
                onChange={(e) => update({ points: e.target.valueAsNumber })}
              />
            </label>
          )}
          <p className="attendance-note">
            {editing.revoke
              ? msg("撤销本次军团 PAP，保留发分记录。")
              : msg(
                  "{0} 个角色 × {1} 分 = {2} 分",
                  eligible,
                  Number.isFinite(editing.points) ? editing.points : 0,
                  Number.isFinite(editing.points)
                    ? eligible * editing.points
                    : 0,
                )}
          </p>
          <label>
            {editing.revoke ? msg("撤销原因") : msg("发分说明")}
            <textarea
              rows={3}
              autoFocus={!editing.revoke}
              required
              maxLength={200}
              value={editing.reason}
              disabled={m.isPending}
              onChange={(e) => update({ reason: e.target.value })}
            />
          </label>
          {m.isError && <p role="alert">{m.error.message}</p>}
        </FormDialog>
      )}
      {event.can_convert &&
        event.state === "closed" &&
        event.pap_issued &&
        !editing && (
          <CoinConversion
            key={`${event.id}/${event.version}`}
            user={user}
            csrf={csrf}
            event={event}
          />
        )}
      {history && (
        <div className="attendance-pap-history">
          {q.isError ? (
            <p role="alert">{q.error.message}</p>
          ) : !q.data ? (
            <p role="status">{msg("正在读取")}</p>
          ) : (
            <>
              {q.data.items.length === 0 && (
                <p className="attendance-note">{msg("暂无发分记录")}</p>
              )}
              {q.data.items.map((r) => (
                <div className="attendance-pap-row" key={r.id}>
                  <EveImage
                    kind="character"
                    id={r.character_id}
                    className="attendance-avatar"
                  />
                  <div className="attendance-event-copy">
                    <strong>{r.name}</strong>
                    <small>
                      {r.reason} · {formatDate(r.created_at)}
                    </small>
                  </div>
                  <strong className="attendance-pap-score">
                    {r.delta > 0 ? "+" : ""}
                    {r.delta}
                    <small>
                      {msg("余额")} {r.balance}
                    </small>
                  </strong>
                </div>
              ))}
              {(after || q.data.next_cursor) && (
                <div className="attendance-toolbar">
                  <Button
                    variant="outline"
                    disabled={!after}
                    onClick={() => setAfter("")}
                  >
                    {msg("返回首批")}{" "}
                  </Button>
                  <Button
                    variant="outline"
                    disabled={!q.data.next_cursor}
                    onClick={() => setAfter(q.data!.next_cursor)}
                  >
                    {msg("后续明细")}{" "}
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </Card>
  );
}

export function PAPPanel({
  user,
  csrf,
  context,
  select,
}: {
  user: string;
  csrf: string;
  context: Context;
  select: (id: string) => void;
}) {
  const client = useQueryClient();
  const [corp, setCorp] = useState("");
  const [period, setPeriod] = useState("month");
  const [page, setPage] = useState(0);
  const q = useQuery({
    queryKey: ["attendance", "pap", user, corp, period, page],
    queryFn: ({ signal }) => getPAP(corp, period, page, signal),
  });
  useEffect(() => {
    if (!q.data?.more) return;
    const next = page + 1;
    void client.prefetchQuery({
      queryKey: ["attendance", "pap", user, corp, period, next],
      queryFn: ({ signal }) => getPAP(corp, period, next, signal),
      staleTime: 30_000,
    });
  }, [client, corp, page, period, q.data?.more, user]);
  const [batch, setBatch] = useState(false);
  const pending = useQuery({
    queryKey: ["attendance", "pap-pending", user],
    queryFn: ({ signal }) => getPendingPAP(signal),
  });
  return (
    <div className="attendance-section pap-page">
      <div className="pap-corporation-content">
        {pending.isError && <p role="alert">{pending.error.message}</p>}
        <div className="attendance-toolbar">
          {context.corporations.length > 0 && (
            <Select
              label={msg("军团 PAP 范围")}
              value={corp || "self"}
              onValueChange={(v) => {
                setCorp(v === "self" ? "" : v);
                setPage(0);
              }}
              options={[
                { value: "self", label: msg("我的积分") },
                ...context.corporations.map((c) => ({
                  value: c.id,
                  label: msg("{0} · 全团", c.name),
                })),
              ]}
            />
          )}
          <Select
            label={msg("军团 PAP 周期")}
            value={period}
            onValueChange={(v) => {
              setPeriod(v);
              setPage(0);
            }}
            options={[
              { value: "month", label: msg("本月") },
              { value: "30d", label: msg("近 30 天") },
            ]}
          />
          {pending.data && pending.data.events.length > 0 && (
            <Button
              className="pap-toolbar-action"
              onClick={() => setBatch(true)}
              disabled={pending.isFetching}
            >
              <Award aria-hidden="true" />
              {msg("统一发放")}
              <span className="attendance-count-badge">
                {pending.data.events.length}
              </span>
            </Button>
          )}
        </div>
        {q.isError ? (
          <p role="alert">{q.error.message}</p>
        ) : !q.data ? (
          <p role="status">{msg("正在读取")}</p>
        ) : (
          <>
            <div className="attendance-metrics">
              {[
                {
                  icon: <Award />,
                  label: corp ? msg("全团积分") : msg("我的积分"),
                  value: q.data.points,
                },
                {
                  icon: <CalendarCheck2 />,
                  label: msg("集结次数"),
                  value: q.data.events,
                },
                {
                  icon: <UsersRound />,
                  label: msg("出勤角色人次"),
                  value: q.data.participations,
                },
              ].map((v) => (
                <Card
                  key={v.label}
                  className="attendance-card attendance-metric"
                >
                  <span className="attendance-metric-icon" aria-hidden="true">
                    {v.icon}
                  </span>
                  <div>
                    <small>{v.label}</small>
                    <strong>{v.value.toLocaleString(getLocale())}</strong>
                  </div>
                </Card>
              ))}
            </div>
            <Card className="attendance-card">
              <div className="attendance-card-heading">
                <h2>{msg("军团 PAP 明细")}</h2>
                <small>{msg("多角色累加")}</small>
              </div>
              {q.data.rows.length === 0 ? (
                <p className="attendance-note">{msg("暂无军团 PAP")}</p>
              ) : (
                q.data.rows.map((r) => (
                  <button
                    className="attendance-pap-row attendance-pap-link"
                    key={`${r.event_id}:${r.character_id}`}
                    onClick={() => select(r.event_id)}
                  >
                    <EveImage
                      kind="character"
                      id={r.character_id}
                      className="attendance-avatar"
                    />
                    <span className="attendance-event-copy">
                      <strong>{r.name}</strong>
                      <small>
                        {r.title} · {formatDate(r.starts_at)}
                      </small>
                    </span>
                    <strong className="attendance-pap-score">
                      {r.points} {msg("分")}{" "}
                    </strong>
                    <ArrowRight size={16} aria-hidden="true" />
                  </button>
                ))
              )}
            </Card>
            {(page > 0 || q.data.more) && (
              <div className="attendance-toolbar">
                <Button
                  variant="outline"
                  disabled={page === 0}
                  onClick={() => setPage((v) => v - 1)}
                >
                  {msg("上一页")}{" "}
                </Button>
                <Button
                  variant="outline"
                  disabled={!q.data.more}
                  onClick={() => setPage((v) => v + 1)}
                >
                  {msg("下一页")}{" "}
                </Button>
              </div>
            )}
          </>
        )}
      </div>
      {batch && pending.data && (
        <PAPBatchIssue
          csrf={csrf}
          events={pending.data.events}
          close={() => setBatch(false)}
          onComplete={() => {
            void pending.refetch();
            void q.refetch();
            void client.invalidateQueries({ queryKey: ["attendance"] });
            void client.invalidateQueries({ queryKey: ["exchange"] });
          }}
        />
      )}
    </div>
  );
}
