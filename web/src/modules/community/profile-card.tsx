import { msg } from "@/lib/i18n";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, useRef } from "react";
import {
  MessageCircle,
  Headphones,
  UsersRound,
  Pencil,
  RefreshCw,
  LoaderCircle,
} from "lucide-react";
import { getModuleCatalog } from "@/app/catalog";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { APIError } from "@/lib/http";
import { useToast } from "@/components/ui/toast-context";
import { Select } from "@/components/ui/select";
import {
  getProfile,
  saveProfile,
  validateDetails,
  type Profile,
  type Binding,
  createQQGroupApplication,
  getQQGroupApplication,
  type QQGroupApplication,
  type QQGroupApplicationChallenge,
  getQQGroupOptions,
  type QQGroupSetting,
} from "./api";

export function CommunityProfile({
  userID,
  csrf,
}: {
  userID: string;
  csrf: string;
}) {
  const catalog = useQuery({
    queryKey: ["host", "modules"],
    queryFn: ({ signal }) => getModuleCatalog(signal),
  });
  if (!catalog.data?.some((m) => m.id === "community" && m.api_version === 1))
    return null;
  return <ProfileCard key={userID} userID={userID} csrf={csrf} />;
}
function ProfileCard({ userID, csrf }: { userID: string; csrf: string }) {
  const client = useQueryClient();
  const toast = useToast();
  const key = ["community", "profile", userID];
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => getProfile(signal),
  });
  const [editing, setEditing] = useState(false);
  const [reload, setReload] = useState(0);
  const editButton = useRef<HTMLButtonElement>(null);
  const groupApplication = useQuery({
    queryKey: ["community", "qq-group-application", userID],
    queryFn: ({ signal }) => getQQGroupApplication(signal),
  });
  const groupOptions = useQuery({
    queryKey: ["community", "qq-group-options"],
    queryFn: ({ signal }) => getQQGroupOptions(signal),
    enabled: !!query.data?.qq.value,
  });
  const [selectedGroup, setSelectedGroup] = useState("");
  const groupApply = useMutation({
    mutationFn: () => createQQGroupApplication(csrf, selectedGroup),
    onSuccess: (application) => client.setQueryData(["community", "qq-group-application", userID], application),
  });
  const p = query.data;
  return (
    <Card className="community-card">
      <CardContent className="community-content">
        <div className="account-section-heading">
          <div className="account-section-label">
            <UsersRound size={18} aria-hidden="true" />
            <h2>{msg("社区资料")}</h2>
          </div>
          {p?.complete && !editing && (
            <IconAction
              label={msg("修改社区资料")}
              ref={editButton}
              onClick={() => {
                setEditing(true);
              }}
            >
              <Pencil aria-hidden="true" />
            </IconAction>
          )}
        </div>
        {query.isPending ? (
          <div
            role="status"
            aria-label={msg("正在读取社区资料")}
            className="community-loading"
          >
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          </div>
        ) : query.isError && !p ? (
          <div className="account-feedback" role="alert">
            <p>{msg("社区资料读取失败，请重试。")}</p>
            <IconAction
              label={msg("重新读取社区资料")}
              onClick={() => void query.refetch()}
            >
              <RefreshCw aria-hidden="true" />
            </IconAction>
          </div>
        ) : p && (editing || !p.complete) ? (
          <ProfileEditor
            key={reload}
            profile={p}
            csrf={csrf}
            onCancel={p.complete ? () => setEditing(false) : undefined}
            onReload={async () => {
              const result = await query.refetch();
              if (result.error) throw result.error;
              setReload((n) => n + 1);
            }}
            onSaved={async (saved) => {
              client.setQueryData(key, saved);
              setEditing(false);
              toast.success(msg("资料已保存"));
              requestAnimationFrame(() => editButton.current?.focus());
              await client.invalidateQueries({ queryKey: ["access"] });
            }}
          />
        ) : (
          p && (
            <>
              <div className="community-bindings">
                <BindingSummary label="QQ" binding={p.qq} icon="qq" />
                <BindingSummary label="KOOK" binding={p.kook} icon="kook" />
              </div>
              {p.qq.value && (
                <QQGroupAdmission
                  application={groupApplication.data}
                  challenge={groupApply.data}
                  options={groupOptions.data?.items ?? []}
                  selectedGroup={selectedGroup}
                  onGroupChange={setSelectedGroup}
                  pending={groupApply.isPending}
                  error={groupApply.error}
                  onApply={() => groupApply.mutate()}
                />
              )}
            </>
          )
        )}
      </CardContent>
    </Card>
  );
}

function QQGroupAdmission({
  application,
  challenge,
  options,
  selectedGroup,
  onGroupChange,
  pending,
  error,
  onApply,
}: {
  application: QQGroupApplication | null | undefined;
  challenge: QQGroupApplicationChallenge | undefined;
  options: QQGroupSetting[];
  selectedGroup: string;
  onGroupChange: (value: string) => void;
  pending: boolean;
  error: Error | null;
  onApply: () => void;
}) {
  const active = application && ["pending", "approving", "approved", "bound"].includes(application.status);
  const requiresChoice = options.length > 1;
  const targetGroup = selectedGroup || (options.length === 1 ? options[0].group_openid : "");
  return (
    <div className="community-bot-bind">
      {requiresChoice && !active && (
        <Select
          label={msg("申请加入 QQ 群")}
          value={selectedGroup}
          onValueChange={onGroupChange}
          options={options.map((option) => ({
            value: option.group_openid,
            label: option.label || option.group_openid,
          }))}
          placeholder={msg("请选择 QQ 群")}
          disabled={pending}
        />
      )}
      {application?.status === "bound" ? (
        <p role="status">{msg("QQ 群身份已绑定")}</p>
      ) : active ? (
        <p role="status">
          {application.status === "approved" ? msg("入群申请已通过，等待入群确认") : msg("入群申请处理中")}
        </p>
      ) : (
        <Button type="button" variant="outline" disabled={pending || !targetGroup} onClick={onApply}>
          {pending && <LoaderCircle className="animate-spin" aria-hidden="true" />}
          {msg("申请加入 QQ 群")}
        </Button>
      )}
      {challenge?.code && (
        <p role="status">
          {msg("请将此申请码填写到 QQ 入群验证信息中")} <code>{challenge.code}</code>
        </p>
      )}
      {error && <p className="login-error">{error.message}</p>}
    </div>
  );
}
function BindingSummary({
  label,
  binding,
  icon,
}: {
  label: string;
  binding: Binding;
  icon: "qq" | "kook";
}) {
  const Icon = icon === "qq" ? MessageCircle : Headphones;
  const confirmed = binding.confirmation === "confirmed";
  return (
    <div className="community-binding">
      <span className="community-platform-icon">
        <Icon aria-hidden="true" />
      </span>
      <div className="community-binding-text">
        <h3>{label}</h3>
        <p>{binding.value || msg("未填写")}</p>
        <span
          className={`community-confirmation ${confirmed ? "is-confirmed" : ""}`}
          title={`${label} ${icon === "qq" ? msg("入群") : msg("加入服务器")}${confirmed ? msg("已确认") : binding.confirmation === "unfilled" ? msg("资料未填写") : msg("待确认")}`}
        >
          {confirmed
            ? msg("已确认")
            : binding.confirmation === "unfilled"
              ? msg("未填写")
              : msg("待确认")}
        </span>
      </div>
    </div>
  );
}
function ProfileEditor({
  profile,
  csrf,
  onCancel,
  onSaved,
  onReload,
}: {
  profile: Profile;
  csrf: string;
  onCancel?: () => void;
  onSaved: (p: Profile) => Promise<void>;
  onReload: () => Promise<void>;
}) {
  const [qq, setQQ] = useState(profile.qq.value);
  const [kook, setKOOK] = useState(profile.kook.value);
  // Retain the revision with the draft, even if a background refetch changes props.
  const [version] = useState(profile.version);
  const [errors, setErrors] = useState({ qq: "", kook: "" });
  const [reloading, setReloading] = useState(false);
  const [reloadError, setReloadError] = useState("");
  const qqRef = useRef<HTMLInputElement>(null);
  const kookRef = useRef<HTMLInputElement>(null);
  const summary = useRef<HTMLDivElement>(null);
  const save = useMutation({
    mutationFn: () =>
      saveProfile(
        { qq_number: qq.trim(), kook_name: kook.trim(), version },
        csrf,
      ),
    onSuccess: onSaved,
    onError: () => requestAnimationFrame(() => summary.current?.focus()),
  });
  const pending = save.isPending || reloading;
  const changedConfirmed =
    (qq.trim() !== profile.qq.value &&
      profile.qq.confirmation === "confirmed") ||
    (kook.trim() !== profile.kook.value &&
      profile.kook.confirmation === "confirmed");
  return (
    <form
      className="community-form"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        const next = validateDetails(qq, kook);
        setErrors(next);
        if (next.qq) {
          qqRef.current?.focus();
          return;
        }
        if (next.kook) {
          kookRef.current?.focus();
          return;
        }
        save.mutate();
      }}
    >
      <div className="community-form-fields">
        <div className="community-field">
          <label htmlFor="community-qq">
            <MessageCircle size={16} aria-hidden="true" />
            {msg("QQ 号")}{" "}
          </label>
          <input
            ref={qqRef}
            id="community-qq"
            name="qq_number"
            inputMode="numeric"
            autoComplete="off"
            required
            maxLength={12}
            value={qq}
            disabled={pending}
            aria-invalid={!!errors.qq}
            aria-describedby={errors.qq ? "community-qq-error" : undefined}
            onChange={(e) => {
              const value = e.target.value;
              setQQ(value);
              setErrors((x) => ({
                ...x,
                qq: x.qq ? validateDetails(value, kook).qq : "",
              }));
            }}
            onBlur={() =>
              setErrors((x) => ({ ...x, qq: validateDetails(qq, kook).qq }))
            }
          />
          {errors.qq && (
            <p id="community-qq-error" className="login-error">
              {errors.qq}
            </p>
          )}
        </div>
        <div className="community-field">
          <label htmlFor="community-kook">
            <Headphones size={16} aria-hidden="true" />
            {msg("KOOK 昵称")}{" "}
          </label>
          <input
            ref={kookRef}
            id="community-kook"
            name="kook_name"
            autoComplete="off"
            required
            maxLength={128}
            value={kook}
            disabled={pending}
            aria-invalid={!!errors.kook}
            aria-describedby={errors.kook ? "community-kook-error" : undefined}
            onChange={(e) => {
              const value = e.target.value;
              setKOOK(value);
              setErrors((x) => ({
                ...x,
                kook: x.kook ? validateDetails(qq, value).kook : "",
              }));
            }}
            onBlur={() =>
              setErrors((x) => ({ ...x, kook: validateDetails(qq, kook).kook }))
            }
          />
          {errors.kook && (
            <p id="community-kook-error" className="login-error">
              {errors.kook}
            </p>
          )}
        </div>
      </div>
      {changedConfirmed && (
        <p className="community-warning">
          {msg("修改后，对应平台需要重新确认。")}
        </p>
      )}
      {(save.error || reloadError) && (
        <div
          ref={summary}
          role="alert"
          tabIndex={-1}
          className="community-form-error"
        >
          <p>{reloadError || save.error?.message}</p>
          {save.error instanceof APIError && save.error.status === 409 && (
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={async () => {
                setReloading(true);
                try {
                  await onReload();
                } catch {
                  setReloadError(msg("读取失败，请重试"));
                } finally {
                  setReloading(false);
                }
              }}
            >
              {msg("读取最新资料")}{" "}
            </Button>
          )}
        </div>
      )}
      <div className="community-form-actions">
        {onCancel && (
          <Button
            type="button"
            variant="outline"
            disabled={pending}
            onClick={onCancel}
          >
            {msg("取消")}{" "}
          </Button>
        )}
        <Button type="submit" disabled={pending} aria-busy={pending}>
          {save.isPending && (
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          )}
          {save.isPending ? msg("正在保存") : msg("保存资料")}
        </Button>
      </div>
    </form>
  );
}
