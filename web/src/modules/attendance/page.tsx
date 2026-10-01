import { msg } from "@/lib/i18n";
import { Modal } from "@/components/ui/dialog";
import { PAPEvent, PAPPanel } from "./pap-panel";
import { AlliancePAPPage } from "./alliance-pap-page";
import { captureSummary } from "./api";
import {
  lazy,
  Suspense,
  useState,
  useEffect,
  useRef,
  type FormEvent,
  type ReactNode,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { Tabs } from "radix-ui";
import {
  Award,
  ArrowLeft,
  ArrowRight,
  CalendarCheck2,
  Check,
  Clock3,
  History,
  ListChecks,
  LockKeyhole,
  Pencil,
  Plus,
  Radio,
  RefreshCw,
  UsersRound,
} from "lucide-react";
import { useSession } from "@/modules/identity";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { EveImage } from "@/components/eve-image";
import {
  getContext,
  getEvents,
  getEvent,
  getOnline,
  getAudit,
  write,
  isEvent,
  isChanged,
  duration,
  formatDate,
  states,
  type Context,
  type Event,
  type Entry,
} from "./api";
import "./attendance.css";
import { BattlePanel } from "./battle-panel";
import { StarLocation } from "./location";
const Chart = lazy(() => import("@/components/charts/bar-comparison"));
function Feedback({
  error,
  text = msg("正在读取"),
  retry,
}: {
  error?: Error | null;
  text?: string;
  retry?: () => void;
}) {
  return (
    <div className="attendance-feedback" role={error ? "alert" : "status"}>
      <p>{error?.message ?? text}</p>
      {retry && (
        <IconAction label={msg("重试")} onClick={retry}>
          <RefreshCw />
        </IconAction>
      )}
    </div>
  );
}
export default function AttendancePage() {
  const session = useSession();
  if (session.isError)
    return (
      <Feedback error={session.error} retry={() => void session.refetch()} />
    );
  if (!session.isSuccess) return <Feedback />;
  if (!session.data.session) return <Navigate to="/login" replace />;
  return (
    <Workspace
      user={session.data.session.user_id}
      csrf={session.data.session.csrf_token}
    />
  );
}
function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const [params, setParams] = useSearchParams();
  const context = useQuery({
    queryKey: ["attendance", "context", user],
    queryFn: ({ signal }) => getContext(signal),
  });
  const view =
    params.get("view") === "pap"
      ? "pap"
      : params.get("view") === "alliance-pap"
        ? "alliance-pap"
        : params.get("view") === "online"
          ? "online"
          : "events";
  const event = params.get("event") ?? "";
  const patch = (values: Record<string, string>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(values)) {
      if (v) next.set(k, v);
      else next.delete(k);
    }
    setParams(next);
  };
  return (
    <Tabs.Root
      className="attendance-page"
      value={view}
      onValueChange={(v) => patch({ view: v, event: "" })}
    >
      <header className="attendance-heading">
        <span className="attendance-mark">
          <CalendarCheck2 aria-hidden="true" />
        </span>
        <h1>{msg("军团考勤")}</h1>
      </header>
      <Tabs.List className="attendance-tabs" aria-label={msg("考勤视图")}>
        <Tabs.Trigger value="events">
          <ListChecks size={17} aria-hidden="true" />
          {msg("活动出勤")}{" "}
        </Tabs.Trigger>
        <Tabs.Trigger value="online">
          <Clock3 size={17} aria-hidden="true" />
          {msg("在线时长")}{" "}
        </Tabs.Trigger>
        <Tabs.Trigger value="pap">
          <Award size={17} aria-hidden="true" />
          {msg("军团 PAP")}{" "}
        </Tabs.Trigger>
        <Tabs.Trigger value="alliance-pap">
          <Award size={17} aria-hidden="true" />
          {msg("联盟 PAP")}{" "}
        </Tabs.Trigger>
      </Tabs.List>
      {context.isError ? (
        <Feedback error={context.error} retry={() => void context.refetch()} />
      ) : !context.data ? (
        <Feedback />
      ) : (
        <>
          <Tabs.Content value="events">
            {event ? (
              <EventDetail
                key={event}
                user={user}
                csrf={csrf}
                id={event}
                context={context.data}
                back={() => patch({ event: "" })}
              />
            ) : (
              <Events
                user={user}
                csrf={csrf}
                context={context.data}
                select={(event) => patch({ event })}
              />
            )}
          </Tabs.Content>

          <Tabs.Content value="pap">
            <PAPPanel
              user={user}
              csrf={csrf}
              context={context.data}
              select={(id) => patch({ view: "events", event: id })}
            />
          </Tabs.Content>
          <Tabs.Content value="alliance-pap">
            <AlliancePAPPage user={user} />
          </Tabs.Content>
          <Tabs.Content value="online">
            <OnlinePanel
              user={user}
              context={context.data}
              member={params.get("member") ?? ""}
            />
          </Tabs.Content>
        </>
      )}
    </Tabs.Root>
  );
}

function Events({
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
  const [before, setBefore] = useState("");
  const [create, setCreate] = useState(false);
  const q = useQuery({
    queryKey: ["attendance", "events", user, before],
    queryFn: ({ signal }) => getEvents(before, signal),
  });
  useEffect(() => {
    if (!q.data?.next_cursor) return;
    const next = q.data.next_cursor;
    void client.prefetchQuery({
      queryKey: ["attendance", "events", user, next],
      queryFn: ({ signal }) => getEvents(next, signal),
      staleTime: 30_000,
    });
  }, [client, q.data?.next_cursor, user]);
  return (
    <div className="attendance-section">
      <div className="attendance-toolbar">
        <h2>{msg("活动记录")}</h2>
        <span className="attendance-spacer" />
        {context.corporations.length > 0 && (
          <Button onClick={() => setCreate(true)}>
            <Plus />
            {msg("创建活动")}{" "}
          </Button>
        )}
        <IconAction
          label={msg("刷新活动")}
          disabled={q.isFetching}
          onClick={() => void q.refetch()}
        >
          <RefreshCw />
        </IconAction>
      </div>
      {q.isError ? (
        <Feedback error={q.error} retry={() => void q.refetch()} />
      ) : !q.data ? (
        <Feedback />
      ) : q.data.events.length === 0 ? (
        <Card className="attendance-card">
          <div className="attendance-empty">
            <CalendarCheck2 size={32} aria-hidden="true" />
            <p>{msg("暂无活动记录")}</p>
          </div>
        </Card>
      ) : (
        <Card className="attendance-card attendance-events">
          <div className="attendance-event-columns" aria-hidden="true">
            <span>{msg("活动")}</span>
            <span>{msg("军团")}</span>
            <span>{msg("开始时间")}</span>
            <span>{msg("结束时间")}</span>
            <span>{msg("状态")}</span>
            <span>{msg("人数")}</span>
          </div>
          {q.data.events.map((e) => (
            <button
              key={e.id}
              className="attendance-event"
              onClick={() => select(e.id)}
            >
              <span className="attendance-event-copy">
                <strong>{e.title}</strong>
              </span>
              <span className="attendance-event-context">
                <span>
                  {context.corporations.find((c) => c.id === e.corporation_id)
                    ?.name ?? msg("军团 #{0}", e.corporation_id)}
                </span>
                <span className="attendance-event-time">
                  <span className="attendance-time-label">
                    {msg("开始时间")}
                  </span>
                  <time dateTime={e.starts_at}>{formatDate(e.starts_at)}</time>
                </span>
                <span className="attendance-event-time attendance-event-end">
                  <span className="attendance-time-label">
                    {msg("结束时间")}
                  </span>
                  {e.state === "closed" && e.ends_at ? (
                    <time dateTime={e.ends_at}>{formatDate(e.ends_at)}</time>
                  ) : (
                    <span>—</span>
                  )}
                </span>
              </span>
              <span className="attendance-event-meta">
                <span
                  className={`attendance-badge ${e.state === "open" ? "is-open" : ""}`}
                >
                  {e.state === "open" ? msg("进行中") : msg("已结束")}
                </span>
                <span>
                  <UsersRound size={15} aria-hidden="true" />
                  {e.participants} {msg("人")}{" "}
                </span>
              </span>
              <ArrowRight size={18} aria-hidden="true" />
            </button>
          ))}
        </Card>
      )}
      {(before || q.data?.next_cursor) && (
        <div className="attendance-toolbar">
          <Button
            variant="outline"
            disabled={!before}
            onClick={() => setBefore("")}
          >
            {msg("返回最新")}{" "}
          </Button>
          <Button
            variant="outline"
            disabled={!q.data?.next_cursor}
            onClick={() => setBefore(q.data!.next_cursor)}
          >
            {msg("更早活动")} <ArrowRight />
          </Button>
        </div>
      )}
      {create && (
        <Modal
          className="attendance-dialog"
          size="compact"
          title={msg("创建活动")}
          close={() => setCreate(false)}
        >
          <CreateForm csrf={csrf} context={context} done={select} />
        </Modal>
      )}
    </div>
  );
}
function CreateForm({
  csrf,
  context,
  done,
}: {
  csrf: string;
  context: Context;
  done: (id: string) => void;
}) {
  const [corp, setCorp] = useState(context.corporations[0]?.id ?? "");
  const [title, setTitle] = useState("");
  const [at, setAt] = useState(() => {
    const now = new Date();
    return new Date(now.getTime() + 8 * 3600_000).toISOString().slice(0, 16);
  });
  const [key] = useState(() => crypto.randomUUID());
  const m = useMutation({
    mutationFn: () =>
      write(
        "/events",
        csrf,
        {
          corporation_id: corp,
          title,
          starts_at: new Date(`${at}:00+08:00`).toISOString(),
          request_key: key,
        },
        isEvent,
      ),
    onSuccess: (e) => done(e.id),
  });
  return (
    <form
      className="attendance-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (!m.isPending) m.mutate();
      }}
    >
      <label>
        {msg("活动名称")}{" "}
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
          maxLength={80}
          autoFocus
        />
      </label>
      <label>
        {msg("军团")}{" "}
        <Select
          label={msg("活动军团")}
          value={corp}
          onValueChange={setCorp}
          options={context.corporations.map((c) => ({
            value: c.id,
            label: c.name,
          }))}
        />
      </label>
      <label>
        {msg("开始时间（北京时间）")}{" "}
        <input
          type="datetime-local"
          value={at}
          onChange={(e) => setAt(e.target.value)}
          required
        />
      </label>
      {m.isError && <p role="alert">{m.error.message}</p>}
      <Button type="submit" disabled={m.isPending || !corp}>
        {m.isPending ? msg("正在创建") : msg("创建活动")}
      </Button>
    </form>
  );
}
function EventDetail({
  user,
  csrf,
  id,
  context,
  back,
}: {
  user: string;
  csrf: string;
  id: string;
  context: Context;
  back: () => void;
}) {
  const client = useQueryClient();
  const [source, setSource] = useState(context.characters[0]?.id ?? "");
  const [manual, setManual] = useState<Entry | "new" | null>(null);
  const [stateAction, setStateAction] = useState<"close" | "reopen" | null>(
    null,
  );
  const [audit, setAudit] = useState(false);
  const [notice, setNotice] = useState("");
  const q = useQuery({
    queryKey: ["attendance", "event", user, id],
    queryFn: ({ signal }) => getEvent(id, signal),
  });
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    if (q.isSuccess) heading.current?.focus({ preventScroll: true });
  }, [q.isSuccess]);
  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ["attendance"] });
  };
  const capture = useMutation({
    mutationFn: () =>
      write(
        `/events/${id}/capture`,
        csrf,
        {
          source_character_id: source,
          version: q.data!.event.version,
          request_key: crypto.randomUUID(),
        },
        isChanged,
      ),
    onSuccess: (r) => {
      setNotice(captureSummary(r));
      invalidate();
    },
  });
  if (q.isError)
    return (
      <>
        <Button variant="ghost" onClick={back}>
          <ArrowLeft />
          {msg("活动记录")}{" "}
        </Button>
        <Feedback error={q.error} retry={() => void q.refetch()} />
      </>
    );
  if (!q.data) return <Feedback />;
  const e = q.data.event;
  const present = q.data.entries.filter((r) => r.present);
  const corp = context.corporations.find((c) => c.id === e.corporation_id);
  return (
    <div className="attendance-section">
      <div className="attendance-toolbar attendance-detail-heading">
        <IconAction label={msg("返回活动记录")} onClick={back}>
          <ArrowLeft />
        </IconAction>
        <div className="attendance-event-copy">
          <h2 ref={heading} tabIndex={-1}>
            {e.title}
          </h2>
          <small>
            {formatDate(e.starts_at)} ·{" "}
            {corp?.name ?? msg("军团 #{0}", e.corporation_id)}
          </small>
        </div>
        <span
          className={`attendance-badge ${e.state === "open" ? "is-open" : ""}`}
        >
          {e.state === "open" ? msg("进行中") : msg("已结束")}
        </span>
        <span className="attendance-spacer" />
        {e.can_manage && (
          <>
            <IconAction
              label={msg("修订记录")}
              onClick={() => setAudit((v) => !v)}
            >
              <History />
            </IconAction>
            <Button
              variant="outline"
              onClick={() =>
                setStateAction(e.state === "open" ? "close" : "reopen")
              }
              disabled={e.state === "closed" && e.pap_issued}
            >
              <LockKeyhole />
              {e.state === "open"
                ? msg("结束活动")
                : e.pap_issued
                  ? msg("先撤销发分再重开")
                  : msg("重新开启")}
            </Button>
          </>
        )}
      </div>
      <div className="attendance-metrics">
        <Metric
          icon={<UsersRound />}
          label={msg("出勤人数")}
          value={String(e.participants)}
        />
        <Metric
          icon={<Check />}
          label={msg("出勤角色")}
          value={String(present.length)}
        />
        <Metric
          icon={<Pencil />}
          label={msg("人工记录")}
          value={String(
            q.data.entries.filter((r) => r.source === "manual").length,
          )}
        />
      </div>
      <PAPEvent user={user} csrf={csrf} event={e} entries={q.data.entries} />
      {e.can_manage && e.state === "open" && (
        <div className="attendance-toolbar attendance-capture">
          <Select
            label={msg("点名来源角色")}
            value={source}
            onValueChange={setSource}
            options={context.characters.map((c) => ({
              value: c.id,
              label: c.name,
              leading: (
                <EveImage
                  id={c.id}
                  kind="character"
                  className="attendance-tiny-avatar"
                />
              ),
            }))}
          />
          <Button
            disabled={capture.isPending || !source}
            onClick={() => capture.mutate()}
          >
            <Radio />
            {capture.isPending ? msg("正在读取舰队") : msg("舰队点名")}
          </Button>
        </div>
      )}
      {e.can_manage && e.state === "closed" && (
        <div className="attendance-toolbar attendance-capture">
          <Button variant="outline" onClick={() => setManual("new")}>
            <Plus />
            {msg("补录")}{" "}
          </Button>
        </div>
      )}
      {capture.isError && (
        <Feedback error={capture.error} retry={() => void q.refetch()} />
      )}
      {notice && (
        <p role="status" className="attendance-note">
          {notice}
        </p>
      )}
      <Card className="attendance-card">
        <div className="attendance-card-heading">
          <h2>{msg("出勤名单")}</h2>
          <small>{msg("人数按账号统计")}</small>
        </div>
        {q.data.entries.length === 0 ? (
          <div className="attendance-empty">
            <UsersRound size={30} aria-hidden="true" />
            <p>{msg("尚未点名")}</p>
          </div>
        ) : (
          <div className="attendance-roster attendance-battle-roster">
            {q.data.entries.map((row) => (
              <div key={row.character_id} className="attendance-person-record">
                <div className="attendance-person">
                  <EveImage
                    kind="character"
                    id={row.character_id}
                    className="attendance-avatar"
                  />
                  <div className="attendance-event-copy">
                    <strong>{row.name}</strong>
                    <small>
                      {row.source === "manual"
                        ? msg("人工记录")
                        : msg("舰队点名")}
                      {row.account_id === null ? msg(" · 历史未绑定记录") : ""}
                      {row.ship_type_id && row.ship_type_id !== "0"
                        ? ` · ${row.ship_name || `#${row.ship_type_id}`}`
                        : ""}
                      {row.losses ? msg(" · 损失 {0}", row.losses) : ""}
                    </small>
                    <StarLocation
                      id={row.solar_system_id}
                      name={row.solar_system_name}
                      at={row.location_observed_at}
                    />
                  </div>
                  {row.ship_type_id && row.ship_type_id !== "0" && (
                    <EveImage
                      kind="type"
                      id={row.ship_type_id}
                      className="attendance-ship-icon"
                    />
                  )}
                  <span className="attendance-badge">
                    {row.present ? msg("出勤") : msg("已取消")}
                    {!!row.pap_points && msg(" · {0} 分", row.pap_points)}
                  </span>
                  {e.can_manage && e.state === "closed" && (
                    <IconAction
                      label={msg("修订 {0}", row.name)}
                      onClick={() => setManual(row)}
                    >
                      <Pencil />
                    </IconAction>
                  )}
                </div>
                <BattlePanel user={user} csrf={csrf} event={e} entry={row} />
              </div>
            ))}
          </div>
        )}
      </Card>
      {audit && <AuditPanel user={user} id={id} />}
      {manual && (
        <Modal
          title={
            manual === "new" ? msg("补录角色") : msg("修订 {0}", manual.name)
          }
          close={() => setManual(null)}
        >
          <ManualForm
            csrf={csrf}
            event={e}
            entry={manual === "new" ? undefined : manual}
            done={() => {
              setManual(null);
              invalidate();
            }}
          />
        </Modal>
      )}
      {stateAction && (
        <Modal
          title={
            stateAction === "close" ? msg("结束活动") : msg("重新开启活动")
          }
          close={() => setStateAction(null)}
        >
          <StateForm
            action={stateAction}
            event={e}
            csrf={csrf}
            done={() => {
              setStateAction(null);
              invalidate();
            }}
          />
        </Modal>
      )}
    </div>
  );
}
function ManualForm({
  csrf,
  event,
  entry,
  done,
}: {
  csrf: string;
  event: Event;
  entry?: Entry;
  done: () => void;
}) {
  const [id, setID] = useState(entry?.character_id ?? "");
  const [present, setPresent] = useState(entry ? entry.present : true);
  const [reason, setReason] = useState("");
  const [key] = useState(() => crypto.randomUUID());
  const m = useMutation({
    mutationFn: () =>
      write(
        `/events/${event.id}/manual`,
        csrf,
        {
          version: event.version,
          request_key: key,
          character_id: id,
          present,
          reason,
        },
        isChanged,
      ),
    onSuccess: done,
  });
  return (
    <form
      className="attendance-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (!m.isPending) m.mutate();
      }}
    >
      <label>
        {msg("角色 ID")}{" "}
        <input
          value={id}
          onChange={(e) => setID(e.target.value)}
          inputMode="numeric"
          pattern="[1-9][0-9]*"
          required
          readOnly={!!entry}
        />
      </label>
      <label>
        {msg("状态")}{" "}
        <Select
          label={msg("出勤状态")}
          value={present ? "present" : "cancelled"}
          onValueChange={(v) => setPresent(v === "present")}
          options={[
            { value: "present", label: msg("出勤") },
            { value: "cancelled", label: msg("取消出勤") },
          ]}
        />
      </label>
      <label>
        {msg("修订原因")}{" "}
        <input
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          maxLength={200}
          required
        />
      </label>
      {m.isError && <p role="alert">{m.error.message}</p>}
      <Button type="submit" disabled={m.isPending}>
        {m.isPending ? msg("正在保存") : msg("保存")}
      </Button>
    </form>
  );
}
function StateForm({
  action,
  event,
  csrf,
  done,
}: {
  action: "close" | "reopen";
  event: Event;
  csrf: string;
  done: () => void;
}) {
  const [reason, setReason] = useState("");
  const [key] = useState(() => crypto.randomUUID());
  const m = useMutation({
    mutationFn: () =>
      write(
        `/events/${event.id}/${action}`,
        csrf,
        { version: event.version, request_key: key, reason },
        isChanged,
      ),
    onSuccess: done,
  });
  return (
    <form
      className="attendance-form"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        if (!m.isPending) m.mutate();
      }}
    >
      {action === "close" ? (
        <p>{msg("结束后停止点名；补录在活动结束后进行，舰队解散时会自动结束。")}</p>
      ) : (
        <label>
          {msg("重新开启原因")}{" "}
          <input
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            maxLength={200}
            required
          />
        </label>
      )}
      {m.isError && <p role="alert">{m.error.message}</p>}
      <Button type="submit" disabled={m.isPending}>
        {m.isPending ? msg("正在保存") : msg("确认")}
      </Button>
    </form>
  );
}
function Metric({
  icon,
  label,
  value,
}: {
  icon: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <Card className="attendance-card attendance-metric">
      <span className="attendance-metric-icon" aria-hidden="true">
        {icon}
      </span>
      <div>
        <small>{label}</small>
        <strong>{value}</strong>
      </div>
    </Card>
  );
}
function AuditPanel({ user, id }: { user: string; id: string }) {
  const [after, setAfter] = useState("");
  const q = useQuery({
    queryKey: ["attendance", "audit", user, id, after],
    queryFn: ({ signal }) => getAudit(id, after, signal),
  });
  return (
    <Card className="attendance-card">
      <h2>{msg("修订记录")}</h2>
      {q.isError ? (
        <Feedback error={q.error} retry={() => void q.refetch()} />
      ) : !q.data ? (
        <Feedback />
      ) : (
        <>
          {q.data.items.length === 0 && (
            <p className="attendance-note">{msg("暂无修订")}</p>
          )}
          {q.data.items.map((a) => (
            <div key={a.id} className="attendance-audit">
              <History size={16} aria-hidden="true" />
              <div>
                <strong>
                  {
                    {
                      pap: msg("集结分调整"),
                      capture: msg("舰队点名"),
                      manual: msg("人工修订"),
                      close: msg("结束活动"),
                      reopen: msg("重新开启"),
                      auto_close: msg("舰队解散，自动结束"),
                      loss_review: msg("损失确认"),
                      battle_refresh: msg("重试损失同步"),
                    }[a.action]
                  }
                </strong>
                <p>
                  {a.action === "loss_review" && a.loss_id
                    ? msg(
                        "报告 #{0} · {1} · ",
                        a.loss_id,
                        a.loss_state === "confirmed"
                          ? msg("计入")
                          : msg("排除"),
                      )
                    : ""}
                  {a.reason ||
                    (a.action === "capture" ? captureSummary(a) : "")}
                </p>
                <small>
                  {formatDate(a.created_at)} {msg("· 操作人")} {a.actor_id}
                </small>
              </div>
            </div>
          ))}
          {q.data.next_cursor && (
            <Button
              variant="outline"
              onClick={() => setAfter(q.data.next_cursor)}
            >
              {msg("后续记录")}{" "}
            </Button>
          )}
        </>
      )}
    </Card>
  );
}
function OnlinePanel({
  user,
  context,
  member,
}: {
  user: string;
  context: Context;
  member: string;
}) {
  const [corp, setCorp] = useState("");
  const [days, setDays] = useState("7");
  const [table, setTable] = useState(false);
  const q = useQuery({
    queryKey: ["attendance", "online", user, member, corp, days],
    queryFn: ({ signal }) => getOnline(corp, days, member, signal),
    refetchInterval: 60_000,
  });
  const chart =
    q.data?.days.map((d) => ({
      name: d.date.slice(5).replace("-", "/"),
      value: d.samples ? Number((d.seconds / 3600).toFixed(2)) : null,
    })) ?? [];
  const online =
    q.data?.members
      .flatMap((m) => m.characters)
      .filter((c) => c.state === "online").length ?? 0;
  return (
    <div className="attendance-section">
      <div className="attendance-toolbar">
        {member ? (
          <Link
            to={`/members?member=${encodeURIComponent(member)}`}
            className="attendance-back"
          >
            <ArrowLeft size={17} />
            {msg("成员资料")}{" "}
          </Link>
        ) : (
          <Select
            label={msg("统计范围")}
            value={corp}
            onValueChange={setCorp}
            options={[
              { value: "", label: msg("我的角色") },
              ...context.corporations.map((c) => ({
                value: c.id,
                label: c.name,
              })),
            ]}
          />
        )}
        <Select
          label={msg("统计周期")}
          value={days}
          onValueChange={setDays}
          options={[
            { value: "7", label: msg("近 7 天") },
            { value: "30", label: msg("近 30 天") },
          ]}
        />
        <span className="attendance-spacer" />
        <IconAction
          label={msg("刷新在线统计")}
          disabled={q.isFetching}
          onClick={() => void q.refetch()}
        >
          <RefreshCw />
        </IconAction>
      </div>
      {q.isError ? (
        <Feedback error={q.error} retry={() => void q.refetch()} />
      ) : !q.data ? (
        <Feedback />
      ) : (
        <>
          <div className="attendance-metrics">
            <Metric
              icon={<Clock3 />}
              label={corp ? msg("估算在线人时") : msg("估算在线时长")}
              value={
                q.data.days.some((d) => d.samples > 0)
                  ? duration(q.data.seconds)
                  : "—"
              }
            />
            <Metric
              icon={<UsersRound />}
              label={msg("有采样角色")}
              value={String(q.data.observed_characters)}
            />
            <Metric
              icon={<Radio />}
              label={msg("当前在线角色")}
              value={String(online)}
            />
          </div>
          <Card className="attendance-card">
            <div className="attendance-card-heading">
              <div>
                <h2>
                  {msg("每日在线")}
                  {corp ? msg("人时") : msg("时长")}
                </h2>
                <small>{msg("北京时间 · 多角色按账号去重")}</small>
              </div>
              <IconAction
                label={table ? msg("显示图表") : msg("显示每日明细")}
                onClick={() => setTable((v) => !v)}
              >
                <ListChecks />
              </IconAction>
            </div>
            {table ? (
              <div
                className="attendance-days"
                role="table"
                aria-label={msg("每日在线时长")}
              >
                <div role="row">
                  <strong role="columnheader">{msg("日期")}</strong>
                  <strong role="columnheader">{msg("估算时长")}</strong>
                  <strong role="columnheader">{msg("样本")}</strong>
                </div>
                {q.data.days.map((d) => (
                  <div key={d.date} role="row">
                    <span role="cell">{d.date}</span>
                    <span role="cell">
                      {d.samples ? duration(d.seconds) : "—"}
                    </span>
                    <span role="cell">{d.samples.toLocaleString()}</span>
                  </div>
                ))}
              </div>
            ) : (
              <Suspense
                fallback={
                  <div className="attendance-chart-loading" role="status">
                    {msg("正在绘制")}{" "}
                  </div>
                }
              >
                <Chart
                  data={chart}
                  label={msg("每日估算在线小时，无样本的日期未计量")}
                  vertical
                />
              </Suspense>
            )}
            <details className="attendance-method">
              <summary>{msg("统计口径")}</summary>
              <p>
                {msg(
                  "从启用采集后累计；仅连续在线样本之间估算，间隔超过 3 分钟或请求失败即中断。同一账号多角色时段取并集，军团统计再按人数累加。无样本表示未知，不计为离线；保留近 30 天。",
                )}{" "}
              </p>
            </details>
          </Card>
          <Card className="attendance-card">
            <div className="attendance-card-heading">
              <h2>{corp ? msg("成员活跃") : msg("角色状态")}</h2>
              <small>
                {msg("更新于")} {formatDate(q.data.until)}
              </small>
            </div>
            {q.data.members.length === 0 ? (
              <div className="attendance-empty">
                <UsersRound size={30} aria-hidden="true" />
                <p>{msg("暂无已绑定角色")}</p>
              </div>
            ) : (
              q.data.members.map((m) => (
                <section key={m.user_id} className="attendance-online-member">
                  {corp && (
                    <div className="attendance-card-heading">
                      <strong>{m.name}</strong>
                      <strong>{m.samples ? duration(m.seconds) : "—"}</strong>
                    </div>
                  )}
                  <div className="attendance-roster">
                    {m.characters.map((c) => (
                      <div key={c.id} className="attendance-person">
                        <EveImage
                          kind="character"
                          id={c.id}
                          className="attendance-avatar"
                        />
                        <div className="attendance-event-copy">
                          <strong>{c.name}</strong>
                          <small>
                            {c.observed_at
                              ? formatDate(c.observed_at)
                              : msg("尚未采样")}
                          </small>
                        </div>
                        <span
                          className={`attendance-badge ${c.state === "online" ? "is-open" : ""}`}
                        >
                          {states[c.state]}
                        </span>
                      </div>
                    ))}
                  </div>
                </section>
              ))
            )}
          </Card>
        </>
      )}
    </div>
  );
}
