import { RefreshCw } from "lucide-react";
import { IconAction } from "@/components/ui/icon-action";
import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { msg, getLocale } from "@/lib/i18n";
import { FormDialog } from "@/components/ui/form-dialog";
import { Select } from "@/components/ui/select";
import { RewardSummary } from "@/components/reward-summary";
import { getRewardValuation, type Reward, type Shop } from "./rewards-api";
import { isSaved, write, type Context } from "./api";
export function RateForm({
  csrf,
  shop,
  done,
  close,
}: {
  csrf: string;
  shop: Shop;
  done: () => void;
  close: () => void;
}) {
  const [rate, setRate] = useState(shop.isk_per_coin || 1);
  const [key, setKey] = useState(() => crypto.randomUUID());
  const m = useMutation({
    mutationFn: () =>
      write(
        "/rewards/rate",
        csrf,
        { version: shop.version, request_key: key, isk_per_coin: rate },
        isSaved,
      ),
    onSuccess: done,
  });
  return (
    <FormDialog
      close={close}
      busy={m.isPending}
      className="exchange-form"
      title={msg("果壳币价值设置")}
      onSubmit={(e) => {
        e.preventDefault();
        if (!m.isPending) m.mutate();
      }}
    >
      <label>
        {msg("每 1 果壳币的 ISK 价值")}{" "}
        <input
          autoFocus
          type="number"
          min={1}
          max={1e12}
          step={1}
          required
          disabled={m.isPending}
          value={rate}
          onChange={(e) => {
            setRate(e.target.valueAsNumber);
            setKey(crypto.randomUUID());
          }}
        />
      </label>
      <small>{msg("调价仅影响新的兑换申请。")}</small>
      {m.isError && <p role="alert">{m.error.message}</p>}
    </FormDialog>
  );
}
export function RewardForm({
  csrf,
  reward,
  done,
  close,
}: {
  csrf: string;
  reward: Reward;
  done: () => void;
  close: () => void;
}) {
  const selected = reward;
  const [automatic, setAutomatic] = useState(
    reward.pricing?.automatic ?? false,
  );
  const valuation = useQuery({
    queryKey: ["exchange", "valuation", reward.id, reward.version],
    queryFn: ({ signal }) => getRewardValuation(reward, signal),
    retry: false,
    gcTime: 0,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const quoted = valuation.data?.complete && !valuation.isError;
  const [manualValue, setManualValue] = useState<string | null>(
    reward.pricing?.automatic === false ? String(reward.isk_value) : null,
  );
  const value =
    manualValue ??
    (quoted
      ? String(valuation.data!.isk_value)
      : reward.pricing?.automatic && reward.isk_value > 0
        ? String(reward.isk_value)
        : "");
  const ready =
    !valuation.isFetching &&
    value.trim() !== "" &&
    Number.isSafeInteger(Number(value)) &&
    Number(value) >= 1 &&
    Number(value) <= 1e12;
  const [stock, setStock] = useState(reward.stock);
  const [enabled, setEnabled] = useState(reward.enabled);
  const [key, setKey] = useState(() => crypto.randomUUID());
  const change = () => setKey(crypto.randomUUID());
  const refreshValue = async () => {
    // Keep the displayed amount until a complete quote can replace it.
    setManualValue(value);
    change();
    const result = await valuation.refetch();
    if (!result.isError && result.data?.complete) setManualValue(null);
  };
  const m = useMutation({
    mutationFn: () =>
      write(
        "/rewards/items",
        csrf,
        {
          id: selected.id,
          version: selected.version,
          request_key: key,
          type_id: selected.type_id,
          quantity: selected.quantity,
          isk_value: Number(value),
          automatic_pricing: automatic,
          stock,
          enabled,
        },
        isSaved,
      ),
    onSuccess: done,
  });
  return (
    <FormDialog
      close={close}
      busy={m.isPending}
      className="exchange-form"
      title={msg("兑换设置")}
      formLabel={msg("奖励物品配置")}
      size="form"
      disabled={!ready}
      onSubmit={(e) => {
        e.preventDefault();
        if (ready && !m.isPending) m.mutate();
      }}
    >
      <strong>{selected.name}</strong>
      <RewardSummary rewards={selected.content} />
      <div className="exchange-reward-fields">
        <label>
          {msg("价值")}
          <div className="exchange-valuation">
            <input
              type="number"
              min={1}
              max={1e12}
              step={1}
              required
              aria-label={msg("价值")}
              aria-busy={valuation.isFetching}
              disabled={m.isPending || valuation.isFetching}
              placeholder={valuation.isFetching ? msg("正在核价") : "ISK"}
              value={value}
              onChange={(e) => {
                setManualValue(e.target.value);
                setAutomatic(false);
                change();
              }}
            />
            <span className="exchange-note">ISK</span>
            <IconAction
              label={msg("重新核价")}
              disabled={m.isPending || valuation.isFetching}
              onClick={() => void refreshValue()}
            >
              <RefreshCw />
            </IconAction>
          </div>
        </label>
        <label>
          {msg("可用库存（份）")}
          <input
            autoFocus
            type="number"
            min={0}
            max={1e6}
            step={1}
            required
            disabled={m.isPending}
            value={stock}
            onChange={(e) => {
              setStock(e.target.valueAsNumber);
              change();
            }}
          />
        </label>
        <label>
          {msg("上架状态")}
          <Select
            label={msg("上架状态")}
            disabled={m.isPending}
            value={enabled ? "yes" : "no"}
            onValueChange={(v) => {
              setEnabled(v === "yes");
              change();
            }}
            options={[
              { value: "no", label: msg("未上架") },
              { value: "yes", label: msg("已上架") },
            ]}
          />
        </label>
      </div>
      <label>
        {msg("定价方式")}
        <Select
          label={msg("定价方式")}
          value={automatic ? "auto" : "manual"}
          disabled={m.isPending || valuation.isFetching}
          onValueChange={(v) => {
            setAutomatic(v === "auto");
            if (v === "auto") void refreshValue();
            else {
              setManualValue(value);
              change();
            }
          }}
          options={[
            { value: "auto", label: msg("自动核价 · 每 6 小时") },
            { value: "manual", label: msg("手动定价") },
          ]}
        />
      </label>
      {valuation.isFetching && <small role="status">{msg("正在核价")}</small>}
      {reward.pricing?.automatic &&
        ["failed", "incomplete"].includes(reward.pricing.status) && (
          <small role="status">{msg("上次核价未完成，保留原价值")}</small>
        )}
      {valuation.isError && <p role="alert">{valuation.error.message}</p>}
      {valuation.data && !valuation.data.complete && !valuation.isFetching && (
        <p role="alert">{msg("报价不完整，可手动填写价值或重新核价")}</p>
      )}
      {quoted && (
        <small>
          {msg(
            "吉他中间价：{0} ISK",
            valuation.data!.isk_value.toLocaleString(getLocale()),
          )}
          {valuation.data?.observed_at
            ? " · " +
              new Date(valuation.data.observed_at).toLocaleString(getLocale())
            : ""}
        </small>
      )}
      {m.isError && <p role="alert">{m.error.message}</p>}
    </FormDialog>
  );
}
export function SourceForm({
  csrf,
  source,
  done,
  close,
}: {
  csrf: string;
  source: Context["sources"][number];
  done: () => void;
  close: () => void;
}) {
  const [amount, setAmount] = useState(source.minor_per_unit / 100 || 1);
  const [mode, setMode] = useState(source.mode ?? "manual");
  const [key, setKey] = useState(() => crypto.randomUUID());
  const m = useMutation({
    mutationFn: () =>
      write(
        "/sources",
        csrf,
        {
          id: source.id,
          mode,
          minor_per_unit: Math.round(amount * 100),
          version: source.version,
          request_key: key,
        },
        isSaved,
      ),
    onSuccess: done,
  });
  return (
    <FormDialog
      close={close}
      busy={m.isPending}
      className="exchange-form"
      title={msg("发币比例设置")}
      onSubmit={(e) => {
        e.preventDefault();
        if (!m.isPending) m.mutate();
      }}
    >
      <label>
        {msg("每 1 PAP 发放果壳币")}{" "}
        <input
          autoFocus
          type="number"
          min={0.01}
          max={1000000}
          step={0.01}
          required
          disabled={m.isPending}
          value={amount}
          onChange={(e) => {
            setAmount(e.target.valueAsNumber);
            setKey(crypto.randomUUID());
          }}
        />
      </label>
      <Select
        label={msg("兑换方式")}
        value={mode}
        disabled={m.isPending}
        options={[
          { value: "manual", label: msg("手动兑换") },
          { value: "automatic", label: msg("自动兑换") },
        ]}
        onValueChange={(v) => {
          setMode(v as "manual" | "automatic");
          setKey(crypto.randomUUID());
        }}
      />
      <small>
        {source.id === "alliance_pap"
          ? mode === "manual"
            ? msg("管理员按月确认兑换。")
            : msg("首次同步作为基线，后续新增积分自动发币。")
          : mode === "manual"
            ? msg("管理员在活动中确认兑换。")
            : msg("发放新积分时同步发币。")}{" "}
        {msg("切换不补兑历史积分。")}{" "}
      </small>
      {m.isError && <p role="alert">{m.error.message}</p>}
    </FormDialog>
  );
}
