import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Check, ChevronRight, Copy, Landmark, RefreshCw, ShieldCheck } from "lucide-react";
import { Navigate, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { FormDialog } from "@/components/ui/form-dialog";
import { Modal } from "@/components/ui/dialog";
import { IconAction } from "@/components/ui/icon-action";
import { AccountCharacterSelect, type AccountCharacterOption } from "@/components/account-character-select";
import { getLocale, msg } from "@/lib/i18n";
import { getCharacters, useSession } from "@/modules/identity";
import { getProfile } from "@/modules/community/api";
import * as api from "./api";
import "./loan.css";

const money = (v: number) => `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(v / 100)} ISK`;
const loanState = (v: string) => ({ submitted: msg("待审核"), approved: msg("待放款"), active: msg("还款中"), settled: msg("已结清"), rejected: msg("已驳回"), cancelled: msg("已取消"), defaulted: msg("已逾期") }[v] ?? v);

export default function LoanPage() {
  const session = useSession();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/" replace />;
  return <Workspace csrf={session.data.session.csrf_token} userId={session.data.session.user_id} />;
}

function Workspace({ csrf, userId }: { csrf: string; userId: string }) {
  const [principal, setPrincipal] = useState(""); const [interest, setInterest] = useState(""); const [character, setCharacter] = useState(""); const [count, setCount] = useState("1"); const [interval, setInterval] = useState("30"); const [due, setDue] = useState(""); const [message, setMessage] = useState(""); const [applicationOpen, setApplicationOpen] = useState(false); const [applicationBusy, setApplicationBusy] = useState(false);
  const [contributionKind, setContributionKind] = useState<"personal" | "corporation">("personal"); const [contributionCharacter, setContributionCharacter] = useState(""); const [contributionCorporation, setContributionCorporation] = useState(""); const [contributionAmount, setContributionAmount] = useState(""); const [contributionMessage, setContributionMessage] = useState(""); const [contributionOpen, setContributionOpen] = useState(false); const [contributionBusy, setContributionBusy] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const activeView = searchParams.get("view") === "risk" || searchParams.get("view") === "evaluations" ? searchParams.get("view") as "risk" | "evaluations" : "business";
  const setView = (view: "business" | "risk" | "evaluations") => { const next = new URLSearchParams(searchParams); if (view === "business") next.delete("view"); else next.set("view", view); setSearchParams(next); };
  const context = useQuery({ queryKey: ["loan", "context", userId], queryFn: ({ signal }) => api.context(signal), staleTime: 30_000, refetchOnWindowFocus: false });
  const boundCharacters = useQuery({ queryKey: ["identity", "characters", userId], queryFn: ({ signal }) => getCharacters(signal), staleTime: 30_000, refetchOnWindowFocus: false });
  const profile = useQuery({ queryKey: ["community", "profile", userId], queryFn: ({ signal }) => getProfile(signal), staleTime: 60_000, refetchOnWindowFocus: false });
  const list = useQuery({ queryKey: ["loan", "cases", userId], queryFn: ({ signal }) => api.cases(signal), staleTime: 15_000, refetchOnWindowFocus: false });
  const contributions = useQuery({ queryKey: ["loan", "contributions", userId], queryFn: ({ signal }) => api.contributions(signal), staleTime: 15_000, refetchOnWindowFocus: false });
  const guarantees = useQuery({ queryKey: ["loan", "guarantees", userId], queryFn: ({ signal }) => api.guarantees(signal), staleTime: 15_000, refetchOnWindowFocus: false });
  const refresh = () => void Promise.all([context.refetch(), boundCharacters.refetch(), profile.refetch(), list.refetch(), contributions.refetch(), guarantees.refetch()]);
  const submit = async (event: FormEvent) => { event.preventDefault(); setMessage(""); const sharedPool = pools[0]; const borrowerCharacterID = character || characterOptions[0]?.id; if (!sharedPool) { setMessage(msg("统一贷款池暂不可用")); return; } if (!borrowerCharacterID) { setMessage(msg("当前账号没有可用的绑定角色")); return; } setApplicationBusy(true); try { await api.createApplication(csrf, { pool_id: Number(sharedPool.id), borrower_character_id: Number(borrowerCharacterID), principal_minor: Math.round(Number(principal) * 100), interest_minor: Math.round(Number(interest || 0) * 100), installment_count: Number(count), interval_days: Number(interval), first_due_at: new Date(due).toISOString() }); setMessage(""); setApplicationOpen(false); await Promise.all([context.refetch(), list.refetch()]); } catch (e) { setMessage(e instanceof Error ? e.message : msg("贷款申请失败")); } finally { setApplicationBusy(false); } };
  const contribute = async (event: FormEvent) => { event.preventDefault(); setContributionMessage(""); const sourceCharacterID = contributionCharacter || characterOptions[0]?.id; if (!sourceCharacterID) { setContributionMessage(msg("当前账号没有可用的绑定角色")); return; } setContributionBusy(true); try { await api.createContribution(csrf, { lender_kind: contributionKind, source_character_id: Number(sourceCharacterID), corporation_id: contributionKind === "corporation" ? Number(contributionCorporation) : 0, amount_minor: Math.round(Number(contributionAmount) * 100) }); setContributionMessage(msg("出借额度已提交，请完成入金合同")); setContributionAmount(""); setContributionOpen(false); await Promise.all([contributions.refetch(), context.refetch()]); } catch (e) { setContributionMessage(e instanceof Error ? e.message : msg("出借额度提交失败")); } finally { setContributionBusy(false); } };
  if (context.isError || boundCharacters.isError || list.isError || contributions.isError || guarantees.isError) return <div className="loan-feedback" role="alert"><p>{(context.error ?? boundCharacters.error ?? list.error ?? contributions.error ?? guarantees.error)?.message}</p><Button variant="outline" onClick={refresh}>{msg("重试")}</Button></div>;
  const pools = context.data?.pools.filter((p) => p.state === "open") ?? []; const summary = context.data?.pool_summary; const nickname = profile.data?.kook.value?.trim() || undefined; const characterOptions: AccountCharacterOption[] = (boundCharacters.data?.characters.filter((character) => character.status === "active") ?? []).map((character) => ({ ...character, account_id: userId, nickname })); const custodianOptions: AccountCharacterOption[] = context.data?.custodian_characters?.length ? context.data.custodian_characters : characterOptions;
  return <div className="loan-page"><header className="loan-heading"><div className="loan-title"><span className="loan-mark"><Landmark aria-hidden="true" /></span><div><h1>{msg("贷款")}</h1><p>{msg("统一资金池：成员可提交出借额度，完成合同入金后供借款使用")}</p></div></div><div className="loan-heading-actions"><Button variant="outline" disabled={context.isFetching || list.isFetching || contributions.isFetching} onClick={refresh}><RefreshCw size={16} aria-hidden="true" />{msg("刷新")}</Button></div></header>
    <nav className="loan-tabs" role="tablist" aria-label={msg("贷款页面")}>{([['business', msg('贷款业务')], ['risk', msg('风控设置')], ['evaluations', msg('评估记录')]] as const).map(([view, label]) => ((view !== 'risk' || context.data?.can_manage_pool) && <button key={view} type="button" role="tab" className={activeView === view ? 'is-active' : ''} aria-selected={activeView === view} onClick={() => setView(view)}>{label}</button>))}</nav>
    {activeView === "business" && <>    {summary && <section className="loan-credit"><div><span>{msg("待核验入金")}</span><strong>{money(summary.pending_minor)}</strong></div><div><span>{msg("已入池资金")}</span><strong>{money(summary.cash_minor)}</strong></div><div><span>{msg("可放贷余额")}</span><strong>{money(summary.available_minor)}</strong></div></section>}
    {context.data?.credit && <><section className="loan-credit loan-credit-assessment"><div><span>{msg("系统信用评分")}</span><strong>{context.data.credit.score ?? "—"}/100</strong><small>{msg("系统自动评估")}</small></div><div><span>{msg("系统总额度")}</span><strong>{money(context.data.credit.total_limit_minor)}</strong><small>{msg("按评估结果动态计算")}</small></div><div><span>{msg("系统无担保额度")}</span><strong>{money(context.data.credit.unsecured_limit_minor)}</strong><small>{msg("按评估结果动态计算")}</small></div></section><section className="loan-credit-factors" aria-label={msg("评分因子")}><div><span>{msg("履约")}</span><strong>{context.data.credit.repayment_points ?? 0}/55</strong></div><div><span>{msg("负债压力")}</span><strong>{context.data.credit.leverage_points ?? 0}/20</strong></div><div><span>{msg("担保/抵押履历")}</span><strong>{context.data.credit.security_points ?? 0}/10</strong></div><div><span>{msg("军团活动集结率")}</span><strong>{context.data.credit.corporation_attendance_points ?? 0}/3</strong><small>{(context.data.credit.corporation_eligible_events ?? 0) > 0 ? `${Math.round((context.data.credit.corporation_attendance_bps ?? 0) / 100)}% · ${context.data.credit.corporation_attended_events ?? 0}/${context.data.credit.corporation_eligible_events}` : msg("暂无有效活动数据")}</small></div><div><span>{msg("近6个月联盟 PAP 满足")}</span><strong>{context.data.credit.alliance_fulfillment_points ?? 0}/2</strong><small>{(context.data.credit.alliance_eligible_months ?? 0) > 0 ? `${Math.round((context.data.credit.alliance_fulfillment_bps ?? 0) / 100)}% · ${context.data.credit.alliance_fulfilled_months ?? 0}/${context.data.credit.alliance_eligible_months}` : msg("暂无完整联盟快照")}</small></div><div><span>{msg("资产规模评分")}</span><strong>0/10</strong><small>{msg("资产数据源尚未接入")}</small></div></section><p className="loan-credit-factor-note">{msg("PAP 总分 5 分由两项组成：军团活动集结率 3 分，近 6 个月联盟 PAP 满足情况 2 分；缺失或未完成的数据保持中性。资产规模评分当前固定为 0 分。")}</p></>}
    {context.data?.credit.state !== "active" && <div className="loan-notice" role="status"><ShieldCheck size={18} aria-hidden="true" /><span>{msg("系统信用评估尚未通过，审批前需要完成履约评估。")}</span></div>}
    <div className="loan-action-bar" aria-label={msg("贷款")}><div className="loan-action-option"><div><strong>{msg("提交出借额度")}</strong><p>{msg("出借方先提交额度，再创建转入统一托管方的 ISK 合同；完成核验后才计入可放贷余额。")}</p></div><Button variant="outline" onClick={() => { setContributionMessage(""); setContributionOpen(true); }} disabled={pools.length === 0 || characterOptions.length === 0}>{msg("开始出借")}</Button></div><div className="loan-action-option"><div><strong>{msg("提交贷款申请")}</strong><p>{msg("固定总利息按期等额分期，所有申请自动进入全站唯一贷款池。")}</p></div><Button onClick={() => { setMessage(""); setApplicationOpen(true); }} disabled={context.isPending || pools.length === 0 || characterOptions.length === 0}>{msg("开始申请")}</Button></div>{contributionMessage && !contributionOpen && <p className="loan-form-message" role="status">{contributionMessage}</p>}{message && !applicationOpen && <p className="loan-form-message" role="alert">{message}</p>}{contributionOpen && <FormDialog title={msg("提交出借额度")} close={() => { if (!contributionBusy) setContributionOpen(false); }} busy={contributionBusy} submitLabel={msg("提交出借额度")} disabled={pools.length === 0 || characterOptions.length === 0} className="loan-form loan-contribution-form" onSubmit={contribute}><label>{msg("出借类型")}<select autoFocus value={contributionKind} onChange={(e) => setContributionKind(e.target.value as "personal" | "corporation")}><option value="personal">{msg("个人")}</option><option value="corporation">{msg("军团")}</option></select></label><AccountCharacterSelect label={msg("出借角色")} characters={characterOptions} value={contributionCharacter || characterOptions[0]?.id || ""} onChange={setContributionCharacter} />{contributionKind === "corporation" && <label>{msg("出借军团 ID")}<input value={contributionCorporation} onChange={(e) => setContributionCorporation(e.target.value)} inputMode="numeric" required /></label>}<label>{msg("出借额度（ISK）")}<input value={contributionAmount} onChange={(e) => setContributionAmount(e.target.value)} inputMode="decimal" required /></label>{contributionMessage && <p className="loan-form-message" role="alert">{contributionMessage}</p>}</FormDialog>}{applicationOpen && <FormDialog title={msg("提交贷款申请")} close={() => { if (!applicationBusy) setApplicationOpen(false); }} busy={applicationBusy} submitLabel={msg("提交申请")} disabled={pools.length === 0 || characterOptions.length === 0} className="loan-form loan-application-form" onSubmit={submit}><p className="loan-dialog-note">{msg("申请会自动进入全站唯一贷款池，提交后等待评分、额度、担保或抵押审批。")}</p><AccountCharacterSelect label={msg("收款角色")} characters={characterOptions} value={character || characterOptions[0]?.id || ""} onChange={setCharacter} autoFocus /><label>{msg("本金（ISK）")}<input value={principal} onChange={(e) => setPrincipal(e.target.value)} inputMode="decimal" required /></label><label>{msg("总利息（ISK）")}<input value={interest} onChange={(e) => setInterest(e.target.value)} inputMode="decimal" required /></label><label>{msg("期数")}<input value={count} onChange={(e) => setCount(e.target.value)} inputMode="numeric" required /></label><label>{msg("间隔天数")}<input value={interval} onChange={(e) => setInterval(e.target.value)} inputMode="numeric" required /></label><label>{msg("首期到期时间")}<input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} required /></label>{message && <p className="loan-form-message" role="alert">{message}</p>}</FormDialog>}</div>
    {(contributions.data?.items.length ?? 0) > 0 && <section className="loan-list"><div className="loan-section-head"><h2>{msg("出借记录")}</h2></div><div className="loan-deposit-list" role="list">{contributions.data?.items.map((item) => <ContributionRow key={item.id} csrf={csrf} item={item} pool={pools[0]} characters={characterOptions} onDone={() => void Promise.all([contributions.refetch(), context.refetch()])} />)}</div></section>}
    {(guarantees.data?.items.length ?? 0) > 0 && <section className="loan-list"><div className="loan-section-head"><div><h2>{msg("待我确认的担保")}</h2><p>{msg("接受担保会占用你的系统责任额度，贷款结清后自动释放。")}</p></div></div><div className="loan-contributions">{guarantees.data?.items.map((item) => <GuaranteeInvite key={item.id} csrf={csrf} item={item} onDone={() => void Promise.all([guarantees.refetch(), context.refetch()])} />)}</div></section>}
    <section className="loan-list"><div className="loan-section-head"><h2>{msg("我的贷款")}</h2></div>{list.isPending ? <p role="status">{msg("正在读取")}</p> : list.data?.items.length ? <div className="loan-table-wrap"><table><thead><tr><th>{msg("编号")}</th><th>{msg("贷款池")}</th><th>{msg("本金")}</th><th>{msg("总应还")}</th><th>{msg("状态")}</th><th>{msg("操作")}</th></tr></thead><tbody>{list.data.items.map((item) => <CaseRow key={item.id} csrf={csrf} userId={userId} item={item} administrator={Boolean(context.data?.administrator)} onDone={refresh} />)}</tbody></table></div> : <div className="loan-empty"><Landmark size={28} aria-hidden="true" /><p>{msg("暂无贷款记录")}</p></div>}</section></>}
    {activeView === "risk" && <PolicySettings csrf={csrf} pools={pools} custodianOptions={custodianOptions} context={context.data} onRefresh={() => void context.refetch()} />}
    {activeView === "evaluations" && <EvaluationHistory />}
  </div>;
}

function PolicySettings({ csrf, pools, custodianOptions, context, onRefresh }: { csrf: string; pools: api.Pool[]; custodianOptions: AccountCharacterOption[]; context?: api.Context; onRefresh: () => void }) {
  const policy = useQuery({ queryKey: ["loan", "policy"], queryFn: ({ signal }) => api.policy(signal), enabled: Boolean(context?.can_manage_pool), staleTime: 30_000 });
  const [draftState, setDraftState] = useState<api.CreditPolicy | null>(null);
  const [message, setMessage] = useState(""); const [busy, setBusy] = useState(false);
  const [custodian, setCustodian] = useState(""); const [custodianOpen, setCustodianOpen] = useState(false);
  const draft = draftState ?? policy.data?.policy;
  const currentCustodian = custodian || pools[0]?.custodian_character_id || custodianOptions[0]?.id || "";
  if (!context?.can_manage_pool) return <section className="loan-list"><div className="loan-empty"><ShieldCheck size={28} aria-hidden="true" /><p>{msg("当前账号没有贷款风控配置权限")}</p></div></section>;
  const update = (key: keyof api.CreditPolicy, value: number | string) => setDraftState((current) => { const base = current ?? policy.data?.policy; return base ? { ...base, [key]: value } : current; });
  const save = async (event: FormEvent) => { event.preventDefault(); if (!draft) return; setBusy(true); setMessage(""); try { await api.updatePolicy(csrf, draft as unknown as Record<string, unknown>); setDraftState(null); setMessage(msg("风控设置已保存")); await policy.refetch(); } catch (e) { setMessage(e instanceof Error ? e.message : msg("风控设置保存失败")); } finally { setBusy(false); } };
  const saveCustodian = async (event: FormEvent) => { event.preventDefault(); const pool = pools[0]; if (!pool || !currentCustodian) return; setBusy(true); setMessage(""); try { await api.updateCustodian(csrf, { pool_id: Number(pool.id), character_id: Number(currentCustodian), version: pool.version }); setCustodian(""); setCustodianOpen(false); onRefresh(); setMessage(msg("贷款专员已更新")); } catch (e) { setMessage(e instanceof Error ? e.message : msg("贷款专员设置失败")); } finally { setBusy(false); } };
  return <div className="loan-settings-grid"><section className="loan-list"><div className="loan-section-head"><div><h2>{msg("授信规则")}</h2><p>{msg("收入证据用于计算个人基础授信，最终放款仍受统一本金池可用余额限制。")}</p></div><span className="loan-state">v{draft?.version ?? "—"}</span></div>{policy.isPending || !draft ? <p role="status">{msg("正在读取")}</p> : <form className="loan-policy-form" onSubmit={save}><label>{msg("收入观察窗口（天）")}<input type="number" min="30" max="730" value={draft.income_window_days} onChange={(e) => update("income_window_days", Number(e.target.value))} /></label><label>{msg("收入负债率（bps）")}<input type="number" min="0" max="10000" value={draft.debt_ratio_bps} onChange={(e) => update("debt_ratio_bps", Number(e.target.value))} /></label><label>{msg("授信期限（月）")}<input type="number" min="1" max="60" value={draft.term_months} onChange={(e) => update("term_months", Number(e.target.value))} /></label><label>{msg("评分系数（bps）")}<input type="number" min="0" max="20000" value={draft.score_factor_bps} onChange={(e) => update("score_factor_bps", Number(e.target.value))} /></label><label>{msg("个人授信硬上限（ISK）")}<input type="number" min="0" value={draft.hard_cap_minor / 100} onChange={(e) => update("hard_cap_minor", Math.round(Number(e.target.value) * 100))} /></label><label>{msg("最低收入样本数")}<input type="number" min="0" max="12" value={draft.min_income_samples} onChange={(e) => update("min_income_samples", Number(e.target.value))} /></label><label>{msg("收入数据最长有效时间（小时）")}<input type="number" min="1" max="8760" value={draft.stale_after_hours} onChange={(e) => update("stale_after_hours", Number(e.target.value))} /></label><label>{msg("担保覆盖上限（bps）")}<input type="number" min="0" max="10000" value={draft.guarantee_cap_bps} onChange={(e) => update("guarantee_cap_bps", Number(e.target.value))} /></label><label>{msg("抵押覆盖上限（bps）")}<input type="number" min="0" max="10000" value={draft.collateral_cap_bps} onChange={(e) => update("collateral_cap_bps", Number(e.target.value))} /></label><label>{msg("收入数据不足时") }<select value={draft.insufficient_income_policy} onChange={(e) => update("insufficient_income_policy", e.target.value)}><option value="fallback_pool">{msg("回退到资金池评分")}</option><option value="zero">{msg("暂停个人授信")}</option></select></label><div className="loan-form-actions"><Button type="submit" disabled={busy}>{msg("保存规则")}</Button></div>{message && <p className="loan-form-message" role="status">{message}</p>}</form>}</section><section className="loan-list"><div className="loan-section-head"><div><h2>{msg("合同处理人")}</h2><p>{msg("放款与出借合同统一发送给所选个人 EVE 角色。")}</p></div><Button variant="outline" onClick={() => setCustodianOpen(true)}>{msg("设置贷款专员")}</Button></div><dl className="loan-policy-summary"><div><dt>{msg("当前角色")}</dt><dd>{pools[0]?.custodian_character_name || pools[0]?.custodian_character_id || msg("未配置")}</dd></div><div><dt>{msg("担保覆盖上限")}</dt><dd>{draft ? `${draft.guarantee_cap_bps / 100}%` : "—"}</dd></div><div><dt>{msg("抵押覆盖上限")}</dt><dd>{draft ? `${draft.collateral_cap_bps / 100}%` : "—"}</dd></div></dl></section>{custodianOpen && <FormDialog title={msg("设置贷款专员")} close={() => { if (!busy) setCustodianOpen(false); }} busy={busy} submitLabel={msg("保存设置")} onSubmit={saveCustodian} className="loan-form loan-settings-form"><AccountCharacterSelect label={msg("贷款专员角色")} characters={custodianOptions} value={currentCustodian} onChange={setCustodian} autoFocus /></FormDialog>}</div>;
}

function EvaluationHistory() {
  const query = useQuery({ queryKey: ["loan", "evaluations"], queryFn: ({ signal }) => api.evaluations(signal), staleTime: 30_000 });
  return <section className="loan-list"><div className="loan-section-head"><div><h2>{msg("信用评估记录")}</h2><p>{msg("每次贷款审批都会保存当时的评分、额度和规则版本。")}</p></div></div>{query.isPending ? <p role="status">{msg("正在读取")}</p> : query.isError ? <p className="loan-form-message" role="alert">{query.error.message}</p> : query.data?.items.length ? <div className="loan-table-wrap"><table className="loan-history-table"><thead><tr><th>{msg("时间")}</th><th>{msg("规则版本")}</th><th>{msg("评分")}</th><th>{msg("总额度")}</th><th>{msg("无担保额度")}</th><th>{msg("状态")}</th></tr></thead><tbody>{query.data.items.map((item) => <tr key={item.id}><td>{new Date(item.evaluated_at).toLocaleString(getLocale())}</td><td>{item.policy_version}</td><td>{item.score}/100</td><td>{money(item.total_limit_minor)}</td><td>{money(item.unsecured_limit_minor)}</td><td><span className="loan-state">{item.state}</span></td></tr>)}</tbody></table></div> : <div className="loan-empty"><p>{msg("暂无评估记录")}</p></div>}</section>;
}

function CopyField({ label, value }: { label: string; value: string }) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  return <div className="loan-contract-copy">
    <label>{label}<input readOnly value={value} onFocus={(event) => event.currentTarget.select()} /></label>
    <IconAction type="button" label={msg("复制 {0}", label)} disabled={!value} onClick={async () => {
      try { await navigator.clipboard.writeText(value); setStatus("copied"); }
      catch { setStatus("failed"); }
    }}>{status === "copied" ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}</IconAction>
    {status === "copied" && <small role="status">{msg("已复制")}</small>}
    {status === "failed" && <small role="alert">{msg("复制失败，请手动复制")}</small>}
  </div>;
}

function ContributionRow({ csrf, item, pool, characters, onDone }: { csrf: string; item: api.Contribution; pool?: api.Pool; characters: AccountCharacterOption[]; onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const run = async (action: () => Promise<unknown>) => {
    if (busy) return;
    setBusy(true); setMessage("");
    try { await action(); onDone(); setOpen(false); }
    catch (e) { setMessage(e instanceof Error ? e.message : msg("取消出借失败")); }
    finally { setBusy(false); }
  };
  const sourceName = item.source_character_name || characters.find((character) => character.id === item.source_character_id)?.name || item.source_character_id;
  const state = item.state === "funded" ? msg("已入池") : item.state === "pending" ? msg("待入金核验") : msg("已取消");
  const lender = item.lender_kind === "corporation" ? msg("军团出借") : msg("个人出借");
  const recipient = pool?.custodian_character_id;
  const recipientLabel = msg("贷款专员角色");
  const recipientName = pool?.custodian_character_name || recipient || "";
  return <div role="listitem">
    <button type="button" className="loan-deposit-row" onClick={() => { setMessage(""); setOpen(true); }} aria-haspopup="dialog">
      <span className="loan-deposit-identity"><strong>{sourceName}</strong><small>{lender}</small></span>
      <strong className="loan-deposit-amount">{money(item.amount_minor)}</strong>
      <span className="loan-state">{state}</span><ChevronRight size={18} aria-hidden="true" />
    </button>
    {open && <Modal title={msg("出借详情")} busy={busy} close={() => setOpen(false)} className="loan-deposit-dialog"
      footer={item.state === "pending" ? <div className="loan-form-actions">
        <Button variant="outline" disabled={busy} onClick={() => void run(() => api.cancelContribution(csrf, item.id, { version: item.version }))}>{msg("取消出借")}</Button>
      </div> : undefined}>
      <dl className="loan-deposit-summary">
        <div><dt>{msg("出借角色")}</dt><dd>{sourceName}</dd></div>
        <div><dt>{msg("出借类型")}</dt><dd>{lender}</dd></div>
        <div><dt>{msg("金额")}</dt><dd>{money(item.amount_minor)}</dd></div>
        <div><dt>{msg("状态")}</dt><dd>{state}</dd></div>
        <div><dt>{msg("创建时间")}</dt><dd>{new Date(item.created_at).toLocaleString(getLocale())}</dd></div>
        {item.contract_id && <div><dt>{msg("游戏合同 ID")}</dt><dd>{item.contract_id}</dd></div>}
      </dl>
      {item.state === "pending" && <>
        <section className="loan-contract-handoff" aria-label={msg("合同信息")}>
          <h3>{msg("合同信息")}</h3>
          <p>{msg("物品交换合同，无物品；合同完成并同步后由系统自动核验入金。")}</p>
          <div className="loan-contract-copy-grid">
            <CopyField label={recipientLabel} value={recipientName} />
            <CopyField label={msg("支付金额 / ISK")} value={String(item.amount_minor / 100)} />
            <CopyField label={msg("合同备注")} value={"GNV-LOAN-DEPOSIT-" + item.id} />
          </div>
        </section>
      </>}
      {message && <p className="loan-form-message" role="alert">{message}</p>}
    </Modal>}
  </div>;
}

function GuaranteeInvite({ csrf, item, onDone }: { csrf: string; item: api.Guarantee; onDone: () => void }) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const decide = async (state: "accepted" | "rejected") => {
    setBusy(true); setMessage("");
    try { await api.decideGuarantee(csrf, item.id, { version: item.version, state }); onDone(); }
    catch (e) { setMessage(e instanceof Error ? e.message : msg("担保决定失败")); }
    finally { setBusy(false); }
  };
  return <article className="loan-contribution"><div><strong>{money(item.amount_minor)}</strong><span>{item.case_public_id ? `${msg("贷款申请")} ${item.case_public_id}` : msg("贷款担保邀请")} · {item.state}</span></div>{item.state === "invited" && <div className="loan-form-actions"><Button disabled={busy} onClick={() => void decide("accepted")}>{msg("接受担保")}</Button><Button disabled={busy} variant="outline" onClick={() => void decide("rejected")}>{msg("拒绝")}</Button></div>}{message && <p className="loan-form-message" role="alert">{message}</p>}</article>;
}

function CaseRow({ csrf, userId, item, administrator, onDone }: { csrf: string; userId: string; item: api.Case; administrator: boolean; onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<"guarantee" | "collateral" | null>(null);
  const [guarantorCharacter, setGuarantorCharacter] = useState("");
  const [guaranteeAmount, setGuaranteeAmount] = useState("");
  const [contractKind, setContractKind] = useState<"character" | "corporation">("character");
  const [contractOwner, setContractOwner] = useState("");
  const [contractId, setContractId] = useState("");
  const [valuation, setValuation] = useState("");
  const [haircut, setHaircut] = useState("7000");
  const [covered, setCovered] = useState("");
  const [busy, setBusy] = useState(false); const [message, setMessage] = useState("");
  const detail = useQuery({ queryKey: ["loan", "case", item.id], queryFn: ({ signal }) => api.detail(item.id, signal), enabled: open, staleTime: 10_000 });
  const borrower = userId === item.borrower_account_id;
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setMessage("");
    try {
      if (mode === "guarantee") await api.createGuarantee(csrf, item.id, { guarantor_character_id: Number(guarantorCharacter), amount_minor: Math.round(Number(guaranteeAmount) * 100) });
      if (mode === "collateral") await api.createCollateral(csrf, item.id, { owner_account_id: userId, contract_kind: contractKind, contract_owner_id: Number(contractOwner), contract_id: Number(contractId), valuation_minor: Math.round(Number(valuation) * 100), haircut_bps: Number(haircut), covered_minor: Math.round(Number(covered) * 100) });
      setMode(null); setOpen(true); onDone(); await detail.refetch();
    } catch (e) { setMessage(e instanceof Error ? e.message : msg("贷款保障资料提交失败")); }
    finally { setBusy(false); }
  };
  const decideCollateral = async (id: string, version: number, state: "approved" | "disputed") => { setBusy(true); setMessage(""); try { await api.decideCollateral(csrf, id, { version, state }); await detail.refetch(); onDone(); } catch (e) { setMessage(e instanceof Error ? e.message : msg("抵押审批失败")); } finally { setBusy(false); } };
  return <><tr><td>{item.public_id}</td><td>{item.pool_name}</td><td>{money(item.principal_minor)}</td><td>{money(item.total_due_minor)}</td><td><span className={`loan-state loan-state-${item.state}`}>{loanState(item.state)}</span></td><td><Button variant="outline" onClick={() => { setMessage(""); setOpen(true); }}>{msg("查看保障")}</Button></td></tr>
    {open && <FormDialog title={`${msg("贷款详情")} · ${item.public_id}`} close={() => { if (!busy) setOpen(false); }} busy={busy} showSubmit={false} onSubmit={() => undefined} className="loan-detail-dialog"><p>{msg("担保由担保人确认，抵押由管理者审批；合同物品由后台读取。")}</p>{detail.isPending && <p role="status">{msg("正在读取")}</p>}{detail.data && <><h3>{msg("担保")}</h3>{detail.data.guarantees.length ? detail.data.guarantees.map((g) => <div className="loan-security-row" key={g.id}><span>{money(g.amount_minor)} · {g.state}</span></div>) : <p>{msg("暂无担保记录")}</p>}<h3>{msg("抵押")}</h3>{detail.data.collateral.length ? detail.data.collateral.map((c) => <div className="loan-security-row" key={c.id}><span>{money(c.covered_minor)} · {c.state}</span>{administrator && c.state === "proposed" && <span><Button disabled={busy} onClick={() => void decideCollateral(c.id, c.version, "approved")}>{msg("批准抵押")}</Button><Button variant="outline" disabled={busy} onClick={() => void decideCollateral(c.id, c.version, "disputed")}>{msg("提出异议")}</Button></span>}</div>) : <p>{msg("暂无抵押记录")}</p>}</>}{borrower && (item.state === "submitted" || item.state === "awaiting_acceptance") && <div className="loan-form-actions"><Button onClick={() => { setMessage(""); setOpen(false); setMode("guarantee"); }}>{msg("添加担保")}</Button><Button variant="outline" onClick={() => { setMessage(""); setOpen(false); setMode("collateral"); }}>{msg("登记抵押")}</Button></div>}{message && <p className="loan-form-message" role="alert">{message}</p>}</FormDialog>}
    {mode === "guarantee" && <FormDialog title={msg("添加担保邀请")} close={() => { if (!busy) setMode(null); }} busy={busy} submitLabel={msg("发送邀请")} className="loan-form loan-application-form" onSubmit={submit}><p className="loan-dialog-note">{msg("请输入担保人的已绑定角色 ID，后台会解析其本站账号。")}</p><label>{msg("担保角色 ID")}<input autoFocus value={guarantorCharacter} onChange={(e) => setGuarantorCharacter(e.target.value)} inputMode="numeric" required /></label><label>{msg("担保金额（ISK）")}<input value={guaranteeAmount} onChange={(e) => setGuaranteeAmount(e.target.value)} inputMode="decimal" required /></label>{message && <p className="loan-form-message" role="alert">{message}</p>}</FormDialog>}
    {mode === "collateral" && <FormDialog title={msg("登记抵押")} close={() => { if (!busy) setMode(null); }} busy={busy} submitLabel={msg("提交抵押")} className="loan-form loan-application-form" onSubmit={submit}><p className="loan-dialog-note">{msg("提交已完成的物品合同。物品清单由后台合同快照核验，不接受手工填写。")}</p><label>{msg("合同类型")}<select autoFocus value={contractKind} onChange={(e) => setContractKind(e.target.value as "character" | "corporation")}><option value="character">{msg("个人角色")}</option><option value="corporation">{msg("军团")}</option></select></label><label>{msg("合同所有者 ID")}<input value={contractOwner} onChange={(e) => setContractOwner(e.target.value)} inputMode="numeric" required /></label><label>{msg("合同 ID")}<input value={contractId} onChange={(e) => setContractId(e.target.value)} inputMode="numeric" required /></label><label>{msg("估值（ISK）")}<input value={valuation} onChange={(e) => setValuation(e.target.value)} inputMode="decimal" required /></label><label>{msg("折扣后覆盖比例（bps）")}<input value={haircut} onChange={(e) => setHaircut(e.target.value)} inputMode="numeric" required /></label><label>{msg("覆盖金额（ISK）")}<input value={covered} onChange={(e) => setCovered(e.target.value)} inputMode="decimal" required /></label>{message && <p className="loan-form-message" role="alert">{message}</p>}</FormDialog>}
  </>;
}
