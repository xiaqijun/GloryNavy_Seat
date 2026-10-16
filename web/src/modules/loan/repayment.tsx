import { getLocale, msg } from "@/lib/i18n";

import { repaymentDateBound, type RepaymentChoice } from "./repayment-model";

export function RepaymentFields({ value, onChange }: { value: RepaymentChoice; onChange: (value: RepaymentChoice) => void }) {
  const update = (patch: Partial<RepaymentChoice>) => onChange({ ...value, ...patch });
  return <>
    <label>{msg("还款方式")}<select aria-label={msg("还款方式")} value={value.mode} onChange={e => update({ mode: e.target.value as RepaymentChoice["mode"] })}>
      <option value="term">{msg("按期限")}</option>
      <option value="date">{msg("指定日期")}</option>
      <option value="installments">{msg("分期还款")}</option>
    </select></label>
    {value.mode === "term" && <>
      <label>{msg("借款期限")}<select aria-label={msg("借款期限")} value={value.term} onChange={e => update({ term: e.target.value })}>
        <option value="30">{msg("1 个月")}</option>
        <option value="90">{msg("3 个月")}</option>
        <option value="180">{msg("半年")}</option>
        <option value="365">{msg("1 年")}</option>
        <option value="custom">{msg("自定义")}</option>
      </select></label>
      {value.term === "custom" && <label>{msg("借款天数（1–365）")}<input type="number" min="1" max="365" step="1" required value={value.customDays} onChange={e => update({ customDays: e.target.value })} /></label>}
      <p className="loan-dialog-note">{msg("{0} 天，放款后起算，到期一次还清。", value.term === "custom" ? value.customDays || "—" : value.term)}</p>
    </>}
    {value.mode === "date" && <>
      <label>{msg("还款日期")}<input type="date" min={repaymentDateBound(1)} max={repaymentDateBound(365)} required value={value.date} onChange={e => update({ date: e.target.value })} /></label>
      <p className="loan-dialog-note">{msg("到期一次还清，截止当日 23:59:59（北京时间）。")}</p>
      <details className="loan-terms-help"><summary>{msg("计息规则")}</summary><p>{msg("利率在申请时冻结，按实际放款日至还款日计息，至少 1 天；放款前为预计金额，最终金额不超过预计值。")}</p></details>
    </>}
    {value.mode === "installments" && <>
      <label>{msg("期数")}<input type="number" min="2" max="120" step="1" required value={value.count} onChange={e => update({ count: e.target.value })} /></label>
      <label>{msg("间隔天数")}<select aria-label={msg("间隔天数")} value={value.interval} onChange={e => update({ interval: e.target.value })}>{[1, 3, 7, 14, 30].map(days => <option key={days} value={days}>{msg("{0} 天", days)}</option>)}</select></label>
      <p className="loan-dialog-note">{msg("放款后按所选间隔还款。")}</p>
    </>}
  </>;
}

type Terms = { installment_count: number; interval_days: number; first_due_at?: string | null; requested_due_at?: string | null };
export function RepaymentSummary({ item }: { item: Terms }) {
  const due = item.requested_due_at || item.first_due_at;
  return <>
    <p>{msg("还款方式")}: {item.installment_count === 1 ? msg("到期一次还清") : msg("分 {0} 期，每 {1} 天还款", item.installment_count, item.interval_days)}</p>
    {due ? <p>{item.installment_count === 1 ? msg("到期还款时间") : msg("首期还款时间")}: {new Date(due).toLocaleString(getLocale(), item.requested_due_at ? { timeZone: "Asia/Shanghai" } : undefined)}{item.requested_due_at && ` ${msg("（北京时间）")}`}</p> : <p>{msg("放款核验后 {0} 天首次还款", item.interval_days)}</p>}
    {item.requested_due_at && !item.first_due_at && <p>{msg("当前应还金额为预计值，放款核验后按实际借款天数确定，不会超过申请时的预计金额。")}</p>}
  </>;
}
