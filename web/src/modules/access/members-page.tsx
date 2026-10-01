import { msg } from "@/lib/i18n";
import { getModuleCatalog } from "@/app/catalog";
import { Clock3, Orbit } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  ChevronLeft,
  ChevronRight,
  Crown,
  FileText,
  RefreshCw,
  Search,
  ShieldCheck,
  UsersRound,
} from "lucide-react";
import { Card } from "@/components/ui/card";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import { useManagementAccess } from "./use-management-access";
import { getMembers } from "./manage-api";
import { getMemberData } from "./members-api";
import { roleLabel } from "./role-labels";
import {
  getCharacterSync,
  resourceLabel,
  syncLabel,
  syncDate,
  contractDetailLabel,
} from "@/modules/eve/sync-api";
import "./members.css";

function Feedback({
  error,
  text,
  retry,
}: {
  error?: Error | null;
  text?: string;
  retry?: () => void;
}) {
  return (
    <div className="member-feedback" role={error ? "alert" : "status"}>
      <UsersRound aria-hidden="true" size={26} />
      <p>{error?.message ?? text}</p>
      {retry && (
        <IconAction label={msg("重试读取成员")} onClick={retry}>
          <RefreshCw />
        </IconAction>
      )}
    </div>
  );
}
export default function MembersPage() {
  const { session, access } = useManagementAccess();
  if (session.isSuccess && !session.data.session)
    return <Navigate to="/login" replace />;
  if (session.isError || access.isError)
    return (
      <Feedback
        error={session.error ?? access.error}
        retry={() => {
          void session.refetch();
          void access.refetch();
        }}
      />
    );
  if (!access.isSuccess) return <Feedback text={msg("正在读取权限")} />;
  if (!access.data.administrator)
    return <Feedback text={msg("仅站点管理员可查看成员数据")} />;
  return <MembersWorkspace actor={session.data!.session!.user_id} />;
}
function MembersWorkspace({ actor }: { actor: string }) {
  const [params, setParams] = useSearchParams();
  const member = params.get("member");
  const patch = (v: Record<string, string>) => {
    const next = new URLSearchParams(params);
    for (const [k, value] of Object.entries(v)) {
      if (value) next.set(k, value);
      else next.delete(k);
    }
    setParams(next);
  };
  return (
    <div className="members-page">
      <div className="page-heading member-heading">
        <div>
          {member && (
            <IconAction
              label={msg("返回成员列表")}
              onClick={() => patch({ member: "", character: "" })}
            >
              <ArrowLeft />
            </IconAction>
          )}
          <h1>{member ? msg("成员资料") : msg("成员")}</h1>
        </div>
      </div>
      {member ? (
        <MemberDetails
          key={member}
          actor={actor}
          member={member}
          selected={params.get("character")}
          select={(id) => patch({ character: id })}
        />
      ) : (
        <MemberDirectory actor={actor} params={params} patch={patch} />
      )}
    </div>
  );
}
function MemberDirectory({
  actor,
  params,
  patch,
}: {
  actor: string;
  params: URLSearchParams;
  patch: (v: Record<string, string>) => void;
}) {
  const search = params.get("q") ?? "",
    after = params.get("after") ?? "";
  const [draft, setDraft] = useState(search);
  const trail = (params.get("trail") ?? "").split(",").filter(Boolean);
  const query = useQuery({
    queryKey: ["access", "member-directory", actor, search, after],
    queryFn: ({ signal }) => getMembers(search, after, signal),
    refetchInterval: 30_000,
  });
  return (
    <Card className="member-directory py-0">
      <form
        className="member-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          patch({ q: draft.trim(), after: "", trail: "" });
        }}
      >
        <label className="sr-only" htmlFor="member-search">
          {msg("搜索成员")}{" "}
        </label>
        <input
          id="member-search"
          placeholder={msg("角色名称或 ID")}
          maxLength={200}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <IconAction type="submit" label={msg("搜索成员")}>
          <Search />
        </IconAction>
        <IconAction
          label={msg("刷新成员列表")}
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw />
        </IconAction>
      </form>
      {query.isPending ? (
        <Feedback text={msg("正在读取成员")} />
      ) : query.isError ? (
        <Feedback error={query.error} retry={() => void query.refetch()} />
      ) : (
        <>
          <div className="member-list">
            {query.data.items.length === 0 ? (
              <Feedback text={msg("没有匹配的成员")} />
            ) : (
              query.data.items.map((m) => (
                <button
                  className="member-list-row"
                  key={m.user_id}
                  onClick={() => patch({ member: m.user_id, character: "" })}
                >
                  <EveImage
                    kind="character"
                    id={m.character_id}
                    className="member-avatar"
                  />
                  <span className="member-copy">
                    <strong>{m.name}</strong>
                    <span>
                      {m.character_count} {msg("个角色")}{" "}
                      {m.administrator ? msg(" · 管理员") : ""}
                    </span>
                  </span>
                  <ArrowRight size={18} aria-hidden="true" />
                </button>
              ))
            )}
          </div>
          <div className="member-pager">
            <span>
              {msg("第")} {trail.length + 1} {msg("页")}
            </span>
            <IconAction
              label={msg("上一页成员")}
              disabled={!after}
              onClick={() => {
                const prev = trail.slice(0, -1);
                patch({
                  after: trail.at(-1) === "0" ? "" : (trail.at(-1) ?? ""),
                  trail: prev.join(","),
                });
              }}
            >
              <ChevronLeft />
            </IconAction>
            <IconAction
              label={msg("下一页成员")}
              disabled={!query.data.next}
              onClick={() =>
                patch({
                  after: query.data.next,
                  trail: [...trail, after || "0"].join(","),
                })
              }
            >
              <ChevronRight />
            </IconAction>
          </div>
        </>
      )}
    </Card>
  );
}
function MemberDetails({
  actor,
  member,
  selected,
  select,
}: {
  actor: string;
  member: string;
  selected: string | null;
  select: (id: string) => void;
}) {
  const catalog = useQuery({
    queryKey: ["host", "modules"],
    queryFn: ({ signal }) => getModuleCatalog(signal),
  });
  const query = useQuery({
    queryKey: ["access", "member-data", actor, member],
    queryFn: ({ signal }) => getMemberData(member, signal),
    refetchInterval: 30_000,
  });
  const heading = useRef<HTMLHeadingElement>(null);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, []);
  useEffect(() => {
    heading.current?.focus();
  }, [query.isSuccess]);
  if (query.isPending) return <Feedback text={msg("正在读取成员资料")} />;
  if (query.isError)
    return <Feedback error={query.error} retry={() => void query.refetch()} />;
  const data = query.data;
  const ch = selected
    ? data.characters.find((c) => c.id === selected)
    : (data.characters.find((c) => c.is_main) ?? data.characters[0]);
  const fact = data.access.characters.find((f) => f.character_id === ch?.id);
  const fresh =
    !!fact &&
    ["ready", "retry"].includes(fact.state) &&
    Date.parse(fact.valid_until) > now;
  return (
    <>
      <div
        className="member-characters"
        role="group"
        aria-label={msg("成员角色")}
      >
        {data.characters.map((c) => (
          <button
            key={c.id}
            aria-pressed={c.id === ch?.id}
            onClick={() => select(c.id)}
          >
            <EveImage
              kind="character"
              id={c.id}
              className="member-small-avatar"
            />
            <span className="member-character-name">{c.name}</span>
            {c.is_main && <Crown size={15} aria-label={msg("主角色")} />}
            {c.id === ch?.id && <Check size={16} aria-hidden="true" />}
          </button>
        ))}
      </div>
      {!ch ? (
        <Feedback text={msg("该角色已不可用")} />
      ) : (
        <div className="member-detail-grid">
          <Card className="member-profile py-0">
            <div className="member-identity">
              <EveImage
                kind="character"
                id={ch.id}
                className="member-large-avatar"
              />
              <div className="member-copy">
                <h2 ref={heading} tabIndex={-1}>
                  {ch.name}
                </h2>
                <span>
                  #{ch.id}
                  {ch.status === "blocked" ? msg(" · 身份异常") : ""}
                </span>
              </div>
              <IconAction
                label={msg("刷新成员资料")}
                disabled={query.isFetching}
                onClick={() => void query.refetch()}
              >
                <RefreshCw />
              </IconAction>
            </div>
            <div className="member-corporation">
              <EveImage
                kind="corporation"
                id={fact?.corporation.id ?? "0"}
                className="member-avatar"
              />
              <div className="member-copy">
                <strong>
                  {fact?.corporation.name || msg("军团资料未同步")}
                </strong>
                <span>
                  {fact?.corporation.id && fact.corporation.id !== "0"
                    ? `#${fact.corporation.id}`
                    : msg("等待 EVE 授权与同步")}
                </span>
              </div>
              <span className={`member-badge ${fresh ? "is-ready" : ""}`}>
                {fresh
                  ? msg("已同步")
                  : fact?.state === "reauthorize"
                    ? msg("需授权")
                    : msg("待更新")}
              </span>
            </div>
            <div className="member-role-groups">
              {(
                [
                  [msg("职务"), fact?.roles],
                  [msg("总部"), fact?.roles_at_hq],
                  [msg("基地"), fact?.roles_at_base],
                  [msg("其他"), fact?.roles_at_other],
                ] as const
              )
                .filter(
                  ([label, roles]) => label === msg("职务") || !!roles?.length,
                )
                .map(([label, roles]) => (
                  <div key={label}>
                    <span>{label}</span>
                    <div>
                      {roles?.length ? (
                        roles.map((role) => (
                          <span className="member-badge" key={role}>
                            {roleLabel(role)}
                          </span>
                        ))
                      ) : (
                        <span className="member-muted">{msg("未分配")}</span>
                      )}
                    </div>
                  </div>
                ))}
            </div>
            <div className="member-platform-roles">
              <ShieldCheck size={17} aria-hidden="true" />
              <span>{msg("平台角色")}</span>
              {data.access.administrator && (
                <span className="member-badge is-ready">{msg("管理员")}</span>
              )}
              {data.access.site_roles.map((r) => (
                <span className="member-badge" key={r.id}>
                  {r.name}
                </span>
              ))}
              {!data.access.administrator && !data.access.site_roles.length && (
                <span className="member-muted">{msg("未分配")}</span>
              )}
            </div>
            {ch.status === "active" && (
              <div className="member-data-links">
                <Link
                  className="member-link"
                  to={`/contracts?${new URLSearchParams({ member, kind: "character", owner: ch.id })}`}
                >
                  <FileText size={18} aria-hidden="true" />
                  {msg("查看合同")} <ArrowRight size={16} aria-hidden="true" />
                </Link>
                {catalog.data?.some((m) => m.id === "wallet") && (
                  <Link
                    className="member-link"
                    to={`/wallet?${new URLSearchParams({ member, owner: ch.id })}`}
                  >
                    {msg("查看钱包")}{" "}
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                )}
                {catalog.data?.some((m) => m.id === "skills") && (
                  <Link
                    className="member-link"
                    to={`/skills?${new URLSearchParams({ member })}`}
                  >
                    {msg("技能管理")}{" "}
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                )}
                {catalog.data?.some((m) => m.id === "fittings") && (
                  <Link
                    className="member-link"
                    to={`/fittings?${new URLSearchParams({ member })}`}
                  >
                    <Orbit size={18} aria-hidden="true" />
                    {msg("舰船配置")}{" "}
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                )}
                {catalog.data?.some((m) => m.id === "attendance") && (
                  <Link
                    className="member-link"
                    to={`/attendance?${new URLSearchParams({ view: "online", member })}`}
                  >
                    <Clock3 size={18} aria-hidden="true" />
                    {msg("在线时长")}{" "}
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                )}
              </div>
            )}
            {ch.status === "active" && (
              <MemberSync key={ch.id} actor={actor} character={ch.id} />
            )}
          </Card>
          <Card className="member-community py-0">
            <h2>{msg("社区资料")}</h2>
            {data.community ? (
              (
                [
                  ["QQ", data.community.qq],
                  ["KOOK", data.community.kook],
                ] as const
              ).map(([label, b]) => (
                <div key={label}>
                  <span className="member-muted">{label}</span>
                  <strong>{b.value || msg("未填写")}</strong>
                  <span
                    className={`member-badge ${b.confirmation === "confirmed" ? "is-ready" : ""}`}
                  >
                    {
                      {
                        unfilled: msg("未填写"),
                        pending: msg("待确认"),
                        confirmed: msg("已确认"),
                      }[b.confirmation]
                    }
                  </span>
                </div>
              ))
            ) : (
              <p className="member-muted">{msg("社区模块未启用")}</p>
            )}
          </Card>
        </div>
      )}
    </>
  );
}
function MemberSync({
  actor,
  character,
}: {
  actor: string;
  character: string;
}) {
  const query = useQuery({
    queryKey: ["eve", "sync", "member-read", actor, character],
    queryFn: ({ signal }) => getCharacterSync(character, signal),
    refetchInterval: 30_000,
  });
  return (
    <section className="member-sync" aria-label={msg("角色同步状态")}>
      <h3>{msg("ESI 同步")}</h3>
      {query.isPending ? (
        <p className="member-muted" role="status">
          {msg("正在读取同步状态")}{" "}
        </p>
      ) : query.isError ? (
        <Feedback error={query.error} retry={() => void query.refetch()} />
      ) : query.data.targets.length === 0 ? (
        <p className="member-muted">{msg("暂无同步记录")}</p>
      ) : (
        query.data.targets.map((t) => (
          <div key={t.id}>
            <strong>{resourceLabel(t.resource)}</strong>
            <span className="member-badge">{syncLabel(t)}</span>
            <time dateTime={t.last_success_at ?? undefined}>
              {syncDate(t.last_success_at)}
            </time>
            {contractDetailLabel(t) && <small>{contractDetailLabel(t)}</small>}
          </div>
        ))
      )}
    </section>
  );
}
