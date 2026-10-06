import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Landmark, RefreshCw, ShieldCheck } from "lucide-react";
import { Navigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import { useSession } from "@/modules/identity";
import * as api from "./api";
import "./loan.css";

const money = (v: number) => `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(v / 100)} ISK`;
const state = (v: string) => ({ submitted: msg("待审核"), approved: msg("待放款"), active: msg("还款中"), settled: msg("已结清"), rejected: msg("已驳回"), cancelled: msg("已取消"), defaulted: msg("已逾期") }[v] ?? v);

export default function LoanPage() {
  const session = useSession();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/login" replace />;
  return <Workspace csrf={session.data.session.csrf_token} />;
}

function Workspace({ csrf }: { csrf: string }) {
  const [pool, setPool] = useState("");
  const [principal, setPrincipal] = useState("");
  const [interest, setInterest] = useState("");
  const [character, setCharacter] = useState("");
  const [count, setCount] = useState("1");
  const [interval, setInterval] = useState("30");
  const [due, setDue] = useState("");
  const [message, setMessage] = useState("");
  const context = useQuery({ queryKey: ["loan", "context"], queryFn: ({ signal }) => api.context(signal), staleTime: 30_000 });
  const list = useQuery({ queryKey: ["loan", "cases"], queryFn: ({ signal }) => api.cases(signal), staleTime: 15_000 });
  const submit = async (event: FormEvent) => { event.preventDefault(); setMessage(""); try { await api.createApplication(csrf, { pool_id: Number(pool), borrower_character_id: Number(character), principal_minor: Math.round(Number(principal) * 100), interest_minor: Math.round(Number(interest || 0) * 100), installment_count: Number(count), interval_days: Number(interval), first_due_at: new Date(due).toISOString() }); setMessage(msg("贷款申请已提交")); await Promise.all([context.refetch(), list.refetch()]); } catch (e) { setMessage(e instanceof Error ? e.message : msg("贷款申请失败")); } };
  if (context.isError || list.isError) return <div className="loan-feedback" role="alert"><p>{(context.error ?? list.error)?.message}</p><Button variant="outline" onClick={() => void Promise.all([context.refetch(), list.refetch()])}>{msg("重试")}</Button></div>;
  const pools = context.data?.pools.filter((p) => p.state === "open") ?? [];
  return <div className="loan-page"><header className="loan-heading"><div className="loan-title"><span className="loan-mark"><Landmark aria-hidden="true" /></span><div><h1>{msg("贷款")}</h1><p>{msg("管理个人与军团 ISK 贷款")}</p></div></div><Button variant="outline" disabled={context.isFetching || list.isFetching} onClick={() => void Promise.all([context.refetch(), list.refetch()])}><RefreshCw size={16} aria-hidden="true" />{msg("刷新")}</Button></header>
    {context.data?.credit.state !== "active" && <div className="loan-notice" role="status"><ShieldCheck size={18} aria-hidden="true" /><span>{msg("信用额度尚未配置，审批前需要管理员完成评分和授信。")}</span></div>}
    {context.data && <section className="loan-credit"><div><span>{msg("信用评分")}</span><strong>{context.data.credit.score ?? "—"}</strong></div><div><span>{msg("总额度")}</span><strong>{money(context.data.credit.total_limit_minor)}</strong></div><div><span>{msg("无担保额度")}</span><strong>{money(context.data.credit.unsecured_limit_minor)}</strong></div></section>}
    <section className="loan-apply"><div className="loan-section-head"><div><h2>{msg("提交贷款申请")}</h2><p>{msg("固定总利息按期等额分期，合同核验通过后才开始计入债务。")}</p></div></div><form onSubmit={submit} className="loan-form"><label>{msg("贷款池")}<select value={pool} onChange={(e) => setPool(e.target.value)} required><option value="">{msg("请选择")}</option>{pools.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label><label>{msg("收款角色 ID")}<input value={character} onChange={(e) => setCharacter(e.target.value)} inputMode="numeric" required /></label><label>{msg("本金（ISK）")}<input value={principal} onChange={(e) => setPrincipal(e.target.value)} inputMode="decimal" required /></label><label>{msg("总利息（ISK）")}<input value={interest} onChange={(e) => setInterest(e.target.value)} inputMode="decimal" required /></label><label>{msg("期数")}<input value={count} onChange={(e) => setCount(e.target.value)} inputMode="numeric" required /></label><label>{msg("间隔天数")}<input value={interval} onChange={(e) => setInterval(e.target.value)} inputMode="numeric" required /></label><label>{msg("首期到期时间")}<input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} required /></label><div className="loan-form-actions"><Button type="submit" disabled={context.isPending || pools.length === 0}>{msg("提交申请")}</Button>{message && <span role="status">{message}</span>}</div></form></section>
    <section className="loan-list"><div className="loan-section-head"><h2>{msg("我的贷款")}</h2></div>{list.isPending ? <p role="status">{msg("正在读取")}</p> : list.data?.items.length ? <div className="loan-table-wrap"><table><thead><tr><th>{msg("编号")}</th><th>{msg("贷款池")}</th><th>{msg("本金")}</th><th>{msg("总应还")}</th><th>{msg("状态")}</th></tr></thead><tbody>{list.data.items.map((item) => <tr key={item.id}><td>{item.public_id}</td><td>{item.pool_name}</td><td>{money(item.principal_minor)}</td><td>{money(item.total_due_minor)}</td><td><span className={`loan-state loan-state-${item.state}`}>{state(item.state)}</span></td></tr>)}</tbody></table></div> : <div className="loan-empty"><Landmark size={28} aria-hidden="true" /><p>{msg("暂无贷款记录")}</p></div>}</section>
  </div>;
}
