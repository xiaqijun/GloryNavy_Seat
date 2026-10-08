import { msg } from "@/lib/i18n";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  Clock3,
  History,
  KeyRound,
  Gauge,
  ListChecks,
  RefreshCw,
  Search,
  TriangleAlert,
} from "lucide-react";
import { useState } from "react";
import { Navigate, useSearchParams } from "react-router-dom";
import { Tabs } from "radix-ui";
import { RatePanel } from "./rate-panel";
import { TokenPanel } from "./token-panel";
import { Card, CardContent } from "@/components/ui/card";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import { useManagementAccess } from "@/modules/access";
import {
  getSyncTargets,
  getSyncRuns,
  refreshSync,
  resourceLabel,
  contractDetailLabel,
  reasonLabel,
  syncLabel,
  syncDate,
  type SyncTarget,
} from "./sync-api";
import "./sync.css";

export default function SyncPage() {
  const { session, access } = useManagementAccess();
  if (session.isSuccess && !session.data.session)
    return <Navigate to="/" replace />;
  if (session.isError || access.isError)
    return (
      <div className="sync-feedback" role="alert">
        <p>{msg("权限信息读取失败")}</p>
        <IconAction
          label={msg("重试权限信息")}
          onClick={() => {
            void session.refetch();
            void access.refetch();
          }}
        >
          <RefreshCw />
        </IconAction>
      </div>
    );
  if (session.isPending || access.isPending)
    return (
      <p role="status" className="muted">
        {msg("正在读取权限")}{" "}
      </p>
    );
  if (!access.data.administrator && !access.data.can_manage_sync)
    return (
      <div>
        <h1>{msg("ESI 同步")}</h1>
        <p className="sync-feedback">{msg("没有同步管理权限")}</p>
      </div>
    );
  return (
    <SyncWorkspace
      user={session.data!.session!.user_id}
      csrf={session.data!.session!.csrf_token}
    />
  );
}
function SyncWorkspace({ user, csrf }: { user: string; csrf: string }) {
  const [params, setParams] = useSearchParams();
  const selected = params.get("view");
  const view =
    selected === "tokens" || selected === "rate-limits" ? selected : "tasks";
  return (
    <Tabs.Root
      className="sync-page"
      value={view}
      onValueChange={(v) => setParams(v === "tasks" ? {} : { view: v })}
    >
      <div className="page-heading">
        <h1>{msg("ESI 同步")}</h1>
      </div>
      <Tabs.List className="sync-tabs" aria-label={msg("同步管理视图")}>
        <Tabs.Trigger className="sync-tab" value="tasks">
          <ListChecks size={16} aria-hidden="true" />
          {msg("同步任务")}{" "}
        </Tabs.Trigger>
        <Tabs.Trigger className="sync-tab" value="rate-limits">
          <Gauge size={16} aria-hidden="true" />
          {msg("令牌桶")}{" "}
        </Tabs.Trigger>
        <Tabs.Trigger className="sync-tab" value="tokens">
          <KeyRound size={16} aria-hidden="true" />
          {msg("登录令牌")}{" "}
        </Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="tasks" className="sync-panel">
        <SyncManagement user={user} csrf={csrf} />
      </Tabs.Content>
      <Tabs.Content value="rate-limits" className="sync-panel">
        <RatePanel user={user} />
      </Tabs.Content>
      <Tabs.Content value="tokens" className="sync-panel">
        <TokenPanel user={user} />
      </Tabs.Content>
    </Tabs.Root>
  );
}
function SyncManagement({ user, csrf }: { user: string; csrf: string }) {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("");
  const [draft, setDraft] = useState("");
  const [cursors, setCursors] = useState<string[]>([""]);
  const [notice, setNotice] = useState("");
  const client = useQueryClient();
  const query = useQuery({
    queryKey: [
      "eve",
      "sync",
      "management",
      user,
      search,
      filter,
      cursors.at(-1),
    ],
    queryFn: ({ signal }) =>
      getSyncTargets(search, filter, cursors.at(-1) ?? "", signal),
    refetchInterval: (q) =>
      q.state.data?.targets.some((t) => ["queued", "running"].includes(t.state))
        ? 5_000
        : 30_000,
    refetchIntervalInBackground: false,
  });
  const retry = useMutation({
    mutationFn: (id: string) => refreshSync(id, csrf, undefined, true),
    onSuccess: async (rows) => {
      setNotice(
        rows.some((r) => r.outcome === "deferred")
          ? msg("已安排，将在可更新时重试")
          : msg("已加入同步队列"),
      );
      await client.invalidateQueries({ queryKey: ["eve", "sync"] });
    },
  });
  return (
    <div className="sync-page">
      <Card className="py-0">
        <CardContent className="sync-list-content">
          <div className="sync-toolbar">
            <IconAction
              label={msg("刷新同步列表")}
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              <RefreshCw aria-hidden="true" />
            </IconAction>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                setSearch(draft.trim());
                setCursors([""]);
              }}
            >
              <label className="sr-only" htmlFor="sync-search">
                {msg("搜索角色")}{" "}
              </label>
              <input
                id="sync-search"
                placeholder={msg("搜索角色名称或 ID")}
                value={draft}
                maxLength={80}
                onChange={(e) => setDraft(e.target.value)}
              />
              <IconAction type="submit" label={msg("搜索角色")}>
                <Search aria-hidden="true" />
              </IconAction>
            </form>
            <label>
              <span className="sr-only">{msg("同步状态")}</span>
              <select
                aria-label={msg("同步状态")}
                value={filter}
                onChange={(e) => {
                  setFilter(e.target.value);
                  setCursors([""]);
                }}
              >
                <option value="">{msg("全部状态")}</option>
                <option value="failed">{msg("同步失败")}</option>
                <option value="blocked">{msg("需要处理")}</option>
                <option value="deferred">{msg("稍后重试")}</option>
                <option value="running">{msg("同步中")}</option>
                <option value="queued">{msg("已排队")}</option>
                <option value="idle">{msg("等待下次同步")}</option>
              </select>
            </label>
          </div>
          {notice && (
            <p role="status" className="sync-notice">
              {notice}
            </p>
          )}
          {retry.error && (
            <p role="alert" className="login-error">
              {retry.error.message}
            </p>
          )}
          {query.isPending ? (
            <p role="status" className="sync-feedback">
              {msg("正在读取同步任务")}{" "}
            </p>
          ) : query.isError ? (
            <div role="alert" className="sync-feedback">
              <p>{msg("同步列表读取失败")}</p>
              <IconAction
                label={msg("重试同步列表")}
                onClick={() => void query.refetch()}
              >
                <RefreshCw />
              </IconAction>
            </div>
          ) : (
            <>
              {!query.data.available && (
                <p className="sync-offline">
                  <TriangleAlert size={16} aria-hidden="true" />
                  {msg("同步服务未就绪")}{" "}
                </p>
              )}
              {query.data.targets.length === 0 ? (
                <p className="sync-feedback">{msg("暂无匹配的同步任务")}</p>
              ) : (
                <div
                  className="sync-table"
                  role="table"
                  aria-label={msg("ESI 同步任务")}
                >
                  <div className="sync-table-head" role="row">
                    {[
                      msg("角色"),
                      msg("数据项"),
                      msg("状态"),
                      msg("最近成功"),
                      msg("下次执行"),
                      msg("操作"),
                    ].map((s) => (
                      <span role="columnheader" key={s}>
                        {s}
                      </span>
                    ))}
                  </div>
                  {query.data.targets.map((t) => (
                    <TargetRow
                      key={t.id}
                      target={t}
                      disabled={!query.data.available || retry.isPending}
                      retry={() => {
                        setNotice("");
                        retry.mutate(t.id);
                      }}
                    />
                  ))}
                </div>
              )}
              <div className="sync-pagination">
                <span>
                  {msg("第")} {cursors.length} {msg("页")}
                </span>
                <IconAction
                  label={msg("上一页")}
                  disabled={cursors.length === 1}
                  onClick={() => setCursors((c) => c.slice(0, -1))}
                >
                  <ChevronLeft />
                </IconAction>
                <IconAction
                  label={msg("下一页")}
                  disabled={!query.data.next_cursor}
                  onClick={() =>
                    setCursors((c) => [...c, query.data.next_cursor!])
                  }
                >
                  <ChevronRight />
                </IconAction>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
function TargetRow({
  target: t,
  disabled,
  retry,
}: {
  target: SyncTarget;
  disabled: boolean;
  retry: () => void;
}) {
  const [open, setOpen] = useState(false);
  const runs = useQuery({
    queryKey: ["eve", "sync", "runs", t.id],
    queryFn: ({ signal }) => getSyncRuns(t.id, signal),
    enabled: open,
  });
  return (
    <div className="sync-table-group" role="rowgroup">
      <div className="sync-table-row" role="row">
        <div className="sync-person" role="cell">
          <EveImage
            id={t.character_id}
            kind="character"
            className="sync-avatar"
          />
          <div>
            <strong>{t.name || t.character_id}</strong>
            <span className="sync-time">{t.character_id}</span>
          </div>
        </div>
        <span role="cell" className="sync-dataset">
          {resourceLabel(t.resource)}
        </span>
        <div role="cell" className="sync-status-cell">
          <span
            className={`sync-state ${t.freshness === "fresh" && t.state === "idle" ? "is-fresh" : ["blocked", "failed"].includes(t.state) ? "is-error" : ""}`}
          >
            <Clock3 size={14} aria-hidden="true" />
            {syncLabel(t)}
          </span>
          {t.reason && (
            <span className="sync-reason">{reasonLabel(t.reason)}</span>
          )}
          {contractDetailLabel(t) && (
            <span className="sync-reason">{contractDetailLabel(t)}</span>
          )}
        </div>
        <div role="cell" className="sync-last">
          <span className="sync-mobile-label">{msg("最近成功")}</span>
          <time dateTime={t.last_success_at ?? undefined}>
            {syncDate(t.last_success_at)}
          </time>
        </div>
        <div role="cell" className="sync-next">
          <span className="sync-mobile-label">{msg("下次执行")}</span>
          {t.state === "blocked" ? (
            msg("等待处理")
          ) : (
            <time dateTime={t.next_due_at}>{syncDate(t.next_due_at)}</time>
          )}
        </div>
        <div role="cell" className="sync-actions">
          <IconAction
            label={msg(
              "重试{0}的{1}",
              t.name || t.character_id,
              resourceLabel(t.resource),
            )}
            disabled={
              disabled || ["queued", "running", "blocked"].includes(t.state)
            }
            onClick={retry}
          >
            <RefreshCw />
          </IconAction>
          <IconAction
            label={msg(
              "{0}{1}运行记录",
              open ? msg("收起") : msg("查看"),
              resourceLabel(t.resource),
            )}
            aria-expanded={open}
            onClick={() => setOpen(!open)}
          >
            <History />
          </IconAction>
        </div>
      </div>
      {open && (
        <div className="sync-history" role="row">
          <div role="cell" aria-colspan={6}>
            {runs.isPending ? (
              <p role="status">{msg("正在读取记录")}</p>
            ) : runs.isError ? (
              <div role="alert">
                {msg("运行记录读取失败")}{" "}
                <IconAction
                  label={msg("重试运行记录")}
                  onClick={() => void runs.refetch()}
                >
                  <RefreshCw />
                </IconAction>
              </div>
            ) : runs.data.runs.length ? (
              <ul>
                {runs.data.runs.map((run) => (
                  <li key={run.id}>
                    <time dateTime={run.started_at}>
                      {syncDate(run.started_at)}
                    </time>
                    <span>
                      {run.outcome === "success"
                        ? msg("成功")
                        : run.outcome === "running"
                          ? msg("运行中")
                          : reasonLabel(run.reason) || msg("失败")}
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p>{msg("暂无运行记录")}</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
