import { FulfillmentSummary } from "./fulfillment-summary";
import { msg, getLocale } from "@/lib/i18n";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, Link } from "react-router-dom";
import { useApprovalEnabled, approvalLink } from "@/modules/approval/api";
import {
  HeartHandshake,
  ShieldPlus,
  Sprout,
  Rocket,
  Coins,
  Plus,
  Settings2,
  Users,
  ArrowUpRight,
  ChevronRight,
  Gauge,
} from "lucide-react";
import { useSession } from "@/modules/identity";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import { Modal } from "@/components/ui/dialog";
import { IconAction } from "@/components/ui/icon-action";
import { getData } from "@/lib/http";
import { EveImage } from "@/components/eve-image";
import * as api from "./api";
import { LossEvidence } from "./loss-browser";
import { DeliveryContracts, DeliverySummary } from "./delivery-contracts";
import { LossCaseContent, LossCaseEvidence, LossReviewOverview } from "./loss-case-content";
import { ValuationView } from "./valuation";
import { GrowthEditor, RewardSummary } from "./growth-editor";
import "./welfare.css";
import { GrowthProject } from "./growth-project";
import { GrowthStatus } from "./growth-status";
import { useGrowthStatus } from "./growth-query";
import { ActivityPanel } from "./activity-panel";
const groups = [
  {
    id: "loss",
    label: msg("补损"),
    icon: ShieldPlus,
    kinds: api.activeLossKinds,
  },
  {
    id: "growth",
    label: msg("成长福利"),
    icon: Sprout,
    kinds: ["growth_gila", "growth_ishtar", "growth_loki", "growth_absolution"],
  },
  {
    id: "capital",
    label: msg("旗舰补贴"),
    icon: Rocket,
    kinds: ["supercarrier", "titan"],
  },
  { id: "activity", label: msg("活动福利"), icon: HeartHandshake, kinds: ["activity"] },
  { id: "grant", label: msg("果壳币"), icon: Coins, kinds: ["grant"] },
];
const opts = (keys: string[]) =>
  keys.map((value) => ({ value, label: api.kinds[value] }));
const blank: api.Config = {
  enabled: false,
  effective_at: "",
  ship_type_id: "0",
  fitting_id: "0",
  skill_plan_id: "0",
  reference_minor: 0,
  day_zone: "",
  note: "",
};
export default function WelfarePage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/login" replace />;
  return (
    <Workspace user={s.data.session.user_id} csrf={s.data.session.csrf_token} />
  );
}
function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const [selected, setSelected] = useState("");
  const root = useQuery({
    queryKey: ["welfare", "context", user],
    queryFn: ({ signal }) => api.context("", signal),
  });
  const corp = selected || root.data?.corporations[0]?.id || "";
  if (root.isError)
    return (
      <p role="alert">
        {root.error.message}
        <Button onClick={() => void root.refetch()}>{msg("重试")}</Button>
      </p>
    );
  return (
    <div className="welfare-page">
      <header className="welfare-heading">
        <span className="welfare-brand">
          <HeartHandshake aria-hidden="true" />
        </span>
        <h1>{msg("军团福利")}</h1>
        {root.data && (
          <Select
            label={msg("军团")}
            value={corp}
            onValueChange={setSelected}
            options={root.data.corporations.map((c) => ({
              value: c.id,
              label: c.name,
            }))}
          />
        )}
      </header>
      {!root.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : !corp ? (
        <Card>{msg("暂无可用军团")}</Card>
      ) : (
        <Board key={corp} corp={corp} user={user} csrf={csrf} />
      )}
    </div>
  );
}
function Board({
  corp,
  user,
  csrf,
}: {
  corp: string;
  user: string;
  csrf: string;
}) {
  const [group, setGroup] = useState("loss");
  const [selectedKind, setKind] = useState("srp");
  const [selectedAll, setAll] = useState(false);
  const approvalEnabled = useApprovalEnabled();
  const [before, setBefore] = useState("");
  const [modal, setModal] = useState("");
  const [item, setItem] = useState<api.Case | null>(null);
  const client = useQueryClient();
  const c = useQuery({
    queryKey: ["welfare", "context", user, corp],
    queryFn: ({ signal }) => api.context(corp, signal),
  });
  const corporation = c.data?.corporations.find((x) => x.id === corp);
  const manage = !!corporation?.can_manage;
  const compensate = !!corporation?.can_compensate;
  const canManageGroup = group === "loss" ? manage || compensate : manage;
  const all = selectedAll && (!approvalEnabled || group === "grant");
  const growthPolicies =
    c.data?.policies.filter((p) => p.kind.startsWith("growth_")) || [];
  const kind =
    group === "growth"
      ? growthPolicies.some((p) => p.kind === selectedKind)
        ? selectedKind
        : growthPolicies[0]?.kind || ""
      : selectedKind;
  const currentPolicy = c.data?.policies.find((p) => p.kind === kind);
  const [growthCharacter, setGrowthCharacter] = useState("");
  const growthChars =
    c.data?.characters.filter((x) => x.corporation_id === corp) || [];
  const growthChar = growthChars.some((x) => x.id === growthCharacter)
    ? growthCharacter
    : growthChars[0]?.id || "";
  const kindOptions =
    group === "growth"
      ? growthPolicies.map((p) => ({
          value: p.kind,
          label: api.projectLabel(p.kind, p.config),
        }))
      : opts(groups.find((g) => g.id === group)!.kinds);
  const listKind = group === "growth" ? "growth" : group === "activity" ? "activity" : kind;
  const q = useQuery({
    queryKey: ["welfare", "cases", user, corp, all && canManageGroup, listKind, before],
    queryFn: ({ signal }) =>
      api.list(corp, all && canManageGroup, listKind, before, signal),
    // The default personal list is independently authorized by the server.
    // Only the administrator-wide scope needs the context result first.
    enabled: !!listKind && (!selectedAll || !!c.data),
    refetchInterval: 30_000,
  });
  useEffect(() => {
    if (!q.data?.next_cursor) return;
    const next = q.data.next_cursor;
    void client.prefetchQuery({
      queryKey: ["welfare", "cases", user, corp, all && canManageGroup, listKind, next],
      queryFn: ({ signal }) => api.list(corp, all && canManageGroup, listKind, next, signal),
      staleTime: 30_000,
    });
  }, [all, canManageGroup, client, corp, listKind, q.data?.next_cursor, user]);
  const done = () => {
    setModal("");
    setItem(null);
    void client.invalidateQueries({ queryKey: ["welfare"] });
    void client.invalidateQueries({ queryKey: ["approval"] });
    void client.invalidateQueries({ queryKey: ["exchange"] });
  };
  const change = (id: string) => {
    setGroup(id);
    setKind(
      id === "growth"
        ? growthPolicies[0]?.kind || ""
        : groups.find((g) => g.id === id)!.kinds[0],
    );
    setBefore("");
  };
  return (
    <>
      <nav className="welfare-tabs" aria-label={msg("福利项目")}>
        {groups.map((g) => (
          <button
            key={g.id}
            aria-current={group === g.id ? "page" : undefined}
            onClick={() => change(g.id)}
          >
            <g.icon size={20} aria-hidden="true" />
            <span>{g.label}</span>
          </button>
        ))}
      </nav>
      {group !== "activity" && (group !== "growth" || c.data?.administrator) && (
        <div
          className={`welfare-toolbar${group === "growth" ? " welfare-growth-toolbar" : ""}`}
        >
          {group !== "growth" && group !== "activity" && (
            <Select
              label={msg("福利类型")}
              value={kind}
              onValueChange={(v) => {
                setKind(v);
                setBefore("");
              }}
              options={kindOptions}
              disabled={!kindOptions.length}
            />
          )}
          {canManageGroup &&
            group !== "growth" && group !== "activity" &&
            (!approvalEnabled || group === "grant") && (
              <Select
                label={msg("申请范围")}
                value={all ? "all" : "mine"}
                onValueChange={(v) => {
                  setAll(v === "all");
                  setBefore("");
                }}
                options={[
                  { value: "mine", label: msg("我的记录") },
                  { value: "all", label: msg("管理记录") },
                ]}
              />
            )}
          <span className="welfare-spacer" />
          {group === "loss" && c.data?.administrator && (
            <Button variant="outline" onClick={() => setModal("loss-policy")}>
              <Settings2 size={16} />
              {msg("补损设置")}
            </Button>
          )}
          {group === "growth" && c.data?.administrator ? (
            <details className="welfare-growth-management">
              <summary>
                <Settings2 size={16} aria-hidden="true" />
                {msg("项目管理")}
              </summary>
              <div className="welfare-growth-management-actions">
                <Button
                  variant="outline"
                  onClick={() => setModal("new-growth")}
                >
                  <Plus size={16} />
                  {msg("新增项目")}
                </Button>
                <Button variant="outline" onClick={() => setModal("profile")}>
                  <Users size={16} />
                  {msg("历史领取")}
                </Button>
              </div>
            </details>
          ) : (
            <>
              {c.data?.administrator && group === "capital" && (
                <>
                  <IconAction
                    label={msg("旗舰资格")}
                    onClick={() => setModal("profile")}
                  >
                    <Users size={18} />
                  </IconAction>
                  <IconAction
                    label={msg(
                      "{0}规则",
                      groups.find((g) => g.id === group)!.label,
                    )}
                    onClick={() => setModal("configure")}
                    disabled={!kind}
                  >
                    <Settings2 size={18} />
                  </IconAction>
                </>
              )}
            </>
          )}
          {group !== "growth" && group !== "activity" &&
            ((group !== "grant" && group !== "loss") ||
              c.data?.administrator) && (
              <Button
                disabled={!kind}
                onClick={() => {
                  setModal(group === "grant" ? "grant" : "apply");
                }}
              >
                <Plus size={16} />
                {group === "grant"
                  ? msg("发放果壳币")
                  : group === "loss"
                    ? msg("手动录入")
                    : msg("申请")}
              </Button>
            )}
        </div>
      )}
      {c.isError && <p role="alert">{c.error.message}</p>}
      {group === "loss" && c.data && <LossQuotaCard quotas={c.data.loss_quotas || []} />}
      {group === "growth" && (
        <div className="welfare-growth-projects">
          {growthPolicies.map((policy) => (
            <GrowthProject
              key={policy.kind}
              corp={corp}
              policy={policy}
              characters={growthChars}
              admin={!!c.data?.administrator}
              apply={(character) => {
                setKind(policy.kind);
                setGrowthCharacter(character);
                setModal("apply");
              }}
              edit={() => {
                setKind(policy.kind);
                setModal("configure");
              }}
            />
          ))}
          {c.data && !growthPolicies.length && <p>{msg("暂无成长项目")}</p>}
        </div>
      )}
      {group === "activity" && c.data && <ActivityPanel corp={corp} csrf={csrf} policies={c.data.policies} characters={growthChars} administrator={c.data.administrator} done={done} />}
      {group === "growth" && (
        <div className="welfare-growth-records-heading">
          <h2>{all && canManageGroup ? msg("管理记录") : msg("我的记录")}</h2>
          {canManageGroup && !approvalEnabled && (
            <Select
              label={msg("申请范围")}
              value={all ? "all" : "mine"}
              onValueChange={(v) => {
                setAll(v === "all");
                setBefore("");
              }}
              options={[
                { value: "mine", label: msg("我的记录") },
                { value: "all", label: msg("管理记录") },
              ]}
            />
          )}
        </div>
      )}
      {group === "activity" && <div className="welfare-growth-records-heading"><h2>{msg("我的记录")}</h2></div>}
      <Card
        className={
          "welfare-list" + (group === "growth" || group === "activity" ? " welfare-growth-records" : "") + (group === "activity" ? " welfare-activity-records" : "")
        }
      >
        {q.isError ? (
          <p role="alert">
            {q.error.message}
            <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
          </p>
        ) : !q.data ? (
          <p role="status">{msg("正在读取")}</p>
        ) : !q.data.items.length ? (
          <div className="welfare-empty">
            <HeartHandshake size={32} aria-hidden="true" />
            <p>
              {msg(
                "暂无{0}记录",
                group === "growth"
                  ? msg("成长福利")
                  : api.projectLabel(kind, currentPolicy?.config),
              )}
            </p>
          </div>
        ) : (
          <>
            <div className="welfare-row welfare-columns" aria-hidden="true">
              <span>{msg("项目 / 角色")}</span>
              <span>{msg("状态")}</span>
              {group !== "growth" && group !== "activity" && <span>{msg("核准金额")}</span>}
              <span>{msg("提交时间")}</span>
              <span />
            </div>
            {q.data.items.map((v) => (
              <button
                className="welfare-row"
                key={v.id}
                onClick={() => setItem(v)}
              >
                <span className="welfare-object">
                  {v.kind === "grant" ? (
                    <Coins size={26} aria-hidden="true" />
                  ) : v.kind.startsWith("activity_") ? (
                    <HeartHandshake size={26} aria-hidden="true" />
                  ) : (
                    <EveImage
                      kind="type"
                      id={v.detail.ship_type_id}
                      className="welfare-ship-image"
                    />
                  )}
                  <span>
                    <strong>{api.caseLabel(v)}</strong>
                    <small>
                      {v.detail.character_name || msg("果壳币奖励")} · #{v.id}
                    </small>
                  </span>
                </span>
                <span className={`welfare-status state-${v.state}`}>
                  {api.states[v.state]}
                </span>
                {group !== "growth" && group !== "activity" && (
                  <span className="welfare-amount">
                    {v.award_minor ? api.money(v.award_minor) : "—"}
                    {v.award_minor > 0 && (
                      <small>
                        {v.kind === "grant" ? msg("果壳币") : "ISK"}
                      </small>
                    )}
                  </span>
                )}
                <time>{api.date(v.created_at)}</time>
                <ChevronRight size={16} aria-hidden="true" />
              </button>
            ))}
          </>
        )}
      </Card>
      {(before || q.data?.next_cursor) && (
        <div className="welfare-paging">
          <Button
            variant="outline"
            disabled={!before}
            onClick={() => setBefore("")}
          >
            {msg("最新记录")}{" "}
          </Button>
          <Button
            variant="outline"
            disabled={!q.data?.next_cursor}
            onClick={() => setBefore(q.data?.next_cursor || "")}
          >
            {msg("下一页")}{" "}
          </Button>
        </div>
      )}
      {c.data &&
        modal === "apply" &&
        (group !== "loss" || c.data.administrator) && (
          <Apply
            initialGrowthCharacter={growthChar}
            corp={corp}
            csrf={csrf}
            kind={kind}
            context={c.data}
            close={() => setModal("")}
            done={done}
          />
        )}
      {c.data?.administrator &&
        modal === "configure" &&
        (group === "capital" ||
          (group === "growth" && !kind.startsWith("growth_fitting_"))) && (
          <Configure
            corp={corp}
            csrf={csrf}
            kind={kind}
            title={msg("{0}规则", groups.find((g) => g.id === group)!.label)}
            allowedKinds={groups.find((g) => g.id === group)!.kinds}
            context={c.data}
            close={() => setModal("")}
            done={done}
          />
        )}
      {c.data?.administrator &&
        group === "growth" &&
        (modal === "new-growth" ||
          (modal === "configure" && kind.startsWith("growth_fitting_"))) && (
          <GrowthEditor
            corp={corp}
            csrf={csrf}
            policy={modal === "configure" ? currentPolicy : undefined}
            policies={growthPolicies}
            close={() => setModal("")}
            done={(k) => {
              setKind(k);
              done();
            }}
          />
        )}
      {c.data?.administrator &&
        modal === "profile" &&
        (group === "growth" || group === "capital") && (
          <Profiles
            policies={growthPolicies}
            scope={group}
            corp={corp}
            csrf={csrf}
            close={() => setModal("")}
            done={done}
          />
        )}
      {modal === "grant" && (
        <Grant corp={corp} csrf={csrf} close={() => setModal("")} done={done} />
      )}
      {modal === "loss-policy" && c.data?.administrator && (
        <LossPolicyEditor
          corp={corp}
          csrf={csrf}
          kind={kind}
          policy={currentPolicy}
          close={() => setModal("")}
          done={done}
        />
      )}
      {item && (
        <CaseView
          user={user}
          csrf={csrf}
          item={item}
          policy={c.data?.policies.find((p) => p.kind === item.kind)}
          manage={((item.kind === "srp" || item.kind === "solo") ? canManageGroup : manage) && (!approvalEnabled || item.kind === "grant")}
          admin={!!c.data?.administrator}
          approvalEntry={approvalEnabled && (item.kind === "srp" || item.kind === "solo" ? canManageGroup : manage) && item.kind !== "grant"}
          close={() => setItem(null)}
          done={done}
        />
      )}
    </>
  );
}
function LossQuotaCard({ quotas }: { quotas: api.LossQuota[] }) {
  const periods = [
    ["daily", msg("今日")],
    ["weekly", msg("本周")],
    ["monthly", msg("本月")],
  ] as const;
  return (
    <section className="welfare-quota-card" aria-labelledby="welfare-quota-title">
      <div className="welfare-quota-heading">
        <span className="welfare-quota-icon"><Gauge size={18} aria-hidden="true" /></span>
        <div>
          <h2 id="welfare-quota-title">{msg("补损额度")}</h2>
        </div>
      </div>
      <div className="welfare-quota-grid">
        {quotas.map((quota) => (
          <div className="welfare-quota-kind" key={quota.kind}>
            <div className="welfare-quota-kind-heading">
              <strong>{api.kinds[quota.kind]}</strong>
              <div className="welfare-quota-legend" aria-hidden="true">
                <span><i className="is-used" />{msg("已用")}</span>
                <span><i className="is-reserved" />{msg("预占")}</span>
              </div>
            </div>
            <div className="welfare-quota-periods">
              {periods.map(([key, label]) => {
                const period = quota[key];
                const limited = period.cap_minor !== undefined;
                const capacity = period.cap_minor || 0;
                const usedRatio = limited && capacity
                  ? Math.min(100, (period.used_minor / capacity) * 100)
                  : 0;
                const reservedRatio = limited && capacity
                  ? Math.min(100 - usedRatio, (period.reserved_minor / capacity) * 100)
                  : 0;
                return (
                  <div className="welfare-quota-period" key={key}>
                    <div className="welfare-quota-line">
                      <span>{label}</span>
                      <strong>
                        {api.money(period.used_minor)} / {limited ? api.money(period.cap_minor || 0) : msg("不限")}
                      </strong>
                    </div>
                    <div
                      className={`welfare-quota-bar${limited ? "" : " is-unlimited"}`}
                      role="img"
                      aria-label={`${label}：${api.money(period.used_minor)}${period.reserved_minor > 0 ? `，${msg("待审批预占 {0} ISK", api.money(period.reserved_minor))}` : ""}`}
                    >
                      {limited && <span className="is-used" style={{ width: `${usedRatio}%` }} />}
                      {limited && <span className="is-reserved" style={{ width: `${reservedRatio}%` }} />}
                    </div>
                    <div className="welfare-quota-metrics" aria-label={`${label} ${msg("额度明细")}`}>
                      {period.reserved_minor > 0 && (
                        <span
                          className="is-reserved"
                          title={msg("待审批预占")}
                          aria-label={msg("待审批预占 {0} ISK", api.money(period.reserved_minor))}
                        >
                          {api.money(period.reserved_minor)} ISK
                        </span>
                      )}
                      {limited && (
                        <span
                          className="is-available"
                          title={msg("可用")}
                          aria-label={msg("可用 {0} ISK", api.money(period.remaining_minor || 0))}
                        >
                          {api.money(period.remaining_minor || 0)} ISK
                        </span>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
type EditorProps = {
  size?: "compact" | "form";
  title: string;
  csrf: string;
  close: () => void;
  done: () => void;
  children: ReactNode;
  command: () => object;
  label?: string;
  disabled?: boolean;
};
function Editor({
  size,
  title,
  csrf,
  close,
  done,
  children,
  command,
  label = msg("保存"),
  disabled,
}: EditorProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [key, setKey] = useState(() => crypto.randomUUID());
  return (
    <Modal
      title={title}
      size={size}
      close={close}
      busy={busy}
      footer={
        <>
          <Button variant="outline" disabled={busy} onClick={close}>
            {msg("取消")}{" "}
          </Button>
          <Button form="welfare-form" type="submit" disabled={busy || disabled}>
            {busy ? msg("正在保存") : label}
          </Button>
        </>
      }
    >
      <form
        id="welfare-form"
        className="welfare-form"
        onChange={() => {
          setKey(crypto.randomUUID());
          setError("");
        }}
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            await api.post("commands", csrf, {
              ...command(),
              request_key: key,
            });
            done();
          } catch (e) {
            setError(e instanceof Error ? e.message : msg("保存失败"));
          } finally {
            setBusy(false);
          }
        }}
      >
        <fieldset disabled={busy}>{children}</fieldset>
        {error && <p role="alert">{error}</p>}
      </form>
    </Modal>
  );
}
function Field({
  label,
  value,
  onChange,
  type = "text",
  required = false,
  wide = false,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  required?: boolean;
  wide?: boolean;
}) {
  return (
    <label className={wide ? "welfare-field wide" : "welfare-field"}>
      <span>{label}</span>
      {type === "textarea" ? (
        <textarea
          required={required}
          maxLength={3000}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <input
          type={type}
          required={required}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          step={type === "number" ? "0.01" : undefined}
        />
      )}
    </label>
  );
}
function ShipPicker({
  corp,
  value,
  setValue,
}: {
  corp: string;
  value: string;
  setValue: (v: string) => void;
}) {
  const [text, setText] = useState("");
  const [search, setSearch] = useState("");
  const q = useQuery({
    queryKey: ["welfare", "ships", corp, search],
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/welfare/ships?${new URLSearchParams({ corporation_id: corp, q: search })}`,
        (v): v is { items: { id: string; name: string }[] } =>
          !!v &&
          typeof v === "object" &&
          "items" in v &&
          Array.isArray(v.items) &&
          v.items.every(
            (x) => x && typeof x.id === "string" && typeof x.name === "string",
          ),
        signal,
      ),
    enabled: search.length >= 2,
  });
  return (
    <div className="welfare-field wide">
      <div className="welfare-ship-search">
        <Field label={msg("舰船名称")} value={text} onChange={setText} />
        <Button
          type="button"
          variant="outline"
          disabled={text.trim().length < 2}
          onClick={() => {
            if (search !== text.trim()) setValue("");
            setSearch(text.trim());
          }}
        >
          {msg("搜索")}{" "}
        </Button>
      </div>
      <Select
        label={msg("损失或购入舰船")}
        value={value}
        onValueChange={setValue}
        options={(q.data?.items || []).map((v) => ({
          value: v.id,
          label: v.name,
          leading: <EveImage kind="type" id={v.id} />,
        }))}
      />
      {q.isError && <p role="alert">{q.error.message}</p>}
      {q.data && !q.data.items.length && <small>{msg("没有找到舰船")}</small>}
    </div>
  );
}
export function Apply({
  initialGrowthCharacter,
  initialLoss,
  corp,
  csrf,
  kind,
  context,
  close,
  done,
}: {
  initialGrowthCharacter?: string;
  initialLoss?: api.Loss | null;
  corp: string;
  csrf: string;
  kind: string;
  context: api.Context;
  close: () => void;
  done: () => void;
}) {
  const chars = context.characters.filter(
    (c) =>
      c.corporation_id === corp &&
      (!initialLoss || c.id === initialLoss.character_id),
  );
  const [linkedLoss, setLinkedLoss] = useState(initialLoss);
  const [character, setCharacter] = useState(
    initialLoss?.character_id || initialGrowthCharacter || chars[0]?.id || "",
  );
  const [ship, setShip] = useState(initialLoss?.ship_type_id || "");
  const [openedAt] = useState(() => Date.now());
  const [km, setKM] = useState(initialLoss?.id || "");
  const [contract, setContract] = useState("");
  const [event, setEvent] = useState(
    initialLoss?.reimbursement?.attendance_event_id || "",
  );
  const [at, setAt] = useState("");
  const [description, setDescription] = useState("");
  const [evidence, setEvidence] = useState(
    initialLoss ? msg("ESI 击毁报告 #{0}", initialLoss.id) : "",
  );
  const [alliance, setAlliance] = useState("unknown");
  const policy = context.policies.find((p) => p.kind === kind);
  const loss = ["srp", "alliance", "solo"].includes(kind);
  const growth = kind.startsWith("growth_");
  const status = useGrowthStatus(corp, character, kind, policy?.version);
  const manual = loss && !initialLoss;
  const ready =
    loss ||
    (policy?.config.enabled &&
      (["supercarrier", "titan"].includes(kind) ||
        Date.parse(policy.config.effective_at) <= openedAt));
  if (["supercarrier", "titan"].includes(kind))
    return (
      <Editor
        title={msg("申请{0}", api.kinds[kind])}
        csrf={csrf}
        close={close}
        done={done}
        disabled={!ready || !/^[1-9]\d*$/.test(contract)}
        label={msg("提交申请")}
        command={() => ({
          action: "apply",
          corporation_id: corp,
          kind,
          detail: { contract_id: contract },
        })}
      >
        {!ready && (
          <p className="welfare-warning wide">
            {msg("该项目尚未开放，请联系管理员。")}
          </p>
        )}
        <Field
          label={msg("购舰合同 ID")}
          value={contract}
          onChange={setContract}
          required
          wide
        />
        <p className="wide">
          {msg(
            "按购舰合同价格的 {0}% 补贴 ISK，通过合同发放。",
            api.capitalRate(kind, policy?.config),
          )}
        </p>
      </Editor>
    );
  return (
    <Editor
      title={
        manual
          ? msg("手动录入 · {0}", api.kinds[kind])
          : msg("申请{0}", api.projectLabel(kind, policy?.config))
      }
      csrf={csrf}
      close={close}
      done={done}
      disabled={
        !ready ||
        !character ||
        (!growth && !ship) ||
        (growth && (status.isError || status.data?.state !== "met")) ||
        (manual && !context.administrator)
      }
      label={manual ? msg("提交录入") : msg("提交申请")}
      command={() => ({
        action: "apply",
        corporation_id: corp,
        kind,
        detail: growth
          ? { character_id: character }
          : {
              synced_loss: !!linkedLoss,
              character_id: character,
              ship_type_id: ship || "0",
              killmail_id: km || "0",
              contract_id: contract || "0",
              event_id: event || "0",
              occurred_at:
                linkedLoss?.occurred_at ||
                (at ? new Date(at).toISOString() : ""),
              description,
              evidence,
              alliance,
            },
      })}
    >
      {!ready && (
        <p className="welfare-warning">
          {msg("该项目规则尚未生效，请联系管理员。")}
        </p>
      )}
      {growth && (
        <div className="wide">
          <RewardSummary rewards={policy?.config.rewards} />
          <GrowthStatus data={status.data} failed={status.isError} />
        </div>
      )}
      <label className="welfare-field wide">
        {msg("领取角色")}{" "}
        {growth && chars.length === 1 ? (
          <span>{chars[0].name}</span>
        ) : (
          <Select
            label={msg("领取角色")}
            value={character}
            disabled={!!initialLoss}
            onValueChange={(v) => {
              setCharacter(v);
              setLinkedLoss(null);
              setShip("");
              setKM("");
              setAt("");
              setEvidence("");
            }}
            options={chars.map((c) => ({ value: c.id, label: c.name }))}
          />
        )}
      </label>
      {linkedLoss && (
        <div className="wide">
          <LossEvidence loss={linkedLoss} />
        </div>
      )}
      {!growth && !linkedLoss && (
        <ShipPicker corp={corp} value={ship} setValue={setShip} />
      )}
      {loss && !linkedLoss && (
        <>
          <Field
            label={msg("击毁报告 ID")}
            value={km}
            onChange={setKM}
            required
          />
          <Field
            label={msg("损失时间（本地）")}
            value={at}
            onChange={setAt}
            type="datetime-local"
            required
          />
          {kind !== "srp" && (
            <Field
              label={msg("活动 ID（选填）")}
              value={event}
              onChange={setEvent}
            />
          )}
        </>
      )}
      {loss && linkedLoss && kind !== "srp" && (
        <Field
          label={msg("活动 ID（选填）")}
          value={event}
          onChange={setEvent}
        />
      )}
      {kind === "srp" && linkedLoss && (
        <small className="wide">
          {msg("关联出勤 #")}
          {event || "—"}
        </small>
      )}
      {!growth && (
        <Field
          label={loss ? msg("购舰合同 ID（选填）") : msg("购舰合同 ID")}
          value={contract}
          onChange={setContract}
          required={!loss}
        />
      )}
      {kind === "solo" && (
        <small className="wide">
          {msg("未填购舰合同，按船体及全部物品的市场中间价核价。")}{" "}
        </small>
      )}
      {kind === "alliance" && (
        <label className="welfare-field">
          {msg("联盟处理")}{" "}
          <Select
            label={msg("联盟处理")}
            value={alliance}
            onValueChange={setAlliance}
            options={[
              { value: "unknown", label: msg("待核实") },
              { value: "pending", label: msg("处理中") },
              { value: "not_paid", label: msg("确认不补") },
              { value: "paid", label: msg("已补偿") },
            ]}
          />
        </label>
      )}
      {!growth && (
        <>
          <Field
            label={msg("情况说明")}
            value={description}
            onChange={setDescription}
            type="textarea"
            required
            wide
          />
          <Field
            label={msg("证明材料或链接")}
            value={evidence}
            onChange={setEvidence}
            type="textarea"
            required
            wide
          />
        </>
      )}
    </Editor>
  );
}
export function Configure({
  corp,
  csrf,
  kind: initial,
  allowedKinds,
  title,
  context,
  close,
  done,
}: {
  corp: string;
  csrf: string;
  kind: string;
  allowedKinds: string[];
  title: string;
  context: api.Context;
  close: () => void;
  done: () => void;
}) {
  const [kind, setKind] = useState(initial);
  const capital = ["supercarrier", "titan"].includes(kind);
  const [more, setMore] = useState(false);
  const current = context.policies.find((p) => p.kind === kind);
  const [config, setConfig] = useState<api.Config>(current?.config || blank);
  const [rate, setRate] = useState(
    String(api.capitalRate(kind, current?.config)),
  );
  const [price, setPrice] = useState(
    String((current?.config.reference_minor || 0) / 100),
  );
  const change = (v: string) => {
    setKind(v);
    const c = context.policies.find((p) => p.kind === v)?.config || blank;
    setConfig(c);
    setRate(String(api.capitalRate(v, c)));
    setPrice(String(c.reference_minor / 100));
  };
  const set = <K extends keyof api.Config>(k: K, v: api.Config[K]) =>
    setConfig((c) => ({ ...c, [k]: v }));
  type Fit = { id: string; name: string; fit: { ship_type_id: string } };
  type Plan = { id: string; name: string };
  const fits = useQuery({
    queryKey: ["welfare", "fits", corp],
    enabled: !capital || more,
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/fittings/library?corporation_id=${corp}`,
        (v): v is Fit[] =>
          Array.isArray(v) &&
          v.every(
            (x) =>
              x &&
              typeof x.id === "string" &&
              typeof x.name === "string" &&
              x.fit &&
              typeof x.fit.ship_type_id === "string",
          ),
        signal,
      ),
  });
  const plans = useQuery({
    queryKey: ["welfare", "plans", corp],
    enabled: !capital || more,
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/skills/plans?corporation_id=${corp}`,
        (v): v is Plan[] =>
          Array.isArray(v) &&
          v.every(
            (x) => x && typeof x.id === "string" && typeof x.name === "string",
          ),
        signal,
      ),
  });
  const linkedSettings = (
    <>
      <label className="welfare-field wide">
        {msg("军团配装")}{" "}
        <Select
          label={msg("军团配装")}
          value={config.fitting_id}
          onValueChange={(v) => {
            set("fitting_id", v);
            set(
              "ship_type_id",
              fits.data?.find((f) => f.id === v)?.fit.ship_type_id || "0",
            );
          }}
          options={[
            { value: "0", label: msg("未关联") },
            ...(fits.data || []).map((f) => ({ value: f.id, label: f.name })),
          ]}
        />
      </label>
      <label className="welfare-field wide">
        {msg("技能要求")}{" "}
        <Select
          label={msg("技能要求")}
          value={config.skill_plan_id}
          onValueChange={(v) => set("skill_plan_id", v)}
          options={[
            { value: "0", label: msg("未关联") },
            ...(plans.data || []).map((p) => ({ value: p.id, label: p.name })),
          ]}
        />
      </label>
      {(fits.isError || plans.isError) && (
        <p role="alert">{msg("配装或技能方案读取失败，请先核对对应模块。")}</p>
      )}
      <Field
        label={capital ? msg("备注（选填）") : msg("适用条件 / 过渡口径")}
        value={config.note}
        onChange={(v) => set("note", v)}
        type="textarea"
        wide
      />
    </>
  );
  return (
    <Editor
      title={title}
      csrf={csrf}
      close={close}
      done={done}
      command={() => ({
        action: "configure",
        corporation_id: corp,
        kind,
        version: current?.version || "0",
        config: {
          ...config,
          effective_at: ["supercarrier", "titan"].includes(kind)
            ? ""
            : config.effective_at,
          reference_minor: capital ? config.reference_minor : api.minor(price),
          subsidy_rate_bps: capital
            ? api.subsidyRate(rate)
            : config.subsidy_rate_bps,
        },
      })}
    >
      <label className="welfare-field wide">
        {msg("项目")}{" "}
        <Select
          label={msg("规则项目")}
          value={kind}
          onValueChange={change}
          options={opts(allowedKinds)}
        />
      </label>
      <label className="welfare-check">
        <input
          type="checkbox"
          checked={config.enabled}
          onChange={(e) => set("enabled", e.target.checked)}
        />
        {msg("开放申请")}{" "}
      </label>
      {capital && (
        <label className="welfare-field">
          <span>{msg("补贴比例（%）")}</span>
          <input
            type="number"
            min="0.01"
            max="100"
            step="0.01"
            required
            value={rate}
            onChange={(e) => setRate(e.target.value)}
          />
        </label>
      )}
      {!capital && (
        <Field
          label={msg("生效时间（含时区）")}
          value={config.effective_at}
          onChange={(v) => set("effective_at", v)}
          required={config.enabled}
          wide
        />
      )}
      {capital ? (
        <details
          className="wide welfare-config-more"
          open={more}
          onToggle={(e) => setMore(e.currentTarget.open)}
        >
          <summary>
            {config.fitting_id !== "0" ||
            config.skill_plan_id !== "0" ||
            config.note
              ? msg("更多设置 · 已配置")
              : msg("更多设置")}
          </summary>
          <div className="welfare-config-fields">{linkedSettings}</div>
        </details>
      ) : (
        linkedSettings
      )}
      {!capital && (
        <Field
          label={msg("正常合同参考价 / ISK")}
          value={price}
          onChange={setPrice}
          required
        />
      )}
      {kind === "solo" && (
        <label className="welfare-field">
          {msg("日限时区")}{" "}
          <Select
            label={msg("日限时区")}
            value={config.day_zone}
            onValueChange={(v) => set("day_zone", v)}
            options={[
              { value: "", label: msg("未确定") },
              { value: "UTC", label: "UTC" },
              { value: "Asia/Shanghai", label: msg("北京时间") },
            ]}
          />
        </label>
      )}
    </Editor>
  );
}
function useMembers(corp: string) {
  return useQuery({
    queryKey: ["welfare", "members", corp],
    queryFn: ({ signal }) => api.members(corp, signal),
  });
}
function uniqueMembers(rows: api.Character[] = []) {
  return [
    ...new Map(
      rows.map((c) => [
        c.account_id,
        { value: c.account_id, label: c.main_character_name || c.name },
      ]),
    ).values(),
  ];
}
const profileTitles = {
  loss: msg("补损资格"),
  growth: msg("历史领取"),
  capital: msg("旗舰资格"),
};
export function Profiles({
  policies = [],
  scope,
  corp,
  csrf,
  close,
  done,
}: {
  scope: "loss" | "growth" | "capital";
  policies?: api.Policy[];
  corp: string;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const m = useMembers(corp);
  const [selected, setSelected] = useState("");
  const member = selected || m.data?.items[0]?.account_id || "";
  const q = useQuery({
    queryKey: ["welfare", "profile", corp, member],
    queryFn: ({ signal }) => api.profile(corp, member, signal),
    enabled: !!member,
  });
  return (
    <>
      {q.data ? (
        <ProfileEdit
          policies={policies}
          scope={scope}
          key={member + q.data.version}
          data={q.data}
          corp={corp}
          csrf={csrf}
          close={close}
          done={done}
          selector={
            <Select
              label={msg("成员")}
              value={member}
              onValueChange={setSelected}
              options={uniqueMembers(m.data?.items)}
            />
          }
        />
      ) : (
        <Modal title={profileTitles[scope]} close={close}>
          <p role={q.isError || m.isError ? "alert" : "status"}>
            {q.error?.message ||
              m.error?.message ||
              (m.data && !member ? msg("暂无可用成员") : msg("正在读取"))}
          </p>
        </Modal>
      )}
    </>
  );
}
function ProfileEdit({
  policies,
  scope,
  data,
  corp,
  csrf,
  close,
  done,
  selector,
}: {
  scope: "loss" | "growth" | "capital";
  policies: api.Policy[];
  data: api.Profile;
  corp: string;
  csrf: string;
  close: () => void;
  done: () => void;
  selector: ReactNode;
}) {
  const [verified, setVerified] = useState(data.verified);
  const [history, setHistory] = useState(data.history);
  const [note, setNote] = useState("");
  return (
    <Editor
      title={profileTitles[scope]}
      csrf={csrf}
      close={close}
      done={done}
      command={() => ({
        action: "profile",
        corporation_id: corp,
        account_id: data.account_id,
        note:
          scope === "capital"
            ? msg("更新旗舰领取资料")
            : scope === "growth"
              ? msg("更新成长福利领取资料")
              : note,
        profile: {
          ...data,
          verified,
          history,
          months: data.months,
        },
      })}
    >
      <div className="wide">{selector}</div>
      {scope === "loss" && (
        <label className="welfare-check wide">
          <input
            type="checkbox"
            checked={verified}
            onChange={(e) => setVerified(e.target.checked)}
          />
          {msg("成员身份已核实")}{" "}
        </label>
      )}
      {(scope === "growth"
        ? [
            ...new Set([
              ...policies.map((p) => p.kind),
              ...Object.keys(data.history).filter((k) =>
                k.startsWith("growth_"),
              ),
            ]),
          ]
        : scope === "capital"
          ? ["supercarrier", "titan"]
          : []
      ).map((k) => (
        <label className="welfare-field" key={k}>
          {api.projectLabel(k, policies.find((p) => p.kind === k)?.config)}
          <Select
            label={msg(
              "{0}历史资格",
              api.projectLabel(k, policies.find((p) => p.kind === k)?.config),
            )}
            value={
              scope !== "loss" && history[k] !== "used"
                ? "unknown"
                : history[k] || "unknown"
            }
            onValueChange={(v) => setHistory((h) => ({ ...h, [k]: v }))}
            options={[
              {
                value: "unknown",
                label: scope !== "loss" ? msg("未登记") : msg("待核实"),
              },
              ...(scope !== "loss"
                ? []
                : [{ value: "unused", label: msg("历史未领取") }]),
              { value: "used", label: msg("历史已领取") },
            ]}
          />
        </label>
      ))}
      {scope === "loss" && (
        <Field
          label={msg("核验依据")}
          value={note}
          onChange={setNote}
          type="textarea"
          required
          wide
        />
      )}
    </Editor>
  );
}
function Grant({
  corp,
  csrf,
  close,
  done,
}: {
  corp: string;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const m = useMembers(corp);
  const [lines, setLines] = useState<{ account_id: string; amount: string }[]>([
    { account_id: "", amount: "" },
  ]);
  const [note, setNote] = useState("");
  const [quote, setQuote] = useState<api.Quote | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [key] = useState(() => crypto.randomUUID());
  const members = uniqueMembers(m.data?.items);
  const body = () => ({
    action: "grant",
    corporation_id: corp,
    note,
    lines: lines.map((l) => ({
      account_id: l.account_id,
      amount_minor: api.minor(l.amount),
    })),
  });
  return (
    <Modal
      title={msg("发放果壳币")}
      close={close}
      busy={busy}
      footer={
        <>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => (quote ? setQuote(null) : close())}
          >
            {quote ? msg("返回修改") : msg("取消")}
          </Button>
          <Button form="grant-form" type="submit" disabled={busy || m.isError}>
            {busy ? msg("正在处理") : quote ? msg("确认发放") : msg("预览发放")}
          </Button>
        </>
      }
    >
      <form
        id="grant-form"
        className="welfare-form"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            if (!quote) {
              const q = await api.post("preview", csrf, body());
              if (!api.isQuote(q)) throw new Error(msg("预览格式异常"));
              setQuote(q);
            } else {
              await api.post("commands", csrf, {
                ...body(),
                token: quote.token,
                request_key: key,
              });
              done();
            }
          } catch (e) {
            setError(e instanceof Error ? e.message : msg("操作失败"));
          } finally {
            setBusy(false);
          }
        }}
      >
        {quote ? (
          <>
            <div className="welfare-grant-total">
              <Coins aria-hidden="true" />
              <strong>{api.money(quote.total_minor)}</strong>
              <span>
                {msg("果壳币 ·")} {quote.lines.length} {msg("人")}
              </span>
            </div>
            {quote.lines.map((l) => (
              <div className="welfare-grant-line" key={l.account_id}>
                <span>
                  {members.find((m) => m.value === l.account_id)?.label}
                </span>
                <strong>{api.money(l.amount_minor)}</strong>
              </div>
            ))}
            <p>{note}</p>
          </>
        ) : (
          <fieldset disabled={busy}>
            {lines.map((l, i) => (
              <div className="welfare-grant-line wide" key={i}>
                <Select
                  label={msg("领取成员 {0}", i + 1)}
                  value={l.account_id}
                  onValueChange={(v) =>
                    setLines((ls) =>
                      ls.map((x, j) => (j === i ? { ...x, account_id: v } : x)),
                    )
                  }
                  options={members.filter(
                    (m) =>
                      m.value === l.account_id ||
                      !lines.some((x) => x.account_id === m.value),
                  )}
                />
                <Field
                  label={msg("币数")}
                  value={l.amount}
                  onChange={(v) =>
                    setLines((ls) =>
                      ls.map((x, j) => (j === i ? { ...x, amount: v } : x)),
                    )
                  }
                  required
                />
                <IconAction
                  label={msg("移除成员 {0}", i + 1)}
                  disabled={lines.length === 1}
                  onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}
                >
                  ×
                </IconAction>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              disabled={lines.length >= 100}
              onClick={() =>
                setLines((ls) => [...ls, { account_id: "", amount: "" }])
              }
            >
              <Plus size={16} />
              {msg("添加成员")}{" "}
            </Button>
            <Field
              label={msg("发放原因")}
              value={note}
              onChange={setNote}
              required
              wide
              type="textarea"
            />
          </fieldset>
        )}
        {(error || m.isError) && (
          <p role="alert">{error || m.error?.message}</p>
        )}
      </form>
    </Modal>
  );
}
function LossPolicyEditor({
  corp,
  csrf,
  kind,
  policy,
  close,
  done,
}: {
  corp: string;
  csrf: string;
  kind: string;
  policy?: api.Policy;
  close: () => void;
  done: () => void;
}) {
  const capValue = (minor?: number) => minor ? String(minor / 100) : "";
  const optionalMinor = (value: string) => value.trim() ? api.minor(value.trim()) : 0;
  const [rate, setRate] = useState(
    String((policy?.config.loss_rate_bps ?? 10000) / 100),
  );
  const [cap, setCap] = useState(capValue(policy?.config.loss_cap_minor));
  const [dailyCap, setDailyCap] = useState(capValue(policy?.config.loss_daily_cap_minor));
  const [weeklyCap, setWeeklyCap] = useState(capValue(policy?.config.loss_weekly_cap_minor));
  const [monthlyCap, setMonthlyCap] = useState(capValue(policy?.config.loss_monthly_cap_minor));
  return (
    <Editor
      title={msg("{0}设置", api.projectLabel(kind))}
      csrf={csrf}
      close={close}
      done={done}
      command={() => {
        const bps = api.subsidyRate(rate);
        const limit = optionalMinor(cap);
        const daily = optionalMinor(dailyCap);
        const weekly = optionalMinor(weeklyCap);
        const monthly = optionalMinor(monthlyCap);
        if (
          limit > 100000000000000 ||
          daily > 100000000000000 ||
          (daily > 0 && daily < 100) ||
          weekly > 100000000000000 ||
          (weekly > 0 && weekly < 100) ||
          monthly > 100000000000000 ||
          (monthly > 0 && monthly < 100)
        )
          throw new Error(msg("请填写有效的补损上限"));
        return {
          action: "configure",
          corporation_id: corp,
          kind,
          version: policy?.version || "0",
          note: msg("更新补损设置"),
          config: {
            ...(policy?.config || blank),
            loss_rate_bps: bps,
            loss_cap_minor: limit,
            loss_daily_cap_minor: daily,
            loss_weekly_cap_minor: weekly,
            loss_monthly_cap_minor: monthly,
          },
        };
      }}
    >
      <Field
        label={msg("补损比例（%）")}
        value={rate}
        onChange={setRate}
        required
        type="number"
      />
      <Field
        label={msg("单笔上限 / ISK")}
        value={cap}
        onChange={setCap}
        type="number"
      />
      <div className="welfare-period-caps wide">
        <Field label={msg("每人每日上限 / ISK")} value={dailyCap} onChange={setDailyCap} type="number" />
        <Field label={msg("每人每周上限 / ISK")} value={weeklyCap} onChange={setWeeklyCap} type="number" />
        <Field label={msg("每人每月上限 / ISK")} value={monthlyCap} onChange={setMonthlyCap} type="number" />
      </div>
      <p className="wide">
        {msg("留空或 0 为不限 · 北京时间 · 已核准计入当期")}
      </p>
    </Editor>
  );
}

export function CaseView({
  user,
  csrf,
  item,
  policy,
  manage,
  admin,
  close,
  done,
  personalActions = true,
  approvalEntry = false,
}: {
  user: string;
  csrf: string;
  item: api.Case;
  policy?: api.Policy;
  manage: boolean;
  admin: boolean;
  close: () => void;
  done: () => void;
  personalActions?: boolean;
  approvalEntry?: boolean;
}) {
  const q = useQuery({
    queryKey: ["welfare", "case", item.id],
    queryFn: ({ signal }) => api.detail(item.id, signal),
    refetchInterval: 30_000,
  });
  const [action, setAction] = useState("");
  const reviewEvidenceRef = useRef<HTMLDetailsElement>(null);
  const [reviewCase, setReviewCase] = useState<api.Case | null>(null);
  // Keep the reviewed version stable while the background detail query refreshes.
  const v = action && reviewCase ? reviewCase : q.data?.item || item;
  const [note, setNote] = useState("");
  const [base, setBase] = useState(String(v.detail.base_minor / 100));
  const [manualPricing, setManualPricing] = useState(false);
  const [confirmedNotDelivered, setConfirmedNotDelivered] = useState(false);
  const capitalSubsidy = ["supercarrier", "titan"].includes(v.kind);
  const loss = api.activeLossKinds.includes(v.kind);
  const unified = loss || capitalSubsidy || v.kind.startsWith("growth_") || v.kind.startsWith("activity_");
  const rewards = v.detail.rewards;
  const coinOnly =
    v.kind.startsWith("growth_") &&
    rewards &&
    rewards.coins_minor > 0 &&
    !rewards.fittings?.length &&
    !rewards.items?.length && !rewards.isk_minor;
  const cancellationAction = [
    "request_cancel",
    "approve_cancel",
    "reject_cancel",
  ].includes(action);
  const lossReview =
    approvalEntry &&
    loss &&
    ["submitted", "information", "external"].includes(v.state);
  const lossReviewAction =
    lossReview && ["approve", "reject", "information", "external"].includes(action);
  const openReviewEvidence = () => {
    if (!reviewEvidenceRef.current) return;
    reviewEvidenceRef.current.open = true;
    reviewEvidenceRef.current.scrollIntoView({ block: "nearest" });
  };
  const automaticPricing = v.kind === "solo" && !manualPricing;
  const [receipt, setReceipt] = useState("");
  const [evidence, setEvidence] = useState(v.detail.evidence);
  const actions: { value: string; label: string }[] = [];
  if (
    personalActions &&
    unified &&
    user === v.account_id &&
    ["approved", "executing"].includes(v.state)
  )
    actions.push({ value: "request_cancel", label: msg("申请取消") });
  if (
    unified &&
    manage &&
    user !== v.account_id &&
    v.state === "cancel_requested"
  )
    actions.push(
      { value: "approve_cancel", label: msg("确认取消") },
      { value: "reject_cancel", label: msg("驳回取消") },
    );
  if (
    personalActions &&
    user === v.account_id &&
    ["submitted", "information", "external"].includes(v.state)
  ) {
    actions.push({ value: "cancel", label: msg("撤回申请") });
    if (v.state === "information")
      actions.push({ value: "resubmit", label: msg("补充材料") });
  }
  if (
    manage &&
    user !== v.account_id &&
    ["submitted", "information", "external"].includes(v.state)
  ) {
    actions.push(
      { value: "approve", label: msg("批准") },
      { value: "information", label: msg("要求补充") },
      { value: "reject", label: msg("驳回") },
    );
    if (v.kind === "alliance")
      actions.push({ value: "external", label: msg("等待联盟") });
  }
  if (manage && user !== v.account_id && v.state === "approved" && !unified)
    actions.push(
      { value: "execute", label: msg("领取交付任务") },
      { value: "void", label: msg("撤销批准") },
    );
  if (
    manage &&
    user !== v.account_id &&
    v.state === "executing" &&
    !capitalSubsidy &&
    !unified &&
    !v.detail.delivery
  )
    actions.push({ value: "complete", label: msg("确认已交付") });
  if (
    manage &&
    user !== v.account_id &&
    coinOnly &&
    !v.detail.delivery &&
    ["approved", "executing"].includes(v.state)
  )
    actions.push({ value: "release_coins", label: msg("发放果壳币") });
  if (admin && v.kind === "grant" && v.state === "completed")
    actions.push({ value: "reverse", label: msg("全额冲正") });
  const content = loss ? (
    <LossCaseContent
      item={v}
      user={user}
      manage={manage}
      csrf={csrf}
      refresh={!action && (user === v.account_id || manage)}
      history={q.data?.history || []}
      error={q.isError ? q.error.message : undefined}
    />
  ) : (
    <div className="welfare-details">
      <div className="welfare-detail-head">
        <span className={`welfare-status state-${v.state}`}>
          {api.states[v.state]}
        </span>
        <strong>
          {api.caseLabel(v)} #{v.id}
        </strong>
        <span>{v.detail.character_name}</span>
      </div>
      <div className="welfare-detail-values">
        {!v.kind.startsWith("growth_") && !v.kind.startsWith("activity_") && (
          <span>
            {msg("核准金额")}{" "}
            <strong>
              {v.award_minor ? api.money(v.award_minor) : "—"}{" "}
              {v.kind === "grant" ? msg("币") : "ISK"}
            </strong>
          </span>
        )}
        <span>
          {msg("提交时间")}
          <strong>{api.date(v.created_at)}</strong>
        </span>
      </div>
      <p className="welfare-proof">{v.detail.description}</p>
      <RewardSummary rewards={v.detail.rewards} />
      {v.kind.startsWith("activity_") && (v.detail.image_count || 0) > 0 && <div className="welfare-activity-images">{Array.from({ length: v.detail.image_count || 0 }, (_, i) => <a key={i} href={`/api/v1/welfare/cases/${v.id}/images/${i + 1}`} target="_blank" rel="noreferrer"><img src={`/api/v1/welfare/cases/${v.id}/images/${i + 1}`} alt={msg("活动截图 {0}", i + 1)} loading="lazy" /></a>)}</div>}
      {v.kind.startsWith("activity_") && v.detail.supporting_contract_id && v.detail.supporting_contract_id !== "0" && <p className="welfare-proof">{msg("合同材料 #")}{v.detail.supporting_contract_id}</p>}
      {capitalSubsidy && (
        <div className="welfare-detail-values">
          <span>
            {msg("购舰合同 ID")}
            <strong>{v.detail.contract_id}</strong>
          </span>
          <span>
            {msg("合同价格")}
            <strong>{api.money(v.detail.base_minor)} ISK</strong>
          </span>
          <span>
            {msg("补贴金额")}
            <strong>
              {api.awardPreview(
                v.kind,
                String(v.detail.base_minor / 100),
                false,
                v.detail.rule,
              )}{" "}
              ISK
            </strong>
          </span>
        </div>
      )}
      {capitalSubsidy &&
        !v.detail.purchase &&
        ["submitted", "information", "external"].includes(v.state) && (
          <p className="welfare-warning">
            {msg("旧申请缺少购舰合同核验证据，请撤回后重新申请。")}
          </p>
        )}
      {v.kind === "solo" && (
        <ValuationView
          key={v.id + ":" + v.version}
          item={v}
          csrf={csrf}
          refresh={
            !action &&
            (user === v.account_id || manage) &&
            ["submitted", "information", "external"].includes(v.state)
          }
        />
      )}
      {v.detail.loss_evidence && <LossEvidence loss={v.detail.loss_evidence} />}
      {v.detail.evidence && (
        <details>
          <summary>{msg("证明材料")}</summary>
          <p className="welfare-proof">{v.detail.evidence}</p>
          {v.detail.killmail_id !== "0" && <p>KM #{v.detail.killmail_id}</p>}
          {v.detail.contract_id !== "0" && (
            <p>
              {msg("合同 #")}
              {v.detail.contract_id}
            </p>
          )}
        </details>
      )}
      {v.detail.skill_evidence && (
        <p>
          {msg("技能检查：")}{" "}
          {v.detail.skill_evidence.state === "met"
            ? msg("达标")
            : ["unmet", "missing"].includes(v.detail.skill_evidence.state)
              ? msg("未达标")
              : msg("需核对")}{" "}
          · {api.date(v.detail.skill_evidence.observed_at || "")}
        </p>
      )}
      {v.detail.receipt && (
        <p className="welfare-proof">
          {msg("交付凭证：")}
          {v.detail.receipt}
        </p>
      )}
      {unified && <FulfillmentSummary item={v} user={user} manage={manage} />}
      {!unified && v.detail.delivery && (
        <DeliverySummary
          contract={v.detail.delivery.contract}
          recipient={v.detail.character_name}
          isk={capitalSubsidy}
        />
      )}
      <details>
        <summary>{msg("处理记录")}</summary>
        {q.data?.history.map((h, i) => (
          <div className="welfare-audit" key={i}>
            <time>{api.date(h.created_at)}</time>
            <p>{h.note || msg("提交申请")}</p>
          </div>
        ))}
      </details>
      {q.isError && <p role="alert">{q.error.message}</p>}
    </div>
  );
  if (action === "link_delivery")
    return (
      <DeliveryContracts
        item={v}
        csrf={csrf}
        close={() => setAction("")}
        done={done}
      />
    );
  if (!action)
    return (
      <Modal
        title={msg("福利记录")}
        close={close}
        footer={
          actions.length ? (
            <div className="welfare-actions">
              {actions.map((a) => (
                <Button
                  key={a.value}
                  variant={
                    a.value === "approve" ||
                    a.value === "complete" ||
                    a.value === "link_delivery"
                      ? "default"
                      : "outline"
                  }
                  onClick={() => {
                    setConfirmedNotDelivered(false);
                    setNote("");
                    setReviewCase(v);
                    setAction(a.value);
                  }}
                >
                  {a.label}
                  <ArrowUpRight size={14} />
                </Button>
              ))}
            </div>
          ) : undefined
        }
      >
        {approvalEntry && user !== v.account_id && (
          <Link to={approvalLink("welfare", v.id)}>{msg("前往审批中心")}</Link>
        )}
        {lossReview ? (
          <div className="welfare-loss-review">
            <LossReviewOverview item={v} onOpenEvidence={openReviewEvidence} />
            <details className="welfare-review-support" ref={reviewEvidenceRef}>
              <summary>{msg("损失与核价明细")}</summary>
              <LossCaseEvidence
                item={v}
                csrf={csrf}
                refresh={manage && user !== v.account_id}
                history={q.data?.history || []}
                error={q.isError ? q.error.message : undefined}
                review
              />
            </details>
          </div>
        ) : content}
      </Modal>
    );
  return (
    <Editor
      title={actions.find((a) => a.value === action)?.label || msg("处理申请")}
      size={cancellationAction ? "compact" : "form"}
      csrf={csrf}
      close={() => setAction("")}
      done={done}
      label={msg("确认")}
      disabled={
        (action === "approve_cancel" && !confirmedNotDelivered) ||
        (action === "approve" &&
          ((automaticPricing && v.detail.valuation?.state !== "ready") ||
            (capitalSubsidy && !v.detail.purchase)))
      }
      command={() => ({
        action,
        id: v.id,
        version: v.version,
        corporation_id: v.corporation_id,
        note,
        confirmed_not_delivered:
          action === "approve_cancel" && confirmedNotDelivered,
        manual_pricing: action === "approve" && manualPricing,
        ...(action === "approve" && loss
          ? { loss_policy_version: policy?.version || "0" }
          : {}),
        detail: {
          base_minor:
            action === "approve" && !v.kind.startsWith("growth_") && !v.kind.startsWith("activity_")
              ? automaticPricing
                ? v.detail.valuation?.amount_minor || 0
                : api.minor(base)
              : 0,
          receipt,
          evidence,
          discipline: false,
        },
      })}
    >
      <div className="wide">
        {cancellationAction ? (
          <div className="welfare-cancellation-summary">
            <strong>
              {api.caseLabel(v)} #{v.id} · {v.detail.character_name}
            </strong>
            <span>
              {msg("核准金额")} · {api.money(v.award_minor)} ISK
            </span>
            {action !== "request_cancel" && v.detail.cancellation && (
              <p className="welfare-proof">
                {msg("取消原因")}：{v.detail.cancellation.reason}
              </p>
            )}
          </div>
        ) : lossReviewAction ? (
          <LossReviewOverview item={v} onOpenEvidence={openReviewEvidence} />
        ) : (
          content
        )}
      </div>
      {action === "approve_cancel" && (
        <label className="wide welfare-pricing-toggle">
          <input
            type="checkbox"
            checked={confirmedNotDelivered}
            onChange={(e) => setConfirmedNotDelivered(e.target.checked)}
          />
          {msg("已在游戏中核对：未发放，且相关发放合同已撤销")}
        </label>
      )}
      {action === "approve" && v.kind === "solo" && (
        <label className="wide welfare-pricing-toggle">
          <input
            type="checkbox"
            checked={manualPricing}
            onChange={(e) => {
              setManualPricing(e.target.checked);
              setBase(
                String(
                  (v.detail.valuation?.amount_minor || v.detail.base_minor) /
                    100,
                ),
              );
            }}
          />
          {msg("人工核价")}{" "}
        </label>
      )}
      {action === "approve" &&
        !v.kind.startsWith("growth_") &&
        !v.kind.startsWith("activity_") &&
        !capitalSubsidy &&
        !automaticPricing && (
          <Field
            label={
              ["srp", "alliance", "solo"].includes(v.kind)
                ? msg("核价金额 / ISK")
                : msg("核准计价基数 / ISK")
            }
            value={base}
            onChange={setBase}
            required
            wide
          />
        )}
      {action === "approve" && loss && (
        <div className="wide welfare-loss-preview">
          <span>
            {msg("补损比例")}{" "}
            {((policy?.config.loss_rate_bps ?? 10000) / 100).toLocaleString(
              getLocale(),
            )}
            %
          </span>
          <span>
            {msg("单笔上限")}{" "}
            {policy?.config.loss_cap_minor
              ? `${api.money(policy.config.loss_cap_minor)} ISK`
              : msg("不限制")}
          </span>
          {([
            ["日额度", policy?.config.loss_daily_cap_minor],
            ["周额度", policy?.config.loss_weekly_cap_minor],
            ["月额度", policy?.config.loss_monthly_cap_minor],
          ] as const).map(([label, cap]) =>
            cap ? (
              <span key={label}>
                {msg(label)} {api.money(cap)} ISK
              </span>
            ) : null,
          )}
          <strong>
            {msg("核准金额")}{" "}
            {api.money(
              api.lossPreview(
                automaticPricing ? v.detail.valuation?.amount_minor || 0 : base,
                policy?.config,
              ),
            )}{" "}
            ISK
          </strong>
        </div>
      )}
      {lossReviewAction && (
        <details className="wide welfare-review-support" ref={reviewEvidenceRef}>
          <summary>{msg("损失与核价明细")}</summary>
          <LossCaseEvidence
            item={v}
            csrf={csrf}
            refresh={false}
            history={q.data?.history || []}
            error={q.isError ? q.error.message : undefined}
            review
          />
        </details>
      )}
      {action === "approve" && v.kind === "capital" && (
        <p className="wide">
          {msg("预计补贴：")}{" "}
          <strong>{api.awardPreview(v.kind, base, false)}</strong> ISK
        </p>
      )}
      {action === "complete" && (
        <Field
          label={msg("实际付款 / 合同接取凭证")}
          value={receipt}
          onChange={setReceipt}
          type="textarea"
          required
          wide
        />
      )}
      {action === "resubmit" && (
        <Field
          label={msg("补充证明")}
          value={evidence}
          onChange={setEvidence}
          type="textarea"
          required
          wide
        />
      )}
      {action === "reverse" && (
        <p className="welfare-warning wide">
          {msg("将扣回")} {api.money(v.award_minor)}{" "}
          {msg("果壳币。若已消费，可能形成欠额。")}{" "}
        </p>
      )}
      {action === "void" && (
        <p className="welfare-warning wide">
          {msg("仅在尚未游戏交付时撤销，将释放该申请占用的资格。")}{" "}
        </p>
      )}
      <Field
        label={
          action === "request_cancel"
            ? msg("取消原因")
            : action === "reject_cancel"
              ? msg("驳回原因")
              : action === "complete"
                ? msg("核对说明")
                : action === "approve" && manualPricing
                  ? msg("人工核价原因")
                  : msg("处理说明")
        }
        value={note}
        onChange={setNote}
        type="textarea"
        required={action !== "cancel"}
        wide
      />
    </Editor>
  );
}
