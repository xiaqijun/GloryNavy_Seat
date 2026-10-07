import { lazy, Suspense, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Award, Check, RefreshCw, Settings2 } from "lucide-react";
import { getLocale, msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Modal } from "@/components/ui/dialog";
import { Card } from "@/components/ui/card";
import { useSession } from "@/modules/identity";
import { write } from "./api";
import { isSaved } from "./pap-api";
import { formatDate } from "./api";
import { AllianceCoinConversion } from "./alliance-coin-conversion";
import {
  getAlliancePAP,
  getAlliancePAPMembers,
  getPAPRequirement,
  type Requirement,
} from "./alliance-pap-api";
import { EveImage } from "@/components/eve-image";
import { AlliancePAPMembers } from "./alliance-pap-members";
import "./pap-requirement.css";

const RingProgress = lazy(() => import("@/components/charts/ring-progress"));

export function PAPRequirement({
  user,
  settings = false,
  chart = false,
}: {
  user: string;
  settings?: boolean;
  chart?: boolean;
}) {
  const client = useQueryClient();
  const session = useSession(settings);
  const [editing, setEditing] = useState<Requirement | null>(null);
  const [value, setValue] = useState(3);
  const q = useQuery({
    queryKey: ["attendance", "pap-requirement", user],
    queryFn: ({ signal }) => getPAPRequirement(signal),
    refetchInterval: 30000,
  });
  const alliance = useQuery({
    queryKey: ["attendance", "alliance-pap", user],
    queryFn: ({ signal }) => getAlliancePAP(signal),
    refetchInterval: 30000,
  });
  const members = useQuery({
    queryKey: ["attendance", "alliance-pap-members", user, alliance.data?.month ?? ""],
    queryFn: ({ signal }) => getAlliancePAPMembers(alliance.data!.month, signal),
    enabled: alliance.data?.can_manage === true && alliance.data.available,
    refetchInterval: 30000,
  });
  const save = useMutation({
    mutationFn: () =>
      write(
        "/pap-requirement",
        session.data!.session!.csrf_token,
        { monthly_points: value, version: editing!.version },
        isSaved,
      ),
    onSuccess: () => {
      setEditing(null);
      void client.invalidateQueries({
        queryKey: ["attendance", "pap-requirement"],
      });
    },
  });
  return (
    <>
      <Card
        className={`pap-requirement${chart ? " pap-requirement-chart" : ""}`}
      >
        <div className="pap-alliance-header">
          <div className="pap-alliance-title">
            <span className="pap-alliance-icon" aria-hidden="true">
              <Award size={18} />
            </span>
            <div>
              <h2>{msg("联盟 PAP")}</h2>
              <small>
                {msg("每人每月最低联盟 PAP")} · {q.data?.monthly_points ?? 3}
              </small>
            </div>
          </div>
          {settings && q.data?.can_manage && (
            <IconAction
              label={msg("集结分要求配置")}
              onClick={() => {
                setEditing(q.data!);
                setValue(q.data!.monthly_points);
                save.reset();
              }}
            >
              <Settings2 size={17} aria-hidden="true" />
            </IconAction>
          )}
        </div>
        {q.isError ? (
          <div className="pap-status pap-status-error" role="alert">
            <span>{msg("集结分要求暂不可用")}</span>
            <Button variant="ghost" onClick={() => void q.refetch()}>
              {msg("重试")}
            </Button>
          </div>
        ) : !q.data ? (
          <div className="pap-status" role="status">
            {msg("正在读取")}
          </div>
        ) : alliance.isError ? (
          <div className="pap-status pap-status-error" role="alert">
            <span>{msg("联盟 PAP 暂不可用")}</span>
            <Button variant="ghost" onClick={() => void alliance.refetch()}>
              <RefreshCw size={15} />
              {msg("重试")}
            </Button>
          </div>
        ) : (
          <div className="pap-alliance-layout">
            <div className="pap-alliance-summary">
              <span className="pap-summary-label">{msg("本月联盟 PAP")}</span>
              <div className="pap-summary-main">
                <div>
                  <strong className="pap-summary-total">
                    {alliance.data?.available
                      ? alliance.data.points.toLocaleString(getLocale(), {
                          maximumFractionDigits: 2,
                        })
                      : "—"}
                  </strong>
                  <span className="pap-summary-target">
                    {msg("目标")} {q.data.monthly_points} PAP / {msg("人")}
                  </span>
                </div>
                {alliance.data?.available && (
                  <Suspense
                    fallback={<div className="ring-progress-placeholder" />}
                  >
                    <RingProgress
                      value={alliance.data.points}
                      target={q.data.monthly_points}
                      label={msg("本月联盟 PAP")}
                    />
                  </Suspense>
                )}
              </div>
              <span
                className={`pap-status-pill ${alliance.data?.available && alliance.data.complete ? "is-complete" : ""}`}
              >
                {alliance.data?.available && alliance.data.complete ? (
                  <>
                    <Check size={14} />
                    {msg("达标")}
                  </>
                ) : alliance.data?.state === "syncing" ? (
                  msg("联盟 PAP 同步中")
                ) : (
                  msg("联盟 PAP 暂无可用数据")
                )}
              </span>
            </div>
            <div className="pap-alliance-details">
              <div className="pap-alliance-details-heading">
                <strong>{msg("联盟 PAP 明细")}</strong>
                <small>
                  {alliance.data?.characters.length ?? 0} {msg("个角色")}
                  {alliance.data?.last_synced_at
                    ? ` · ${msg("同步于")} ${formatDate(alliance.data.last_synced_at)}`
                    : ""}
                </small>
              </div>
              {!alliance.data?.available ||
              alliance.data.characters.length === 0 ? (
                <span className="pap-alliance-empty">
                  {msg("暂无已绑定联盟 PAP")}
                </span>
              ) : (
                <div className="pap-alliance-character-list">
                  {alliance.data.characters.map((character) => (
                    <div
                      className="pap-alliance-character"
                      key={character.character_id}
                    >
                      <EveImage
                        kind="character"
                        id={String(character.character_id)}
                        className="pap-alliance-avatar"
                      />
                      <span>{character.character_name}</span>
                      <strong>
                        {character.pap.toLocaleString(getLocale(), {
                          maximumFractionDigits: 2,
                        })}{" "}
                        {msg("PAP")}
                      </strong>
                    </div>
                  ))}
                </div>
              )}
              {alliance.data?.can_manage && (
                <AllianceCoinConversion
                  user={user}
                  csrf={session.data?.session?.csrf_token ?? ""}
                  version={alliance.data.version}
                />
              )}
              {alliance.data?.can_manage && alliance.data.available && (
                <AlliancePAPMembers
                  report={members.data}
                  loading={members.isLoading}
                  error={members.error}
                  retry={() => void members.refetch()}
                />
              )}
            </div>
          </div>
        )}
      </Card>
      {editing && (
        <Modal
          title={msg("集结分要求配置")}
          close={() => {
            if (!save.isPending) setEditing(null);
          }}
          busy={save.isPending}
        >
          <form
            className="pap-requirement-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (Number.isInteger(value) && value >= 1 && value <= 100000)
                save.mutate();
            }}
          >
            <label>
              {msg("每人每月最低联盟 PAP")}
              <input
                type="number"
                min={1}
                max={100000}
                step={1}
                required
                value={Number.isNaN(value) ? "" : value}
                onChange={(e) => setValue(e.target.valueAsNumber)}
              />
            </label>
            <p className="muted">
              {msg("同一自然人所有角色合计，按北京时间自然月统计。")}
            </p>
            {save.isError && <p role="alert">{save.error.message}</p>}
            <Button
              type="submit"
              disabled={save.isPending || !session.data?.session}
            >
              {save.isPending ? msg("正在保存") : msg("保存")}
            </Button>
          </form>
        </Modal>
      )}
    </>
  );
}
