import { msg, getLocale } from "@/lib/i18n";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import {
  BriefcaseBusiness,
  Check,
  ChevronDown,
  Clock3,
  CloudCheck,
  Crown,
  LoaderCircle,
  MapPin,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
  UsersRound,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { EveImage } from "@/components/eve-image";
import { IconAction } from "@/components/ui/icon-action";
import { Button } from "@/components/ui/button";
import { getAccountAccess } from "./api";

import { roleLabel } from "./role-labels";

export function AuthorizationSummary({
  syncContent,
  identityHeader,
  userID,
  characterID,
  onReauthorize,
  submitting,
  disabled = false,
}: {
  syncContent?: ReactNode;
  identityHeader?: ReactNode;
  userID: string;
  characterID: string;
  onReauthorize: () => void;
  submitting: boolean;
  disabled?: boolean;
}) {
  const query = useQuery({
    queryKey: ["access", "me", userID],
    queryFn: ({ signal }) => getAccountAccess(signal),
    refetchInterval: 30_000,
  });
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, []);
  const fact = query.data?.characters.find(
    (c) => c.character_id === characterID,
  );
  const fresh =
    !!fact &&
    ["ready", "retry"].includes(fact.state) &&
    Date.parse(fact.valid_until) > now;
  const state = fact?.state ?? "reauthorize";
  const statusText =
    state === "pending"
      ? msg("正在同步职务")
      : state === "reauthorize"
        ? msg("需要完成 EVE 授权")
        : !fresh
          ? msg("职务已过期，游戏职务权限暂停。")
          : state === "retry"
            ? msg("同步重试中")
            : msg("已同步");
  const StatusIcon =
    state === "pending" || state === "retry"
      ? RefreshCw
      : fresh
        ? Check
        : TriangleAlert;
  return (
    <section className="account-authorization" aria-label={msg("军团授权")}>
      {query.isPending ? (
        <Card className="account-character-detail">
          {identityHeader}
          <CardContent
            className="account-loading"
            role="status"
            aria-label={msg("正在读取授权")}
            aria-busy="true"
          >
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          </CardContent>
        </Card>
      ) : query.isError ? (
        <Card className="account-character-detail">
          {identityHeader}
          <CardContent className="account-feedback" role="alert">
            <p className="login-error">{msg("授权状态读取失败，请重试。")}</p>
            <IconAction
              label={msg("刷新授权状态")}
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              <RefreshCw aria-hidden="true" />
            </IconAction>
          </CardContent>
        </Card>
      ) : (
        <div className="account-detail-grid">
          <Card className="account-corporation-card account-character-detail">
            {identityHeader}
            <CardContent className="account-card-content">
              <div className="account-section-label">
                <BriefcaseBusiness size={18} aria-hidden="true" />
                <h2>{msg("军团与职务")}</h2>
              </div>
              <div className="account-corporation-summary">
                <div className="account-corporation">
                  <EveImage
                    id={fact?.corporation.id ?? "0"}
                    kind="corporation"
                    className="account-corporation-logo"
                  />
                  <h3>{fact?.corporation.name || msg("军团信息待同步")}</h3>
                </div>
                {fresh ? (
                  <div
                    className="account-role-tags"
                    aria-label={msg("军团职务")}
                  >
                    {fact.character_id === fact.corporation.ceo_id && (
                      <span className="account-role-tag">
                        <Crown size={14} aria-hidden="true" />
                        CEO
                      </span>
                    )}
                    {fact.roles.slice(0, 6).map((r) => (
                      <span className="account-role-tag" key={r} title={r}>
                        {roleLabel(r)}
                      </span>
                    ))}
                    {fact.roles.length === 0 &&
                      fact.character_id !== fact.corporation.ceo_id && (
                        <span className="muted">{msg("无军团职务")}</span>
                      )}
                  </div>
                ) : (
                  <p className="muted">
                    {state === "pending"
                      ? msg("等待职务同步")
                      : msg("暂无有效职务")}
                  </p>
                )}
              </div>
              {fresh && fact.roles.length > 6 && (
                <details className="account-location-roles">
                  <summary>
                    <BriefcaseBusiness size={16} aria-hidden="true" />
                    <span>
                      {msg("更多职务（")}
                      {fact.roles.length - 6}）
                    </span>
                    <ChevronDown size={16} aria-hidden="true" />
                  </summary>
                  <div className="account-role-tags">
                    {fact.roles.slice(6).map((r) => (
                      <span className="account-role-tag" key={r} title={r}>
                        {roleLabel(r)}
                      </span>
                    ))}
                  </div>
                </details>
              )}
              {fresh &&
                fact.roles_at_hq.length +
                  fact.roles_at_base.length +
                  fact.roles_at_other.length >
                  0 && (
                  <details className="account-location-roles">
                    <summary>
                      <MapPin size={16} aria-hidden="true" />
                      <span>{msg("地点职务")}</span>
                      <ChevronDown size={16} aria-hidden="true" />
                    </summary>
                    {(
                      [
                        [msg("总部"), fact.roles_at_hq],
                        [msg("基地"), fact.roles_at_base],
                        [msg("其他地点"), fact.roles_at_other],
                      ] as const
                    ).map(
                      ([name, roles]) =>
                        roles.length > 0 && (
                          <div className="account-location-row" key={name}>
                            <h3>{name}</h3>
                            <div className="account-role-tags">
                              {roles.map((r) => (
                                <span
                                  className="account-role-tag account-role-tag-neutral"
                                  key={r}
                                  title={r}
                                >
                                  {roleLabel(r)}
                                </span>
                              ))}
                            </div>
                          </div>
                        ),
                    )}
                  </details>
                )}
            </CardContent>
            <div className="account-detail-footer">
              {syncContent ? (
                <section
                  className="account-footer-section"
                  aria-label={msg("ESI 同步")}
                >
                  {syncContent}
                  {(state === "reauthorize" || fact?.needs_authorization) && (
                    <Button
                      onClick={onReauthorize}
                      disabled={submitting || disabled}
                    >
                      {submitting ? msg("正在前往 EVE") : msg("更新 EVE 授权")}
                    </Button>
                  )}
                </section>
              ) : (
                <section
                  className="account-footer-section"
                  aria-label={msg("ESI 授权")}
                >
                  <div className="account-section-heading">
                    <div className="account-section-label">
                      <CloudCheck size={18} aria-hidden="true" />
                      <h2>{msg("ESI 授权")}</h2>
                    </div>
                    <IconAction
                      label={msg("刷新授权状态")}
                      disabled={query.isFetching}
                      aria-busy={query.isFetching}
                      onClick={() => void query.refetch()}
                    >
                      <RefreshCw
                        className={
                          query.isFetching ? "animate-spin" : undefined
                        }
                        aria-hidden="true"
                      />
                    </IconAction>
                  </div>
                  <p
                    role="status"
                    className={`account-sync-status ${fresh && state === "ready" ? "is-ready" : "is-attention"}`}
                  >
                    <StatusIcon size={16} aria-hidden="true" />
                    {statusText}
                  </p>
                  {fact && !fact.synced_at.startsWith("0001-") && (
                    <p className="account-sync-time">
                      <Clock3 size={14} aria-hidden="true" />
                      <span className="sr-only">{msg("最近同步：")}</span>
                      <time dateTime={fact.synced_at}>
                        {new Date(fact.synced_at).toLocaleString(getLocale(), {
                          hour12: false,
                        })}
                      </time>
                    </p>
                  )}
                  {fact?.needs_authorization && state !== "reauthorize" && (
                    <p className="authorization-notice">
                      {msg("授权范围已更新，请重新授权。")}{" "}
                    </p>
                  )}
                  {(state === "reauthorize" || fact?.needs_authorization) && (
                    <div>
                      <Button
                        type="button"
                        onClick={onReauthorize}
                        disabled={submitting || disabled}
                        aria-busy={submitting}
                      >
                        {submitting
                          ? msg("正在前往 EVE")
                          : msg("更新 EVE 授权")}
                      </Button>
                    </div>
                  )}
                </section>
              )}
              <section
                className="account-footer-section"
                aria-label={msg("平台角色")}
              >
                <div className="account-section-label">
                  <ShieldCheck size={18} aria-hidden="true" />
                  <h2>{msg("平台角色")}</h2>
                </div>
                <div className="account-role-tags" aria-label={msg("平台角色")}>
                  {query.data!.administrator && (
                    <span className="account-role-tag">
                      <Crown size={14} aria-hidden="true" />
                      {msg("本站管理员")}{" "}
                    </span>
                  )}
                  {query.data!.site_roles.map((r) => (
                    <span
                      className="account-role-tag account-role-tag-neutral"
                      key={r.id}
                    >
                      {r.name}
                    </span>
                  ))}
                  {!query.data!.administrator &&
                    query.data!.site_roles.length === 0 && (
                      <span className="account-unassigned">
                        <UsersRound size={16} aria-hidden="true" />
                        {msg("未分配角色")}{" "}
                      </span>
                    )}
                </div>
              </section>
            </div>
          </Card>
        </div>
      )}
    </section>
  );
}
