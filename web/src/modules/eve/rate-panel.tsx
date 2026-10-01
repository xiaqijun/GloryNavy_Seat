import { msg, getLocale } from "@/lib/i18n";
import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useMemo, useState } from "react";
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  RefreshCw,
  Search,
  Gauge,
  Activity,
  Timer,
  ChartNoAxesCombined,
  TableProperties,
  CircleCheck,
  CircleHelp,
  CirclePause,
  Clock3,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import {
  getRateLimits,
  getRateRoutes,
  bucketState,
  type RateBucket,
} from "./rate-api";
import { syncDate } from "./sync-api";
import "./rate.css";

const BarComparison = lazy(() => import("@/components/charts/bar-comparison"));

const number = (v: number | null) =>
  v === null ? "—" : v.toLocaleString(getLocale());
const when = (v: string | null) => (v ? syncDate(v) : "—");

export function RatePanel({ user }: { user: string }) {
  const [draft, setDraft] = useState("");
  const [search, setSearch] = useState("");
  const [cursors, setCursors] = useState([""]);
  const query = useQuery({
    queryKey: ["eve", "rate-limits", user, search, cursors.at(-1)],
    queryFn: ({ signal }) => getRateLimits(search, cursors.at(-1)!, signal),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });
  return (
    <Card className="py-0">
      <CardContent className="sync-list-content">
        <div className="sync-toolbar">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              setSearch(draft.trim());
              setCursors([""]);
            }}
          >
            <label className="sr-only" htmlFor="bucket-search">
              {msg("搜索令牌桶或角色")}{" "}
            </label>
            <input
              id="bucket-search"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              maxLength={80}
              placeholder={msg("搜索令牌桶或角色")}
            />
            <IconAction type="submit" label={msg("搜索令牌桶")}>
              <Search />
            </IconAction>
          </form>
          <IconAction
            label={msg("刷新令牌桶")}
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw />
          </IconAction>
        </div>
        {query.isPending ? (
          <p role="status" className="sync-feedback">
            {msg("正在读取令牌桶")}{" "}
          </p>
        ) : query.isError ? (
          <div role="alert" className="sync-feedback">
            {msg("令牌桶读取失败")}{" "}
            <IconAction
              label={msg("重试令牌桶")}
              onClick={() => void query.refetch()}
            >
              <RefreshCw />
            </IconAction>
          </div>
        ) : (
          <>
            <p className="rate-caption">
              <Activity size={14} aria-hidden="true" />{" "}
              {msg("配额为响应快照，消耗按响应头累计。")}{" "}
            </p>
            {query.data.buckets.length ? (
              <div className="rate-list">
                {query.data.buckets.map((b) => (
                  <BucketRow
                    key={b.id}
                    bucket={b}
                    user={user}
                    now={Date.parse(query.data.observed_at)}
                  />
                ))}
              </div>
            ) : (
              <p className="sync-feedback">{msg("暂无匹配的令牌桶")}</p>
            )}
            <div className="sync-pagination">
              <span>
                {msg("第")} {cursors.length} {msg("页")}
              </span>
              <IconAction
                label={msg("上一页令牌桶")}
                disabled={cursors.length === 1}
                onClick={() => setCursors((c) => c.slice(0, -1))}
              >
                <ChevronLeft />
              </IconAction>
              <IconAction
                label={msg("下一页令牌桶")}
                disabled={!query.data.next_cursor}
                onClick={() =>
                  setCursors((c) => [...c, query.data.next_cursor])
                }
              >
                <ChevronRight />
              </IconAction>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function BucketRow({
  bucket: b,
  user,
  now,
}: {
  bucket: RateBucket;
  user: string;
  now: number;
}) {
  const [open, setOpen] = useState(false);
  const label = b.group;
  const state = bucketState(b, now);
  const waiting = state === msg("限流等待") || state === msg("本地预算等待");
  const StateIcon = waiting
    ? CirclePause
    : state === msg("尚未计量")
      ? CircleHelp
      : state === msg("快照已过期")
        ? Clock3
        : CircleCheck;
  return (
    <section
      className="rate-bucket"
      aria-label={`${label} ${b.name || b.character_id || msg("公共出口")}`}
    >
      <div className="rate-bucket-main">
        <div className="rate-identity">
          <div className="rate-owner">
            {b.character_id ? (
              <EveImage
                id={b.character_id}
                kind="character"
                className="sync-avatar"
              />
            ) : (
              <span className="rate-public">
                <Gauge size={20} aria-hidden="true" />
              </span>
            )}
            <div>
              <h2>{label}</h2>
              <span className="sync-time">
                {b.name || b.character_id || msg("公共出口")}
              </span>
            </div>
          </div>
          <span
            className={`sync-state ${waiting ? "is-error" : ""}`}
          >
            <StateIcon size={14} aria-hidden="true" />
            {state}
          </span>
          <span className="sync-time rate-observed">
            <Clock3 size={14} aria-hidden="true" />
            <time dateTime={b.header_at || undefined}>{when(b.header_at)}</time>
          </span>
        </div>
        <div className="rate-quota">
          <span className="rate-label">
            <Gauge size={14} aria-hidden="true" /> {msg("ESI 剩余 / 容量")}{" "}
          </span>
          <strong>
            {number(b.remaining)} <span>/ {number(b.capacity)}</span>
          </strong>
          {b.capacity !== null && b.capacity > 0 && b.remaining !== null && (
            <meter
              min={0}
              max={b.capacity}
              value={Math.min(b.remaining, b.capacity)}
              aria-label={msg("{0}最近响应剩余配额", label)}
            />
          )}
          <span className="sync-time rate-window">
            <Timer size={14} aria-hidden="true" />
            <span>
              {b.window_seconds
                ? b.window_seconds % 60 === 0
                  ? msg("{0} 分钟", b.window_seconds / 60)
                  : msg("{0} 秒", b.window_seconds)
                : msg("窗口未知")}
            </span>
            <span>
              {b.policy_source === "openapi"
                ? msg("官方配置")
                : b.policy_source === "response"
                  ? msg("响应配置")
                  : msg("配置未知")}
            </span>
          </span>
        </div>
        <div className="rate-total">
          <span className="rate-label">
            <ChartNoAxesCombined size={14} aria-hidden="true" />{" "}
            {msg("已计量消耗")}{" "}
          </span>
          <strong>{number(b.used_tokens)}</strong>
          <span className="sync-time">
            {number(b.network_requests)} {msg("次请求")}{" "}
            {b.unmeasured_requests > 0
              ? msg(" · {0} 次未计量", number(b.unmeasured_requests))
              : ""}
          </span>
        </div>
        <IconAction
          label={msg("{0}{1}接口消耗", open ? msg("收起") : msg("查看"), label)}
          aria-expanded={open}
          aria-controls={`bucket-${b.id}`}
          onClick={() => setOpen(!open)}
        >
          {open ? <ChevronUp /> : <ChevronDown />}
        </IconAction>
      </div>
      {open && (
        <div id={`bucket-${b.id}`} className="rate-detail">
          <dl className="token-facts">
            <div>
              <dt>{msg("本地可用（估算）")}</dt>
              <dd>{number(b.local_remaining)}</dd>
            </div>
            <div>
              <dt>{msg("下次配额恢复")}</dt>
              <dd>{when(b.local_recovery_at)}</dd>
            </div>
            <div>
              <dt>{msg("ESI 重试时间")}</dt>
              <dd>{when(b.retry_at)}</dd>
            </div>
            <div>
              <dt>{msg("累计起点")}</dt>
              <dd>{when(b.observed_since)}</dd>
            </div>
          </dl>
          {[b.local_blocked_until, b.egress_blocked_until].some(
            (t) => t && Date.parse(t) > now,
          ) && (
            <p className="token-note">
              {msg("本地限流至")} {when(b.local_blocked_until)}{" "}
              {msg("· 出口保护至")} {when(b.egress_blocked_until)}
            </p>
          )}
          <RouteUsage bucket={b.id} user={user} />
        </div>
      )}
    </section>
  );
}

function RouteUsage({ bucket, user }: { bucket: string; user: string }) {
  const [cursors, setCursors] = useState([""]);
  const [table, setTable] = useState(false);
  const query = useQuery({
    queryKey: ["eve", "rate-routes", user, bucket, cursors.at(-1)],
    queryFn: ({ signal }) => getRateRoutes(bucket, cursors.at(-1)!, signal),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });
  const chartData = useMemo(
    () =>
      (query.data?.routes ?? [])
        .toSorted((a, b) => b.used_tokens - a.used_tokens)
        .slice(0, 10)
        .map((r) => ({
          name:
            routeLabel(r.route) + (r.measured_responses ? "" : msg("\n未计量")),
          value: r.measured_responses ? r.used_tokens : null,
        })),
    [query.data],
  );
  if (query.isPending)
    return (
      <p role="status" className="sync-feedback">
        {msg("正在读取接口消耗")}{" "}
      </p>
    );
  if (query.isError)
    return (
      <div role="alert" className="sync-feedback">
        {msg("接口消耗读取失败")}{" "}
        <IconAction
          label={msg("重试接口消耗")}
          onClick={() => void query.refetch()}
        >
          <RefreshCw />
        </IconAction>
      </div>
    );
  return (
    <>
      {query.data.routes.length > 0 && (
        <>
          <div className="rate-chart-toolbar">
            <h3>
              <ChartNoAxesCombined size={16} aria-hidden="true" />{" "}
              {msg("本页接口消耗")}{" "}
              {query.data.routes.length > 10 ? msg(" · 前 10 项") : ""}
            </h3>
            <IconAction
              label={table ? msg("查看消耗图表") : msg("查看接口明细")}
              aria-pressed={table}
              onClick={() => setTable(!table)}
            >
              {table ? <ChartNoAxesCombined /> : <TableProperties />}
            </IconAction>
          </div>
          {!table && (
            <Suspense
              fallback={
                <p role="status" className="rate-chart-loading">
                  {msg("正在加载图表")}{" "}
                </p>
              }
            >
              <BarComparison
                data={chartData}
                label={msg("本页接口已计量消耗")}
              />
            </Suspense>
          )}
        </>
      )}
      {table && (
        <div
          className="rate-routes"
          role="table"
          aria-label={msg("接口令牌消耗")}
        >
          <div className="rate-route-head" role="row">
            {[
              msg("接口"),
              msg("请求 / 缓存"),
              msg("累计消耗"),
              msg("最近消耗"),
              msg("等待 / 限流"),
            ].map((s) => (
              <span role="columnheader" key={s}>
                {s}
              </span>
            ))}
          </div>
          {query.data.routes.map((r) => (
            <div className="rate-route" role="row" key={r.route}>
              <div className="rate-route-name" role="cell">
                <code>{r.route}</code>
                <span className="sync-time">
                  {when(r.last_response_at)}
                  {r.last_status ? ` · HTTP ${r.last_status}` : ""}
                </span>
              </div>
              <div role="cell">
                <span className="rate-mobile-label">{msg("请求 / 缓存")}</span>
                {number(r.network_requests)} / {number(r.cache_hits)}
              </div>
              <div role="cell">
                <span className="rate-mobile-label">{msg("累计消耗")}</span>
                <strong>{number(r.used_tokens)}</strong>
                {r.unmeasured_requests > 0 && (
                  <span className="sync-time">
                    {number(r.unmeasured_requests)} {msg("次未计量")}{" "}
                  </span>
                )}
              </div>
              <div role="cell">
                <span className="rate-mobile-label">{msg("最近消耗")}</span>
                {number(r.last_used)}
                <span className="sync-time">
                  {r.measured_responses
                    ? msg(
                        "均次 {0}",
                        number(
                          Number(
                            (r.used_tokens / r.measured_responses).toFixed(2),
                          ),
                        ),
                      )
                    : msg("尚未计量")}
                </span>
              </div>
              <div role="cell">
                <span className="rate-mobile-label">{msg("等待 / 限流")}</span>
                {number(r.local_waits)} / {number(r.upstream_limits)}
              </div>
            </div>
          ))}
        </div>
      )}
      {!query.data.routes.length && (
        <p className="sync-feedback">{msg("暂无接口记录")}</p>
      )}
      {(cursors.length > 1 || query.data.next_cursor) && (
        <div className="sync-pagination">
          <span>
            {msg("第")} {cursors.length} {msg("页")}
          </span>
          <IconAction
            label={msg("上一页接口")}
            disabled={cursors.length === 1}
            onClick={() => setCursors((c) => c.slice(0, -1))}
          >
            <ChevronLeft />
          </IconAction>
          <IconAction
            label={msg("下一页接口")}
            disabled={!query.data.next_cursor}
            onClick={() => setCursors((c) => [...c, query.data.next_cursor])}
          >
            <ChevronRight />
          </IconAction>
        </div>
      )}
    </>
  );
}

function routeLabel(route: string) {
  if (/\/contracts\/\{id\}\/items\/$/.test(route)) return msg("物品明细");
  if (/\/contracts\/\{id\}\/bids\/$/.test(route)) return msg("竞价记录");
  if (/\/contracts\/$/.test(route)) return msg("合同列表");
  if (/\/roles\/$/.test(route)) return msg("军团职务");
  return route;
}
