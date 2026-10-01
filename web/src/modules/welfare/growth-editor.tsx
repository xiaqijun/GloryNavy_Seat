import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { msg } from "@/lib/i18n";
import { getData } from "@/lib/http";
import { Modal } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";

import { RewardSummary } from "@/components/reward-summary";
import { getCatalog, type CatalogEntry } from "@/modules/exchange/catalog-api";
import { Link } from "react-router-dom";
import * as api from "./api";

type Fit = { id: string; name: string; fit: { ship_type_id: string } };
type Item = { id: string; name: string };
export { RewardSummary } from "@/components/reward-summary";

export function GrowthEditor({
  corp,
  csrf,
  policy,
  policies,
  close,
  done,
}: {
  corp: string;
  csrf: string;
  policy?: api.Policy;
  policies: api.Policy[];
  close: () => void;
  done: (kind: string) => void;
}) {
  const [fitting, setFitting] = useState(policy?.config.fitting_id || "");
  const [enabled, setEnabled] = useState(policy?.config.enabled ?? true);
  const [effective, setEffective] = useState(() => {
    const d = new Date(policy?.config.effective_at || Date.now());
    return new Date(d.getTime() - d.getTimezoneOffset() * 60_000)
      .toISOString()
      .slice(0, 16);
  });
  const [plan, setPlan] = useState(policy?.config.skill_plan_id || "0");
  const [note, setNote] = useState(policy?.config.note || "");
  const [legacyRewards] = useState<api.GrowthRewards>(
    policy?.config.rewards || { fittings: [], items: [], coins_minor: 0 },
  );
  const [coins, setCoins] = useState(
    String((policy?.config.rewards?.coins_minor || 0) / 100),
  );
  const [rewardID, setRewardID] = useState(policy?.config.reward_id || "");
  const library = useQuery({
    queryKey: ["catalog", "all"],
    queryFn: ({ signal }) => getCatalog(signal),
  });
  const choices = (library.data || []).filter(
    (r) =>
      !r.archived && r.content.fittings.every((f) => f.corporation_id === corp),
  );
  const reward: CatalogEntry | undefined = choices.find(
    (r) => r.id === rewardID,
  );
  const rewards: api.GrowthRewards = {
    ...(reward?.content || { fittings: [], items: [] }),
    coins_minor: 0,
  };
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const submission = useRef({ fingerprint: "", key: crypto.randomUUID() });
  const errorRef = useRef<HTMLParagraphElement>(null);
  const fits = useQuery({
    queryKey: ["welfare", "fits", corp],
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/fittings/library?corporation_id=${corp}`,
        (v): v is Fit[] =>
          Array.isArray(v) &&
          v.every(
            (x) =>
              x &&
              typeof x.id === "string" &&
              typeof x.name === "string" &&
              typeof x.fit?.ship_type_id === "string",
          ),
        signal,
      ),
  });
  const plans = useQuery({
    queryKey: ["welfare", "plans", corp],
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/skills/plans?corporation_id=${corp}`,
        (v): v is Item[] =>
          Array.isArray(v) &&
          v.every(
            (x) => x && typeof x.id === "string" && typeof x.name === "string",
          ),
        signal,
      ),
  });
  const name =
    fits.data?.find((f) => f.id === fitting)?.name ||
    policy?.config.project_name ||
    "";
  const selected = fits.data?.find((f) => f.id === fitting);
  let coinMinor = NaN;
  try {
    coinMinor = api.minor(coins);
  } catch {
    /* Preserve partial number input while editing. */
  }
  const disabling = !!policy && !enabled;
  const valid =
    !!fitting &&
    (disabling ||
      (!fits.isError &&
        (!rewardID || !!reward) &&
        Number.isSafeInteger(coinMinor) &&
        coinMinor >= 0 &&
        coinMinor <= 1e12 &&
        (!(rewards.isk_minor || 0) || (rewards.isk_minor || 0) >= 100) &&
        (rewards.fittings.length > 0 ||
          rewards.items.length > 0 ||
          (rewards.isk_minor || 0) > 0 ||
          coinMinor > 0)));
  function choose(id: string) {
    setFitting(id);
  }
  const fitOptions = (fits.data || []).map((f) => ({
    value: f.id,
    label: f.name,
  }));
  return (
    <Modal
      title={policy ? msg("项目配置") : msg("新增成长项目")}
      close={close}
      busy={busy}
      footer={
        <>
          <Button variant="outline" disabled={busy} onClick={close}>
            {msg("取消")}
          </Button>
          <Button form="growth-config" type="submit" disabled={busy || !valid}>
            {busy ? msg("正在保存") : msg("保存")}
          </Button>
        </>
      }
    >
      <form
        id="growth-config"
        className="welfare-form"
        onChange={() => {
          setError("");
        }}
        onSubmit={async (e) => {
          e.preventDefault();
          if (!valid) return;
          setBusy(true);
          setError("");
          try {
            const kind = policy?.kind || `growth_fitting_${fitting}`;
            const command = {
              action: "configure",
              corporation_id: corp,
              kind,
              version: policy?.version || "0",
              config: {
                reward_id: reward?.id || "0",
                reward_version: reward?.version || "0",
                enabled,
                effective_at: new Date(effective).toISOString(),
                fitting_id: fitting,
                ship_type_id:
                  selected?.fit.ship_type_id ||
                  policy?.config.ship_type_id ||
                  "0",
                skill_plan_id: plan,
                reference_minor: 0,
                day_zone: "",
                note,
                rewards: {
                  coins_minor: coinMinor,
                  fittings: rewards.fittings.map((f) => ({
                    fitting_id: f.fitting_id,
                    quantity: f.quantity,
                  })),
                  items: rewards.items.map((i) => ({
                    type_id: i.type_id,
                    quantity: i.quantity,
                  })),
                },
              },
            };
            const fingerprint = JSON.stringify(command);
            if (submission.current.fingerprint !== fingerprint)
              submission.current = { fingerprint, key: crypto.randomUUID() };
            await api.post("commands", csrf, {
              ...command,
              request_key: submission.current.key,
            });
            done(kind);
          } catch (err) {
            setError(err instanceof Error ? err.message : msg("保存失败"));
            requestAnimationFrame(() => errorRef.current?.focus());
          } finally {
            setBusy(false);
          }
        }}
      >
        <fieldset disabled={busy}>
          <label className="welfare-field wide">
            {msg("项目舰船配置")}
            <Select
              label={msg("项目舰船配置")}
              value={fitting}
              onValueChange={choose}
              disabled={!!policy}
              options={
                policy
                  ? [{ value: fitting, label: name }]
                  : fitOptions.filter(
                      (f) =>
                        !policies.some(
                          (p) => p.kind === `growth_fitting_${f.value}`,
                        ),
                    )
              }
            />
          </label>
          {fits.isError && (
            <p className="wide" role="alert">
              {fits.error.message}
            </p>
          )}
          {fits.data?.length === 0 && (
            <p className="wide">{msg("请先在舰船配置中导入军团方案。")}</p>
          )}
          <label className="welfare-check">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            {msg("开放申请")}
          </label>
          <label className="welfare-field wide">
            {msg("生效时间（本地）")}
            <input
              type="datetime-local"
              required={enabled}
              value={effective}
              onChange={(e) => setEffective(e.target.value)}
            />
          </label>
          <label className="welfare-field wide">
            {msg("技能要求方案（选填）")}
            <Select
              label={msg("技能要求方案（选填）")}
              value={plan}
              onValueChange={setPlan}
              options={[
                { value: "0", label: msg("不关联") },
                ...(plans.data || []).map((p) => ({
                  value: p.id,
                  label: p.name,
                })),
              ]}
            />
          </label>
          {plans.isError && (
            <p className="wide" role="alert">
              {plans.error.message}
            </p>
          )}
          <label className="welfare-field wide">
            {msg("实物奖励")}
            <Select
              label={msg("实物奖励")}
              value={rewardID}
              onValueChange={setRewardID}
              options={[
                { value: "", label: msg("不发放实物") },
                ...choices.map((r) => ({ value: r.id, label: r.name })),
              ]}
            />
          </label>
          {library.isError && <p role="alert">{library.error.message}</p>}
          {reward && <RewardSummary rewards={reward.content} />}
          {!policy?.config.reward_id &&
            legacyRewards.fittings.length + legacyRewards.items.length > 0 && (
              <small>{msg("旧实物奖励请重新从奖励库选择")}</small>
            )}
          <Link to="/rewards" target="_blank" rel="noreferrer">
            {msg("管理奖励库")}
          </Link>
          <label className="welfare-field wide">
            {msg("果壳币奖励")}
            <input
              type="number"
              min={0}
              max={10000000000}
              step="0.01"
              required
              value={coins}
              onChange={(e) => setCoins(e.target.value)}
            />
          </label>
          <small className="wide">{msg("确认交付后，果壳币自动入账。")}</small>
          <label className="welfare-field wide">
            {msg("适用条件")}
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              maxLength={1000}
            />
          </label>
        </fieldset>
        {error && (
          <p role="alert" tabIndex={-1} ref={errorRef}>
            {error}
          </p>
        )}
      </form>
    </Modal>
  );
}
