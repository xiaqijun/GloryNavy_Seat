import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, RefreshCw, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import { getLocale, msg } from "@/lib/i18n";
import { getScopeOptions } from "@/modules/access/scope-api";
import * as api from "./qq-structure-api";

export function QQStructure({ user, csrf }: { user: string; csrf: string }) {
  const [saved, setSaved] = useState(false);
  const query = useQuery({ queryKey: ["community", "qq-structure", user], queryFn: ({ signal }) => api.settings(signal) });
  if (query.error) return <Card><CardContent className="reminder-config"><div className="reminder-title"><h2><Bot size={18} aria-hidden="true" />{msg("QQ群建筑提醒")}</h2></div><p role="alert">{query.error.message}</p><Button variant="outline" onClick={() => void query.refetch()}>{msg("重试")}</Button></CardContent></Card>;
  if (!query.data) return <p role="status">{msg("正在加载")}</p>;
  return <QQStructureEditor key={`${user}:${query.data.revision}`} initial={query.data} user={user} csrf={csrf} saved={saved} setSaved={setSaved} />;
}

function QQStructureEditor({ initial, user, csrf, saved, setSaved }: { initial: api.QQStructureSettings; user: string; csrf: string; saved: boolean; setSaved: (value: boolean) => void }) {
  const [enabled, setEnabled] = useState(initial.enabled);
  const [group, setGroup] = useState(initial.group_openid);
  const [corp, setCorp] = useState(initial.corporation_id === "0" ? "" : initial.corporation_id);
  const client = useQueryClient();
  const corporations = useQuery({ queryKey: ["access", "scope-options", user], queryFn: ({ signal }) => getScopeOptions(signal) });
  const records = useQuery({ queryKey: ["community", "qq-structure-deliveries", user], queryFn: ({ signal }) => api.deliveries(signal), refetchInterval: 30000 });
  const save = useMutation({ mutationFn: () => api.save({ enabled, group_openid: group.trim(), corporation_id: corp || "0", revision: initial.revision }, csrf), onSuccess: (next) => { setSaved(true); client.setQueryData(["community", "qq-structure", user], next); void client.invalidateQueries({ queryKey: ["community", "qq-structure-deliveries", user] }); } });
  const options = (corporations.data?.corporations ?? []).map(c => ({ value: c.id, label: c.name || msg("军团 #{0}", c.id) }));
  if (initial.corporation_id !== "0" && !options.some(c => c.value === initial.corporation_id)) options.unshift({ value: initial.corporation_id, label: initial.corporation_name || msg("军团 #{0}", initial.corporation_id) });
  const dirty = enabled !== initial.enabled || group.trim() !== initial.group_openid || (corp || "0") !== initial.corporation_id;
  const invalidGroup = /^\d+$/.test(group.trim());
  const states: Record<string, string> = { pending: "等待发送", sending: "发送中", sent: "已发送", rejected: "发送失败", unknown: "结果待确认", obsolete: "已失效" };
  return <Card><CardContent className="reminder-config">
    <div className="reminder-title"><h2><Bot size={18} aria-hidden="true" />{msg("QQ群建筑提醒")}</h2><span className="muted">{initial.enabled ? msg("已启用") : msg("已停用")}</span></div>
    <div className="qq-structure-fields">
      <div className="reminder-sender"><label>{msg("军团")}</label><Select label={msg("军团")} value={corp} options={options} disabled={save.isPending || corporations.isPending} placeholder={msg("选择军团")} onValueChange={value => { setSaved(false); setCorp(value); }} /></div>
      <div className="reminder-sender"><label htmlFor="qq-structure-group">{msg("群 OpenID")}</label><input id="qq-structure-group" value={group} maxLength={256} disabled={save.isPending} aria-invalid={invalidGroup} aria-describedby={invalidGroup ? "qq-structure-group-error" : undefined} onChange={event => { setSaved(false); setGroup(event.target.value); }} autoComplete="off" />{invalidGroup && <span id="qq-structure-group-error" role="alert">{msg("请填写官方群 OpenID，不能使用数字群号")}</span>}</div>
      <label className="reminder-toggle"><input type="checkbox" checked={enabled} disabled={save.isPending} onChange={event => { setSaved(false); setEnabled(event.target.checked); }} />{msg("启用推送")}</label>
      <Button disabled={save.isPending || !dirty || invalidGroup || (enabled && (!corp || !group.trim()))} onClick={() => save.mutate()}><Save aria-hidden="true" />{save.isPending ? msg("正在保存") : msg("保存")}</Button>
    </div>
    {!initial.bot_ready && <p role="status">{msg("请先启用官方 QQ 机器人")}</p>}
    {initial.bot_changed && <p role="status">{msg("机器人已更换，请重新填写群 OpenID")}</p>}
    {dirty && <p>{msg("更改配置将重新建立基线，未发送的旧提醒不会转发")}</p>}
    {saved && <p role="status">{msg("建筑提醒配置已保存")}</p>}
    {corporations.error && <p role="alert">{corporations.error.message}</p>}
    {save.error && <div role="alert"><p>{save.error.message}</p><Button variant="outline" onClick={() => void client.invalidateQueries({ queryKey: ["community", "qq-structure", user] })}>{msg("刷新配置")}</Button></div>}
    <div className="reminder-title qq-structure-record-heading"><h3>{msg("QQ群发送记录")}</h3><Button variant="outline" disabled={records.isFetching} onClick={() => void records.refetch()} aria-label={msg("刷新发送记录")}><RefreshCw aria-hidden="true" /></Button></div>
    {records.error ? <p role="alert">{records.error.message}</p> : records.isPending ? <p role="status">{msg("正在加载")}</p> : records.data.length === 0 ? <p className="muted">{msg("暂无发送记录")}</p> : <div className="reminder-table-scroll qq-structure-records"><table className="reminder-table"><thead><tr><th>{msg("时间")}</th><th>{msg("内容")}</th><th>{msg("状态")}</th></tr></thead><tbody>{records.data.map(item => <tr key={item.id}><td data-label={msg("时间")}><time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString(getLocale())}</time></td><td data-label={msg("内容")}>{item.body}</td><td data-label={msg("状态")}>{msg(states[item.status] ?? "结果待确认")}{item.error_code !== 0 && ` (${item.error_code})`}</td></tr>)}</tbody></table></div>}
  </CardContent></Card>;
}
