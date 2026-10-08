import { Check, UsersRound } from "lucide-react";
import { getLocale, msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import type { AlliancePAPMembersReport } from "./alliance-pap-api";

export function AlliancePAPMembers({
  report,
  loading,
  error,
  months,
  month,
  onMonthChange,
  retry,
}: {
  report?: AlliancePAPMembersReport;
  loading: boolean;
  error: Error | null;
  months: string[];
  month: string;
  onMonthChange: (month: string) => void;
  retry: () => void;
}) {
  return (
    <section className="pap-alliance-members" aria-labelledby="pap-alliance-members-title">
      <div className="pap-alliance-details-heading">
        <strong id="pap-alliance-members-title">
          <UsersRound size={16} aria-hidden="true" /> {msg("成员联盟 PAP")}
        </strong>
        {report?.available && (
          <small>{msg("{0} 个成员", report.members.length)}</small>
        )}
      </div>
      {months.length > 0 && (
        <label>
          {msg("查看月份")}
          <select value={month} onChange={(event) => onMonthChange(event.target.value)}>
            {months.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
      )}
      {loading ? (
        <span className="pap-alliance-empty" role="status">{msg("正在读取")}</span>
      ) : error ? (
        <div className="pap-status pap-status-error" role="alert">
          <span>{error.message}</span>
          <Button variant="ghost" onClick={retry}>{msg("重试")}</Button>
        </div>
      ) : months.length === 0 ? (
        <span className="pap-alliance-empty">{msg("暂无可查看的联盟 PAP 月份")}</span>
      ) : !report?.available || report.members.length === 0 ? (
        <span className="pap-alliance-empty">{msg("暂无已绑定成员联盟 PAP")}</span>
      ) : (
        <div className="pap-alliance-member-list">
          {report.members.map((member) => (
            <div className="pap-alliance-member" key={member.user_id}>
              <div className="pap-alliance-member-name">
                <strong>{member.name || member.user_id}</strong>
                <small>{msg("{0} 个角色", member.characters.length)}</small>
              </div>
              <span className="pap-alliance-member-points">
                {member.points.toLocaleString(getLocale(), { maximumFractionDigits: 2 })} {msg("PAP")}
              </span>
              <span className={`pap-alliance-member-status${member.achieved ? " is-complete" : ""}`}>
                {member.achieved && <Check size={14} aria-hidden="true" />}
                {member.achieved ? msg("达标") : msg("未达标")}
              </span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
