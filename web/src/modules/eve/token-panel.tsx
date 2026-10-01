import { msg, getLocale } from "@/lib/i18n";
import { useQuery } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  History,
  KeyRound,
  RefreshCw,
  Search,
  TriangleAlert,
} from "lucide-react";
import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { EveImage } from "@/components/eve-image";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import {
  getTokens,
  getTokenEvents,
  tokenStates,
  tokenReason,
  type TokenObservation,
} from "./token-api";
import { syncDate } from "./sync-api";
import "./tokens.css";

const when = (value: string | null) => (value ? syncDate(value) : "—");
const number = (value: number) => value.toLocaleString(getLocale());
export function TokenPanel({ user }: { user: string }) {
  const [draft, setDraft] = useState("");
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("");
  const [cursors, setCursors] = useState([""]);
  const query = useQuery({
    queryKey: ["eve", "tokens", user, search, filter, cursors.at(-1)],
    queryFn: ({ signal }) => getTokens(search, filter, cursors.at(-1)!, signal),
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
            <label htmlFor="token-search" className="sr-only">
              {msg("搜索令牌角色")}{" "}
            </label>
            <input
              id="token-search"
              value={draft}
              maxLength={80}
              placeholder={msg("搜索角色名称或 ID")}
              onChange={(e) => setDraft(e.target.value)}
            />
            <IconAction type="submit" label={msg("搜索令牌角色")}>
              <Search />
            </IconAction>
          </form>
          <Select
            label={msg("令牌状态")}
            value={filter}
            onValueChange={(v) => {
              setFilter(v);
              setCursors([""]);
            }}
            options={[
              { value: "", label: msg("全部状态") },
              ...Object.entries(tokenStates).map(([value, label]) => ({
                value,
                label,
              })),
            ]}
          />
          <IconAction
            label={msg("刷新令牌列表")}
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw />
          </IconAction>
        </div>
        {query.isPending ? (
          <p role="status" className="sync-feedback">
            {msg("正在读取令牌状态")}{" "}
          </p>
        ) : query.isError ? (
          <div role="alert" className="sync-feedback">
            {msg("令牌状态读取失败")}{" "}
            <IconAction
              label={msg("重试令牌列表")}
              onClick={() => void query.refetch()}
            >
              <RefreshCw />
            </IconAction>
          </div>
        ) : (
          <>
            {!query.data.tokens.length ? (
              <p className="sync-feedback">{msg("没有匹配的角色")}</p>
            ) : (
              <div
                className="token-table"
                role="table"
                aria-label={msg("ESI 令牌观测")}
              >
                <div className="token-head" role="row">
                  {[
                    msg("角色"),
                    msg("令牌状态"),
                    msg("访问令牌到期"),
                    msg("最近刷新成功"),
                    msg("请求 / 缓存"),
                    msg("记录"),
                  ].map((v) => (
                    <span key={v} role="columnheader">
                      {v}
                    </span>
                  ))}
                </div>
                {query.data.tokens.map((t) => (
                  <TokenRow
                    key={`${t.character_id}:${t.generation}`}
                    token={t}
                    user={user}
                  />
                ))}
              </div>
            )}
            <div className="sync-pagination">
              <span>
                {msg("第")} {cursors.length} {msg("页")}
              </span>
              <IconAction
                label={msg("上一页令牌")}
                disabled={cursors.length === 1}
                onClick={() => setCursors((v) => v.slice(0, -1))}
              >
                <ChevronLeft />
              </IconAction>
              <IconAction
                label={msg("下一页令牌")}
                disabled={!query.data.next_cursor}
                onClick={() =>
                  setCursors((v) => [...v, query.data.next_cursor])
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

function TokenRow({
  token: t,
  user,
}: {
  token: TokenObservation;
  user: string;
}) {
  const [open, setOpen] = useState(false);
  const events = useQuery({
    queryKey: ["eve", "token-events", user, t.character_id, t.generation],
    queryFn: ({ signal }) => getTokenEvents(t.character_id, signal),
    enabled: open,
    refetchInterval: open ? 30_000 : false,
    refetchIntervalInBackground: false,
  });
  const unhealthy = ["refresh_failed", "reauthorize"].includes(t.state);
  const StateIcon = unhealthy ? TriangleAlert : KeyRound;
  return (
    <div className="token-group" role="rowgroup">
      <div className="token-row" role="row">
        <div role="cell" className="sync-person token-person">
          <EveImage
            kind="character"
            id={t.character_id}
            className="sync-avatar"
          />
          <div>
            <strong>{t.name || t.character_id}</strong>
            <span className="sync-time">{t.character_id}</span>
          </div>
        </div>
        <div role="cell" className="token-state">
          <span
            className={`sync-state ${unhealthy ? "is-error" : t.state === "valid" ? "is-fresh" : ""}`}
          >
            <StateIcon size={14} aria-hidden="true" />
            {tokenStates[t.state]}
          </span>
          <span className="sync-time">
            {t.scopes.length} {msg("项授权")}
          </span>
        </div>
        <div role="cell" className="token-expiry">
          <span className="token-mobile-label">{msg("访问令牌到期")}</span>
          <time dateTime={t.access_expires_at ?? undefined}>
            {when(t.access_expires_at)}
          </time>
        </div>
        <div role="cell" className="token-refresh">
          <span className="token-mobile-label">{msg("最近刷新成功")}</span>
          <time dateTime={t.last_refresh_success_at ?? undefined}>
            {when(t.last_refresh_success_at)}
          </time>
          {t.last_refresh_reason && (
            <span className="token-error">
              {tokenReason(t.last_refresh_reason)}
            </span>
          )}
        </div>
        <div role="cell" className="token-count">
          <span className="token-mobile-label">{msg("请求 / 缓存")}</span>
          <span>
            {number(t.network_requests)} / {number(t.cache_hits)}
          </span>
          {t.rate_limit_waits > 0 && (
            <span className="sync-time">
              {msg("限流等待")} {number(t.rate_limit_waits)}
            </span>
          )}
        </div>
        <div role="cell" className="token-action">
          <IconAction
            label={msg(
              "{0}{1}的令牌记录",
              open ? msg("收起") : msg("查看"),
              t.name || t.character_id,
            )}
            aria-expanded={open}
            aria-controls={`token-detail-${t.character_id}`}
            onClick={() => setOpen((v) => !v)}
          >
            <History />
          </IconAction>
        </div>
      </div>
      {open && (
        <div
          className="token-detail"
          role="row"
          id={`token-detail-${t.character_id}`}
        >
          <div role="cell" aria-colspan={6}>
            <dl className="token-facts">
              {[
                [msg("授权代次"), t.generation],
                [msg("开始观测"), when(t.observed_since)],
                [msg("令牌复用"), number(t.reuse_count)],
                [msg("最近取用"), when(t.last_used_at)],
                [
                  msg("刷新成功 / 失败"),
                  `${number(t.refresh_successes)} / ${number(t.refresh_failures)}`,
                ],
                [msg("最近刷新尝试"), when(t.last_refresh_attempt_at)],
                [msg("请求失败"), number(t.request_failures)],
                [msg("最近请求"), when(t.last_request_at)],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
            <p className="token-note">
              {msg(
                "到期按需刷新；统计从本次授权观测开始。缓存命中不发送 ESI 请求。",
              )}{" "}
            </p>
            {t.last_request_reason && (
              <p className="token-request-notice" role="status">
                {tokenReason(t.last_request_reason)}
                {t.last_request_status > 0
                  ? ` · HTTP ${t.last_request_status}`
                  : ""}
              </p>
            )}
            <details className="token-scopes">
              <summary>
                {msg("授权范围 ·")} {t.scopes.length}
              </summary>
              <ul>
                {t.scopes.map((scope) => (
                  <li key={scope}>{scope}</li>
                ))}
              </ul>
            </details>
            <h2 className="token-history-title">{msg("最近记录")}</h2>
            {events.isPending ? (
              <p role="status">{msg("正在读取记录")}</p>
            ) : events.isError ? (
              <div role="alert">
                {msg("令牌记录读取失败")}{" "}
                <IconAction
                  label={msg("重试令牌记录")}
                  onClick={() => void events.refetch()}
                >
                  <RefreshCw />
                </IconAction>
              </div>
            ) : events.data.events.length ? (
              <ul className="token-events">
                {events.data.events.map((e) => (
                  <li key={e.id}>
                    <time dateTime={e.occurred_at}>{when(e.occurred_at)}</time>
                    <span
                      className={
                        e.outcome === "refresh_failed" ? "token-error" : ""
                      }
                    >
                      {e.outcome === "authorized"
                        ? msg("完成授权")
                        : e.outcome === "refresh_success"
                          ? msg("刷新成功")
                          : tokenReason(e.reason) || msg("刷新失败")}
                    </span>
                    <span className="sync-time">
                      {msg("代次")} {e.generation}
                      {e.outcome !== "authorized" &&
                        ` · ${number(e.duration_ms)} ms`}
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="muted">{msg("暂无观测记录")}</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
