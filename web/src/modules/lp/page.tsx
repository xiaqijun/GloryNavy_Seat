import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { Coins, RefreshCw, Search } from "lucide-react";
import { useSession } from "@/modules/identity";
import { getLocale, msg } from "@/lib/i18n";
import { Select } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import * as api from "./api";
import "./lp.css";

const markets = [ ["10000002","吉他"], ["10000043","艾玛"], ["10000032","多迪谢"], ["10000030","伦斯"], ["10000042","赫克"] ];
const displayName = (n?: api.Name) => n ? (getLocale()==="en" ? n.en||n.zh : n.zh||n.en) : "";
const num = (v: number) => v.toLocaleString(getLocale(),{maximumFractionDigits:2});
const money = (v: number | null) => v === null ? "—" : `${Math.abs(v)>=1e6 ? new Intl.NumberFormat("en-US",{notation:"compact",maximumFractionDigits:2}).format(v) : num(v)} ISK`;
const searchText = (text: string) => text.normalize("NFKC").trim().toLocaleLowerCase();

export default function LPPage() {
  const session = useSession();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/" replace />;
  return <QueryPage key={session.data.session.user_id} user={session.data.session.user_id} />;
}
function QueryPage({user}:{user:string}) {
  const [faction,setFaction] = useState("");
  const [corp,setCorp] = useState("1000130");
  const [corpQuery,setCorpQuery] = useState("");
  const [region,setRegion] = useState("10000002");
  const [mode,setMode] = useState<"sell"|"buy">("sell");
  const [tax,setTax] = useState("0");
  const [query,setQuery] = useState("");
  const [sort,setSort] = useState("efficiency");
  const [page,setPage] = useState(1);
  const catalog = useQuery({queryKey:["lp",user,"catalog"],queryFn:({signal})=>api.catalog(signal),staleTime:300000,retry:1});
  const offers = useQuery({queryKey:["lp",user,"offers",corp],queryFn:({signal})=>api.offers(corp,signal),enabled:!!catalog.data && !!corp,staleTime:300000,retry:1});
  const prices = useQuery({queryKey:["lp",user,"prices",corp,region,offers.dataUpdatedAt],queryFn:({signal})=>api.prices(region,offers.data!.offers,signal),enabled:!!offers.data && !offers.isFetching && !offers.isError,staleTime:60000,retry:1});
  const names = offers.data?.names;
  const typeName = (id: number) => displayName(names?.[id]) || `#${id}`;
  const taxValue = tax.trim() === "" ? NaN : Number(tax);
  const taxValid = Number.isFinite(taxValue) && taxValue>=0 && taxValue<=100;
  const ready = !offers.isFetching && !offers.isError && !!prices.data && !prices.isError && !prices.isFetching;
  const rows = useMemo(()=> {
    const terms = searchText(query).split(/\s+/).filter(Boolean);
    return (offers.data?.offers || []).map(offer=>({offer,...api.estimate(offer,ready ? prices.data!.items : {},mode,taxValue)})).filter(row=> {
      const n=offers.data?.names[row.offer.type_id];
      const productName=offers.data?.names[row.product.type_id];
      const text=searchText(`${n?.zh} ${n?.en} ${row.offer.type_id} ${productName?.zh} ${productName?.en}`);
      return terms.every(term=>text.includes(term));
    }).sort((a,b)=> {
      if (sort==="name") return displayName(offers.data?.names[a.offer.type_id]).localeCompare(displayName(offers.data?.names[b.offer.type_id]),getLocale());
      if (sort==="lp") return a.offer.lp_cost-b.offer.lp_cost || a.offer.ak_cost-b.offer.ak_cost;
      if (a.profit===null || b.profit===null) return a.profit===b.profit ? a.offer.offer_id-b.offer.offer_id : a.profit===null ? 1 : -1;
      if (sort==="profit") return b.profit-a.profit;
      const rank=(p:string|null)=>p==="LP" ? 0 : p==="AK" ? 1 : 2;
      return rank(a.point)-rank(b.point) || (b.efficiency ?? b.profit)-(a.efficiency ?? a.profit);
    });
  },[offers.data,query,ready,prices.data,mode,taxValue,sort]);
  const pages=Math.max(1,Math.ceil(rows.length/25));
  const current=Math.min(page,pages);
  const corporations=(catalog.data?.corporations||[]).filter(c=>(!faction || String(c.faction_id)===faction) && searchText(`${c.zh} ${c.en} ${c.id}`).includes(searchText(corpQuery)));
  const chooseCorp=(v:string)=>{setCorp(v);setPage(1);};
  const pending=catalog.isFetching || offers.isFetching || (!!offers.data && prices.isFetching);
  const error=catalog.error || offers.error || prices.error;
  const refresh=()=> { if(catalog.isError || !catalog.data) void catalog.refetch(); else if(prices.isError && offers.data && !offers.isError) void prices.refetch(); else void offers.refetch(); };
  const columns=[msg("兑换物品"),msg("点数"),msg("ISK 与材料成本"),msg("产出估值"),msg("预计收益"),msg("单位点数收益")];
  return <div className="lp-page">
    <header className="lp-heading"><Coins aria-hidden="true"/><h1>{msg("LP 兑换查询")}</h1><IconAction label={msg("刷新数据")} disabled={pending} onClick={refresh}><RefreshCw aria-hidden="true"/></IconAction></header>
    <section className="lp-panel lp-filters" aria-label={msg("查询条件")}>
      <label><span>{msg("NPC 阵营")}</span><Select label={msg("NPC 阵营")} value={faction} options={[{value:"",label:msg("全部阵营")},...(catalog.data?.factions||[]).map(f=>({value:String(f.id),label:displayName(f)}))]} onValueChange={v=>{setFaction(v);setCorpQuery("");if(v && !catalog.data?.corporations.some(c=>String(c.id)===corp && String(c.faction_id)===v)){chooseCorp(String(catalog.data?.corporations.find(c=>String(c.faction_id)===v)?.id || ""));}}}/></label>
      <div className="lp-corporation"><label htmlFor="lp-corp-search">{msg("NPC 军团")}</label><input id="lp-corp-search" aria-label={msg("搜索 NPC 军团")} value={corpQuery} onChange={e=>setCorpQuery(e.target.value)} placeholder={msg("搜索名称或 ID")}/><Select label={msg("NPC 军团")} value={corp} options={corporations.map(c=>({value:String(c.id),label:displayName(c)}))} disabled={!corporations.length} onValueChange={chooseCorp}/>{!catalog.isPending && !catalog.isError && !corporations.length && <span role="status">{msg("没有匹配的军团")}</span>}</div>
      <label><span>{msg("市场")}</span><Select label={msg("市场")} value={region} options={markets.map(([value,label])=>({value,label:msg(label)}))} onValueChange={v=>{setRegion(v);setPage(1);}}/></label>
      <label><span>{msg("售价依据")}</span><Select label={msg("售价依据")} value={mode} options={[{value:"sell",label:msg("最低卖单")},{value:"buy",label:msg("最高买单")}]} onValueChange={v=>{setMode(v as "sell"|"buy");setPage(1);}}/></label>
      <label><span>{msg("税费 / %")}</span><input type="number" min="0" max="100" step="0.01" inputMode="decimal" value={tax} aria-invalid={!taxValid} onChange={e=>setTax(e.target.value)}/></label>
    </section>
    {!taxValid && <p role="alert">{msg("税费须为 0 至 100")}</p>}
    {error && <div className="lp-error" role="alert"><span>{error.message}</span><Button variant="outline" disabled={pending} onClick={refresh}>{msg("重试")}</Button></div>}
    <section className="lp-panel lp-results" aria-label={msg("兑换项目")} aria-busy={pending}>
      <div className="lp-toolbar"><label className="lp-search"><Search size={18} aria-hidden="true"/><input aria-label={msg("搜索兑换物品")} placeholder={msg("物品中文名、英文名或 ID")} value={query} onChange={e=>{setQuery(e.target.value);setPage(1);}}/></label><Select label={msg("排序")} value={sort} options={[{value:"efficiency",label:msg("单位点数收益")},{value:"profit",label:msg("预计收益")},{value:"lp",label:msg("LP 成本")},{value:"name",label:msg("物品名称")}]} onValueChange={v=>{setSort(v);setPage(1);}}/><span>{msg("{0} 项",rows.length)}</span></div>
      {pending && <p role="status" className="lp-state">{catalog.isFetching || offers.isFetching ? msg("正在读取 LP 商店…") : msg("正在查询市场价格…")}</p>}
      {!pending && !offers.isError && offers.data && rows.length===0 && <p className="lp-state" role="status">{query ? msg("没有匹配的兑换项目") : msg("该军团暂无兑换项目")}</p>}
      {rows.length>0 && <table><thead><tr>{columns.map(c=><th scope="col" key={c}>{c}</th>)}</tr></thead><tbody>{rows.slice((current-1)*25,current*25).map(row=> <tr key={row.offer.offer_id}>
        <td data-label={columns[0]}><strong>{typeName(row.offer.type_id)}</strong><span className="lp-quantity">{row.offer.blueprint ? msg("{0} 流程",row.offer.quantity) : `× ${num(row.offer.quantity)}`}</span>{row.offer.blueprint && <div>{msg("产出")} {typeName(row.product.type_id)} × {num(row.product.quantity)}</div>}
          <details><summary>{msg("材料明细")}</summary><div className="lp-materials"><p>{msg("商店 ISK")}：{money(row.offer.isk_cost)}</p>{row.offer.required_items.map((m,i)=><p key={`r${i}`}>{typeName(m.type_id)} × {num(m.quantity)}</p>)}{row.offer.blueprint && <><p><b>{msg("制造材料（ME 0）")}</b></p>{row.offer.blueprint.materials.map((m,i)=><p key={`b${i}`}>{typeName(m.type_id)} × {num(m.quantity*row.offer.quantity)}</p>)}</>}{!row.materials.length && <p>{msg("无需材料")}</p>}</div></details>
        </td>
        <td data-label={columns[1]}>{num(row.offer.lp_cost)} LP{row.offer.ak_cost>0 && <div>{num(row.offer.ak_cost)} AK</div>}</td>
        <td data-label={columns[2]} title={row.cost===null ? undefined : `${num(row.cost)} ISK`}>{money(row.cost)}</td>
        <td data-label={columns[3]} title={row.revenue===null ? undefined : `${num(row.revenue)} ISK`}>{money(row.revenue)}</td>
        <td data-label={columns[4]} className={row.profit!==null && row.profit<0 ? "lp-negative" : ""} title={row.profit===null ? undefined : `${num(row.profit)} ISK`}>{row.profit===null ? (ready && taxValid ? msg("报价不完整") : "—") : money(row.profit)}</td>
        <td data-label={columns[5]}><b>{row.efficiency===null ? "—" : `${num(row.efficiency)} ISK/${row.point}`}</b></td>
      </tr>)}</tbody></table>}
      <footer className="lp-pagination"><span>{msg("第 {0} / {1} 页",current,pages)}</span><Button variant="outline" disabled={current<=1} onClick={()=>setPage(current-1)}>{msg("上一页")}</Button><Button variant="outline" disabled={current>=pages} onClick={()=>setPage(current+1)}>{msg("下一页")}</Button></footer>
    </section>
    <details className="lp-method"><summary>{msg("计算口径与数据来源")}</summary><p>{msg("收益 = 产出估值 × (1 − 税费) − 商店 ISK − 兑换材料 − 制造材料。材料按所选市场最低卖单估价；蓝图按 ME 0 和流程数计算，不含制造安装费与运输成本。")}</p><p>{msg("市场价格仅供参考，不保证成交。LP 与 AK 分别排序，同时消耗两种点数时不计算单位收益。")}</p><p>ESI · SDE {offers.data?.sde_build || catalog.data?.build || "—"} · {prices.data?.source === "esi" ? "ESI Market" : prices.data?.source || "Fuzzwork / ESI"}{ready && prices.data?.observed_at && ` · ${new Date(prices.data.observed_at).toLocaleString(getLocale())}`}</p></details>
  </div>;
}
