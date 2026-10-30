import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Check, ChevronRight, Copy, Landmark, RefreshCw, ShieldCheck } from "lucide-react";
import { Navigate, useSearchParams } from "react-router-dom";
import { initialRepayment, repaymentInput } from "./repayment-model";
import { RepaymentFields } from "./repayment";
import { PolicyField } from "./policy-field";
import { Button } from "@/components/ui/button";
import { FormDialog } from "@/components/ui/form-dialog";
import { ConfirmDialog, Modal } from "@/components/ui/dialog";
import { IconAction } from "@/components/ui/icon-action";
import { useToast } from "@/components/ui/toast-context";
import { AccountCharacterSelect, type AccountCharacterOption } from "@/components/account-character-select";
import { getLocale, msg } from "@/lib/i18n";
import { getCharacters, useSession } from "@/modules/identity";
import { getProfile } from "@/modules/community/api";
import { CollateralPicker } from "./collateral-picker";
import { CollateralDelivery } from "./collateral-delivery";
import { LoanCaseDetail, CollateralItems } from "./case-detail";
import * as api from "./api";
import { LoanCreditSummary, LoanDashboardPanel } from "./dashboard-page";
import { LoanOfficerSelect, selectedOfficer } from "./officer-select";
import { LoanAmount } from "./amount";
import { parseSecurityAmount } from "./security-format";
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
  const [principal, setPrincipal] = useState(""); const [character, setCharacter] = useState(""); const [repayment, setRepayment] = useState(initialRepayment); const [message, setMessage] = useState(""); const [applicationOpen, setApplicationOpen] = useState(false); const [applicationBusy, setApplicationBusy] = useState(false);
  const [contributionKind, setContributionKind] = useState<"personal" | "corporation">("personal"); const [contributionCharacter, setContributionCharacter] = useState(""); const [contributionCorporation, setContributionCorporation] = useState(""); const [contributionAmount, setContributionAmount] = useState(""); const [contributionMessage, setContributionMessage] = useState(""); const [contributionOpen, setContributionOpen] = useState(false); const [contributionBusy, setContributionBusy] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedView = searchParams.get("view");
  const activeView = requestedView === "business" || requestedView === "risk" || requestedView === "evaluations" || requestedView === "dashboard" ? requestedView : "dashboard";
  const setView = (view: "dashboard" | "business" | "risk" | "evaluations") => { const next = new URLSearchParams(searchParams); next.set("view", view); setSearchParams(next); };
  const context = useQuery({ queryKey: ["loan", "context", userId], queryFn: ({ signal }) => api.context(signal), staleTime: 30_000, refetchOnWindowFocus: false });
  const boundCharacters = useQuery({ queryKey: ["identity", "characters", userId], queryFn: ({ signal }) => getCharacters(signal), staleTime: 30_000, refetchOnWindowFocus: false });
  const profile = useQuery({ queryKey: ["community", "profile", userId], queryFn: ({ signal }) => getProfile(signal), staleTime: 60_000, refetchOnWindowFocus: false });
  const list = useQuery({ queryKey: ["loan", "cases", userId], queryFn: ({ signal }) => api.cases(signal), staleTime: 15_000, refetchOnWindowFocus: false });
  const contributions = useQuery({ queryKey: ["loan", "contributions", userId], queryFn: ({ signal }) => api.contributions(signal), staleTime: 15_000, refetchOnWindowFocus: false });
  const guarantees = useQuery({ queryKey: ["loan", "guarantees", userId], queryFn: ({ signal }) => api.guarantees(signal), staleTime: 15_000, refetchOnWindowFocus: false });
  const guarantorCandidates = useQuery({ queryKey: ["loan", "guarantee-candidates", userId], queryFn: ({ signal }) => api.guaranteeCandidates(signal), staleTime: 60_000, refetchOnWindowFocus: false });
  const refresh = () => void Promise.all([context.refetch(), boundCharacters.refetch(), profile.refetch(), list.refetch(), contributions.refetch(), guarantees.refetch(), guarantorCandidates.refetch()]);
  const submit = async (event: FormEvent) => { event.preventDefault(); setMessage(""); const sharedPool = pools[0]; const borrowerCharacterID = character || characterOptions[0]?.id; if (!sharedPool) { setMessage(msg("统一贷款池暂不可用")); return; } if (!borrowerCharacterID) { setMessage(msg("当前账号没有可用的绑定角色")); return; } setApplicationBusy(true); try { await api.createApplication(csrf, { pool_id: Number(sharedPool.id), borrower_character_id: Number(borrowerCharacterID), principal_minor: Math.round(Number(principal) * 100), ...repaymentInput(repayment) }); setMessage(""); setApplicationOpen(false); await Promise.all([context.refetch(), list.refetch()]); } catch (e) { setMessage(e instanceof Error ? e.message : msg("贷款申请失败")); } finally { setApplicationBusy(false); } };
  const contribute = async (event: FormEvent) => { event.preventDefault(); setContributionMessage(""); const sourceCharacterID = contributionCharacter || characterOptions[0]?.id; if (!sourceCharacterID) { setContributionMessage(msg("当前账号没有可用的绑定角色")); return; } setContributionBusy(true); try { await api.createContribution(csrf, { lender_kind: contributionKind, source_character_id: Number(sourceCharacterID), corporation_id: contributionKind === "corporation" ? Number(contributionCorporation) : 0, amount_minor: Math.round(Number(contributionAmount) * 100) }); setContributionMessage(msg("出借额度已提交，请完成入金合同")); setContributionAmount(""); setContributionOpen(false); await Promise.all([contributions.refetch(), context.refetch()]); } catch (e) { setContributionMessage(e instanceof Error ? e.message : msg("出借额度提交失败")); } finally { setContributionBusy(false); } };
  if (context.isError || boundCharacters.isError || list.isError || contributions.isError || guarantees.isError) return <div className="loan-feedback" role="alert"><p>{(context.error ?? boundCharacters.error ?? list.error ?? contributions.error ?? guarantees.error)?.message}</p><Button variant="outline" onClick={refresh}>{msg("重试")}</Button></div>;
  const pools = context.data?.pools.filter((p) => p.state === "open") ?? []; const nickname = profile.data?.kook.value?.trim() || undefined; const characterOptions: AccountCharacterOption[] = (boundCharacters.data?.characters.filter((character) => character.status === "active") ?? []).map((character) => ({ ...character, account_id: userId, nickname })); const officers = context.data?.loan_officers ?? [];
  return <div className="loan-page" data-view={activeView}><header className="loan-heading"><div className="loan-title"><span className="loan-mark"><Landmark aria-hidden="true" /></span><div><h1>{msg("贷款")}</h1></div></div><div className="loan-heading-actions"><IconAction label={msg("刷新")} disabled={context.isFetching || list.isFetching || contributions.isFetching} onClick={refresh}><RefreshCw size={18} aria-hidden="true" /></IconAction></div></header>
    <nav className="loan-tabs" role="tablist" aria-label={msg("贷款页面")}>{([['dashboard', msg('贷款看板')], ['business', msg('贷款办理')], ['risk', msg('风控设置')], ['evaluations', msg('评估记录')]] as const).map(([view, label]) => ((view !== 'risk' || context.data?.can_manage_pool) && <button key={view} type="button" role="tab" className={activeView === view ? 'is-active' : ''} aria-selected={activeView === view} onClick={() => setView(view)}>{label}</button>))}</nav>
    {activeView === "dashboard" && (context.data ? <LoanDashboardPanel context={context.data} /> : <p role="status">{msg("正在读取")}</p>)}
    {activeView === "business" && <div className="loan-business-layout">
    <section className="loan-business-toolbar" aria-label={msg("贷款办理")}>{context.data ? <LoanCreditSummary credit={context.data.credit} /> : <p role="status">{msg("正在读取")}</p>}<div className="loan-business-actions"><Button variant="outline" onClick={() => { setContributionMessage(""); setContributionOpen(true); }} disabled={pools.length === 0 || characterOptions.length === 0}>{msg("出借资金")}</Button><Button onClick={() => { setMessage(""); setApplicationOpen(true); }} disabled={context.isPending || pools.length === 0 || characterOptions.length === 0}>{msg("申请借款")}</Button></div>{contributionMessage && !contributionOpen && <p className="loan-form-message" role="status">{contributionMessage}</p>}{message && !applicationOpen && <p className="loan-form-message" role="alert">{message}</p>}{contributionOpen && <FormDialog size="form" title={msg("出借资金")} close={() => { if (!contributionBusy) setContributionOpen(false); }} busy={contributionBusy} submitLabel={msg("提交出借")} disabled={pools.length === 0 || characterOptions.length === 0} className="loan-form loan-contribution-form" onSubmit={contribute}><AccountCharacterSelect label={msg("出借角色")} characters={characterOptions} value={contributionCharacter || characterOptions[0]?.id || ""} onChange={setContributionCharacter} autoFocus /><label>{msg("出借额度（ISK）")}<input value={contributionAmount} onChange={(e) => setContributionAmount(e.target.value)} inputMode="decimal" required /></label><label>{msg("出借类型")}<select aria-label={msg("出借类型")} value={contributionKind} onChange={(e) => setContributionKind(e.target.value as "personal" | "corporation")}><option value="personal">{msg("个人")}</option><option value="corporation">{msg("军团")}</option></select></label>{contributionKind === "corporation" && <label>{msg("出借军团 ID")}<input value={contributionCorporation} onChange={(e) => setContributionCorporation(e.target.value)} inputMode="numeric" required /></label>}{contributionMessage && <p className="loan-form-message" role="alert">{contributionMessage}</p>}</FormDialog>}{applicationOpen && <FormDialog size="form" title={msg("申请借款")} close={() => { if (!applicationBusy) setApplicationOpen(false); }} busy={applicationBusy} submitLabel={msg("提交申请")} disabled={pools.length === 0 || characterOptions.length === 0} className="loan-form loan-application-form" onSubmit={submit}><AccountCharacterSelect label={msg("收款角色")} characters={characterOptions} value={character || characterOptions[0]?.id || ""} onChange={setCharacter} autoFocus /><label>{msg("本金（ISK）")}<input value={principal} onChange={(e) => setPrincipal(e.target.value)} inputMode="decimal" required /></label><div className="loan-repayment-fields"><RepaymentFields value={repayment} onChange={setRepayment} /></div>{message && <p className="loan-form-message" role="alert">{message}</p>}</FormDialog>}</section>
    <section className="loan-list loan-business-loans"><div className="loan-section-head"><h2>{msg("我的贷款")}</h2></div>{list.isPending ? <p role="status">{msg("正在读取")}</p> : list.data?.items.length ? <div className="loan-table-wrap"><table className="loan-cases-table"><thead><tr><th>{msg("编号")}</th><th>{msg("状态")}</th><th>{msg("本金")}</th><th>{msg("总应还")}</th><th>{msg("操作")}</th></tr></thead><tbody>{list.data.items.map((item) => <CaseRow key={item.id} officers={officers} csrf={csrf} userId={userId} item={item} administrator={Boolean(context.data?.administrator)} guarantors={guarantorCandidates.data?.items ?? []} characters={characterOptions} pool={context.data?.pools.find(p => p.id === item.pool_id)} defaultHaircutBPS={Number(pools[0]?.config?.collateral_haircut_bps) || 7000} onDone={refresh} />)}</tbody></table></div> : <p className="loan-empty-inline">{msg("暂无贷款记录")}</p>}</section>
    <div className="loan-business-secondary">
    <section id="loan-guarantees" className="loan-list loan-business-guarantees"><div className="loan-section-head"><h2>{msg("我的担保邀请")}</h2></div>{guarantees.isPending ? <p role="status">{msg("正在读取")}</p> : guarantees.data?.items.length ? <div className="loan-contributions">{guarantees.data.items.map((item) => <GuaranteeInvite key={item.id} csrf={csrf} item={item} onDone={() => void Promise.all([guarantees.refetch(), context.refetch()])} />)}</div> : <p className="loan-empty-inline">{msg("暂无担保邀请")}</p>}</section>
    {(contributions.data?.items.length ?? 0) > 0 && <section className="loan-list loan-business-contributions"><div className="loan-section-head"><h2>{msg("出借记录")}</h2></div><div className="loan-deposit-list" role="list">{contributions.data?.items.map((item) => <ContributionRow key={item.id} csrf={csrf} item={item} officers={officers} characters={characterOptions} onDone={() => void Promise.all([contributions.refetch(), context.refetch()])} />)}</div></section>}
    </div></div>}
    {activeView === "risk" && <PolicySettings csrf={csrf} pools={pools} context={context.data} onRefresh={() => void context.refetch()} />}
    {activeView === "evaluations" && <EvaluationHistory />}
  </div>;
}

function PolicySettings({ csrf, pools, context, onRefresh }: { csrf: string; pools: api.Pool[]; context?: api.Context; onRefresh: () => void }) {
  const toast = useToast();
  const policy = useQuery({ queryKey: ["loan", "policy"], queryFn: ({ signal }) => api.policy(signal), enabled: Boolean(context?.can_manage_pool), staleTime: 30_000 });
  const [draftState, setDraftState] = useState<api.CreditPolicy | null>(null);
  const [message, setMessage] = useState(""); const [busy, setBusy] = useState(false);
  const [adminFunding, setAdminFunding] = useState(""); const [adminFundingOpen, setAdminFundingOpen] = useState(false);
  const draft = draftState ?? policy.data?.policy;
  if (!context?.can_manage_pool) return <section className="loan-list"><div className="loan-empty"><ShieldCheck size={28} aria-hidden="true" /><p>{msg("当前账号没有贷款风控配置权限")}</p></div></section>;
  const update = (key: keyof api.CreditPolicy, value: number | string) => setDraftState((current) => { const base = current ?? policy.data?.policy; return base ? { ...base, [key]: value } : current; });
  const save = async (event: FormEvent) => { event.preventDefault(); if (!draft) return; setBusy(true); setMessage(""); try { await api.updatePolicy(csrf, draft as unknown as Record<string, unknown>); setDraftState(null); toast.success(msg("风控设置已保存")); await policy.refetch(); } catch (e) { setMessage(e instanceof Error ? e.message : msg("风控设置保存失败")); } finally { setBusy(false); } };
  const saveAdminFunding = async (event: FormEvent) => { event.preventDefault(); const pool = pools[0]; const amount = Math.round(Number(adminFunding) * 100); if (!pool || !Number.isSafeInteger(amount) || amount <= 0) return; setBusy(true); setMessage(""); try { await api.addAdminFunding(csrf, { pool_id: Number(pool.id), amount_minor: amount }); setAdminFunding(""); setAdminFundingOpen(false); onRefresh(); toast.success(msg("管理员入池资金已登记")); } catch (e) { setMessage(e instanceof Error ? e.message : msg("管理员入池资金登记失败")); } finally { setBusy(false); } };
  return <div className="loan-settings-grid"><section className="loan-list"><div className="loan-section-head"><div><h2>{msg("授信规则")}</h2><p>{msg("收入和个人资产合成个人额度，借款超出剩余额度的部分需担保或抵押。")}</p></div><span className="loan-state">v{draft?.version ?? "—"}</span></div>{policy.isPending || !draft ? <p role="status">{msg("正在读取")}</p> : <form className="loan-policy-form" onSubmit={save}><PolicyField label={msg("钱包观察窗口（天）")} help={msg("汇总已同步的钱包流水，排除同一账号角色间转账；收入按每 30 天折算，消费不提高额度。")}><input type="number" min="30" max="730" value={draft.income_window_days} onChange={(e) => update("income_window_days", Number(e.target.value))} /></PolicyField><PolicyField label={msg("年利率（bps）")} help={msg("按年名义利率计算固定单利；100 bps = 1%。申请提交时按年利率 × 期数 × 间隔天数 ÷ 365 计算利息，并将利率与利息一起冻结到贷款条款。")}><input type="number" min="0" max="10000" value={draft.annual_rate_bps} onChange={(e) => update("annual_rate_bps", Number(e.target.value))} /></PolicyField><PolicyField label={msg("收入负债率（bps）")} help={msg("收入按此比例和授信期限测算。100 bps = 1%，3000 bps = 30%。")}><input type="number" min="0" max="10000" value={draft.debt_ratio_bps} onChange={(e) => update("debt_ratio_bps", Number(e.target.value))} /></PolicyField><PolicyField label={msg("个人资产授信比例（bps）")} help={msg("个人资产总值 × 此比例计入个人额度，默认 1000 bps（10%）；与收入测算合成一个额度。") }><input type="number" min="0" max="10000" value={draft.asset_credit_ltv_bps} onChange={(e) => update("asset_credit_ltv_bps", Number(e.target.value))} /></PolicyField><PolicyField label={msg("授信期限（月）")} help={msg("按月收入、收入负债率和此期限测算基础额度。审批另按实际还款期限提示风险。") }><input type="number" min="1" max="60" value={draft.term_months} onChange={(e) => update("term_months", Number(e.target.value))} /></PolicyField><PolicyField label={msg("评分系数（bps）")} help={msg("额度先乘以信用分 ÷ 100，再乘此系数。10000 bps = 100%（不额外调整），5000 bps = 50%（再减半），0 表示额度为 0；不改变信用分本身。") }><input type="number" min="0" max="20000" value={draft.score_factor_bps} onChange={(e) => update("score_factor_bps", Number(e.target.value))} /></PolicyField><PolicyField label={msg("最低钱包样本数")} help={msg("每个成功同步钱包流水的角色最多计 1 个样本；最低为 0 时仍需有效收入。") }><input type="number" min="0" max="12" value={draft.min_income_samples} onChange={(e) => update("min_income_samples", Number(e.target.value))} /></PolicyField><PolicyField label={msg("钱包数据最长有效时间（小时）")} help={msg("钱包流水、资产和活动证据分别检查有效时间；超过设定小时数不计入评估。72 小时等于 3 天。") }><input type="number" min="1" max="8760" value={draft.stale_after_hours} onChange={(e) => update("stale_after_hours", Number(e.target.value))} /></PolicyField><PolicyField label={msg("担保覆盖上限（bps）")} help={msg("限制每份新担保邀请的金额：贷款本金 × 此比例。10000 bps = 100%，5000 bps = 50%。担保人接受时还需通过责任额度校验；此项不是多份担保合计上限。") }><input type="number" min="0" max="10000" value={draft.guarantee_cap_bps} onChange={(e) => update("guarantee_cap_bps", Number(e.target.value))} /></PolicyField><PolicyField label={msg("单人借款上限（bps）")} help={msg("0 表示不额外限制；10000 bps 等于资金池总额的 100%。")}><input type="number" min="0" max="10000" value={draft.borrower_limit_bps ?? 0} onChange={(e) => update("borrower_limit_bps", Number(e.target.value))} /></PolicyField><PolicyField label={msg("钱包证据不足时")} help={msg("收入不足时，按所选规则回退资金池评分或将收入部分归零；有效个人资产仍按配置计入。") }><select value={draft.insufficient_income_policy} onChange={(e) => update("insufficient_income_policy", e.target.value)}><option value="fallback_pool">{msg("回退到资金池评分")}</option><option value="zero">{msg("收入部分归零")}</option></select></PolicyField><div className="loan-form-actions"><Button type="submit" disabled={busy}>{msg("保存规则")}</Button></div>{message && <p className="loan-form-message" role="status">{message}</p>}</form>}</section>{context.administrator && <section className="loan-list"><Button onClick={() => { setMessage(""); setAdminFundingOpen(true); }}>{msg("管理员添加资金")}</Button></section>}{adminFundingOpen && <FormDialog title={msg("管理员添加资金")} close={() => { if (!busy) setAdminFundingOpen(false); }} busy={busy} submitLabel={msg("登记入池资金")} onSubmit={saveAdminFunding} className="loan-form loan-settings-form"><p className="loan-dialog-note">{msg("这笔资金直接写入贷款池账本，不创建或核验游戏合同。")}</p><label>{msg("入池金额（ISK）")}<input value={adminFunding} onChange={(e) => setAdminFunding(e.target.value)} inputMode="decimal" min="1" required autoFocus /></label>{message && <p className="loan-form-message" role="alert">{message}</p>}</FormDialog>}</div>;
}

function EvaluationHistory() {
  const query = useQuery({ queryKey: ["loan", "evaluations"], queryFn: ({ signal }) => api.evaluations(signal), staleTime: 30_000 });
  return <section className="loan-list"><div className="loan-section-head"><div><h2>{msg("信用评估记录")}</h2><p>{msg("借款超出剩余个人额度的部分，需要担保或抵押。评估记录保留当时的评分、额度和规则版本。")}</p></div></div>{query.isPending ? <p role="status">{msg("正在读取")}</p> : query.isError ? <p className="loan-form-message" role="alert">{query.error.message}</p> : query.data?.items.length ? <div className="loan-table-wrap"><table className="loan-history-table"><thead><tr><th>{msg("时间")}</th><th>{msg("规则版本")}</th><th>{msg("评分")}</th><th>{msg("个人额度")}</th><th>{msg("状态")}</th></tr></thead><tbody>{query.data.items.map((item) => <tr key={item.id}><td>{new Date(item.evaluated_at).toLocaleString(getLocale())}</td><td>{item.policy_version}</td><td>{item.score}/100</td><td><LoanAmount minor={item.total_limit_minor} /></td><td><span className="loan-state">{item.state}</span></td></tr>)}</tbody></table></div> : <div className="loan-empty"><p>{msg("暂无评估记录")}</p></div>}</section>;
}

function CopyField({ label, value }: { label: string; value: string }) {
  const toast = useToast();
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  return <div className="loan-contract-copy">
    <label>{label}<input readOnly value={value} onFocus={(event) => event.currentTarget.select()} /></label>
    <IconAction type="button" label={msg("复制 {0}", label)} disabled={!value} onClick={async () => {
      try { await navigator.clipboard.writeText(value); setStatus("copied"); toast.success(msg("已复制")); }
      catch { setStatus("failed"); }
    }}>{status === "copied" ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}</IconAction>
    {status === "failed" && <small role="alert">{msg("复制失败，请手动复制")}</small>}
  </div>;
}

function ContributionRow({ csrf, item, officers, characters, onDone }: { csrf: string; item: api.Contribution; officers: api.Character[]; characters: AccountCharacterOption[]; onDone: () => void }) {
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
  const [officer, setOfficer] = useState("");
  const choices = officers.filter(c => item.lender_kind === "corporation" || c.id !== item.source_character_id);
  const recipientName = selectedOfficer(choices, officer)?.name ?? "";
  return <div role="listitem">
    <button type="button" className="loan-deposit-row" onClick={() => { setMessage(""); setOpen(true); }} aria-haspopup="dialog">
      <span className="loan-deposit-identity"><strong>{sourceName}</strong><small>{lender}</small></span>
      <strong className="loan-deposit-amount"><LoanAmount minor={item.amount_minor} /></strong>
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
          <p>{msg("创建无物品的物品交换合同，完成并同步后自动入账。")}</p>
          <div className="loan-contract-copy-grid">
            <LoanOfficerSelect officers={choices} value={officer} onChange={setOfficer} /><CopyField label={msg("接收角色")} value={recipientName} />
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
  return <article className="loan-contribution"><div><strong><LoanAmount minor={item.amount_minor} /></strong><span>{item.case_public_id ? `${msg("贷款申请")} ${item.case_public_id}` : msg("贷款担保邀请")} · {item.state}</span></div>{item.state === "invited" && <div className="loan-form-actions"><Button disabled={busy} onClick={() => void decide("accepted")}>{msg("接受担保")}</Button><Button disabled={busy} variant="outline" onClick={() => void decide("rejected")}>{msg("拒绝")}</Button></div>}{message && <p className="loan-form-message" role="alert">{message}</p>}</article>;
}

function CaseRow({ csrf, userId, item, administrator, guarantors, characters, officers, pool, defaultHaircutBPS, onDone }: { csrf: string; userId: string; item: api.Case; administrator: boolean; guarantors: api.Character[]; characters: AccountCharacterOption[]; officers: api.Character[]; pool?: api.Pool; defaultHaircutBPS: number; onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<"guarantee" | "collateral" | null>(null);
  const [delivery, setDelivery] = useState<api.CollateralRequest | null>(null);
  const [cancelRequest, setCancelRequest] = useState<api.CollateralRequest | null>(null);
  const [guarantorCharacter, setGuarantorCharacter] = useState("");
  const [guaranteeDraft, setGuaranteeDraft] = useState<string | null>(null);
  const [busy, setBusy] = useState(false); const [message, setMessage] = useState("");
  const detail = useQuery({ queryKey: ["loan", "case", item.id], queryFn: ({ signal }) => api.detail(item.id, signal), enabled: open || mode !== null, staleTime: 0 });
  const current = detail.data?.case ?? item;
  const security = detail.data?.security;
  const guaranteeAmount = security?.credit_ready && !security.legacy_pending ? security.unallocated_minor : 0;
  const allocation = guaranteeDraft === null ? guaranteeAmount : parseSecurityAmount(guaranteeDraft);
  const borrower = userId === current.borrower_account_id;
  const editable = borrower && ["submitted", "awaiting_acceptance"].includes(current.state);
  const [cancelOpen, setCancelOpen] = useState(false);
  const saved = () => { setMode(null); setDelivery(null); setMessage(""); setOpen(true); onDone(); void detail.refetch(); };
  const cancelApplication = async () => {
    if (busy) return;
    setBusy(true); setMessage("");
    try { await api.cancelApplication(csrf, item.id, current.version); setCancelOpen(false); setOpen(false); onDone(); await detail.refetch(); }
    catch (e) { setMessage(e instanceof Error ? e.message : msg("撤销申请失败")); onDone(); await detail.refetch(); }
    finally { setBusy(false); }
  };
  const requestCancel = () => { setMessage(""); setOpen(false); setCancelOpen(true); };
  const submit = async (event: FormEvent) => {
    event.preventDefault(); if (busy || !allocation || allocation > guaranteeAmount || detail.isFetching || detail.isError) return; setBusy(true); setMessage("");
    try { await api.createGuarantee(csrf, item.id, { guarantor_character_id: Number(guarantorCharacter), amount_minor: guaranteeAmount, allocation_minor: allocation }); saved(); }
    catch (e) { setMessage(e instanceof Error ? e.message : msg("贷款保障资料提交失败")); void detail.refetch(); }
    finally { setBusy(false); }
  };
  const decideCollateral = async (id: string, version: number, state: "approved" | "disputed") => { setBusy(true); setMessage(""); try { await api.decideCollateral(csrf, id, { version, state }); await detail.refetch(); onDone(); } catch (e) { setMessage(e instanceof Error ? e.message : msg("抵押审批失败")); } finally { setBusy(false); } };
  return <><tr><td data-label={msg("编号")}>{item.public_id}</td><td data-label={msg("状态")}><span className={`loan-state loan-state-${item.state}`}>{loanState(item.state)}</span></td><td data-label={msg("本金")}><LoanAmount minor={item.principal_minor} /></td><td data-label={msg("总应还")}><LoanAmount minor={item.total_due_minor} />{item.requested_due_at && !item.first_due_at && ` ${msg("（预计）")}`}</td><td className="loan-case-actions"><Button variant="outline" onClick={() => { setMessage(""); setOpen(true); }}>{msg("查看详情")}</Button>{borrower && item.state === "submitted" && <Button variant="outline" disabled={busy} onClick={requestCancel}>{msg("撤销申请")}</Button>}</td></tr>
    {cancelOpen && <ConfirmDialog title={msg("撤销贷款申请")} description={msg("撤销后将结束此申请，相关担保和抵押额度会释放。")} confirmLabel={msg("确认撤销申请")} busy={busy} error={message} onConfirm={() => void cancelApplication()} onClose={() => setCancelOpen(false)} />}
    {open && <Modal title={msg("贷款详情")} size="form" close={() => setOpen(false)} busy={busy} className="loan-detail-dialog" footer={<>
      {borrower && current.state === "submitted" && <Button variant="outline" disabled={busy} onClick={requestCancel}>{msg("撤销申请")}</Button>}
      {editable && detail.data && !detail.isError && guaranteeAmount > 0 && <><Button variant="outline" disabled={busy || guarantors.length === 0} onClick={() => { setMessage(""); setOpen(false); setGuaranteeDraft(null); setGuarantorCharacter(guarantors.find(g => g.id === guarantorCharacter)?.id || guarantors[0]?.id || ""); setMode("guarantee"); void detail.refetch(); }}>{msg("找人担保")}</Button><Button disabled={busy || characters.length === 0} onClick={() => { setMessage(""); setOpen(false); setMode("collateral"); }}>{msg("物品抵押")}</Button></>}
    </>}>
      {detail.isPending && <p role="status">{msg("正在读取")}</p>}
      {detail.isError ? <div role="alert"><p>{detail.error.message}</p><Button variant="outline" onClick={() => void detail.refetch()}>{msg("重试")}</Button></div> : detail.data && <LoanCaseDetail detail={detail.data} pool={pool} officers={officers} status={loanState(current.state)} collateralAction={c => administrator && !borrower && c.state === "proposed" ? <div className="loan-contract-actions"><Button disabled={busy} onClick={() => void decideCollateral(c.id, c.version, "approved")}>{msg("批准抵押")}</Button><Button variant="outline" disabled={busy} onClick={() => void decideCollateral(c.id, c.version, "disputed")}>{msg("提出异议")}</Button></div> : null} requests={detail.data.collateral_requests?.filter(r => r.state !== "delivered").map(r => <article className="loan-collateral-record" key={r.id}>
        <div className="loan-detail-identity"><strong>{r.public_id}</strong><span className="loan-state">{r.state === "awaiting_delivery" ? msg("待交付") : msg("已取消")}</span></div>
        <CollateralItems items={r.items} />{(r.coverage_minor ?? 0) > 0 && <dl className="loan-detail-facts"><div><dt>{msg("抵押额度")}</dt><dd>{money(r.coverage_minor!)}</dd></div><div><dt>{msg("保存时物品总估值")}</dt><dd>{money(r.valuation_minor ?? 0)}</dd></div></dl>}
        {r.state === "awaiting_delivery" && <><p className="loan-dialog-note">{msg("在游戏中创建个人物品交换合同，交付以下全部物品，收取与支付均为 0 ISK。")}</p><div className="loan-contract-copy-grid">
          <CopyField label={msg("交付角色")} value={characters.find(c => c.id === r.source_character_id)?.name || r.source_character_id} />
          <CopyField label={msg("接收角色")} value={officers.find(c => c.id === r.recipient_character_id)?.name || r.recipient_character_id} />
          <CopyField label={msg("合同备注")} value={r.public_id} />
        </div>{borrower && ["submitted", "awaiting_acceptance", "approved"].includes(current.state) && <div className="loan-contract-actions"><Button onClick={() => { setOpen(false); setDelivery(r); }}>{msg("关联已完成合同")}</Button><Button variant="outline" onClick={() => { setMessage(""); setOpen(false); setCancelRequest(r); }}>{msg("取消待交付")}</Button></div>}</>}
      </article>)} />}
      {message && <p className="loan-form-message" role="alert">{message}</p>}
    </Modal>}
    {mode === "guarantee" && <FormDialog title={msg("找人担保")} close={() => { setMode(null); setOpen(true); }} busy={busy} disabled={!guarantorCharacter || !allocation || allocation > guaranteeAmount || detail.isFetching || detail.isError} submitLabel={msg("发送邀请")} className="loan-form loan-application-form" onSubmit={submit}><p className="loan-dialog-note">{msg("默认填入剩余缺口，可调整本次担保金额。")}</p><AccountCharacterSelect label={msg("担保角色")} characters={guarantors} value={guarantorCharacter} onChange={setGuarantorCharacter} autoFocus /><label>{msg("担保金额（ISK）")}<input inputMode="decimal" value={guaranteeDraft ?? (guaranteeAmount > 0 ? String(guaranteeAmount / 100) : "")} onChange={e => setGuaranteeDraft(e.target.value)} /></label>{guaranteeDraft !== null && (!allocation || allocation > guaranteeAmount) && <p role="alert">{msg("请输入不超过剩余缺口的有效金额。")}</p>}{message && <p className="loan-form-message" role="alert">{message}</p>}</FormDialog>}
    {mode === "collateral" && <CollateralPicker officers={officers} csrf={csrf} item={current} security={security!} characters={characters} close={() => { setMode(null); setOpen(true); }} saved={saved} />}
    {delivery && <CollateralDelivery csrf={csrf} userId={userId} request={delivery} haircut={(delivery.coverage_minor ?? 0) > 0 ? 8000 : defaultHaircutBPS} close={() => { setDelivery(null); setOpen(true); }} saved={saved} />}
    {cancelRequest && <ConfirmDialog title={msg("取消待交付")} description={msg("取消后可重新选择物品。若游戏合同已交付，请先联系贷款专员处理退还。")} busy={busy} error={message} onClose={() => { setCancelRequest(null); setOpen(true); }} onConfirm={async () => {
      setBusy(true); setMessage("");
      try { await api.cancelCollateralRequest(csrf, cancelRequest.id, cancelRequest.version); setCancelRequest(null); saved(); }
      catch (e) { setMessage(e instanceof Error ? e.message : msg("取消抵押物失败")); }
      finally { setBusy(false); }
    }} />}
  </>;
}
