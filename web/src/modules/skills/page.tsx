import { msg, getLocale } from "@/lib/i18n";
import { PlanEditor } from "./plan-editor";
import { ConfirmDialog } from "@/components/ui/dialog";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useSearchParams } from "react-router-dom";
import {
  GraduationCap,
  Clock3,
  ListChecks,
  Plus,
  Pencil,
  Trash2,
  Search,
  CheckCircle2,
  CircleHelp,
  RefreshCw,
} from "lucide-react";
import { useSession } from "@/modules/identity";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { EveImage } from "@/components/eve-image";
import * as api from "./api";
import "./skills.css";

const fmt = (n: number | null | undefined) =>
  n == null ? "—" : n.toLocaleString(getLocale());
const date = (v: string | null) =>
  v
    ? new Date(v).toLocaleString(getLocale(), {
        month: "numeric",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";
const stateLabel = {
  met: msg("已达标"),
  missing: msg("未达标"),
  unknown: msg("待确认"),
};
function Levels({ value }: { value: number }) {
  return (
    <span className="skill-levels" aria-label={msg("已训练 {0} 级", value)}>
      {[1, 2, 3, 4, 5].map((i) => (
        <i key={i} className={i <= value ? "filled" : ""} />
      ))}
      <b>{value}</b>
    </span>
  );
}
function Meta({ value }: { value: api.Meta }) {
  const reason =
    value.reason === "missing_scope"
      ? msg("缺少授权")
      : value.reason === "reauthorize"
        ? msg("需要重新授权")
        : value.status === "blocked"
          ? msg("同步受阻")
          : value.status === "pending"
            ? msg("待同步")
            : msg("已同步");
  return (
    <span className="skill-meta">
      {reason}
      {value.observed_at && ` · ${date(value.observed_at)}`}
    </span>
  );
}
export default function SkillsPage() {
  const session = useSession();
  const [params] = useSearchParams();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.isSuccess) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/login" replace />;
  return (
    <Workspace
      key={session.data.session.user_id + (params.get("member") ?? "")}
      csrf={session.data.session.csrf_token}
      member={params.get("member") ?? ""}
      initialPlan={params.get("plan") ?? ""}
      initialCorp={params.get("corporation") ?? ""}
    />
  );
}
function Workspace({
  csrf,
  member,
  initialPlan,
  initialCorp,
}: {
  csrf: string;
  member: string;
  initialPlan: string;
  initialCorp: string;
}) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState("");
  const [tab, setTab] = useState(initialPlan ? "plans" : "overview");
  const [search, setSearch] = useState("");
  const [group, setGroup] = useState("");
  const ctx = useQuery({
    queryKey: ["skills", "context", member],
    queryFn: ({ signal }) => api.context(member, signal),
  });
  const chars = ctx.data?.characters ?? [];
  const character = chars.find((c) => c.id === selected) ?? chars[0];
  const snapshot = useQuery({
    queryKey: ["skills", "snapshot", character?.id],
    queryFn: ({ signal }) => api.snapshot(character!.id, signal),
    enabled: !!character,
    refetchInterval: 60_000,
  });
  const data = snapshot.data;
  const rows = data?.skills ?? [];
  const groups = [...new Set(rows.map((r) => r.group))].sort();
  const filtered = rows.filter(
    (r) =>
      (!group || r.group === group) &&
      (!search ||
        r.name.toLowerCase().includes(search.toLowerCase()) ||
        r.id === search),
  );
  const errors = [ctx.error, snapshot.error].filter(Boolean);
  return (
    <div className="skills-page">
      <header className="skills-header">
        <h1>{msg("技能管理")}</h1>
        <div className="skills-actions">
          <Select
            label={msg("查看角色")}
            value={character?.id ?? ""}
            onValueChange={(v) => {
              setSelected(v);
              setGroup("");
            }}
            options={chars.map((c) => ({
              value: c.id,
              label: c.name,
              leading: (
                <EveImage
                  kind="character"
                  id={c.id}
                  className="skill-avatar-small"
                />
              ),
            }))}
          />
          <IconAction
            label={msg("刷新技能数据")}
            disabled={ctx.isFetching || snapshot.isFetching}
            onClick={() => void qc.invalidateQueries({ queryKey: ["skills"] })}
          >
            <RefreshCw size={18} />
          </IconAction>
        </div>
      </header>
      {member && <p className="muted">{msg("成员数据 · 只读技能记录")}</p>}
      {errors.map((e, i) => (
        <p role="alert" key={i}>
          {e?.message}
        </p>
      ))}
      {ctx.isPending && <p role="status">{msg("正在读取角色")}</p>}
      {ctx.isSuccess && !character && (
        <Card className="skills-card">
          <p>{msg("暂无可用角色")}</p>
        </Card>
      )}
      {character && (
        <>
          <Card className="skill-summary">
            <div className="skill-identity">
              <EveImage
                kind="character"
                id={character.id}
                className="skill-avatar"
              />
              <div>
                <strong>{character.name}</strong>
                {data && <Meta value={data.skills_meta} />}
              </div>
            </div>
            <dl>
              <div>
                <dt>{msg("技能点")}</dt>
                <dd>
                  {fmt(data?.total_sp)}
                  <small> SP</small>
                </dd>
              </div>
              <div>
                <dt>{msg("未分配")}</dt>
                <dd>
                  {fmt(data?.unallocated_sp)}
                  <small> SP</small>
                </dd>
              </div>
              <div>
                <dt>{msg("满级技能")}</dt>
                <dd>
                  {data?.skills_meta.observed_at
                    ? rows.filter((r) => r.trained === 5).length
                    : "—"}
                  <small>
                    {" "}
                    / {data?.skills_meta.observed_at ? rows.length : "—"}
                  </small>
                </dd>
              </div>
            </dl>
          </Card>
          <nav className="skill-tabs" aria-label={msg("技能视图")}>
            {[
              { id: "overview", label: msg("技能总览"), icon: GraduationCap },
              { id: "queue", label: msg("训练队列"), icon: Clock3 },
              { id: "plans", label: msg("军团要求"), icon: ListChecks },
            ].map((t) => (
              <Button
                key={t.id}
                variant={tab === t.id ? "default" : "ghost"}
                aria-pressed={tab === t.id}
                onClick={() => setTab(t.id)}
              >
                <t.icon size={18} />
                {t.label}
              </Button>
            ))}
          </nav>
          {snapshot.isPending && tab !== "plans" && (
            <p role="status">{msg("正在读取技能")}</p>
          )}
          {tab === "overview" && data && (
            <Card className="skills-card">
              <div className="skills-toolbar">
                <label className="skill-search">
                  <Search size={18} aria-hidden="true" />
                  <input
                    aria-label={msg("搜索技能")}
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder={msg("搜索技能")}
                  />
                </label>
                <Select
                  label={msg("技能分类")}
                  value={group}
                  onValueChange={setGroup}
                  options={[
                    { value: "", label: msg("全部分类") },
                    ...groups.map((g) => ({ value: g, label: g })),
                  ]}
                />
                <span className="muted">
                  {filtered.length} {msg("项")}
                </span>
              </div>
              {!data.skills_meta.observed_at ? (
                <p className="skills-empty">
                  {data.skills_meta.status === "blocked"
                    ? msg("请在我的角色页面重新授权")
                    : msg("技能正在等待同步")}
                </p>
              ) : filtered.length === 0 ? (
                <p className="skills-empty">{msg("没有匹配的技能")}</p>
              ) : (
                <div className="skill-list">
                  {filtered.map((k) => (
                    <div className="skill-row" key={k.id}>
                      <span className="skill-mark">
                        <GraduationCap size={19} aria-hidden="true" />
                      </span>
                      <div className="skill-info">
                        <strong>{k.name}</strong>
                        <small>
                          {k.group} · {fmt(k.points)} SP
                          {k.queue_applied ? msg(" · 队列已完成") : ""}
                        </small>
                        {k.active !== null && k.active !== k.trained && (
                          <small>
                            {msg("当前生效")} {k.active} {msg("级")}
                          </small>
                        )}
                      </div>
                      <Levels value={k.trained} />
                    </div>
                  ))}
                </div>
              )}
            </Card>
          )}
          {tab === "queue" && data && <Queue data={data} />}
          {tab === "plans" && (
            <Plans
              key={member}
              csrf={csrf}
              member={member}
              corporations={ctx.data?.corporations ?? []}
              defaultCorp={initialCorp || character.corporation_id}
              initialPlan={initialPlan}
            />
          )}
        </>
      )}
    </div>
  );
}
function Queue({ data }: { data: api.Snapshot }) {
  const [showCompleted, setShowCompleted] = useState(false);
  const list = data.queue.filter(
    (q) => showCompleted || q.state !== "completed",
  );
  const labels: Record<string, string> = {
    training: msg("训练中"),
    scheduled: msg("排队中"),
    paused: msg("已暂停"),
    awaiting_sync: msg("等待完成确认"),
    completed: msg("已完成"),
  };
  return (
    <Card className="skills-card">
      <div className="skills-toolbar">
        <h2>{msg("训练队列")}</h2>
        <Meta value={data.queue_meta} />
        <label className="skill-toggle">
          <input
            type="checkbox"
            checked={showCompleted}
            onChange={(e) => setShowCompleted(e.target.checked)}
          />
          {msg("显示已完成")}{" "}
        </label>
      </div>
      {!data.queue_meta.observed_at ? (
        <p className="skills-empty">
          {data.queue_meta.status === "blocked"
            ? msg("请在我的角色页面补充训练队列授权")
            : msg("训练队列正在等待同步")}
        </p>
      ) : list.length === 0 ? (
        <p className="skills-empty">{msg("暂无待训练技能")}</p>
      ) : (
        <ol className="skill-queue">
          {list.map((q) => {
            const at = Date.parse(data.calculated_at);
            const start = q.start ? Date.parse(q.start) : 0;
            const finish = q.finish ? Date.parse(q.finish) : 0;
            const progress =
              finish > start
                ? Math.max(
                    0,
                    Math.min(100, ((at - start) / (finish - start)) * 100),
                  )
                : 0;
            return (
              <li
                key={q.position}
                className={q.state === "training" ? "current" : ""}
              >
                <span className="queue-number">{q.position + 1}</span>
                <div className="queue-main">
                  <div className="skills-row">
                    <strong>{q.name}</strong>
                    <span>
                      {q.level} {msg("级")}
                    </span>
                  </div>
                  <div className="skills-row muted">
                    <span>{labels[q.state]}</span>
                    <span>
                      {q.finish
                        ? msg("{0} 结束", date(q.finish))
                        : msg("未排定时间")}
                    </span>
                  </div>
                  {q.state === "training" && (
                    <progress
                      aria-label={msg("{0} 本次训练进度估算", q.name)}
                      value={progress}
                      max={100}
                    />
                  )}
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </Card>
  );
}
function Plans({
  csrf,
  member,
  corporations,
  defaultCorp,
  initialPlan,
}: {
  csrf: string;
  member: string;
  corporations: api.Corporation[];
  defaultCorp: string;
  initialPlan: string;
}) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState(defaultCorp);
  const corp = corporations.find((c) => c.id === selected) ?? corporations[0];
  const [planID, setPlanID] = useState(initialPlan);
  const [all, setAll] = useState(false);
  const [editing, setEditing] = useState<api.Plan | "new" | null>(null);
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState<api.Plan | null>(null);
  const [deleteError, setDeleteError] = useState("");
  const plans = useQuery({
    queryKey: ["skills", "plans", corp?.id],
    queryFn: ({ signal }) => api.plans(corp!.id, signal),
    enabled: !!corp,
  });
  const plan = plans.data?.find((p) => p.id === planID) ?? plans.data?.[0];
  const [cursor, setCursor] = useState({ key: "", value: "" });
  const cursorKey = `${plan?.id}:${all}:${member}:${corp?.can_manage}`;
  const after = cursor.key === cursorKey ? cursor.value : "";
  const check = useQuery({
    queryKey: [
      "skills",
      "check",
      plan?.id,
      member,
      all && corp?.can_manage,
      after,
    ],
    queryFn: ({ signal }) =>
      api.check(plan!.id, member, !!(all && corp?.can_manage), after, signal),
    enabled: !!plan,
    refetchInterval: 60_000,
  });
  const catalog = useQuery({
    queryKey: ["skills", "catalog"],
    queryFn: ({ signal }) => api.catalog(signal),
    staleTime: 3600_000,
  });
  const types = catalog.data?.items ?? [];
  const named = new Map(types.map((t) => [t.id, t.name]));
  const remove = async () => {
    if (!deleting || busy) return;
    setBusy(true);
    setDeleteError("");
    try {
      await api.save(deleting.id, { ...deleting, request_key: "" }, csrf, true);
      await qc.invalidateQueries({ queryKey: ["skills", "plans"] });
      setPlanID("");
      setDeleting(null);
    } catch (e) {
      setDeleteError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="skill-plans">
      {deleting && (
        <ConfirmDialog
          title={msg("删除技能要求")}
          description={msg("删除“{0}”？此操作无法撤销。", deleting.name)}
          busy={busy}
          error={deleteError}
          confirmLabel={msg("删除要求")}
          onConfirm={() => void remove()}
          onClose={() => setDeleting(null)}
        />
      )}
      <Card className="skills-card">
        <div className="skills-toolbar">
          <Select
            label={msg("军团")}
            value={corp?.id ?? ""}
            onValueChange={(v) => {
              setSelected(v);
              setPlanID("");
              setAll(false);
            }}
            options={corporations.map((c) => ({ value: c.id, label: c.name }))}
          />
          {corp?.can_manage && (
            <Button onClick={() => setEditing("new")}>
              <Plus size={18} />
              {msg("新建要求")}{" "}
            </Button>
          )}
        </div>
        {[plans.error, check.error, catalog.error]
          .filter(Boolean)
          .map((e, i) => (
            <p role="alert" key={i}>
              {e?.message}
            </p>
          ))}
        {!corp ? (
          <p className="skills-empty">{msg("暂无可查看的军团要求")}</p>
        ) : plans.isPending ? (
          <p role="status">{msg("正在读取方案")}</p>
        ) : !plan ? (
          <p className="skills-empty">{msg("尚未配置技能要求")}</p>
        ) : (
          <>
            <div className="skills-toolbar">
              <Select
                label={msg("技能要求方案")}
                value={plan.id}
                onValueChange={setPlanID}
                options={(plans.data ?? []).map((p) => ({
                  value: p.id,
                  label: p.name,
                }))}
              />
              {corp.can_manage && (
                <>
                  <IconAction
                    label={msg("编辑技能要求")}
                    onClick={() => setEditing(plan)}
                  >
                    <Pencil size={18} />
                  </IconAction>
                  <IconAction
                    label={msg("删除技能要求")}
                    disabled={busy}
                    onClick={() => {
                      setDeleteError("");
                      setDeleting(plan);
                    }}
                  >
                    <Trash2 size={18} />
                  </IconAction>
                </>
              )}
            </div>
            <details className="skill-plan-requirements" key={plan.id}>
              <summary>
                {msg("方案要求")}{" "}
                <span className="skill-meta">
                  {plan.requirements.length} {msg("项技能")}
                </span>
              </summary>
              <div className="skill-requirements">
                {plan.requirements.map((r) => (
                  <div className="skills-row" key={r.skill_id}>
                    <span>
                      {named.get(r.skill_id) ?? msg("技能 #{0}", r.skill_id)}
                    </span>
                    <Levels value={r.level} />
                  </div>
                ))}
              </div>
            </details>
            <div className="skills-toolbar">
              <h2>{msg("达标检查")}</h2>
              <span className="skill-meta">{msg("按已训练等级")}</span>
              {corp.can_manage && !member && (
                <Select
                  label={msg("检查范围")}
                  value={all ? "corp" : "self"}
                  onValueChange={(v) => setAll(v === "corp")}
                  options={[
                    { value: "self", label: msg("我的角色") },
                    { value: "corp", label: msg("军团成员") },
                  ]}
                />
              )}
            </div>
            {(after || check.data?.next_cursor) && (
              <div className="skills-toolbar">
                <Button
                  variant="outline"
                  disabled={!after || check.isFetching}
                  onClick={() => setCursor({ key: cursorKey, value: "" })}
                >
                  {msg("返回首页")}{" "}
                </Button>
                <Button
                  variant="outline"
                  disabled={!check.data?.next_cursor || check.isFetching}
                  onClick={() =>
                    setCursor({
                      key: cursorKey,
                      value: check.data!.next_cursor,
                    })
                  }
                >
                  {msg("下一页")}{" "}
                </Button>
              </div>
            )}
            {check.isPending ? (
              <p role="status">{msg("正在检查")}</p>
            ) : check.data?.items.length === 0 ? (
              <p className="skills-empty">
                {msg("没有符合军团范围的已绑定角色")}
              </p>
            ) : (
              <div className="skill-results">
                {check.data?.items.map((r) => {
                  const heading = (
                    <>
                      <EveImage
                        kind="character"
                        id={r.character.id}
                        className="skill-avatar-result"
                      />
                      <div className="skill-result-identity">
                        <strong>{r.character.name}</strong>
                        {r.state !== "met" && (
                          <span className="skill-sp-gap">
                            {r.remaining_sp === null
                              ? msg("剩余 SP —")
                              : msg("还差 {0} SP", fmt(r.remaining_sp))}
                          </span>
                        )}
                      </div>
                      <span className={`skill-result ${r.state}`}>
                        {r.state === "met" ? (
                          <CheckCircle2 size={16} />
                        ) : (
                          <CircleHelp size={16} />
                        )}{" "}
                        {stateLabel[r.state]}
                      </span>
                      {r.state !== "met" && (
                        <span className="muted">
                          {r.met}/{r.total}
                        </span>
                      )}
                    </>
                  );
                  if (r.state === "met")
                    return (
                      <div className="skill-result-card" key={r.character.id}>
                        <div className="skill-result-heading">{heading}</div>
                      </div>
                    );
                  return (
                    <details className="skill-result-card" key={r.character.id}>
                      <summary className="skill-result-heading">
                        {heading}
                      </summary>
                      <div className="skill-checks">
                        {r.state === "unknown" && (
                          <p className="muted">
                            {msg("缺少可用技能记录，或记录存在冲突。")}{" "}
                          </p>
                        )}
                        {r.checks
                          .filter((c) => c.state !== "met")
                          .map((c) => (
                            <div className="skills-row" key={c.skill_id}>
                              <div className="skill-check-name">
                                <span>{c.name}</span>
                                {c.remaining_sp !== 0 && (
                                  <span className="skill-sp-gap">
                                    {c.remaining_sp === null
                                      ? msg("剩余 SP —")
                                      : msg("还差 {0} SP", fmt(c.remaining_sp))}
                                  </span>
                                )}
                              </div>
                              <span className={`skill-result ${c.state}`}>
                                {c.trained ?? "—"} / {c.required} {msg("级 ·")}{" "}
                                {stateLabel[c.state]}
                              </span>
                            </div>
                          ))}
                      </div>
                    </details>
                  );
                })}
              </div>
            )}
          </>
        )}
      </Card>
      {editing && corp && (
        <PlanEditor
          csrf={csrf}
          corp={corp.id}
          plan={editing === "new" ? null : editing}
          types={types}
          close={() => setEditing(null)}
          saved={(p) => {
            setEditing(null);
            setPlanID(p.id);
            void qc.invalidateQueries({ queryKey: ["skills"] });
          }}
        />
      )}
    </div>
  );
}
