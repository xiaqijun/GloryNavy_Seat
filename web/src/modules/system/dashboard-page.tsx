import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate } from "react-router-dom";
import { lazy, Suspense, useEffect, useRef, useState, type ReactNode } from "react";
import {
  ArrowUpRight,
  Check,
  TrendingUp,
  TrendingDown,
  CalendarCheck2,
  CheckCheck,
  ChevronRight,
  ClipboardCheck,
  Coins,
  Crosshair,
  GraduationCap,
  HeartHandshake,
  LayoutDashboard,
  PackageCheck,
  RefreshCw,
  Ship,
  TriangleAlert,
  Users,
  Wallet,
} from "lucide-react";
import { getModuleCatalog } from "@/app/catalog";
import { Card } from "@/components/ui/card";
import { IconAction } from "@/components/ui/icon-action";
import { Button } from "@/components/ui/button";
import { msg, getLocale } from "@/lib/i18n";
import { useSession, getCharacters } from "@/modules/identity";
import type { Session } from "@/modules/identity/api";
import { getContext, getQueue, approvalLink, lossQueueAmount, type QueueItem } from "@/modules/approval/api";
import { getShop, getOrders } from "@/modules/exchange/rewards-api";
import {
  getAlliancePAP,
  getPAPRequirement,
} from "@/modules/attendance/alliance-pap-api";
import { getAccountAccess } from "@/modules/access/api";
import * as wallet from "@/modules/wallet/api";
import { sumWalletBalances } from "./wallet-total";
import StatusOverview from "./workspace-page";
import { EveImage } from "@/components/eve-image";
import "./workspace.css";

const RingProgress = lazy(() => import("@/components/charts/ring-progress"));

const shortcuts = [
  {
    module: "attendance",
    to: "/attendance",
    label: msg("活动出勤"),
    icon: CalendarCheck2,
  },
  {
    module: "welfare",
    to: "/welfare",
    label: msg("军团福利"),
    icon: HeartHandshake,
  },
  { module: "welfare", to: "/losses", label: msg("舰船损失"), icon: Crosshair },
  {
    module: "skills",
    to: "/skills",
    label: msg("技能管理"),
    icon: GraduationCap,
  },
  { module: "fittings", to: "/fittings", label: msg("舰船配置"), icon: Ship },
  { module: "wallet", to: "/wallet", label: msg("钱包"), icon: Wallet },
];
const queueViews = [
  { id: "pending", label: msg("待审批"), icon: ClipboardCheck },
  { id: "fulfillment", label: msg("待发放"), icon: PackageCheck },
  { id: "exceptions", label: msg("异常"), icon: TriangleAlert },
  { id: "information", label: msg("待补充"), icon: Users },
];
const orderStates = {
  pending: msg("待发放"),
  cancel_requested: msg("取消待审核"),
  fulfilled: msg("已发放"),
  cancelled: msg("已取消"),
};
const number = (v: number) =>
  v.toLocaleString(getLocale(), { maximumFractionDigits: 2 });
const date = (v: string) =>
  new Date(v).toLocaleString(getLocale(), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
function Failure({ retry }: { retry: () => void }) {
  return (
    <div className="desk-feedback" role="alert">
      <TriangleAlert size={18} aria-hidden="true" />
      <span>{msg("数据暂不可用")}</span>
      <Button variant="ghost" onClick={retry}>
        {msg("重试")}
      </Button>
    </div>
  );
}
function Panel({
  title,
  to,
  children,
}: {
  title: string;
  to?: string;
  children: ReactNode;
}) {
  return (
    <Card className="desk-panel">
      <header>
        <h2>{title}</h2>
        {to && (
          <Link className="desk-more" to={to}>
            {msg("查看全部")}
            <ArrowUpRight size={16} aria-hidden="true" />
          </Link>
        )}
      </header>
      {children}
    </Card>
  );
}
export default function Workspace() {
  const catalog = useQuery({
    queryKey: ["host", "modules"],
    queryFn: ({ signal }) => getModuleCatalog(signal),
  });
  const modules = catalog.data?.map((m) => m.id) || [];
  const session = useSession(modules.includes("identity"));
  if (catalog.isError) return <Failure retry={() => void catalog.refetch()} />;
  if (!catalog.data) return <p role="status">{msg("正在读取")}</p>;
  if (!modules.includes("identity")) return <StatusOverview system />;
  if (session.isError) return <Failure retry={() => void session.refetch()} />;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/" replace />;
  return <Dashboard session={session.data.session} modules={modules} />;
}
function Dashboard({
  session,
  modules,
}: {
  session: Session;
  modules: string[];
}) {
  const main = session.main_character || session.character;
  const entries = shortcuts.filter((s) => modules.includes(s.module));
  const access = useQuery({
    queryKey: ["access", "me", session.user_id],
    queryFn: ({ signal }) => getAccountAccess(signal),
    enabled: modules.includes("access"),
    refetchInterval: 30000,
  });
  return (
    <div className="desk-page">
      <header className="desk-heading">
        <span className="desk-mark">
          <LayoutDashboard aria-hidden="true" />
        </span>
        <h1>{msg("工作台")}</h1>
        <Link to="/account" className="desk-identity">
          <img
            src={`https://images.evetech.net/characters/${main.id}/portrait?size=64`}
            alt=""
            width="32"
            height="32"
          />
          <span>{main.name}</span>
          <ChevronRight size={16} aria-hidden="true" />
        </Link>
      </header>
      {modules.includes("approval") &&
        access.isSuccess &&
        access.data.administrator && <ReviewDesk user={session.user_id} />}
      <div className="desk-personal">
        <Characters user={session.user_id} />
        {modules.includes("exchange") && <CoinBalance user={session.user_id} />}
        {modules.includes("attendance") && (
          <PAPSummary user={session.user_id} />
        )}
        {modules.includes("wallet") && (
          <WalletSummary user={session.user_id} main={main.id} />
        )}
      </div>
      {entries.length > 0 && (
        <section aria-labelledby="desk-shortcuts">
          <h2 id="desk-shortcuts">{msg("常用入口")}</h2>
          <div className="desk-shortcuts">
            {entries.map(({ to, label, icon: Icon }) => (
              <Link key={to} to={to}>
                <Icon size={21} aria-hidden="true" />
                <span>{label}</span>
                <ArrowUpRight size={16} aria-hidden="true" />
              </Link>
            ))}
          </div>
        </section>
      )}
      {modules.includes("exchange") && <RecentOrders user={session.user_id} />}
    </div>
  );
}
function Characters({ user }: { user: string }) {
  const q = useQuery({
    queryKey: ["identity", "characters", user],
    queryFn: ({ signal }) => getCharacters(signal),
    staleTime: 30000,
  });
  const rows = [...(q.data?.characters || [])].sort(
    (a, b) => Number(b.is_main) - Number(a.is_main),
  );
  return (
    <Panel title={msg("我的角色")} to="/account">
      {q.isError && <Failure retry={() => void q.refetch()} />}
      {!q.data ? (
        !q.isError && (
          <p className="desk-empty" role="status">
            {msg("正在读取")}
          </p>
        )
      ) : (
        <>
          <div className="desk-character-summary">
            <Users size={18} aria-hidden="true" />
            <strong>{number(rows.length)}</strong>
            <span>{msg("已绑定角色")}</span>
          </div>
          <div className="desk-characters">
            {rows.slice(0, 6).map((c) => (
              <Link to="/account" key={c.id}>
                <img
                  src={`https://images.evetech.net/characters/${c.id}/portrait?size=64`}
                  alt=""
                  width="36"
                  height="36"
                  loading="lazy"
                />
                <span>
                  <strong>{c.name}</strong>
                  {c.status === "blocked" ? (
                    <small>{msg("已停用")}</small>
                  ) : (
                    c.is_main && <small>{msg("主角色")}</small>
                  )}
                </span>
              </Link>
            ))}
          </div>
          {!rows.length && <p className="desk-empty">{msg("暂无绑定角色")}</p>}
        </>
      )}
    </Panel>
  );
}
function CoinBalance({ user }: { user: string }) {
  const q = useQuery({
    queryKey: ["exchange", "shop", user, ""],
    queryFn: ({ signal }) => getShop("", signal),
    refetchInterval: 30000,
  });
  return (
    <Panel title={msg("果壳币")} to="/exchange">
      {q.isError && <Failure retry={() => void q.refetch()} />}
      {!q.data ? (
        !q.isError && (
          <p className="desk-empty" role="status">
            {msg("正在读取")}
          </p>
        )
      ) : (
        <>
          <div className="desk-balance">
            <span className="desk-mark">
              <Coins aria-hidden="true" />
            </span>
            <div>
              <span>{msg("可用余额")}</span>
              <strong>{number(q.data.available_minor / 100)}</strong>
            </div>
          </div>
          <dl className="desk-wallet-details">
            <div>
              <dt>{msg("兑换占用")}</dt>
              <dd>{number(q.data.reserved_minor / 100)}</dd>
            </div>
            <div>
              <dt>{msg("累计消费")}</dt>
              <dd>{number(q.data.spent_minor / 100)}</dd>
            </div>
          </dl>
        </>
      )}
    </Panel>
  );
}
function PAPSummary({ user }: { user: string }) {
  const q = useQuery({
    queryKey: ["attendance", "alliance-pap", user],
    queryFn: ({ signal }) => getAlliancePAP(signal),
    refetchInterval: 30000,
  });
  const requirement = useQuery({
    queryKey: ["attendance", "pap-requirement", user],
    queryFn: ({ signal }) => getPAPRequirement(signal),
    refetchInterval: 30000,
  });
  const target = requirement.data?.monthly_points;
  const report = q.data;
  const achieved =
    target !== undefined && !!report?.available && report.points >= target;
  return (
    <Panel title={msg("联盟 PAP")} to="/attendance?view=alliance-pap">
      {q.isError && <Failure retry={() => void q.refetch()} />}
      {!q.data ? (
        !q.isError && (
          <p className="desk-empty" role="status">
            {msg("正在读取")}
          </p>
        )
      ) : (
        <>
          <div className="desk-pap-overview">
            {report?.available &&
            target !== undefined &&
            !requirement.isError ? (
              <Suspense
                fallback={<div className="ring-progress-placeholder" />}
              >
                <RingProgress
                  value={report.points}
                  target={target}
                  label={msg("本月联盟 PAP")}
                />
              </Suspense>
            ) : (
              <strong className="desk-pap-number">
                {report?.available ? number(report.points) : "—"}
              </strong>
            )}
            <div className="desk-pap-copy">
              <span>{msg("本月联盟 PAP")}</span>
              <strong>
                {target !== undefined && !requirement.isError
                  ? msg("月度目标 {0} PAP", target)
                  : "—"}
              </strong>
              {report?.available &&
              target !== undefined &&
              !requirement.isError ? (
                <span
                  className={
                    "desk-pap-status" + (achieved ? " is-complete" : "")
                  }
                >
                  {achieved && <Check size={16} aria-hidden="true" />}
                  {achieved
                    ? msg("达标")
                    : msg(
                        "还差 {0} PAP",
                        number(Math.max(0, target - report.points)),
                      )}
                </span>
              ) : (
                <small>
                  {report?.available
                    ? msg("集结分要求暂不可用")
                    : msg("暂无联盟 PAP 数据")}
                </small>
              )}
            </div>
          </div>
          {requirement.isError && (
            <Failure retry={() => void requirement.refetch()} />
          )}
          <ul className="desk-pap-characters">
            {report?.characters.slice(0, 3).map((c) => (
              <li key={c.character_id}>
                <EveImage
                  kind="character"
                  id={String(c.character_id)}
                  className="desk-avatar"
                />
                <span>{c.character_name}</span>
                <strong>
                  {number(c.pap)} <small>PAP</small>
                </strong>
              </li>
            ))}
          </ul>
        </>
      )}
    </Panel>
  );
}
function WalletSummary({ user, main }: { user: string; main: string }) {
  const from = monthStart().toISOString();
  const q = useQuery({
    queryKey: ["wallet", "summary", user, from],
    queryFn: ({ signal }) => wallet.summary(from, signal),
    refetchInterval: 60000,
  });
  const characters = useQuery({
    queryKey: ["identity", "characters", user],
    queryFn: ({ signal }) => getCharacters(signal),
    staleTime: 30000,
  });
  const rows = [...(characters.data?.characters || [])].sort(
    (a, b) => Number(b.id === main) - Number(a.id === main),
  );
  return (
    <Panel title={msg("个人钱包")} to="/wallet">
      {q.isError || characters.isError ? (
        <Failure
          retry={() => {
            void q.refetch();
            void characters.refetch();
          }}
        />
      ) : !q.data || !characters.data ? (
        <p className="desk-empty" role="status">
          {msg("正在读取")}
        </p>
      ) : !rows.length ? (
        <p className="desk-empty">{msg("暂无绑定角色")}</p>
      ) : (
        <WalletBalances
          characters={rows}
          summary={q.data}
        />
      )}
    </Panel>
  );
}
function WalletBalances({
  characters,
  summary,
}: {
  characters: { id: string; name: string }[];
  summary: wallet.Summary;
}) {
  const byID = new Map(summary.items.map((item) => [item.owner_id, item]));
  const values = summary.items
    .map((item) => item.balance)
    .filter(
      (v): v is string => typeof v === "string" && wallet.money(v) !== "—",
    );
  const total = sumWalletBalances(values);
  const complete = values.length === characters.length;
  const speeds = walletSpeeds(summary.items, characters.length);
  return (
    <>
      <div className="desk-balance desk-isk">
        <div>
          <span>
            {complete
              ? msg("全部角色合计（ISK）")
              : msg("已读取余额合计（ISK）")}
          </span>
          <strong>{wallet.money(total)}</strong>
        </div>
      </div>
      {!complete && (
        <p className="desk-wallet-note" role="status">
          {msg("已读取 {0}/{1} 个角色钱包", values.length, characters.length)}
        </p>
      )}
      <div className="desk-wallet-speed" aria-label={msg("本月钱包速度")}>
        <div>
          <span>
            <TrendingUp size={16} aria-hidden="true" />
            {msg("赚钱速度")}
          </span>
          <strong>{speeds.income == null ? "—" : number(speeds.income)}</strong>
          <small>
            ISK / {msg("日")} · {msg("本月日均")}
          </small>
        </div>
        <div>
          <span>
            <TrendingDown size={16} aria-hidden="true" />
            {msg("花钱速度")}
          </span>
          <strong>
            {speeds.expense == null ? "—" : number(speeds.expense)}
          </strong>
          <small>
            ISK / {msg("日")} · {msg("本月日均")}
          </small>
        </div>
      </div>
      <ul className="desk-wallet-list">
        {characters.slice(0, 3).map((c) => {
          const snapshot = byID.get(c.id);
          return (
            <li key={c.id}>
              <Link to={snapshot ? "/wallet?owner=" + c.id : "/account"}>
                <EveImage kind="character" id={c.id} className="desk-avatar" />
                <span className="desk-wallet-person">
                  <strong>{c.name}</strong>
                  <small>
                    {!snapshot ? (
                      msg("钱包不可读取")
                    ) : snapshot.observed_at ? (
                      <time dateTime={snapshot.observed_at}>
                        {date(snapshot.observed_at)}
                      </time>
                    ) : (
                      msg("暂无余额记录")
                    )}
                  </small>
                </span>
                <span className="desk-wallet-value">
                  {wallet.money(snapshot?.balance)}
                </span>
              </Link>
            </li>
          );
        })}
      </ul>
    </>
  );
}

function monthStart() {
  const now = new Date();
  return new Date(now.getFullYear(), now.getMonth(), 1);
}

function walletSpeeds(
  rows: wallet.Summary["items"],
  characterCount: number,
) {
  if (rows.length !== characterCount)
    return { income: null as number | null, expense: null as number | null };
  const incomeTotal = sumWalletBalances(rows.map((row) => row.income));
  const expenseTotal = sumWalletBalances(rows.map((row) => row.expense));
  if (incomeTotal === null || expenseTotal === null)
    return { income: null as number | null, expense: null as number | null };
  const income = Number(incomeTotal);
  const expense = Number(expenseTotal);
  if (!Number.isFinite(income) || !Number.isFinite(expense))
    return { income: null as number | null, expense: null as number | null };
  const days = Math.max(
    (Date.now() - monthStart().getTime()) / (24 * 60 * 60 * 1000),
    1,
  );
  return { income: income / days, expense: expense / days };
}

function reviewAmount(item: QueueItem): string {
  const loss = lossQueueAmount(item);
  if (loss?.label === "pending")
    return msg(item.kind === "solo" ? "待核价" : "审核时确定");
  if (item.unit.toLowerCase() === "isk")
    return `${loss?.label === "quote" ? `${msg("核价金额")} · ` : ""}${number((loss?.amount ?? item.amount_minor) / 100)} ISK`;
  if (item.unit === "coin")
    return `${number(item.amount_minor / 100)} ${msg("果壳币")}`;
  return msg("实物奖励");
}

function ReviewDesk({ user }: { user: string }) {
  const [view, setView] = useState("pending");
  const [knownCounts, setKnownCounts] = useState<{
    user: string;
    values: Record<string, number>;
  } | null>(null);
  const client = useQueryClient();
  const warmState = useRef<{ user: string; started: boolean; cancelled: boolean } | null>(null);
  useEffect(() => {
    const state = { user, started: false, cancelled: false };
    warmState.current = state;
    return () => { state.cancelled = true; };
  }, [user]);
  const context = useQuery({
    queryKey: ["approval", "context", user],
    queryFn: ({ signal }) => getContext(signal),
    refetchInterval: 30000,
  });
  const params = new URLSearchParams({ view });
  const q = useQuery({
    queryKey: ["approval", "queue", user, params.toString()],
    queryFn: ({ signal }) => getQueue(params, signal),
    // Dashboard only mounts for confirmed site admins; the list API checks
    // source access again. Start it alongside context to avoid another RTT.
    staleTime: 30000,
    refetchInterval: 30000,
  });
  const warmView = (target: string) => {
    if (context.data?.allowed !== true || target === view) return;
    const next = new URLSearchParams({ view: target });
    void client.prefetchQuery({
      queryKey: ["approval", "queue", user, next.toString()],
      queryFn: ({ signal }) => getQueue(next, signal),
      staleTime: 30000,
    });
  };
  // Load the other categories one at a time after the visible list is ready.
  // An intended category can jump the queue through hover or keyboard focus.
  useEffect(() => {
    const state = warmState.current;
    if (
      !state || state.user !== user || state.started ||
      context.data?.allowed !== true || !q.data || q.data.unavailable.length > 0
    ) return;
    state.started = true;
    void (async () => {
      for (const target of queueViews) {
        if (state.cancelled) return;
        if (target.id === view) continue;
        const next = new URLSearchParams({ view: target.id });
        await client.prefetchQuery({
          queryKey: ["approval", "queue", user, next.toString()],
          queryFn: ({ signal }) => getQueue(next, signal),
          staleTime: 30000,
        });
      }
    })();
  }, [client, context.data?.allowed, q.data, user, view]);
  const counts = q.data?.counts ?? (
    knownCounts?.user === user ? knownCounts.values : undefined
  );
  const incomplete =
    context.isError ||
    !!context.data?.unavailable.length ||
    q.isError ||
    !!q.data?.unavailable.length;
  const refresh = () => {
    void context.refetch();
    if (context.data?.allowed) void q.refetch();
  };
  if (
    context.isError ||
    (!context.data?.allowed && context.data?.unavailable.length)
  )
    return (
      <Panel title={msg("审批待办")}>
        <Failure retry={refresh} />
      </Panel>
    );
  if (!context.data?.allowed) return null;
  return (
    <section className="desk-review" aria-labelledby="desk-review-title">
      <header className="desk-section-heading">
        <h2 id="desk-review-title">{msg("审批待办")}</h2>
        <div>
          <Link className="desk-more" to={`/approvals?view=${view}`}>
            {msg("审批中心")}
            <ArrowUpRight size={16} aria-hidden="true" />
          </Link>
          <IconAction
            label={msg("刷新待办")}
            disabled={q.isFetching || context.isFetching}
            onClick={refresh}
          >
            <RefreshCw size={17} aria-hidden="true" />
          </IconAction>
        </div>
      </header>
      {incomplete && (
        <div className="desk-feedback" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <span>{msg("部分待办暂不可用，请重试")}</span>
        </div>
      )}
      <div className="desk-metrics" role="group" aria-label={msg("待办分类")}>
        {queueViews.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            type="button"
            aria-pressed={view === id}
            onPointerEnter={() => warmView(id)}
            onFocus={() => warmView(id)}
            onClick={() => {
              if (id !== view && view === "pending" && q.data?.unavailable.length === 0)
                setKnownCounts({ user, values: q.data.counts });
              setView(id);
            }}
          >
            <span className="desk-metric-label">
              <Icon size={18} aria-hidden="true" />
              {label}
            </span>
            <strong>
              {incomplete || counts?.[id] === undefined
                ? "—"
                : number(counts[id])}
            </strong>
          </button>
        ))}
      </div>
      <Card className="desk-queue">
        {!q.data ? (
          <p className="desk-empty" role="status">
            {q.isError ? msg("数据暂不可用") : msg("正在读取")}
          </p>
        ) : q.data.items.length ? (
          <ul>
            {q.data.items.slice(0, 5).map((item) => (
              <li key={`${item.source}:${item.id}`}>
                <Link to={approvalLink(item.source, item.id)}>
                  <span className="desk-task-icon">
                    <ClipboardCheck size={19} aria-hidden="true" />
                  </span>
                  <span className="desk-task-copy">
                    <strong>
                      {item.title ||
                        (item.source === "exchange"
                          ? msg("奖励兑换")
                          : msg("军团福利"))}{" "}
                      <small>#{item.id}</small>
                    </strong>
                  </span>
                  <span className="desk-task-person">
                    {item.applicant || item.recipient}
                  </span>
                  <span className="desk-task-amount">
                    {reviewAmount(item)}
                  </span>
                  <time dateTime={item.time}>{date(item.time)}</time>
                  <ChevronRight size={18} aria-hidden="true" />
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className="desk-empty">
            <CheckCheck size={20} aria-hidden="true" />
            {incomplete ? msg("暂无可显示的待办") : msg("当前分类暂无待办")}
          </p>
        )}
      </Card>
    </section>
  );
}
function RecentOrders({ user }: { user: string }) {
  const q = useQuery({
    queryKey: ["exchange", "orders", user, false, ""],
    queryFn: ({ signal }) => getOrders(false, "", signal),
    refetchInterval: 30000,
  });
  return (
    <Panel title={msg("最近兑换")} to="/exchange">
      {q.isError && <Failure retry={() => void q.refetch()} />}
      {!q.data ? (
        !q.isError && (
          <p className="desk-empty" role="status">
            {msg("正在读取")}
          </p>
        )
      ) : !q.data.items.length ? (
        <p className="desk-empty">
          <PackageCheck size={20} aria-hidden="true" />
          {msg("暂无兑换记录")}
        </p>
      ) : (
        <ul className="desk-orders">
          {q.data.items.slice(0, 5).map((order) => (
            <li key={order.id}>
              <div>
                <strong>
                  {order.name} <small>× {number(order.quantity)}</small>
                </strong>
                <span>{order.recipient_name}</span>
              </div>
              <time dateTime={order.created_at}>{date(order.created_at)}</time>
              <span>
                {number(order.coins_minor / 100)} {msg("果壳币")}
              </span>
              <span className="desk-order-state">
                {orderStates[order.state]}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
