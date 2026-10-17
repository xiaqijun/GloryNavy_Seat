import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, RefreshCw, Save, Send, UsersRound } from "lucide-react";
import { msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { getAccountAccess } from "@/modules/access/api";
import { getOneBotSettings, getOneBotStatus, saveOneBotSettings, testOneBotMention, type OneBotSettings as Settings } from "./onebot-api";

export function OneBotSettings({ userID, csrf }: { userID: string; csrf: string }) {
  const access = useQuery({ queryKey: ["access", "me", userID], queryFn: ({ signal }) => getAccountAccess(signal) });
  const settings = useQuery({ queryKey: ["community", "onebot", userID], queryFn: ({ signal }) => getOneBotSettings(signal), enabled: access.data?.administrator === true });
  if (access.isPending) return <p role="status">{msg("正在加载")}</p>;
  if (access.error) return <p role="alert" className="login-error">{access.error.message}</p>;
  if (!access.data?.administrator) return <p role="alert">{msg("仅管理员可访问")}</p>;
  if (settings.error) return <p role="alert" className="login-error">{settings.error.message}</p>;
  if (!settings.data) return <p role="status">{msg("正在加载")}</p>;
  return <OneBotEditor key={settings.data.revision} initial={settings.data} csrf={csrf} userID={userID} />;
}

function OneBotEditor({ initial, csrf, userID }: { initial: Settings; csrf: string; userID: string }) {
  const client = useQueryClient();
  const [enabled, setEnabled] = useState(initial.enabled);
  const [botQQ, setBotQQ] = useState(initial.bot_qq);
  const [apiBase, setAPIBase] = useState(initial.api_base);
  const [token, setToken] = useState("");
  const [secret, setSecret] = useState("");
  const [groups, setGroups] = useState(initial.groups);
  const connection = useMutation({ mutationFn: getOneBotStatus });
  const test = useMutation({ mutationFn: (group: string) => testOneBotMention(group, csrf), retry: false });
  const save = useMutation({
    mutationFn: () => saveOneBotSettings({ enabled, api_base: apiBase, bot_qq: botQQ, token, secret, revision: initial.revision, groups }, csrf),
    onSuccess: (next) => {
      setToken(""); setSecret("");
      client.setQueryData(["community", "onebot", userID], next);
      void client.invalidateQueries({ queryKey: ["community", "profile"] });
      void client.invalidateQueries({ queryKey: ["community", "qq-group-options"] });
    },
  });
  const dirty = enabled !== initial.enabled || apiBase !== initial.api_base || botQQ !== initial.bot_qq || token !== "" || secret !== "" || JSON.stringify(groups) !== JSON.stringify(initial.groups);
  const choices = [...(connection.data?.groups ?? [])];
  for (const group of groups) if (!choices.some((g) => g.group_id === group.group_id)) choices.push(group);
  const busy = save.isPending || connection.isPending || test.isPending;
  return <>
    <Card className="community-card"><CardContent className="community-content">
      <div className="account-section-heading">
        <div className="account-section-label"><Bot size={18} aria-hidden="true" /><h2>NapCatQQ</h2></div>
        <label className="qq-group-enabled"><input type="checkbox" checked={enabled} disabled={busy} onChange={(e) => setEnabled(e.target.checked)} />{msg("启用")}</label>
      </div>
      <div className="qq-bot-settings-grid">
        <div className="community-field"><label htmlFor="onebot-qq">{msg("机器人 QQ 号")}</label><input id="onebot-qq" inputMode="numeric" maxLength={12} value={botQQ} disabled={busy} onChange={(e) => setBotQQ(e.target.value)} autoComplete="off" /></div>
        <div className="community-field"><label htmlFor="onebot-api">{msg("API 地址")}</label><input id="onebot-api" maxLength={256} value={apiBase} disabled={busy} onChange={(e) => setAPIBase(e.target.value)} autoComplete="off" /></div>
        <div className="community-field"><label htmlFor="onebot-token">{msg("访问令牌")}</label><input id="onebot-token" type="password" maxLength={512} value={token} disabled={busy} onChange={(e) => setToken(e.target.value)} autoComplete="new-password" placeholder={initial.token_configured ? msg("留空不更改") : msg("请输入密钥")} /></div>
        <div className="community-field"><label htmlFor="onebot-secret">{msg("回调密钥")}</label><input id="onebot-secret" type="password" maxLength={512} value={secret} disabled={busy} onChange={(e) => setSecret(e.target.value)} autoComplete="new-password" placeholder={initial.secret_configured ? msg("留空不更改") : msg("请输入密钥")} /></div>
      </div>
      <div className="community-form-actions">
        <Button type="button" disabled={busy || !dirty} onClick={() => save.mutate()}><Save aria-hidden="true" />{save.isPending ? msg("正在保存") : msg("保存")}</Button>
        <Button type="button" variant="outline" disabled={busy || dirty || !initial.enabled} onClick={() => connection.mutate()}><RefreshCw aria-hidden="true" />{connection.isPending ? msg("正在连接") : msg("连接并获取群")}</Button>
      </div>
      {connection.data && !dirty && <p role="status" className="account-notice">{msg("已连接：{0}", connection.data.nickname || connection.data.bot_qq)}</p>}
      {(save.error || connection.error) && <p role="alert" className="login-error">{(save.error || connection.error)?.message}</p>}
    </CardContent></Card>
    <Card className="community-card"><CardContent className="community-content">
      <div className="account-section-heading"><div className="account-section-label"><UsersRound size={18} aria-hidden="true" /><h2>{msg("接收群")}</h2></div></div>
      {choices.length === 0 && <p className="muted">{msg("连接后选择群")}</p>}
      <div className="onebot-group-list">{choices.map((group) => {
        const selected = groups.some((g) => g.group_id === group.group_id);
        return <div className="onebot-group-row" key={group.group_id}>
          <label className="onebot-group-choice"><input type="checkbox" checked={selected} disabled={busy} onChange={(e) => setGroups(e.target.checked ? [...groups, group] : groups.filter((g) => g.group_id !== group.group_id))} /><span><strong>{group.name || group.group_id}</strong><span className="muted">{group.group_id}</span></span></label>
          <label className="qq-group-enabled"><input type="checkbox" checked={groups.find((g) => g.group_id === group.group_id)?.notify_approvals === true} disabled={busy || !selected} onChange={(e) => setGroups(groups.map((g) => g.group_id === group.group_id ? { ...g, notify_approvals: e.target.checked } : g))} aria-label={msg("在 {0} 启用审批提醒", group.name || group.group_id)} />{msg("审批提醒")}</label>
          <Button type="button" variant="outline" disabled={busy || dirty || !selected || !initial.enabled} onClick={() => test.mutate(group.group_id)} aria-label={msg("在 {0} 测试 @我", group.name || group.group_id)}><Send aria-hidden="true" />{msg("测试 @我")}</Button>
        </div>;
      })}</div>
      {dirty && <Button type="button" disabled={busy} onClick={() => save.mutate()}><Save aria-hidden="true" />{msg("保存")}</Button>}
      {test.error && <p role="alert" className="login-error">{test.error.message}</p>}
      {test.isSuccess && <p role="status" className="account-notice">{msg("测试消息已提交")}</p>}
    </CardContent></Card>
  </>;
}
