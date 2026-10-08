import { msg, getLocale } from "@/lib/i18n";
import { ConfirmDialog as Confirm } from "@/components/ui/dialog";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { Tabs } from "radix-ui";
import { Link, Navigate } from "react-router-dom";
import {
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  Crown,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Trash2,
  UserRound,
  UsersRound,
  X,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import { APIError } from "@/lib/http";
import { useManagementAccess } from "./use-management-access";
import {
  getRoles,
  getPermissions,
  getMembers,
  getAudit,
  saveRole,
  deleteRole,
  assignRole,
  type Role,
  type Member,
} from "./manage-api";
import { RoleEditor } from "./role-editor";
import "./management.css";

export default function ManagementPage() {
  const { session, access, allowed } = useManagementAccess();
  if (session.isSuccess && !session.data.session)
    return <Navigate to="/" replace />;
  if (session.isError || access.isError)
    return (
      <Feedback
        retry={() => {
          void session.refetch();
          void access.refetch();
        }}
      >
        {msg("权限信息读取失败")}{" "}
      </Feedback>
    );
  if (session.isPending || access.isPending)
    return (
      <p role="status" className="muted">
        {msg("正在读取权限")}{" "}
      </p>
    );
  if (!allowed)
    return (
      <div className="access-page">
        <h1>{msg("权限管理")}</h1>
        <Feedback>{msg("没有权限管理权限")}</Feedback>
      </div>
    );
  return (
    <Management
      user={session.data!.session!.user_id}
      csrf={session.data!.session!.csrf_token}
    />
  );
}
function Feedback({
  children,
  retry,
}: {
  children: ReactNode;
  retry?: () => void;
}) {
  return (
    <div className="access-feedback" role="alert">
      <ShieldCheck size={24} aria-hidden="true" />
      <p>{children}</p>
      {retry ? (
        <Button variant="outline" onClick={retry}>
          {msg("重试")}{" "}
        </Button>
      ) : (
        <Button asChild variant="outline">
          <Link to="/account">{msg("我的角色")}</Link>
        </Button>
      )}
    </div>
  );
}
function Management({ user, csrf }: { user: string; csrf: string }) {
  const client = useQueryClient();
  const roles = useQuery({
    queryKey: ["access", "roles", user],
    queryFn: ({ signal }) => getRoles(signal),
  });
  const permissions = useQuery({
    queryKey: ["access", "catalog", user],
    queryFn: ({ signal }) => getPermissions(signal),
  });
  const [selected, setSelected] = useState<Role | null>(null);
  const [removing, setRemoving] = useState<Role | null>(null);
  const [notice, setNotice] = useState("");
  const [editorKey, setEditorKey] = useState(0);
  const refresh = () => client.invalidateQueries({ queryKey: ["access"] });
  const save = useMutation({
    onMutate: () => setNotice(""),
    mutationFn: (r: Role) => saveRole(r, csrf),
    onSuccess: async (_, r) => {
      await refresh();
      const result = await roles.refetch();
      setSelected(result.data?.find((x) => x.id === r.id) ?? null);
      setEditorKey((k) => k + 1);
      setNotice(msg("角色已保存"));
    },
    onError: () => {
      void client.invalidateQueries({ queryKey: ["access", "me"] });
    },
  });
  const remove = useMutation({
    mutationFn: (r: Role) => deleteRole(r, csrf),
    onSuccess: async () => {
      if (selected?.id === removing?.id) setSelected(null);
      setRemoving(null);
      setNotice(msg("角色已删除"));
      await refresh();
    },
  });
  const busy = save.isPending || remove.isPending;
  return (
    <div className="access-page">
      <div className="page-heading">
        <h1>{msg("权限管理")}</h1>
        <IconAction
          label={msg("刷新权限管理")}
          disabled={busy}
          onClick={() => void refresh()}
        >
          <RefreshCw aria-hidden="true" />
        </IconAction>
      </div>
      <Tabs.Root defaultValue="roles" onValueChange={() => setNotice("")}>
        <Tabs.List className="access-tabs" aria-label={msg("权限管理分区")}>
          <Tabs.Trigger value="roles">
            <ShieldCheck size={18} aria-hidden="true" />
            {msg("角色配置")}{" "}
          </Tabs.Trigger>
          <Tabs.Trigger value="members">
            <UsersRound size={18} aria-hidden="true" />
            {msg("成员授权")}{" "}
          </Tabs.Trigger>
          <Tabs.Trigger value="audit">
            <ClipboardList size={18} aria-hidden="true" />
            {msg("操作记录")}{" "}
          </Tabs.Trigger>
        </Tabs.List>
        {notice && (
          <p className="account-notice" role="status">
            {notice}
          </p>
        )}
        {roles.isError || permissions.isError ? (
          <Feedback retry={() => void refresh()}>
            {roles.error?.message ?? permissions.error?.message}
          </Feedback>
        ) : roles.isPending || permissions.isPending ? (
          <p role="status" className="muted">
            {msg("正在读取配置")}{" "}
          </p>
        ) : (
          <>
            <Tabs.Content value="roles">
              <div className="access-role-layout">
                <div className="access-role-sidebar">
                  <div className="access-section-heading">
                    <span className="muted">
                      {roles.data.length} {msg("个角色")}
                    </span>
                    <Button
                      disabled={busy}
                      onClick={() => {
                        setSelected({
                          id: crypto.randomUUID(),
                          name: "",
                          version: "0",
                          grants: [],
                        });
                        save.reset();
                        setNotice("");
                      }}
                    >
                      <Plus aria-hidden="true" />
                      {msg("新建角色")}{" "}
                    </Button>
                  </div>
                  <div
                    className="access-role-list"
                    role="group"
                    aria-label={msg("平台角色列表")}
                  >
                    {roles.data.map((r) => (
                      <div className="access-role-row" key={r.id}>
                        <button
                          type="button"
                          aria-pressed={selected?.id === r.id}
                          disabled={busy}
                          onClick={() => {
                            setSelected(r);
                            setEditorKey((k) => k + 1);
                            save.reset();
                            setNotice("");
                          }}
                        >
                          <span className="access-role-icon">
                            <ShieldCheck size={20} aria-hidden="true" />
                          </span>
                          <span>
                            <strong>{r.name}</strong>
                            <small>
                              {
                                r.grants.filter((g) =>
                                  permissions.data.some(
                                    (p) => p.id === g.permission,
                                  ),
                                ).length
                              }{" "}
                              {msg("项权限")}{" "}
                            </small>
                          </span>
                        </button>
                        <IconAction
                          label={msg("删除{0}", r.name)}
                          disabled={busy}
                          onClick={() => {
                            setRemoving(r);
                            remove.reset();
                          }}
                        >
                          <Trash2 aria-hidden="true" />
                        </IconAction>
                      </div>
                    ))}
                  </div>
                  {!roles.data.length && (
                    <p className="muted access-empty">{msg("暂无平台角色")}</p>
                  )}
                </div>
                <Card className="access-editor-card">
                  <CardContent>
                    {selected ? (
                      <RoleEditor
                        key={`${selected.id}-${editorKey}`}
                        role={selected}
                        permissions={permissions.data}
                        busy={busy}
                        error={save.error?.message ?? ""}
                        onSave={(r) => save.mutate(r)}
                        onReload={() => {
                          void roles.refetch().then((result) => {
                            const latest = result.data?.find(
                              (r) => r.id === selected.id,
                            );
                            if (latest) {
                              setSelected(latest);
                              setEditorKey((k) => k + 1);
                              save.reset();
                            } else if (!result.error) {
                              setSelected(null);
                              save.reset();
                            }
                          });
                        }}
                      />
                    ) : (
                      <div className="access-empty">
                        <ShieldCheck size={32} aria-hidden="true" />
                        <h2>{msg("选择或新建角色")}</h2>
                      </div>
                    )}
                  </CardContent>
                </Card>
              </div>
            </Tabs.Content>
            <Tabs.Content value="members">
              <Members user={user} csrf={csrf} roles={roles.data} />
            </Tabs.Content>
            <Tabs.Content value="audit">
              <History user={user} />
            </Tabs.Content>
          </>
        )}
      </Tabs.Root>
      {removing && (
        <Confirm
          title={msg("删除“{0}”？", removing.name)}
          description={msg("该角色的全部成员授权将一并撤销。")}
          busy={remove.isPending}
          error={remove.error?.message}
          onClose={() => setRemoving(null)}
          onConfirm={() => remove.mutate(removing)}
        />
      )}
    </div>
  );
}
function Members({
  user,
  csrf,
  roles,
}: {
  user: string;
  csrf: string;
  roles: Role[];
}) {
  const client = useQueryClient();
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [cursors, setCursors] = useState([""]);
  const [selected, setSelected] = useState<Member | null>(null);
  const [roleID, setRoleID] = useState("");
  const [revoke, setRevoke] = useState<Role | null>(null);
  const [notice, setNotice] = useState("");
  const members = useQuery({
    queryKey: ["access", "members", user, query, cursors.at(-1)],
    queryFn: ({ signal }) => getMembers(query, cursors.at(-1)!, signal),
  });
  const current =
    members.data?.items.find((m) => m.user_id === selected?.user_id) ??
    selected;
  const assignment = useMutation({
    mutationFn: ({ role, enabled }: { role: string; enabled: boolean }) =>
      assignRole(current!.user_id, role, enabled, csrf),
    onSuccess: async (_, v) => {
      setRevoke(null);
      setRoleID("");
      setNotice(v.enabled ? msg("角色已授予") : msg("角色已撤销"));
      await client.invalidateQueries({ queryKey: ["access"] });
    },
    onError: (e) => {
      if (e instanceof APIError && e.status === 403)
        void client.invalidateQueries({ queryKey: ["access", "me"] });
    },
  });
  return (
    <div className="access-members">
      <form
        className="access-search"
        onSubmit={(e) => {
          e.preventDefault();
          setQuery(search.trim());
          setCursors([""]);
          setSelected(null);
        }}
      >
        <label className="sr-only" htmlFor="member-search">
          {msg("搜索成员")}{" "}
        </label>
        <Search size={18} aria-hidden="true" />
        <input
          id="member-search"
          placeholder={msg("角色名或角色 ID")}
          value={search}
          maxLength={200}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Button type="submit" disabled={assignment.isPending}>
          {msg("搜索")}{" "}
        </Button>
      </form>
      {members.isError ? (
        <Feedback retry={() => void members.refetch()}>
          {members.error.message}
        </Feedback>
      ) : members.isPending ? (
        <p role="status" className="muted">
          {msg("正在读取成员")}{" "}
        </p>
      ) : (
        <div className="access-member-layout">
          <Card>
            <CardContent className="access-member-list">
              <div className="access-section-heading">
                <h2>
                  <UsersRound size={18} aria-hidden="true" />
                  {msg("成员")}{" "}
                </h2>
                <span className="muted">
                  {msg("第")} {cursors.length} {msg("页")}
                </span>
              </div>
              {members.data.items.map((m) => (
                <button
                  className="access-member-row"
                  type="button"
                  key={m.user_id}
                  aria-pressed={current?.user_id === m.user_id}
                  disabled={assignment.isPending}
                  onClick={() => {
                    setSelected(m);
                    setRoleID("");
                    setNotice("");
                    assignment.reset();
                  }}
                >
                  <EveImage
                    id={m.character_id}
                    kind="character"
                    className="account-tile-portrait"
                  />
                  <span>
                    <strong>{m.name}</strong>
                    <small>
                      {m.character_count} {msg("个角色 ·")} {m.roles.length}{" "}
                      {msg("项平台角色")}{" "}
                    </small>
                  </span>
                  {m.administrator && (
                    <Crown size={16} aria-label={msg("管理员")} />
                  )}
                </button>
              ))}
              {!members.data.items.length && (
                <p className="access-empty muted">{msg("未找到成员")}</p>
              )}
              <div className="access-pagination">
                <IconAction
                  label={msg("上一页成员")}
                  disabled={cursors.length === 1 || assignment.isPending}
                  onClick={() => {
                    setCursors((v) => v.slice(0, -1));
                    setSelected(null);
                  }}
                >
                  <ChevronLeft aria-hidden="true" />
                </IconAction>
                <IconAction
                  label={msg("下一页成员")}
                  disabled={!members.data.next || assignment.isPending}
                  onClick={() => {
                    setCursors((v) => [...v, members.data.next]);
                    setSelected(null);
                  }}
                >
                  <ChevronRight aria-hidden="true" />
                </IconAction>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent>
              {current ? (
                <div className="access-member-detail">
                  <div className="access-member-identity">
                    <EveImage
                      id={current.character_id}
                      kind="character"
                      className="account-portrait"
                    />
                    <div>
                      <h2>{current.name}</h2>
                      <p className="muted">
                        {current.character_count} {msg("个角色共享授权")}{" "}
                      </p>
                    </div>
                  </div>
                  {current.administrator && (
                    <span className="account-role-tag">
                      <Crown size={14} aria-hidden="true" />
                      {msg("本站管理员")}{" "}
                    </span>
                  )}
                  <div className="access-section-heading">
                    <h3>{msg("平台角色")}</h3>
                  </div>
                  <div className="access-assigned">
                    {current.roles.map((r) => (
                      <div key={r.id}>
                        <span>{r.name}</span>
                        <IconAction
                          label={msg("撤销{0}", r.name)}
                          disabled={assignment.isPending}
                          onClick={() => {
                            setRevoke(r);
                            assignment.reset();
                          }}
                        >
                          <X aria-hidden="true" />
                        </IconAction>
                      </div>
                    ))}
                    {!current.roles.length && (
                      <p className="muted">{msg("未分配角色")}</p>
                    )}
                  </div>
                  <form
                    className="access-assign-form"
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (roleID)
                        assignment.mutate({ role: roleID, enabled: true });
                    }}
                  >
                    <label htmlFor="assignment-role">{msg("授予角色")}</label>
                    <select
                      id="assignment-role"
                      value={roleID}
                      disabled={assignment.isPending}
                      onChange={(e) => setRoleID(e.target.value)}
                    >
                      <option value="">{msg("选择平台角色")}</option>
                      {roles
                        .filter(
                          (r) => !current.roles.some((own) => own.id === r.id),
                        )
                        .map((r) => (
                          <option value={r.id} key={r.id}>
                            {r.name}
                          </option>
                        ))}
                    </select>
                    <Button
                      type="submit"
                      disabled={!roleID || assignment.isPending}
                    >
                      <Plus aria-hidden="true" />
                      {msg("授予")}{" "}
                    </Button>
                  </form>
                  {notice && (
                    <p role="status" className="account-notice">
                      {notice}
                    </p>
                  )}
                  {assignment.error && !revoke && (
                    <p role="alert" className="login-error">
                      {assignment.error.message}
                    </p>
                  )}
                </div>
              ) : (
                <div className="access-empty">
                  <UserRound size={32} aria-hidden="true" />
                  <h2>{msg("选择成员")}</h2>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}
      {revoke && current && (
        <Confirm
          title={msg("撤销“{0}”？", revoke.name)}
          description={msg("将移除 {0} 所属账号的此项平台角色。", current.name)}
          busy={assignment.isPending}
          error={assignment.error?.message}
          onClose={() => setRevoke(null)}
          onConfirm={() =>
            assignment.mutate({ role: revoke.id, enabled: false })
          }
        />
      )}
    </div>
  );
}
const actionNames: Record<string, string> = {
  "role.saved": msg("保存角色"),
  "role.deleted": msg("删除角色"),
  "role.assigned": msg("授予角色"),
  "role.unassigned": msg("撤销角色"),
  "administrator.added": msg("设置管理员"),
  "administrator.removed": msg("撤销管理员"),
};
function History({ user }: { user: string }) {
  const [cursors, setCursors] = useState([""]);
  const audit = useQuery({
    queryKey: ["access", "audit", user, cursors.at(-1)],
    queryFn: ({ signal }) => getAudit(cursors.at(-1)!, signal),
  });
  if (audit.isError)
    return (
      <Feedback retry={() => void audit.refetch()}>
        {audit.error.message}
      </Feedback>
    );
  if (audit.isPending)
    return (
      <p role="status" className="muted">
        {msg("正在读取记录")}{" "}
      </p>
    );
  return (
    <Card>
      <CardContent className="access-history">
        <div className="access-section-heading">
          <h2>
            <ClipboardList size={18} aria-hidden="true" />
            {msg("操作记录")}{" "}
          </h2>
          <span className="muted">
            {msg("第")} {cursors.length} {msg("页")}
          </span>
        </div>
        {audit.data.items.map((a) => (
          <details key={a.id} className="access-audit-row">
            <summary>
              <span>{actionNames[a.action] ?? a.action}</span>
              <time dateTime={a.created_at}>
                {new Date(a.created_at).toLocaleString(getLocale(), {
                  hour12: false,
                })}
              </time>
            </summary>
            <dl>
              <dt>{msg("操作者")}</dt>
              <dd>
                {a.actor === "local-operator" ? msg("本机运维") : a.actor}
              </dd>
              <dt>{msg("对象 ID")}</dt>
              <dd>{a.subject}</dd>
              <dt>{msg("记录 ID")}</dt>
              <dd>{a.id}</dd>
            </dl>
          </details>
        ))}
        {!audit.data.items.length && (
          <p className="access-empty muted">{msg("暂无操作记录")}</p>
        )}
        <div className="access-pagination">
          <IconAction
            label={msg("上一页记录")}
            disabled={cursors.length === 1}
            onClick={() => setCursors((v) => v.slice(0, -1))}
          >
            <ChevronLeft aria-hidden="true" />
          </IconAction>
          <IconAction
            label={msg("下一页记录")}
            disabled={!audit.data.next}
            onClick={() => setCursors((v) => [...v, audit.data.next])}
          >
            <ChevronRight aria-hidden="true" />
          </IconAction>
        </div>
      </CardContent>
    </Card>
  );
}
