import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Building2, ChevronDown, Fuel, Layers3, MapPin, RefreshCw, ShieldCheck } from "lucide-react";
import { msg, getLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { useSession } from "@/modules/identity";
import { Navigate } from "react-router-dom";
import * as api from "./api";
import "./structures.css";

const date = (v?: string) => (v ? new Date(v).toLocaleString(getLocale(), { hour12: false }) : "—");
type Filter = "all" | string;

const structureType = (item: api.Structure) => item.type_name?.trim() || (item.kind === "pos" ? msg("POS") : msg("Upwell 建筑"));
const structureTypeKey = (item: api.Structure) => `${item.kind}:${item.type_id}`;
const fuelTone = (value?: string) => {
  if (!value) return "unknown";
  const remaining = Date.parse(value) - Date.now();
  return remaining <= 24 * 60 * 60 * 1000 ? "critical" : remaining <= 72 * 60 * 60 * 1000 ? "warning" : "normal";
};

export default function StructuresPage() {
  const session = useSession();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/login" replace />;
  return <Workspace />;
}

function Workspace() {
  const [filter, setFilter] = useState<Filter>("all");
  const q = useQuery({
    queryKey: ["structures", "overview"],
    queryFn: ({ signal }) => api.structures("", signal),
    staleTime: 60_000,
  });
  const items = q.data?.items ?? [];
  const typeOptions = Array.from(items.reduce((types, item) => {
    const key = structureTypeKey(item);
    const current = types.get(key) ?? { label: structureType(item), count: 0 };
    current.count += 1;
    types.set(key, current);
    return types;
  }, new Map<string, { label: string; count: number }>()));
  const filtered = filter === "all" ? items : items.filter((item) => structureTypeKey(item) === filter);
  const corporations = Array.from(filtered.reduce((groups, item) => {
    const group = groups.get(item.corporation_id) ?? { name: item.corporation_name, items: [] as api.Structure[] };
    group.items.push(item);
    groups.set(item.corporation_id, group);
    return groups;
  }, new Map<string, { name: string; items: api.Structure[] }>()));
  return (
    <div className="structures-page">
      <header className="structures-heading">
        <div className="structures-title">
          <span className="structures-brand"><Building2 aria-hidden="true" /></span>
          <div><h1>{msg("建筑管理")}</h1><p>{msg("按类型查看军团建筑")}</p></div>
        </div>
        <Button variant="outline" disabled={q.isFetching} onClick={() => void q.refetch()}>
          <RefreshCw size={16} aria-hidden="true" /> {msg("刷新")}
        </Button>
      </header>
      {!q.isError && !q.isPending && items.length > 0 && <div className="structures-toolbar" aria-label={msg("建筑分类")}>
        <div className="structure-filters" role="tablist" aria-label={msg("建筑分类")}>
          {[["all", msg("全部"), items.length] as const, ...typeOptions.map(([key, option]) => [key, option.label, option.count] as const)].map(([key, label, count]) => <button key={key} type="button" role="tab" aria-selected={filter === key} className={filter === key ? "is-active" : ""} onClick={() => setFilter(key)}>
            {label}<span>{count}</span>
          </button>)}
        </div>
        <span className="structures-total"><Layers3 size={15} aria-hidden="true" />{msg("共 {0} 个建筑", filtered.length)}</span>
      </div>}
      {q.isError && <div className="structures-feedback" role="alert"><p>{q.error.message}</p><Button variant="outline" onClick={() => void q.refetch()}>{msg("重试")}</Button></div>}
      {!q.isError && q.isPending && <p role="status">{msg("正在读取")}</p>}
      {!q.isError && !q.isPending && items.length === 0 && <div className="structures-empty"><Building2 size={32} aria-hidden="true" /><p>{msg("暂无可查看的建筑")}</p><span>{msg("需要角色拥有军团建筑读取权限，并完成 ESI 授权。")}</span></div>}
      {corporations.map(([corporationID, group]) => {
        const categories = Array.from(group.items.reduce((types, item) => {
          const key = structureTypeKey(item);
          const category = types.get(key) ?? { label: structureType(item), items: [] as api.Structure[] };
          category.items.push(item);
          types.set(key, category);
          return types;
        }, new Map<string, { label: string; items: api.Structure[] }>()));
        return <section className="structure-corporation" key={corporationID}>
        <header className="structure-corporation-head"><div><h2>{group.name}</h2><span>{msg("军团 ID")} {corporationID}</span></div><span>{msg("最近同步")} {date(group.items.reduce((latest, item) => item.observed_at > latest ? item.observed_at : latest, ""))}</span></header>
        {categories.map(([categoryKey, category]) => <div className="structure-category" key={categoryKey}>
          <div className="structure-category-head"><h3>{category.label}</h3><span>{msg("共 {0} 个建筑", category.items.length)}</span></div>
          <div className="structures-grid">
            {category.items.map((item) => <article className="structure-card" key={`${item.kind}:${item.id}`}>
              <div className="structure-card-head"><div className="structure-card-title"><h3>{item.name || msg("未命名建筑")}</h3><div className="structure-location"><MapPin size={16} aria-hidden="true" /><strong>{item.solar_system_name || (item.solar_system_id === "0" ? msg("未知星系") : item.solar_system_id)}</strong></div></div><div className="structure-card-badges"><span className="structure-type-badge">{category.label}</span><span className={`structure-state state-${item.state}`}>{item.state || msg("未知")}</span></div></div>
              {item.kind === "upwell" && <div className={`structure-fuel fuel-${fuelTone(item.fuel_expires)}`}><Fuel size={17} aria-hidden="true" /><span>{msg("燃料到期")}</span><strong>{date(item.fuel_expires)}</strong></div>}
              {item.kind === "pos" && <details className="structure-details"><summary><Fuel size={16} aria-hidden="true" /><span>{msg("燃料仓")}</span><b>{item.fuel?.length ?? 0}</b><ChevronDown size={16} aria-hidden="true" /></summary><div className="structure-detail-body">{item.fuel?.length ? item.fuel.map((f) => <span key={`${f.type_id}:${f.quantity}`}>{f.type_id} × {f.quantity}</span>) : <span>{msg("暂无燃料明细")}</span>}</div></details>}
              {item.services && item.services.length > 0 && <details className="structure-details"><summary><ShieldCheck size={16} aria-hidden="true" /><span>{msg("服务")}</span><b>{item.services.length}</b><ChevronDown size={16} aria-hidden="true" /></summary><div className="structure-detail-body">{item.services.map((s) => <span key={s.name}>{s.name} · {s.state}</span>)}</div></details>}
            </article>)}
          </div>
        </div>)}
      </section>;
      })}
      {!q.isError && !q.isPending && items.length > 0 && filtered.length === 0 && <div className="structures-empty"><Layers3 size={28} aria-hidden="true" /><p>{msg("此分类暂无建筑")}</p></div>}
    </div>
  );
}
