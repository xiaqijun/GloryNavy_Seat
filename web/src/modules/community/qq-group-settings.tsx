import { msg } from "@/lib/i18n";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, Globe, KeyRound, Plus, Save, Trash2, UsersRound } from "lucide-react";
import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { getAccountAccess } from "@/modules/access/api";
import {
  getQQBotSettings,
  getQQGroupSettings,
  saveQQBotSettings,
  saveQQGroupSettings,
  type QQBotSettings as QQBotSettingsData,
  type QQGroupSetting,
} from "./api";

export function QQGroupSettings({ userID, csrf }: { userID: string; csrf: string }) {
  const access = useQuery({
    queryKey: ["access", "me", userID],
    queryFn: ({ signal }) => getAccountAccess(signal),
  });
  const settings = useQuery({
    queryKey: ["community", "qq-group-settings"],
    queryFn: ({ signal }) => getQQGroupSettings(signal),
    enabled: access.data?.administrator === true,
  });
  if (!access.data?.administrator || settings.isPending || settings.isError || !settings.data) return null;
  const key = settings.data.items.map((item) => `${item.group_openid}:${item.label}:${item.enabled ? 1 : 0}`).join("|");
  return <QQGroupSettingsEditor key={key} initial={settings.data.items} csrf={csrf} />;
}

export function QQBotSettings({ userID, csrf }: { userID: string; csrf: string }) {
  const access = useQuery({
    queryKey: ["access", "me", userID],
    queryFn: ({ signal }) => getAccountAccess(signal),
  });
  const settings = useQuery({
    queryKey: ["community", "qq-bot-settings"],
    queryFn: ({ signal }) => getQQBotSettings(signal),
    enabled: access.data?.administrator === true,
  });
  if (!access.data?.administrator || settings.isPending || settings.isError || !settings.data) return null;
  const key = `${settings.data.app_id}:${settings.data.api_base}:${settings.data.secret_configured ? 1 : 0}:${settings.data.initialized ? 1 : 0}`;
  return <QQBotSettingsEditor key={key} initial={settings.data} csrf={csrf} />;
}

function QQBotSettingsEditor({ initial, csrf }: { initial: QQBotSettingsData; csrf: string }) {
  const client = useQueryClient();
  const [appID, setAppID] = useState(initial.app_id);
  const [apiBase, setAPIBase] = useState(initial.api_base);
  const [secret, setSecret] = useState("");
  const save = useMutation({
    mutationFn: () => saveQQBotSettings({ app_id: appID, api_base: apiBase, secret }, csrf),
    onSuccess: (next) => {
      setSecret("");
      client.setQueryData(["community", "qq-bot-settings"], next);
    },
  });
  const busy = save.isPending;
  return (
    <Card className="community-card qq-bot-settings-card">
      <CardContent className="community-content">
        <div className="account-section-heading">
          <div className="account-section-label">
            <Bot size={18} aria-hidden="true" />
            <h2>{msg("官方 QQ Bot")}</h2>
          </div>
          <span className="muted">{msg("管理员配置")}</span>
        </div>
        <p className="muted qq-group-settings-note">
          {msg("AppID、API 地址和密钥均可在此维护；密钥只写入后端加密存储，不会回显。")}
        </p>
        <div className="qq-bot-settings-grid">
          <div className="community-field">
            <label htmlFor="qq-bot-app-id"><KeyRound size={14} aria-hidden="true" />{msg("AppID")}</label>
            <input id="qq-bot-app-id" value={appID} maxLength={128} disabled={busy} onChange={(event) => setAppID(event.target.value)} autoComplete="off" />
          </div>
          <div className="community-field">
            <label htmlFor="qq-bot-api-base"><Globe size={14} aria-hidden="true" />{msg("API 地址")}</label>
            <input id="qq-bot-api-base" value={apiBase} maxLength={256} disabled={busy} onChange={(event) => setAPIBase(event.target.value)} autoComplete="url" />
          </div>
          <div className="community-field qq-bot-secret-field">
            <label htmlFor="qq-bot-secret"><KeyRound size={14} aria-hidden="true" />{msg("App Secret")}</label>
            <input id="qq-bot-secret" type="password" value={secret} maxLength={512} disabled={busy} onChange={(event) => setSecret(event.target.value)} placeholder={initial.secret_configured ? msg("留空保持现有密钥") : msg("请输入密钥")} autoComplete="new-password" />
            <span className={initial.secret_configured ? "status-chip success" : "status-chip muted"}>
              {initial.secret_configured ? msg("密钥已配置") : msg("尚未配置密钥")}
            </span>
          </div>
        </div>
        <div className="community-form-actions">
          <Button type="button" size="sm" disabled={busy} onClick={() => save.mutate()}>
            <Save aria-hidden="true" />{busy ? msg("正在保存") : msg("保存 QQ 配置")}
          </Button>
        </div>
        {save.error && <p role="alert" className="login-error">{save.error.message}</p>}
        {save.isSuccess && <p role="status" className="account-notice">{msg("QQ Bot 配置已保存")}</p>}
      </CardContent>
    </Card>
  );
}

function QQGroupSettingsEditor({ initial, csrf }: { initial: QQGroupSetting[]; csrf: string }) {
  const client = useQueryClient();
  const [items, setItems] = useState(initial);
  const save = useMutation({
    mutationFn: () => saveQQGroupSettings(items, csrf),
    onSuccess: (next) => client.setQueryData(["community", "qq-group-settings"], next),
  });
  const busy = save.isPending;
  return (
    <Card className="community-card qq-group-settings-card">
      <CardContent className="community-content">
        <div className="account-section-heading">
          <div className="account-section-label">
            <UsersRound size={18} aria-hidden="true" />
            <h2>{msg("QQ 入群审批")}</h2>
          </div>
          <span className="muted">{msg("管理员配置")}</span>
        </div>
        <p className="muted qq-group-settings-note">{msg("配置官方 QQ Bot 接收入群申请的群 OpenID；AppID 和密钥仍保存在服务端。")}</p>
        <div className="qq-group-settings-list">
          {items.map((item, index) => (
            <div className="qq-group-settings-row" key={`${index}-${item.group_openid}`}>
              <div className="community-field">
                <label htmlFor={`qq-group-openid-${index}`}>{msg("群 OpenID")}</label>
                <input
                  id={`qq-group-openid-${index}`}
                  value={item.group_openid}
                  maxLength={256}
                  disabled={busy}
                  onChange={(event) => setItems((rows) => rows.map((row, i) => i === index ? { ...row, group_openid: event.target.value } : row))}
                  autoComplete="off"
                />
              </div>
              <div className="community-field">
                <label htmlFor={`qq-group-label-${index}`}>{msg("显示名称")}</label>
                <input
                  id={`qq-group-label-${index}`}
                  value={item.label}
                  maxLength={128}
                  disabled={busy}
                  onChange={(event) => setItems((rows) => rows.map((row, i) => i === index ? { ...row, label: event.target.value } : row))}
                  placeholder={msg("可选")}
                  autoComplete="off"
                />
              </div>
              <label className="qq-group-enabled">
                <input
                  type="checkbox"
                  checked={item.enabled}
                  disabled={busy}
                  onChange={(event) => setItems((rows) => rows.map((row, i) => i === index ? { ...row, enabled: event.target.checked } : row))}
                />
                {msg("启用")}
              </label>
              <IconAction label={msg("移除群配置")} disabled={busy} onClick={() => setItems((rows) => rows.filter((_, i) => i !== index))}>
                <Trash2 aria-hidden="true" />
              </IconAction>
            </div>
          ))}
        </div>
        <div className="community-form-actions">
          <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => setItems((rows) => [...rows, { group_openid: "", label: "", enabled: true }])}>
            <Plus aria-hidden="true" />{msg("添加群")}
          </Button>
          <Button type="button" size="sm" disabled={busy} onClick={() => save.mutate()}>
            <Save aria-hidden="true" />{busy ? msg("正在保存") : msg("保存配置")}
          </Button>
        </div>
        {save.error && <p role="alert" className="login-error">{save.error.message}</p>}
        {save.isSuccess && <p role="status" className="account-notice">{msg("QQ 群配置已保存")}</p>}
      </CardContent>
    </Card>
  );
}
