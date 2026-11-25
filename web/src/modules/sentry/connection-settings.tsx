import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { FormDialog } from "@/components/ui/form-dialog";
import { useToast } from "@/components/ui/toast-context";
import { msg } from "@/lib/i18n";
import { useManagementAccess } from "@/modules/access/use-management-access";
import { getConnection, updateConnection, type ConnectionSettings } from "./api";

const connectionQueryKey = ["sentry", "connection"] as const;

export function ConnectionSettingsButton({ csrf }: { csrf: string }) {
  const { access } = useManagementAccess();
  const [open, setOpen] = useState(false);
  const administrator = access.data?.administrator === true;
  const connection = useQuery({
    queryKey: connectionQueryKey,
    queryFn: ({ signal }) => getConnection(signal),
    enabled: open && administrator,
    refetchOnWindowFocus: false,
  });
  const title = msg("Sentry 接入设置");
  if (!administrator) return null;
  return (
    <>
      <Button type="button" variant="outline" onClick={() => setOpen(true)}>
        <Link2 aria-hidden="true" />{title}
      </Button>
      {open && (connection.data ? (
        <ConnectionForm csrf={csrf} settings={connection.data} close={() => setOpen(false)} />
      ) : (
        <FormDialog title={title} close={() => setOpen(false)} showSubmit={false} onSubmit={() => undefined}>
          {connection.isError
            ? <p role="alert">{connection.error.message} <Button type="button" variant="outline" onClick={() => void connection.refetch()}>{msg("重试")}</Button></p>
            : <p role="status">{msg("正在读取")}</p>}
        </FormDialog>
      ))}
    </>
  );
}

function ConnectionForm({ csrf, settings, close }: { csrf: string; settings: ConnectionSettings; close: () => void }) {
  const client = useQueryClient();
  const toast = useToast();
  const [editingSettings, setEditingSettings] = useState<ConnectionSettings | null>(null);
  const [origin, setOrigin] = useState("");
  const [token, setToken] = useState("");
  const save = useMutation({
    mutationFn: (revision: number) => updateConnection(csrf, {
      origin: origin.trim(),
      revision,
      ...(token.trim() ? { service_token: token.trim() } : {}),
    }),
    onSuccess: (next) => {
      setToken("");
      client.setQueryData(connectionQueryKey, next);
      void client.invalidateQueries({ queryKey: ["sentry", "keys"] });
      void client.invalidateQueries({ queryKey: ["sentry", "time-pricing"] });
      toast.info(msg("Sentry 鉴权通过，配置已保存"));
      close();
    },
  });
  const retainToken = editingSettings?.token_configured === true && origin.trim().replace(/\/+$/, "") === editingSettings.origin;
  return (
    <FormDialog
      title={msg("Sentry 接入设置")}
      close={close}
      className={editingSettings ? "sentry-pricing-form sentry-connection-form" : undefined}
      busy={save.isPending}
      disabled={editingSettings !== null && (!origin.trim() || (!retainToken && !token.trim()) || (!!token.trim() && token.trim().length < 32))}
      submitLabel={editingSettings ? msg("测试并保存") : msg("编辑接入设置")}
      onSubmit={() => {
        if (editingSettings) {
          save.mutate(editingSettings.revision);
        } else {
          setOrigin(settings.origin);
          setEditingSettings(settings);
        }
      }}
    >
      {editingSettings ? (
        <>
          <label>
            <span>{msg("Sentry 服务地址")}</span>
            <input autoFocus type="url" maxLength={2048} required value={origin} onChange={(event) => setOrigin(event.target.value)} />
          </label>
          <label>
            <span>{msg("服务令牌")}</span>
            <input type="password" autoComplete="new-password" maxLength={512} required={!retainToken} value={token}
              onChange={(event) => setToken(event.target.value)} />
          </label>
          {save.isError && <p role="alert">{save.error.message}</p>}
        </>
      ) : (
        <dl className="sentry-connection-summary">
          <dt>{msg("Sentry 服务地址")}</dt>
          <dd>{settings.origin || msg("未配置")}</dd>
          <dt>{msg("服务令牌")}</dt>
          <dd>{settings.token_configured ? msg("已配置") : msg("未配置")}</dd>
        </dl>
      )}
    </FormDialog>
  );
}
