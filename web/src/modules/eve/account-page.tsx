import { msg } from "@/lib/i18n";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Hash,
  House,
  LoaderCircle,
  LogOut,
  RefreshCw,
  Plus,
  Star,
  Unlink,
} from "lucide-react";
import { useState } from "react";
import { AlertDialog } from "radix-ui";
import { Button } from "@/components/ui/button";
import { Link, Navigate } from "react-router-dom";
import { Card, CardContent } from "@/components/ui/card";
import { EveImage } from "@/components/eve-image";
import { IconAction } from "@/components/ui/icon-action";
import {
  useSession,
  logout,
  getCharacters,
  changeCharacter,
} from "@/modules/identity";
import { AuthorizationSummary } from "@/modules/access";
import { getEVEStatus, startCharacterFlow } from "./api";
import { useLoginError } from "./use-login-error";
import { CommunityProfile } from "@/modules/community/profile-card";
import { SyncRows } from "./sync-rows";
import { MergeAccount } from "@/modules/identity/merge-account";

export default function AccountPage() {
  const auth = useSession();
  const status = useQuery({
    queryKey: ["eve", "status"],
    queryFn: ({ signal }) => getEVEStatus(signal),
  });
  const client = useQueryClient();
  const exit = useMutation({
    mutationFn: () => logout(auth.data?.session?.csrf_token ?? ""),
    onSuccess: async () => {
      await client.cancelQueries();
      client.clear();
      window.location.replace("/login");
    },
  });
  const session = auth.data?.session;
  const callbackError = useLoginError(true);
  const [selectedID, setSelectedID] = useState<string>();
  const [confirmUnlink, setConfirmUnlink] = useState(false);
  const [notice, setNotice] = useState(() =>
    new URLSearchParams(window.location.search).get("merged") === "1"
      ? msg("账号已合并")
      : "",
  );
  const characters = useQuery({
    queryKey: ["identity", "characters", session?.user_id],
    queryFn: ({ signal }) => getCharacters(signal),
    enabled: !!session,
  });
  const selected =
    characters.data?.characters.find((c) => c.id === selectedID) ??
    characters.data?.characters.find((c) => c.is_main) ??
    characters.data?.characters[0];
  const update = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "main" | "unlink" }) =>
      changeCharacter(id, action, session?.csrf_token ?? ""),
    onSuccess: async (_, variables) => {
      setConfirmUnlink(false);
      setNotice(
        variables.action === "main" ? msg("主角色已更新") : msg("角色已解绑"),
      );
      await client.cancelQueries({ queryKey: ["access"] });
      client.removeQueries({ queryKey: ["access"] });
      if (
        variables.action === "unlink" &&
        variables.id === session?.character.id
      ) {
        await client.cancelQueries();
        client.clear();
        window.location.replace("/login");
        return;
      }
      await client.invalidateQueries({ queryKey: ["identity"] });
    },
  });
  const authorize = useMutation({
    mutationFn: (id?: string) =>
      startCharacterFlow(session?.csrf_token ?? "", id),
  });
  const busy = update.isPending || authorize.isPending || exit.isPending;
  const identityHeader = selected && (
    <div className="account-profile-content account-identity-header">
      <EveImage
        id={selected.id}
        kind="character"
        className="account-portrait"
      />
      <div className="account-identity">
        <div className="account-identity-title">
          <h2 className="character-name">{selected.name}</h2>
          {selected.is_main && (
            <span className="account-main-marker">
              <Star size={12} aria-hidden="true" />
              {msg("主角色")}{" "}
            </span>
          )}
        </div>
        <p className="account-character-id">
          <Hash size={14} aria-hidden="true" />
          <span className="sr-only">{msg("角色 ID：")}</span>
          {selected.id}
        </p>
        {selected.status === "blocked" && (
          <p className="login-error">{msg("身份待确认，请联系管理员。")}</p>
        )}
      </div>
      <div className="account-character-actions">
        {!selected.is_main && (
          <IconAction
            label={msg("设为主角色")}
            disabled={busy || selected.status !== "active"}
            onClick={() => update.mutate({ id: selected.id, action: "main" })}
          >
            <Star aria-hidden="true" />
          </IconAction>
        )}
        <IconAction
          label={msg("更新角色授权")}
          disabled={
            busy || selected.status !== "active" || !status.data?.configured
          }
          onClick={() => authorize.mutate(selected.id)}
        >
          <RefreshCw aria-hidden="true" />
        </IconAction>
        {!selected.is_main && (
          <IconAction
            label={msg("解绑角色")}
            disabled={busy}
            onClick={() => {
              update.reset();
              setConfirmUnlink(true);
            }}
          >
            <Unlink aria-hidden="true" />
          </IconAction>
        )}
      </div>
    </div>
  );
  if (auth.isSuccess && !session) return <Navigate to="/login" replace />;
  return (
    <div className="account-page">
      <div className="page-heading">
        <h1>{msg("我的角色")}</h1>
        <div className="account-actions">
          <IconAction label={msg("返回工作台")} asChild>
            <Link to="/workspace">
              <House aria-hidden="true" />
            </Link>
          </IconAction>
          {session && (
            <IconAction
              label={msg("退出登录")}
              disabled={busy}
              aria-busy={exit.isPending}
              onClick={() => exit.mutate()}
            >
              <LogOut aria-hidden="true" />
            </IconAction>
          )}
        </div>
      </div>
      {(callbackError || update.error || authorize.error) && !confirmUnlink && (
        <p role="alert" className="login-error account-error">
          {update.error?.message ?? authorize.error?.message ?? callbackError}
        </p>
      )}
      {notice && (
        <p role="status" className="account-notice">
          {notice}
        </p>
      )}
      {exit.isError && (
        <p role="alert" className="login-error account-error">
          {msg("退出失败，请稍后重试。")}{" "}
        </p>
      )}
      <div className="account-layout">
        {session && (
          <div className="grid gap-4">
            <CommunityProfile
              userID={session.user_id}
              csrf={session.csrf_token}
            />
          </div>
        )}
        <div className="account-characters">
          {auth.isPending ||
          status.isPending ||
          (!!session && characters.isPending) ? (
            <Card>
              <CardContent
                className="account-loading"
                role="status"
                aria-label={msg("正在读取角色")}
                aria-busy="true"
              >
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              </CardContent>
            </Card>
          ) : auth.isError || status.isError || characters.isError ? (
            <Card>
              <CardContent className="account-feedback" role="alert">
                <p>{msg("角色信息读取失败，请重试。")}</p>
                <IconAction
                  label={msg("重试角色信息")}
                  onClick={() => {
                    void auth.refetch();
                    void status.refetch();
                    void characters.refetch();
                  }}
                >
                  <RefreshCw aria-hidden="true" />
                </IconAction>
              </CardContent>
            </Card>
          ) : (
            session &&
            selected && (
              <>
                <div className="account-roster-heading">
                  <span className="muted">
                    {characters.data?.characters.length} {msg("个角色")}{" "}
                  </span>
                  <div className="account-roster-actions">
                    <MergeAccount
                      csrf={session.csrf_token}
                      disabled={busy || !status.data?.configured}
                    />
                    <Button
                      disabled={busy || !status.data?.configured}
                      aria-busy={authorize.isPending}
                      onClick={() => {
                        update.reset();
                        setNotice("");
                        authorize.mutate(undefined);
                      }}
                    >
                      {authorize.isPending ? (
                        <LoaderCircle
                          className="animate-spin"
                          aria-hidden="true"
                        />
                      ) : (
                        <Plus aria-hidden="true" />
                      )}
                      {msg("添加角色")}{" "}
                    </Button>
                  </div>
                </div>
                {(characters.data?.characters.length ?? 0) > 1 && (
                  <div
                    className="account-roster"
                    role="group"
                    aria-label={msg("选择查看角色")}
                  >
                    {characters.data!.characters.map((c) => (
                      <button
                        type="button"
                        className="account-character-tile"
                        key={c.id}
                        aria-pressed={selected.id === c.id}
                        onClick={() => {
                          setSelectedID(c.id);
                          setNotice("");
                          update.reset();
                          authorize.reset();
                        }}
                        disabled={busy}
                      >
                        <span className="account-tile-avatar">
                          <EveImage
                            id={c.id}
                            kind="character"
                            className="account-tile-portrait"
                          />
                          {selected.id === c.id && (
                            <span
                              className="account-selected-mark"
                              aria-hidden="true"
                            >
                              <Check size={12} />
                            </span>
                          )}
                        </span>
                        <span className="account-tile-identity">
                          <strong>{c.name}</strong>
                          <span className="account-tile-meta">
                            {c.is_main && (
                              <span className="account-main-marker">
                                <Star size={12} aria-hidden="true" />
                                {msg("主角色")}{" "}
                              </span>
                            )}
                            {c.status === "blocked" && (
                              <span className="login-error">
                                {msg("身份待确认")}
                              </span>
                            )}
                          </span>
                        </span>
                      </button>
                    ))}
                  </div>
                )}
                {!status.data?.corporation_roles && (
                  <Card className="account-character-detail">
                    {identityHeader}
                  </Card>
                )}
                <AlertDialog.Root
                  open={confirmUnlink}
                  onOpenChange={(open) => {
                    if (!update.isPending) setConfirmUnlink(open);
                  }}
                >
                  <AlertDialog.Portal>
                    <AlertDialog.Overlay className="ui-dialog-overlay" />
                    <AlertDialog.Content className="ui-dialog-surface account-dialog">
                      <AlertDialog.Title>
                        {msg("解绑")} {selected.name}？
                      </AlertDialog.Title>
                      <AlertDialog.Description className="ui-dialog-description">
                        {msg("将移除此角色的本站授权和数据访问。")}{" "}
                        {selected.id === session.character.id
                          ? msg("本次登录也会退出。")
                          : msg("其他角色不受影响。")}
                      </AlertDialog.Description>
                      {update.error && (
                        <p role="alert" className="login-error">
                          {update.error.message}
                        </p>
                      )}
                      <div className="account-dialog-actions">
                        <AlertDialog.Cancel asChild>
                          <Button variant="outline" disabled={update.isPending}>
                            {msg("取消")}{" "}
                          </Button>
                        </AlertDialog.Cancel>
                        <Button
                          variant="destructive"
                          disabled={update.isPending}
                          aria-busy={update.isPending}
                          onClick={() =>
                            update.mutate({ id: selected.id, action: "unlink" })
                          }
                        >
                          {update.isPending ? msg("正在解绑") : msg("确认解绑")}
                        </Button>
                      </div>
                    </AlertDialog.Content>
                  </AlertDialog.Portal>
                </AlertDialog.Root>
                {status.data?.corporation_roles && (
                  <AuthorizationSummary
                    syncContent={
                      <SyncRows
                        key={selected.id}
                        characterID={selected.id}
                        userID={session.user_id}
                        csrf={session.csrf_token}
                      />
                    }
                    identityHeader={identityHeader}
                    userID={session.user_id}
                    characterID={selected.id}
                    key={selected.id}
                    onReauthorize={() => authorize.mutate(selected.id)}
                    submitting={authorize.isPending}
                    disabled={
                      busy ||
                      selected.status !== "active" ||
                      !status.data?.configured
                    }
                  />
                )}
              </>
            )
          )}
        </div>
      </div>
    </div>
  );
}
